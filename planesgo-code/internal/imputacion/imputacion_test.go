package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	sinSes bool // Odoo sin el campo ai_session_id (como el de Planes)
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
		r.pasa(6 * time.Minute) // y la espera de 5 min, que cierra el periodo
		g.revisa()
	}
	// Máquina: 10 × 2 min = 20 min → 20 min. Hombre: 10 × 5 min = 50 min → 60 min (bloques de 15).
	g.revisa()
	espera(t, func() bool {
		m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
		return len(m) == 1 && len(h) == 1 && m[0]["unit_amount"] == float64(20)/60 && h[0]["unit_amount"] == 1.0
	})
	m, h := o.con("pgocode-m-claude-code"), o.con("pgocode-h")
	if len(m) != 1 || len(h) != 1 {
		t.Fatalf("líneas: %d máquina, %d hombre (total %d)", len(m), len(h), o.n())
	}
	// Máquina: 10 × 2 min = 20 min → 20 min. Hombre: 10 × 5 min = 50 min → 60 min (bloques de 15).
	if m[0]["unit_amount"] != float64(20)/60 || h[0]["unit_amount"] != 1.0 {
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

// Otro agente el mismo día, proyecto y tarea: su propia línea de máquina; la de hombre es la misma.
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
		return len(o.con("pgocode-m-claude-code")) == 1 && len(o.con("pgocode-m-opencode")) == 1 && len(o.con("pgocode-h")) == 1
	})
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
	if o.n() != 2 {
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
	if _, ab := g.Estado(); len(ab) != 0 {
		t.Fatal("a medianoche el periodo se cierra")
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
