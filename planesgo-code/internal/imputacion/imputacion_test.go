package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// odooFalso apunta las llamadas y responde como Odoo (o falla, si se pide).
type odooFalso struct {
	mu       sync.Mutex
	llamadas []string
	horas    []float64
	ia       map[string]any
	falla    bool
	sig      int
	cronos   []any // cronómetros en marcha que «tiene» Odoo
}

func (o *odooFalso) CallKW(_ context.Context, _, modelo, metodo string, args []any, _ map[string]any, res any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.llamadas = append(o.llamadas, modelo+"."+metodo)
	if o.falla && metodo == "write" {
		return errors.New("Odoo caído")
	}
	var r any = true
	switch modelo + "." + metodo {
	case "project.task.search_read":
		r = []any{}
	case "account.analytic.line.search_read":
		r = o.cronos
		if r == nil {
			r = []any{}
		}
	case "project.task.create":
		r = 7
	case "account.analytic.line.create":
		o.sig++
		r = 98 + o.sig
	case "account.analytic.line.write":
		m := args[1].(map[string]any)
		if h, ok := m["unit_amount"].(float64); ok {
			o.horas = append(o.horas, h)
		}
		if _, ok := m["ai_tokens_total"]; ok {
			o.ia = m
		}
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, res)
}

// Reloj de prueba.
type reloj struct {
	mu sync.Mutex
	t  time.Time
}

func (r *reloj) ahora() time.Time     { r.mu.Lock(); defer r.mu.Unlock(); return r.t }
func (r *reloj) pasa(d time.Duration) { r.mu.Lock(); r.t = r.t.Add(d); r.mu.Unlock() }

func espera(t *testing.T, cond func() bool) {
	for i := 0; i < 100 && !cond(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("no se cumplió a tiempo")
	}
}

func nuevo(o Odoo, r *reloj) *Gestor { return &Gestor{Odoo: o, Ahora: r.ahora} }

// Turno del agente de 10 min + espera de 5 min sin nada: se imputan 15 minutos, con los datos de IA.
func TestTurnoYEspera(t *testing.T) {
	o := &odooFalso{}
	r := &reloj{t: time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)}
	g := nuevo(o, r)
	fin := g.Empieza(Trabajo{Sesion: "s1", Proyecto: 447, UID: 2, Modelo: "Claude Code", Desc: "x"})
	espera(t, func() bool { ab, _ := g.Estado(); return len(ab) == 1 && ab[0].LineaID > 0 })
	g.SumaUso("s1", 1000, 200, 0.05)
	r.pasa(10 * time.Minute)
	fin()
	if ab, _ := g.Estado(); ab[0].Estado != "en-espera" {
		t.Fatalf("estado %s", ab[0].Estado)
	}
	r.pasa(4 * time.Minute)
	g.revisa()
	if ab, _ := g.Estado(); len(ab) != 1 {
		t.Fatal("a los 4 min sigue abierto")
	}
	r.pasa(2 * time.Minute)
	g.revisa()
	_, ce := g.Estado()
	if len(ce) != 1 || ce[0].Horas != 0.25 || ce[0].Estado != "cerrado" {
		t.Fatalf("cerrado: %+v", ce)
	}
	if fmt.Sprint(o.horas) != "[0.25]" || o.ia["ai_tokens_total"] != 1200 || o.ia["ai_session_id"] != "pgocode-s1" {
		t.Errorf("Odoo: horas %v ia %v", o.horas, o.ia)
	}
}

// La actividad humana abre el parte y lo mantiene: 20 min de uso (sin agente) + 5 de espera = 25 min.
func TestActividadHumana(t *testing.T) {
	o := &odooFalso{}
	r := &reloj{t: time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)}
	g := nuevo(o, r)
	tr := Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5, Tarea: "A"}
	g.Actividad(tr)
	if ab, _ := g.Estado(); ab[0].Estado != "activo" {
		t.Fatalf("estado %s", ab[0].Estado)
	}
	for i := 0; i < 5; i++ {
		r.pasa(4 * time.Minute)
		g.revisa()
		g.Actividad(tr)
	}
	r.pasa(2 * time.Minute)
	g.revisa()
	if ab, _ := g.Estado(); ab[0].Estado != "en-espera" {
		t.Fatalf("estado %s", ab[0].Estado)
	}
	r.pasa(4 * time.Minute)
	g.revisa()
	espera(t, func() bool { _, ce := g.Estado(); return len(ce) == 1 })
	if _, ce := g.Estado(); ce[0].Horas != float64(25)/60 {
		t.Errorf("horas %v", ce[0].Horas)
	}
}

