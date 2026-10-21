package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// odooFalso: un Odoo en memoria con sus líneas de parte.
type odooFalso struct {
	mu     sync.Mutex
	lineas map[int]map[string]any
	sig    int
	falla  bool
}

func nuevoOdoo() *odooFalso { return &odooFalso{lineas: map[int]map[string]any{}, sig: 100} }

func (o *odooFalso) CallKW(_ context.Context, _, modelo, metodo string, args []any, _ map[string]any, res any) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.falla && modelo == "account.analytic.line" && metodo != "search_read" {
		return errors.New("Odoo caído")
	}
	var r any = true
	switch modelo + "." + metodo {
	case "project.task.search_read":
		r = []any{}
	case "project.task.create":
		r = 7
	case "account.analytic.line.create":
		o.sig++
		v := map[string]any{"create_date": time.Now().UTC().Format("2006-01-02 15:04:05")}
		for k, x := range args[0].(map[string]any) {
			v[k] = x
		}
		o.lineas[o.sig] = v
		r = o.sig
	case "account.analytic.line.write":
		for _, id := range args[0].([]int) {
			l := o.lineas[id]
			if l == nil {
				return errors.New("MissingError: Record does not exist")
			}
			for k, x := range args[1].(map[string]any) {
				l[k] = x
			}
		}
	case "account.analytic.line.search_read":
		var l []any
		for id, v := range o.lineas {
			if s, _ := v["ai_session_id"].(string); strings.HasPrefix(s, marcaAbierta) {
				x := map[string]any{"id": id, "project_id": []any{v["project_id"], "PROY"}, "task_id": []any{7, "Desarrollo"}}
				for k, y := range v {
					if k != "project_id" && k != "task_id" {
						x[k] = y
					}
				}
				l = append(l, x)
			}
		}
		r = l
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, res)
}

func (o *odooFalso) linea(clase string) map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, v := range o.lineas {
		if s, _ := v["ai_session_id"].(string); strings.HasSuffix(s, "-"+clase) {
			return v
		}
	}
	return nil
}

type reloj struct {
	mu sync.Mutex
	t  time.Time
}

func (r *reloj) ahora() time.Time     { r.mu.Lock(); defer r.mu.Unlock(); return r.t }
func (r *reloj) pasa(d time.Duration) { r.mu.Lock(); r.t = r.t.Add(d); r.mu.Unlock() }

func espera(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 200 && !cond(); i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("no se cumplió a tiempo")
	}
}

var t0 = time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)

// Turno del agente de 10 min, la persona revisa 6 min más y 5 de espera: máquina 10 min (con IA) y hombre 11 min.
func TestDosLineas(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	tr := Trabajo{Sesion: "s1", Proyecto: 447, UID: 2, Modelo: "Claude Code opus", Titulo: "Informe"}
	fin := g.Empieza(tr)
	espera(t, func() bool { return o.linea("m") != nil && o.linea("h") != nil })
	if s := o.linea("m")["ai_session_id"]; s != "pgocode-abierto-s1-m" || o.linea("m")["ai_model"] != "Claude Code opus" {
		t.Fatalf("línea de máquina al abrir: %v", o.linea("m"))
	}
	g.SumaUso("s1", 1000, 200, 0.05)
	r.pasa(10 * time.Minute)
	fin()
	for i := 0; i < 3; i++ {
		r.pasa(2 * time.Minute)
		g.Actividad(tr)
	}
	r.pasa(5 * time.Minute)
	g.revisa()
	espera(t, func() bool { _, ce := g.Estado(); return len(ce) == 1 })
	m, h := o.linea("m"), o.linea("h")
	if m["unit_amount"] != float64(10)/60 || h["unit_amount"] != float64(11)/60 {
		t.Errorf("horas: máquina %v hombre %v", m["unit_amount"], h["unit_amount"])
	}
	if m["ai_session_id"] != "pgocode-s1-m" || h["ai_session_id"] != "pgocode-s1-h" || fmt.Sprint(m["ai_tokens_total"]) != "1200" {
		t.Errorf("cierre: m %v h %v", m, h)
	}
	if _, ok := h["ai_model"]; ok {
		t.Error("la línea de hombre no debe llevar ai_model")
	}
	if !strings.Contains(h["name"].(string), "Informe") {
		t.Errorf("descripción: %v", h["name"])
	}
}

// Solo uso humano: una línea de hombre, ninguna de máquina; tarea canónica por defecto.
func TestSoloHumano(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Actividad(Trabajo{Sesion: "s1", Proyecto: 1})
	espera(t, func() bool { return o.linea("h") != nil })
	r.pasa(6 * time.Minute)
	g.revisa()
	espera(t, func() bool { _, ce := g.Estado(); return len(ce) == 1 })
	_, ce := g.Estado()
	if o.linea("m") != nil || ce[0].Tarea != "Desarrollo" || ce[0].HorasHombre != float64(5)/60 {
		t.Errorf("solo humano: %+v", ce[0])
	}
}

