package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// odooFalso: un Odoo en memoria con sus líneas de parte.
type odooFalso struct {
	mu      sync.Mutex
	lineas  map[int]map[string]any
	sig     int
	falla   bool
	sinSes  bool // Odoo sin el campo ai_session_id (como el de Planes)
	sinIA   bool // Odoo sin ningún campo ai_* (sin el módulo que los añade)
	nCampos int  // consultas a fields_get
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
	case "account.analytic.line.fields_get":
		m := map[string]any{"ai_model": map[string]any{}, "ai_session_id": map[string]any{}, "ai_tokens_input": map[string]any{},
			"ai_tokens_output": map[string]any{}, "ai_tokens_total": map[string]any{}, "ai_cost": map[string]any{}}
		if o.sinSes {
			delete(m, "ai_session_id")
		}
		if o.sinIA {
			m = map[string]any{"name": map[string]any{}}
		}
		o.nCampos++
		r = m
	case "project.task.search_read":
		r = []any{}
	case "project.task.create":
		r = 7
	case "account.analytic.line.create":
		if _, ok := args[0].(map[string]any)["ai_session_id"]; ok && o.sinSes {
			return errors.New("Invalid field 'ai_session_id' on model 'account.analytic.line'")
		}
		o.sig++
		v := map[string]any{}
		for k, x := range args[0].(map[string]any) {
			v[k] = x
		}
		o.lineas[o.sig] = v
		r = o.sig
	case "account.analytic.line.write":
		for _, id := range args[0].([]int) {
			if o.lineas[id] == nil {
				return errors.New("MissingError: Record does not exist")
			}
			for k, x := range args[1].(map[string]any) {
				o.lineas[id][k] = x
			}
		}
	case "account.analytic.line.search_read":
		var l []any
		cumple := func(v map[string]any) bool { // solo las condiciones «=» del dominio
			for _, c := range args[0].([]any) {
				if x, ok := c.([]any); ok && len(x) == 3 && x[1] == "=" && fmt.Sprint(v[x[0].(string)]) != fmt.Sprint(x[2]) {
					return false
				}
				if x, ok := c.([]any); ok && len(x) == 3 && x[1] == "=like" && !strings.HasPrefix(fmt.Sprint(v[x[0].(string)]), strings.TrimSuffix(fmt.Sprint(x[2]), "%")) {
					return false
				}
			}
			return true
		}
		unifica := false // la búsqueda de líneas sueltas lleva un «|» en el dominio
		for _, c := range args[0].([]any) {
			unifica = unifica || c == "|"
		}
		for id, v := range o.lineas {
			if (unifica && v["_suelta"] == true) || (!unifica && v["_suelta"] != true && cumple(v)) {
				l = append(l, map[string]any{"id": id, "date": v["date"], "unit_amount": v["unit_amount"], "project_id": []any{v["project_id"], "MADERAS"},
					"task_id": []any{v["task_id"], "pgocode - Sesión IA"}, "ai_session_id": v["ai_session_id"], "ai_model": v["ai_model"], "name": v["name"]})
			}
		}
		if l == nil {
			l = []any{}
		}
		r = l
	case "account.analytic.line.unlink":
		for _, id := range args[0].([]int) {
			delete(o.lineas, id)
		}
	}
	b, _ := json.Marshal(r)
	return json.Unmarshal(b, res)
}

// lineas con esa marca (ai_session_id).
func (o *odooFalso) con(marca string) []map[string]any {
	o.mu.Lock()
	defer o.mu.Unlock()
	var l []map[string]any
	for _, v := range o.lineas {
		if v["ai_session_id"] == marca {
			c := map[string]any{} // copia: el gestor puede seguir escribiendo la línea
			for k, x := range v {
				c[k] = x
			}
			l = append(l, c)
		}
	}
	return l
}

func (o *odooFalso) n() int { o.mu.Lock(); defer o.mu.Unlock(); return len(o.lineas) }

type reloj struct {
	mu sync.Mutex
	t  time.Time
}

func (r *reloj) ahora() time.Time     { r.mu.Lock(); defer r.mu.Unlock(); return r.t }
func (r *reloj) pasa(d time.Duration) { r.mu.Lock(); r.t = r.t.Add(d); r.mu.Unlock() }

func espera(t *testing.T, cond func() bool) {
	t.Helper()
	for i := 0; i < 1200 && !cond(); i++ { // hasta 6 s: con -race y el equipo cargado va más lento
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatal("no se cumplió a tiempo")
	}
}

var t0 = time.Date(2026, 10, 21, 9, 0, 0, 0, time.Local)

func tr(sesion string) Trabajo {
	return Trabajo{Sesion: sesion, Titulo: "Sesión " + sesion, Proyecto: 447, NombreProy: "MADERAS", UID: 2, OdooSesion: "x", Modelo: "Claude Code opus"}
}