// Si Odoo falla al cerrar, el parte queda pendiente y se reintenta hasta que entra.
func TestReintento(t *testing.T) {
	o := &odooFalso{falla: true}
	r := &reloj{t: time.Now()}
	g := nuevo(o, r)
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5})()
	espera(t, func() bool { ab, _ := g.Estado(); return len(ab) == 1 && ab[0].LineaID > 0 })
	g.Cierra("s1", "prueba")
	if _, ce := g.Estado(); len(ce) != 1 || ce[0].Estado != "pendiente" {
		t.Fatalf("debería quedar pendiente: %+v", ce)
	}
	o.mu.Lock()
	o.falla = false
	o.mu.Unlock()
	g.reintenta()
	if _, ce := g.Estado(); len(ce) != 1 || ce[0].Estado != "cerrado" {
		t.Fatalf("tras el reintento: %+v", ce)
	}
}

// Un parte abierto sobrevive a un cierre inesperado: al volver a arrancar se cierra con su último trabajo.
func TestRecuperacion(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "partes.json")
	o := &odooFalso{}
	r := &reloj{t: time.Now().Add(-time.Hour)}
	g := &Gestor{Odoo: o, Ahora: r.ahora, FicheroEstado: f, Diario: filepath.Join(dir, "imputacion.log")}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5})
	espera(t, func() bool { ab, _ := g.Estado(); return len(ab) == 1 && ab[0].LineaID > 0 })
	g.guarda()
	// «Se cae» pgocode con el turno a medias: otro gestor arranca con el mismo fichero.
	o2 := &odooFalso{}
	g2 := &Gestor{Odoo: o2, FicheroEstado: f, Diario: filepath.Join(dir, "imputacion.log")}
	ctx, cancela := context.WithCancel(context.Background())
	defer cancela()
	g2.Arranca(ctx)
	_, ce := g2.Estado()
	if len(ce) != 1 || ce[0].Estado != "cerrado" || ce[0].Horas != float64(5)/60 {
		t.Fatalf("recuperado: %+v", ce)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "imputacion.log")); len(b) == 0 {
		t.Error("diario vacío")
	}
}

// Cambiar de tarea en la sesión cierra el parte y abre otro.
func TestCambioDeTarea(t *testing.T) {
	o := &odooFalso{}
	g := &Gestor{Odoo: o}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5, Tarea: "A"})()
	time.Sleep(30 * time.Millisecond)
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 6, Tarea: "B"})()
	espera(t, func() bool { _, ce := g.Estado(); return len(ce) == 1 })
	ab, ce := g.Estado()
	if len(ab) != 1 || ab[0].TareaID != 6 || ce[0].TareaID != 5 {
		t.Errorf("abiertos %+v cerrados %+v", ab, ce)
	}
}

// Tras actualizarse (Suspende), la versión nueva retoma el parte: misma línea, mismo inicio, sin cerrarlo.
func TestRetomaTrasActualizar(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "partes.json")
	o := &odooFalso{}
	g := &Gestor{Odoo: o, FicheroEstado: f}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5, UID: 2, OdooSesion: "x"})()
	espera(t, func() bool { ab, _ := g.Estado(); return len(ab) == 1 && ab[0].LineaID > 0 })
	ab, _ := g.Estado()
	g.Suspende()
	g2 := &Gestor{Odoo: &odooFalso{}, FicheroEstado: f}
	ctx, cancela := context.WithCancel(context.Background())
	defer cancela()
	g2.Arranca(ctx)
	ab2, ce2 := g2.Estado()
	if len(ab2) != 1 || len(ce2) != 0 || ab2[0].LineaID != ab[0].LineaID || !ab2[0].Inicio.Equal(ab[0].Inicio) {
		t.Fatalf("antes %+v; después abiertos %+v cerrados %+v", ab, ab2, ce2)
	}
}

// Un cronómetro de pgocode en marcha en Odoo que pgocode no conoce (p. ej. fichero perdido) se retoma.
func TestSincronizaDesdeOdoo(t *testing.T) {
	o := &odooFalso{cronos: []any{map[string]any{"id": 321, "name": "Sesión de pgocode «A»: x", "ai_session_id": "pgocode-s9",
		"timer_start": "2026-10-21 08:00:00", "project_id": []any{447, "MADERAS PLANES"}, "task_id": []any{7, "pgocode - Sesión IA"}}}}
	g := &Gestor{Odoo: o}
	g.Sincroniza(context.Background(), 2, "x")
	ab, _ := g.Estado()
	if len(ab) != 1 || ab[0].LineaID != 321 || ab[0].Sesion != "s9" || ab[0].Proyecto != 447 || ab[0].TareaID != 7 ||
		!ab[0].Inicio.Equal(time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("retomado: %+v", ab)
	}
}