// Las horas de una línea viva se escriben cada 3 minutos (Odoo siempre al día).
func TestLineaViva(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 5})
	espera(t, func() bool { return o.linea("m") != nil })
	r.pasa(4 * time.Minute)
	g.revisa()
	espera(t, func() bool { l := o.linea("m"); return l != nil && l["unit_amount"] != float64(0) })
	if v := o.linea("m")["unit_amount"]; v != float64(4)/60 {
		t.Errorf("máquina a los 4 min: %v", v)
	}
}

// Tras actualizarse, la versión nueva retoma las líneas abiertas que tiene Odoo (sin el fichero) y las cierra bien.
func TestRetomaDesdeOdoo(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: time.Now()}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	fin := g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 7, UID: 2, OdooSesion: "x"})
	espera(t, func() bool { return o.linea("m") != nil && o.linea("h") != nil })
	r.pasa(2 * time.Minute)
	fin()
	g.Suspende()
	g2 := &Gestor{Odoo: o, Ahora: r.ahora}
	g2.Sincroniza(context.Background(), 2, "x")
	ab, _ := g2.Estado()
	if len(ab) != 1 || ab[0].LineaMaquina == 0 || ab[0].LineaHombre == 0 || ab[0].Sesion != "s1" {
		t.Fatalf("retomado: %+v", ab)
	}
	g2.CierraTodo()
	if o.linea("m")["ai_session_id"] != "pgocode-s1-m" {
		t.Errorf("cerrado: %v", o.linea("m"))
	}
}

// Tras un cierre inesperado, al arrancar se retoma lo reciente (fichero) y no se duplican líneas.
func TestRetomaFichero(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "partes.json")
	o := nuevoOdoo()
	r := &reloj{t: time.Now()}
	g := &Gestor{Odoo: o, Ahora: r.ahora, FicheroEstado: f}
	g.Actividad(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 7, UID: 2, OdooSesion: "x"})
	espera(t, func() bool { return o.linea("h") != nil })
	g.guarda()
	g2 := &Gestor{Odoo: o, Ahora: r.ahora, FicheroEstado: f}
	ctx, cancela := context.WithCancel(context.Background())
	defer cancela()
	g2.Arranca(ctx)
	ab, _ := g2.Estado()
	o.mu.Lock()
	n := len(o.lineas)
	o.mu.Unlock()
	if len(ab) != 1 || n != 1 {
		t.Fatalf("abiertos %+v, líneas en Odoo %d", ab, n)
	}
	if b, _ := os.ReadFile(f); len(b) == 0 {
		t.Error("sin estado en disco")
	}
}

// Si la línea ya no existe en Odoo (borrada o validada), no se reintenta sin fin: queda en error.
func TestLineaBorrada(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Actividad(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 7})
	espera(t, func() bool { return o.linea("h") != nil })
	o.mu.Lock()
	o.lineas = map[int]map[string]any{}
	o.mu.Unlock()
	g.Cierra("s1", "prueba")
	_, ce := g.Estado()
	if len(ce) != 1 || ce[0].Estado != "error" || len(g.pendientes) != 0 {
		t.Fatalf("cerrado %+v pendientes %d", ce, len(g.pendientes))
	}
}

// Si Odoo está caído al cerrar, queda pendiente y entra al reintentar.
func TestReintento(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Actividad(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 7})
	espera(t, func() bool { return o.linea("h") != nil })
	o.mu.Lock()
	o.falla = true
	o.mu.Unlock()
	g.Cierra("s1", "prueba")
	if _, ce := g.Estado(); ce[0].Estado != "pendiente" {
		t.Fatalf("debería quedar pendiente: %+v", ce)
	}
	o.mu.Lock()
	o.falla = false
	o.mu.Unlock()
	g.reintenta()
	if _, ce := g.Estado(); len(ce) != 1 || ce[0].Estado != "cerrado" {
		t.Fatalf("tras reintentar: %+v", ce)
	}
}

// A medianoche el parte se cierra (los partes son de un día).
func TestMedianoche(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: time.Date(2026, 10, 21, 23, 58, 0, 0, time.Local)}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Empieza(Trabajo{Sesion: "s1", Proyecto: 1, TareaID: 7})
	espera(t, func() bool { return o.linea("m") != nil })
	r.pasa(4 * time.Minute)
	g.revisa()
	if ab, _ := g.Estado(); len(ab) != 0 {
		t.Fatal("a medianoche el parte se cierra")
	}
}