func TestRedondeo(t *testing.T) {
	for _, c := range []struct {
		seg   float64
		clase string
		q     float64
	}{{1, "h", 0.25}, {15 * 60, "h", 0.25}, {16 * 60, "h", 0.5}, {1, "m", 5.0 / 60}, {11 * 60, "m", 15.0 / 60}, {0, "h", 0}} {
		if r := Redondea(c.seg, c.clase); r != c.q {
			t.Errorf("%v s %s: %v, quiero %v", c.seg, c.clase, r, c.q)
		}
	}
}

// Muchos periodos cortos de varias sesiones en el mismo proyecto y tarea: UNA línea de hombre y UNA de máquina
// del día, con la suma (redondeada: hombre a 15 min, máquina a 5 min).
func TestUnaLineaPorDia(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	for i := 0; i < 10; i++ {
		s := fmt.Sprint("s", i%3)
		fin := g.Empieza(tr(s))
		espera(t, func() bool { return len(o.con("pgocode-m-claude-code")) == 1 })
		r.pasa(2 * time.Minute) // 2 min de agente
		fin()
		g.Actividad(tr(s))
		r.pasa(16 * time.Minute) // la presencia dura su mínimo de 15 min; después se cierra el periodo
		g.revisa()
	}
	// Máquina: 10 × 2 min = 20 min. Hombre: 10 tramos de presencia de 15 min (su mínimo) = 150 min.
	g.revisa()
	espera(t, func() bool {
		m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
		return len(m) == 1 && len(h) == 1 && m[0]["unit_amount"] == float64(20)/60 && h[0]["unit_amount"] == 2.5
	})
	m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
	if len(m) != 1 || len(h) != 1 {
		t.Fatalf("líneas: %d máquina, %d hombre (total %d)", len(m), len(h), o.n())
	}
	if m[0]["unit_amount"] != float64(20)/60 || h[0]["unit_amount"] != 2.5 {
		t.Errorf("horas: máquina %v hombre %v", m[0]["unit_amount"], h[0]["unit_amount"])
	}
	if m[0]["ai_model"] != "Claude Code opus" || fmt.Sprint(m[0]["task_id"]) != "7" {
		t.Errorf("máquina: %v", m[0])
	}
	if _, ok := h[0]["ai_model"]; ok {
		t.Error("la línea de hombre no lleva ai_model")
	}
	hoy, _ := g.Estado()
	if len(hoy) != 2 {
		t.Errorf("estado: %+v", hoy)
	}
}

// Otro agente el mismo día, proyecto y tarea: su propia línea de máquina. Sin nadie delante no hay línea de hombre
// (máquina y hombre son independientes).
func TestOtroAgente(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	fa := g.Empieza(tr("a"))
	b := tr("b")
	b.Modelo = "OpenCode google-vertex/gemini-4-argon"
	fb := g.Empieza(b)
	r.pasa(time.Minute)
	fa()
	fb()
	espera(t, func() bool {
		return len(o.con("pgocode-m-claude-code")) == 1 && len(o.con("pgocode-m-opencode")) == 1
	})
	if h := o.con("pgocode-h"); len(h) != 0 {
		t.Errorf("sin presencia humana no hay línea de hombre: %v", h)
	}
}

// Tras actualizarse (Suspende), la versión nueva continúa en las mismas líneas (no crea otras).
func TestContinuaTrasActualizar(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "estado.json")
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora, FicheroEstado: f}
	fin := g.Empieza(tr("s1"))
	espera(t, func() bool { return o.n() == 1 }) // al empezar el turno solo crece la de máquina
	r.pasa(3 * time.Minute)
	fin()
	g.Suspende()
	g2 := &Gestor{Odoo: o, Ahora: r.ahora, FicheroEstado: f}
	ctx, cancela := context.WithCancel(context.Background())
	defer cancela()
	g2.Arranca(ctx)
	g2.Empieza(tr("s1"))()
	r.pasa(10 * time.Minute)
	g2.revisa()
	espera(t, func() bool { m := o.con("pgocode-m-claude-code"); return len(m) == 1 && m[0]["unit_amount"] == 5.0/60 })
	if o.n() != 1 { // solo la de máquina: no hubo nadie delante
		t.Errorf("se han creado líneas de más: %d", o.n())
	}
}

// Odoo caído: no se pierde nada; al volver se escribe la línea del día.
func TestOdooCaido(t *testing.T) {
	o := nuevoOdoo()
	o.falla = true
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Actividad(tr("s1"))
	r.pasa(6 * time.Minute)
	g.revisa()
	time.Sleep(20 * time.Millisecond)
	if o.n() != 0 {
		t.Fatal("con Odoo caído no debería haber líneas")
	}
	o.mu.Lock()
	o.falla = false
	o.mu.Unlock()
	g.revisa()
	espera(t, func() bool { h := o.con("pgocode-h"); return len(h) == 1 && h[0]["unit_amount"] == 0.25 })
}

