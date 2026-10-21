package imputacion

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func valida(t *testing.T, reg *Registro, dia string, hoy bool) Informe {
	t.Helper()
	f, err := os.Open(reg.Fichero(dia))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	inf, err := Valida(f, hoy)
	if err != nil {
		t.Fatal(err)
	}
	return inf
}

func tipos(inf Informe) string {
	var l []string
	for _, a := range inf.Avisos {
		l = append(l, a.Gravedad+":"+a.Tipo)
	}
	return strings.Join(l, ",")
}

// Un día normal (3 sesiones, 10 turnos) se anota y, al validarlo, cuadra: sin errores y con las mismas horas que
// se escribieron en Odoo.
func TestRegistroCuadra(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	reg := &Registro{Dir: t.TempDir(), Ahora: r.ahora}
	g := &Gestor{Odoo: o, Ahora: r.ahora, Tiempos: reg}
	for i := 0; i < 10; i++ {
		s := fmt.Sprint("s", i%3)
		fin := g.Empieza(tr(s))
		espera(t, func() bool { return len(o.con("pgocode-m-claude-code")) == 1 })
		r.pasa(2 * time.Minute)
		fin()
		g.Actividad(tr(s))
		r.pasa(6 * time.Minute)
		g.revisa()
	}
	g.revisa()
	espera(t, func() bool {
		m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
		return len(m) == 1 && len(h) == 1 && m[0]["unit_amount"] == float64(20)/60 && h[0]["unit_amount"] == 1.0
	})
	g.escribeSucias()
	inf := valida(t, reg, t0.Format("2006-01-02"), false)
	if inf.Errores > 0 {
		t.Errorf("errores: %s", tipos(inf))
	}
	h, m := 0, 0
	for _, l := range inf.Lineas {
		switch {
		case l.Clase == "h" && l.Segundos == 3000 && l.Calculadas == 1 && l.Odoo == 1:
			h++
		case l.Clase == "m" && l.Segundos == 1200 && l.Odoo == float64(20)/60:
			m++
		default:
			t.Errorf("línea: %+v", l)
		}
	}
	if h != 1 || m != 1 {
		t.Errorf("líneas: %+v", inf.Lineas)
	}
	if len(inf.Sesiones) != 3 || inf.Sesiones[0].Turnos != 4 || inf.Sesiones[0].Periodos != 4 {
		t.Errorf("sesiones: %+v", inf.Sesiones)
	}
}

// Si alguna vez se contara dos veces el tiempo humano (dos sesiones a la vez sin descontar), el validador avisa.
func TestRegistroSolape(t *testing.T) {
	l := []string{
		`{"t":"2026-10-21T10:15:00Z","ev":"periodo-cierra","sesion":"a","uid":2,"proyecto":1,"tarea_id":2,"inicio":"2026-10-21T10:00:00Z","fin":"2026-10-21T10:15:00Z","maquina_s":0,"hombre_s":900}`,
		`{"t":"2026-10-21T10:15:00Z","ev":"periodo-cierra","sesion":"b","uid":2,"proyecto":1,"tarea_id":2,"inicio":"2026-10-21T10:05:00Z","fin":"2026-10-21T10:15:00Z","maquina_s":0,"hombre_s":600}`,
	}
	inf, _ := Valida(strings.NewReader(strings.Join(l, "\n")), true)
	if !strings.Contains(tipos(inf), "aviso:solape") {
		t.Errorf("no detecta el solape: %s", tipos(inf))
	}
	// Con el descuento anotado, no avisa.
	l = append(l, `{"t":"2026-10-21T10:15:00Z","ev":"periodo-solape","sesion":"b","solapado_s":600}`)
	inf, _ = Valida(strings.NewReader(strings.Join(l, "\n")), true)
	if strings.Contains(tipos(inf), "solape") {
		t.Errorf("avisa aunque ya se descontó: %s", tipos(inf))
	}
}

// Un registro con fallos preparados: el validador los encuentra todos.
func TestValidaDetecta(t *testing.T) {
	l := []string{
		`{"t":"2026-10-21T10:00:00Z","ev":"periodo-cierra","sesion":"s1","proyecto":1,"tarea_id":2,"inicio":"2026-10-21T09:00:00Z","fin":"2026-10-21T10:00:00Z","maquina_s":600,"hombre_s":600}`,
		`{"t":"2026-10-21T10:00:01Z","ev":"odoo","fecha":"2026-10-21","proyecto":1,"tarea_id":2,"clase":"h","linea":7,"segundos":600,"horas":0.5}`,
		`{"t":"2026-10-21T10:00:02Z","ev":"odoo","fecha":"2026-10-21","proyecto":1,"tarea_id":2,"clase":"h","linea":7,"segundos":900,"horas":0.25,"error":"Odoo caído"}`,
		`{"t":"2026-10-21T10:00:03Z","ev":"uso-sin-periodo","sesion":"s9","entrada":100,"salida":5}`,
		`{"t":"2026-10-21T10:00:04Z","ev":"agente-fin","sesion":"s1","procesos_vivos":2}`,
		`{"t":"2026-10-21T10:00:05Z","ev":"dia-suma","fecha":"2026-10-21","proyecto":1,"tarea_id":2,"clase":"h","mas_s":60,"cerrado_s":60}`,
		`{"t":"2026-10-21T10:00:06Z","ev":"dia-suma","fecha":"2026-10-21","proyecto":1,"tarea_id":2,"clase":"h","mas_s":60,"cerrado_s":500}`,
		`no es json`,
	}
	inf, err := Valida(strings.NewReader(strings.Join(l, "\n")), false)
	if err != nil {
		t.Fatal(err)
	}
	got := tipos(inf)
	for _, q := range []string{"error:periodo", "error:redondeo", "error:odoo", "error:uso", "aviso:procesos", "aviso:acumulado", "aviso:registro"} {
		if !strings.Contains(got, q) {
			t.Errorf("falta %s en %s", q, got)
		}
	}
}

// Dos sesiones a la vez con la persona delante: el tiempo humano se imputa una sola vez y el validador no avisa.
func TestSolapeHumanoUnaVez(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	reg := &Registro{Dir: t.TempDir(), Ahora: r.ahora}
	g := &Gestor{Odoo: o, Ahora: r.ahora, Tiempos: reg}
	g.Actividad(tr("a"))
	g.Actividad(tr("b"))
	for i := 0; i < 10; i++ {
		r.pasa(time.Minute)
		g.Actividad(tr("a"))
		g.Actividad(tr("b"))
	}
	r.pasa(6 * time.Minute)
	g.revisa()
	g.escribeSucias()
	// 10 min de actividad + 5 de espera = 15 min, una vez (no 30): 0,25 h.
	espera(t, func() bool { h := o.con("pgocode-h"); return len(h) == 1 && h[0]["unit_amount"] == 0.25 })
	inf := valida(t, reg, t0.Format("2006-01-02"), true)
	if strings.Contains(tipos(inf), "solape") {
		t.Errorf("avisa de un solape ya descontado: %s", tipos(inf))
	}
}