// A medianoche el periodo se cierra; el día siguiente va a otra línea.
func TestMedianoche(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: time.Date(2026, 10, 21, 23, 58, 0, 0, time.Local)}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Empieza(tr("s1"))
	espera(t, func() bool { return o.n() >= 1 })
	r.pasa(4 * time.Minute)
	g.revisa()
	// El de ayer se cierra; el turno (sin acabar) sigue en un periodo de hoy que empieza a las 00:00.
	if _, ab := g.Estado(); len(ab) != 1 || ab[0].Inicio.Day() != 22 || ab[0].Inicio.Hour() != 0 || !ab[0].Agente {
		t.Fatalf("a medianoche el periodo se cierra y el turno sigue en el día nuevo: %+v", ab)
	}
}

// Las líneas sueltas de versiones anteriores (menos de 5 min) se suman a la línea del día y se borran.
func TestUnificaSueltas(t *testing.T) {
	o := nuevoOdoo()
	for i := 0; i < 17; i++ {
		o.lineas[i+1] = map[string]any{"_suelta": true, "date": "2026-10-21", "unit_amount": 1.0 / 60, "project_id": 447, "task_id": 9,
			"ai_session_id": fmt.Sprintf("pgocode-abierto-s%d-h", i), "name": "Sesión de pgocode «Sitio web»"}
	}
	o.lineas[50] = map[string]any{"_suelta": true, "date": "2026-10-21", "unit_amount": 2.0 / 60, "project_id": 447, "task_id": 9,
		"ai_session_id": "pgocode-s1-m", "ai_model": "Claude Code opus", "name": "x"}
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	res, err := g.UnificaSueltas(context.Background(), 2, "x")
	if err != nil || res.Lineas != 18 || res.Fallos != 0 {
		t.Fatalf("resultado %+v err %v", res, err)
	}
	h, m := o.con("pgocode-h"), o.con("pgocode-m-claude-code")
	if o.n() != 2 || len(h) != 1 || len(m) != 1 {
		t.Fatalf("quedan %d líneas: %v", o.n(), o.lineas)
	}
	// 17 min de hombre → 30 min (bloques de 15); 2 min de máquina → 5 min.
	if h[0]["unit_amount"] != 0.5 || m[0]["unit_amount"] != 5.0/60 || fmt.Sprint(h[0]["task_id"]) != "9" {
		t.Errorf("hombre %v máquina %v", h[0], m[0])
	}
}

// Odoo sin ai_session_id: la línea de máquina lleva igualmente ai_model y tokens, y la del día se reconoce por su
// descripción (no se duplica tras perder el fichero local).
func TestOdooSinSesion(t *testing.T) {
	o := nuevoOdoo()
	o.sinSes = true
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	fin := g.Empieza(tr("s1"))
	r.pasa(3 * time.Minute)
	fin()
	g.revisa()
	maquina := func() map[string]any {
		o.mu.Lock()
		defer o.mu.Unlock()
		for _, v := range o.lineas {
			if strings.HasPrefix(fmt.Sprint(v["name"]), "pgocode (agente Claude Code)") {
				return v
			}
		}
		return nil
	}
	espera(t, func() bool { return maquina() != nil })
	o.mu.Lock()
	var m map[string]any
	for _, v := range o.lineas {
		if strings.HasPrefix(fmt.Sprint(v["name"]), "pgocode (agente Claude Code)") {
			m = v
		}
	}
	o.mu.Unlock()
	if m == nil || m["ai_model"] != "Claude Code opus" {
		t.Fatalf("línea de máquina: %v", o.lineas)
	}
	if _, ok := m["ai_session_id"]; ok {
		t.Error("no debe enviar ai_session_id a un Odoo que no lo tiene")
	}
}

// Presencia humana: cada tramo dura como mínimo 15 minutos desde que empieza (un valor absoluto, no por
// intervención); pasado el mínimo, sigue hasta 5 minutos sin intervenir. Máquina y hombre son independientes.
func TestMinimoHumano(t *testing.T) {
	hombre := func(g *Gestor, s string) float64 {
		g.mu.Lock()
		defer g.mu.Unlock()
		if p := g.periodos[s]; p != nil {
			_, h := g.partes(p)
			return h
		}
		return -1
	}
	cerrado := func(g *Gestor) float64 {
		g.mu.Lock()
		defer g.mu.Unlock()
		for _, d := range g.dias {
			if d.Clase == "h" {
				return d.Cerrado
			}
		}
		return 0
	}
	for _, c := range []struct {
		nombre string
		toques []time.Duration // momentos de las intervenciones
		quiero time.Duration
	}{
		{"una sola intervención: 15 min", []time.Duration{0}, 15 * time.Minute},
		{"varias dentro del mínimo: 15 min, no 15 por cada una", []time.Duration{0, time.Minute, 3 * time.Minute, 9 * time.Minute}, 15 * time.Minute},
		{"pasado el mínimo sigue 5 min tras la última", []time.Duration{0, 14 * time.Minute, 18 * time.Minute}, 23 * time.Minute},
		{"dos tramos separados: 15 + 15", []time.Duration{0, 40 * time.Minute}, 30 * time.Minute},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			r := &reloj{t: t0}
			g := &Gestor{Odoo: nuevoOdoo(), Ahora: r.ahora}
			antes := time.Duration(0)
			for _, x := range c.toques {
				r.pasa(x - antes)
				antes = x
				g.Actividad(tr("s"))
				g.revisa()
			}
			// Mientras dura el mínimo, el periodo no se cierra aunque no haya nadie.
			r.pasa(10 * time.Minute)
			g.revisa()
			if hombre(g, "s") < 0 && c.quiero > 0 && len(c.toques) == 1 {
				t.Fatal("el periodo se ha cerrado antes de su mínimo de 15 minutos")
			}
			r.pasa(time.Hour)
			g.revisa()
			if h := cerrado(g); math.Abs(h-c.quiero.Seconds()) > 0.5 {
				t.Errorf("hombre %.0f s, quiero %.0f s", h, c.quiero.Seconds())
			}
		})
	}
}

// Un turno de 40 min empezado por una intervención: 40 min de máquina y 15 de hombre (a la vez, no repartidos).
func TestHombreYMaquinaIndependientes(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: t0}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	g.Actividad(tr("s"))
	fin := g.Empieza(tr("s"))
	r.pasa(40 * time.Minute)
	fin()
	r.pasa(10 * time.Minute)
	g.revisa()
	espera(t, func() bool {
		m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
		return len(m) == 1 && len(h) == 1 && m[0]["unit_amount"] == 40.0/60 && h[0]["unit_amount"] == 0.25
	})
}

// Un turno que pasa de la medianoche: el de ayer se cierra a las 00:00 y el tiempo de después va a un periodo de
// hoy con el mismo agente (no se pierde ni va a un periodo sin agente).
func TestTurnoTrasMedianoche(t *testing.T) {
	o := nuevoOdoo()
	r := &reloj{t: time.Date(2026, 10, 21, 23, 50, 0, 0, time.Local)}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	fin := g.Empieza(tr("s1"))
	r.pasa(20 * time.Minute) // hasta las 00:10
	g.revisa()
	g.mu.Lock()
	p := g.periodos["s1"]
	g.mu.Unlock()
	if p == nil || p.Inicio.Day() != 22 || p.turnos != 1 || p.T.Modelo != "Claude Code opus" {
		t.Fatalf("periodo de hoy: %+v", p)
	}
	fin()
	r.pasa(10 * time.Minute)
	g.revisa()
	espera(t, func() bool {
		ayer, hoy := 0.0, 0.0
		for _, v := range o.con("pgocode-m-claude-code") {
			switch v["date"] {
			case "2026-10-21":
				ayer = v["unit_amount"].(float64)
			case "2026-10-22":
				hoy = v["unit_amount"].(float64)
			}
		}
		return ayer == 10.0/60 && hoy == 10.0/60 // 10 min antes y 10 después (bloques de 5)
	})
}

// Sin campos de IA (sin el módulo que los añade), se vuelve a preguntar pasados 10 minutos: si se instala con la app
// abierta, los ve sin reiniciar. Con ellos, se pregunta una sola vez.
func TestCamposIAReintento(t *testing.T) {
	o := nuevoOdoo()
	o.sinIA = true
	r := &reloj{t: time.Date(2026, 10, 23, 10, 0, 0, 0, time.Local)}
	g := &Gestor{Odoo: o, Ahora: r.ahora}
	ctx := context.Background()
	if c := g.CamposIA(ctx, "s"); len(c) != 0 {
		t.Fatalf("sin módulo: %v", c)
	}
	r.pasa(9 * time.Minute)
	g.CamposIA(ctx, "s")
	if o.nCampos != 1 {
		t.Fatalf("antes de 10 min no se vuelve a preguntar: %d consultas", o.nCampos)
	}
	o.mu.Lock()
	o.sinIA = false // se instala el módulo
	o.mu.Unlock()
	r.pasa(2 * time.Minute)
	if c := g.CamposIA(ctx, "s"); !c["ai_model"] || o.nCampos != 2 {
		t.Fatalf("pasados 10 min: %v (%d consultas)", c, o.nCampos)
	}
	r.pasa(time.Hour)
	g.CamposIA(ctx, "s")
	if o.nCampos != 2 {
		t.Errorf("con campos, una sola vez: %d consultas", o.nCampos)
	}
}
