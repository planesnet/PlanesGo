package imputacion

// Escenarios de imputación: lo que pasa en una o varias sesiones (turnos del agente, presencia de la persona,
// medianoche, actualizaciones, cierres, Odoo caído, sesión caducada, líneas borradas…) y las líneas del día que
// tienen que quedar en Odoo. Los mismos escenarios se prueban con un Odoo en memoria (go test) y con el Odoo de
// verdad (cmd/imputacion-prueba, en el proyecto PRUEBAS PLANESGO): lo que valida el test es lo que se comprueba
// en Odoo. El reloj es simulado: cada escenario vive sus horas en segundos.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LineaEsperada: una línea del día que tiene que haber en Odoo al acabar (o en una comprobación intermedia).
type LineaEsperada struct {
	Dia         int     // 0: el día de la prueba; -1: el anterior
	Tarea       string  // la tarea: «» la del escenario; «#x» la del escenario con el sufijo « · x»; si no, ese nombre
	Clase       string  // h (hombre) | m (máquina)
	Agente      string  // en las de máquina
	Horas       float64 // unit_amount
	Descripcion string  // si no está vacía, la descripción ha de contener cada parte (separadas por « + », en cualquier orden)
	Tokens      int     // si > 0 y Odoo tiene ai_tokens_total, han de coincidir
}

// Escenario de imputación.
type Escenario struct {
	Clave  string // E01…
	Nombre string
	Pasos  func(s *Sim)
	Lineas []LineaEsperada // al acabar (solo estas: cualquier otra línea de sus tareas es un fallo)
}

// LineaOdoo: una línea de pgocode leída de Odoo.
type LineaOdoo struct {
	ID                                  int
	Fecha, Tarea, Clase, Agente, Nombre string
	Horas                               float64
	Tokens                              int
}

func (l LineaOdoo) String() string {
	a := ""
	if l.Agente != "" {
		a = " " + l.Agente
	}
	return fmt.Sprintf("%s «%s» %s%s %.4f h (línea %d)", l.Fecha, l.Tarea, l.Clase, a, l.Horas, l.ID)
}

// Banco: dónde se ejecutan los escenarios.
type Banco struct {
	Odoo       Odoo   // el Odoo (en memoria o el de verdad)
	Sesion     string // la sesión de Odoo del usuario
	UID        int
	Proyecto   int
	NombreProy string
	Dia        time.Time // el día de la prueba (se toma su fecha)
	Dir        string    // carpeta para el estado local de cada escenario
}

// Sim: un escenario en marcha.
type Sim struct {
	b      Banco
	e      Escenario
	reloj  time.Time
	mu     sync.Mutex
	g      *Gestor
	cancel context.CancelFunc
	odoo   *odooCortable
	sesion string // la sesión de Odoo con la que trabajan las sesiones de pgocode (cambia al caducar)
	dir    string
	fallos []string
	tareas map[string]int // tareas creadas por el escenario (nombre → id)
}

// odooCortable: el Odoo del banco, que se puede «caer» y que rechaza la sesión caducada.
type odooCortable struct {
	Odoo
	mu    sync.Mutex
	caido bool
}

const sesionCaducada = "sesion-caducada-de-prueba"

func (o *odooCortable) CallKW(ctx context.Context, ses, modelo, metodo string, args []any, kw map[string]any, res any) error {
	o.mu.Lock()
	caido := o.caido
	o.mu.Unlock()
	if caido {
		return errors.New("Odoo caído (simulado)")
	}
	if ses == sesionCaducada {
		return errors.New("Session expired (simulado)")
	}
	return o.Odoo.CallKW(ctx, ses, modelo, metodo, args, kw, res)
}

func (s *Sim) ahora() time.Time { s.mu.Lock(); defer s.mu.Unlock(); return s.reloj }

// En pone el reloj en una hora del día de la prueba (dia 0) o del anterior (-1): «09:30». No puede ir hacia atrás.
func (s *Sim) En(dia int, hora string) {
	var h, m int
	fmt.Sscanf(hora, "%d:%d", &h, &m)
	y, mo, d := s.b.Dia.Date()
	t := time.Date(y, mo, d+dia, h, m, 0, 0, time.Local)
	s.mu.Lock()
	if t.Before(s.reloj) {
		s.mu.Unlock()
		s.falla("el reloj no puede ir hacia atrás (%s)", hora)
		return
	}
	s.reloj = t
	s.mu.Unlock()
}

func (s *Sim) falla(f string, a ...any) { s.fallos = append(s.fallos, fmt.Sprintf(f, a...)) }

// nombreTarea: la tarea de una línea esperada o de un trabajo («#x»: la del escenario con un sufijo).
func (s *Sim) nombreTarea(t string) string {
	base := s.e.Clave + " · " + s.e.Nombre
	switch {
	case t == "":
		return base
	case strings.HasPrefix(t, "#"):
		return base + " · " + t[1:]
	}
	return t
}

var modelos = map[string]string{"Claude Code": "Claude Code opus", "OpenCode": "OpenCode google-vertex/gemini-4-argon", "Antigravity": "Antigravity gemini-4-argon-medium"}

// Trabajo de una sesión del escenario (sesion: «A», «B»…; tarea como en LineaEsperada; «-» la de por defecto).
func (s *Sim) Trabajo(sesion, agente, tarea string) Trabajo {
	t := Trabajo{Sesion: s.e.Clave + "-" + sesion, Titulo: "Sesión " + sesion, Proyecto: s.b.Proyecto, NombreProy: s.b.NombreProy,
		Tarea: s.nombreTarea(tarea), UID: s.b.UID, OdooSesion: s.sesionActual(), Modelo: modelos[agente]}
	if tarea == "-" {
		t.Tarea = ""
	}
	if id := s.tareas[t.Tarea]; id > 0 {
		t.TareaID = id
	}
	return t
}

func (s *Sim) sesionActual() string { s.mu.Lock(); defer s.mu.Unlock(); return s.sesion }

// Turno: empieza un turno del agente en la sesión; devuelve su final.
func (s *Sim) Turno(sesion, agente, tarea string) (fin func()) {
	f := s.g.Empieza(s.Trabajo(sesion, agente, tarea))
	s.g.Espera()
	return func() { f(); s.g.Espera() }
}

// Persona: la persona interviene en la sesión.
func (s *Sim) Persona(sesion, tarea string) {
	s.g.Actividad(s.Trabajo(sesion, "", tarea))
	s.g.Espera()
}

// Uso: consumo de IA de un turno.
func (s *Sim) Uso(sesion string, entrada, salida int, coste float64) {
	s.g.SumaUso(s.e.Clave+"-"+sesion, entrada, salida, coste)
}

// Revisa: lo que hace el reloj de pgocode cada 15 s (cerrar periodos, escribir las líneas).
func (s *Sim) Revisa() { s.g.revisa(); s.g.Espera() }

// Cierra el periodo de una sesión (p. ej. al cerrarla).
func (s *Sim) Cierra(sesion string) { s.g.Cierra(s.e.Clave+"-"+sesion, "cerrada a mano"); s.g.Espera() }

// CierraTodo: se sale de pgocode.
func (s *Sim) CierraTodo() { s.g.CierraTodo(); s.g.Espera() }

// Reinicia pgocode: tras actualizarse (suspende: escribe y guarda antes) o tras un cierre inesperado (sin más).
// pierde: además se pierde el fichero local (p. ej. otra instalación).
func (s *Sim) Reinicia(suspende, pierde bool) {
	if suspende {
		s.g.Suspende()
	}
	s.g.Espera()
	s.cancel()
	if pierde {
		os.Remove(s.g.FicheroEstado)
	}
	s.nuevoGestor()
	s.g.Espera()
}

// OdooCae / OdooVuelve: Odoo deja de responder y vuelve.
func (s *Sim) OdooCae()    { s.odoo.mu.Lock(); s.odoo.caido = true; s.odoo.mu.Unlock() }
func (s *Sim) OdooVuelve() { s.odoo.mu.Lock(); s.odoo.caido = false; s.odoo.mu.Unlock() }

// Caduca la sesión de Odoo (lo que se haga desde ahora falla) y ReEntra: el usuario vuelve a entrar.
func (s *Sim) Caduca() { s.mu.Lock(); s.sesion = sesionCaducada; s.mu.Unlock() }
func (s *Sim) ReEntra() {
	s.mu.Lock()
	s.sesion = s.b.Sesion
	s.mu.Unlock()
	s.g.ActualizaSesionOdoo(s.b.UID, s.b.Sesion)
}

// CreaTarea: una tarea del escenario elegida en los ajustes de la sesión (con su id).
func (s *Sim) CreaTarea(tarea string) {
	n := s.nombreTarea(tarea)
	var id int
	if err := s.b.Odoo.CallKW(context.Background(), s.b.Sesion, "project.task", "create", []any{map[string]any{"name": n, "project_id": s.b.Proyecto}}, nil, &id); err != nil {
		s.falla("crear la tarea %s: %v", n, err)
		return
	}
	s.tareas[n] = id
}

// BorraLinea: el usuario borra en Odoo la línea del día de una clase (y agente).
func (s *Sim) BorraLinea(clase, agente, tarea string) {
	for _, l := range s.Lineas() {
		if l.Clase == clase && l.Agente == agente && l.Tarea == s.nombreTarea(tarea) {
			var nada any
			if err := s.b.Odoo.CallKW(context.Background(), s.b.Sesion, "account.analytic.line", "unlink", []any{[]int{l.ID}}, nil, &nada); err != nil {
				s.falla("borrar la línea %d: %v", l.ID, err)
			}
			return
		}
	}
	s.falla("no hay línea %s %s que borrar", clase, agente)
}

// Comprueba (a mitad del escenario) que las líneas son exactamente estas.
func (s *Sim) Comprueba(paso string, esperadas []LineaEsperada) {
	for _, f := range s.compara(esperadas) {
		s.falla("%s: %s", paso, f)
	}
}

// idsTareas: las tareas del escenario en Odoo, por nombre (y «Desarrollo» si es la que usa: la tarea por defecto).
func (s *Sim) idsTareas() map[int]string {
	var l []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	dom := []any{[]any{"project_id", "=", s.b.Proyecto}, []any{"name", "=like", s.e.Clave + " · %"}}
	for _, e := range s.e.Lineas {
		if e.Tarea == TareaDefecto {
			dom = []any{[]any{"project_id", "=", s.b.Proyecto}, "|", []any{"name", "=like", s.e.Clave + " · %"}, []any{"name", "=", TareaDefecto}}
			break
		}
	}
	s.b.Odoo.CallKW(context.Background(), s.b.Sesion, "project.task", "search_read", []any{dom}, map[string]any{"fields": []string{"id", "name"}}, &l)
	m := map[int]string{}
	for _, x := range l {
		m[x.ID] = x.Name
	}
	return m
}

// Lineas: las líneas de pgocode del escenario en Odoo (de sus tareas, el día de la prueba y el anterior).
func (s *Sim) Lineas() []LineaOdoo {
	tareas := s.idsTareas()
	if len(tareas) == 0 {
		return nil
	}
	ids := make([]any, 0, len(tareas))
	for id := range tareas {
		ids = append(ids, id)
	}
	y, m, d := s.b.Dia.Date()
	ayer := time.Date(y, m, d-1, 12, 0, 0, 0, time.Local).Format("2006-01-02")
	dom := []any{[]any{"project_id", "=", s.b.Proyecto}, []any{"task_id", "in", ids}, []any{"date", ">=", ayer},
		[]any{"name", "=like", "pgocode (%"}}
	if s.b.UID > 0 {
		dom = append(dom, []any{"user_id", "=", s.b.UID})
	}
	campos := []string{"id", "date", "task_id", "unit_amount", "name"}
	conTokens := s.g != nil && s.g.CamposIA(context.Background(), s.b.Sesion)["ai_tokens_total"]
	if conTokens {
		campos = append(campos, "ai_tokens_total")
	}
	var l []struct {
		ID     int     `json:"id"`
		Date   string  `json:"date"`
		Task   []any   `json:"task_id"`
		Horas  float64 `json:"unit_amount"`
		Name   string  `json:"name"`
		Tokens float64 `json:"ai_tokens_total"`
	}
	if err := s.b.Odoo.CallKW(context.Background(), s.b.Sesion, "account.analytic.line", "search_read", []any{dom}, map[string]any{"fields": campos}, &l); err != nil {
		s.falla("leer las líneas de Odoo: %v", err)
		return nil
	}
	var r []LineaOdoo
	for _, x := range l {
		lo := LineaOdoo{ID: x.ID, Fecha: x.Date, Horas: x.Horas, Nombre: x.Name, Tokens: int(x.Tokens)}
		if len(x.Task) > 0 {
			if id, ok := x.Task[0].(float64); ok {
				lo.Tarea = tareas[int(id)]
			}
		}
		lo.Clase = "h"
		if a, ok := strings.CutPrefix(x.Name, "pgocode (agente "); ok {
			ag, _, _ := strings.Cut(a, ")")
			lo.Clase, lo.Agente = "m", ag
		}
		r = append(r, lo)
	}
	sort.Slice(r, func(i, j int) bool { return r[i].String() < r[j].String() })
	return r
}

// compara las líneas de Odoo con las esperadas: cada esperada una vez y ninguna más.
func (s *Sim) compara(esperadas []LineaEsperada) []string {
	hay := s.Lineas()
	usada := make([]bool, len(hay))
	var f []string
	y, m, d := s.b.Dia.Date()
	for _, e := range esperadas {
		fecha := time.Date(y, m, d+e.Dia, 12, 0, 0, 0, time.Local).Format("2006-01-02")
		tarea := s.nombreTarea(e.Tarea)
		encontrada := false
		for i, l := range hay {
			if usada[i] || l.Fecha != fecha || l.Tarea != tarea || l.Clase != e.Clase || l.Agente != e.Agente {
				continue
			}
			usada[i], encontrada = true, true
			if math.Abs(l.Horas-e.Horas) > 1e-4 {
				f = append(f, fmt.Sprintf("%s: %.4f h, se esperaban %.4f h", l, l.Horas, e.Horas))
			}
			for _, parte := range strings.Split(e.Descripcion, " + ") {
				if parte != "" && !strings.Contains(l.Nombre, parte) {
					f = append(f, fmt.Sprintf("%s: la descripción «%s» no contiene «%s»", l, l.Nombre, parte))
				}
			}
			if e.Tokens > 0 && s.g.CamposIA(context.Background(), s.b.Sesion)["ai_tokens_total"] && l.Tokens != e.Tokens {
				f = append(f, fmt.Sprintf("%s: %d tokens, se esperaban %d", l, l.Tokens, e.Tokens))
			}
			break
		}
		if !encontrada {
			a := ""
			if e.Agente != "" {
				a = " " + e.Agente
			}
			f = append(f, fmt.Sprintf("falta la línea %s «%s» %s%s %.4f h", fecha, tarea, e.Clase, a, e.Horas))
		}
	}
	for i, l := range hay {
		if !usada[i] {
			f = append(f, "sobra la línea "+l.String())
		}
	}
	return f
}

func (s *Sim) nuevoGestor() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.g = &Gestor{Odoo: s.odoo, Ahora: s.ahora, FicheroEstado: filepath.Join(s.dir, "estado.json"), Diario: filepath.Join(s.dir, "diario.log")}
	s.g.arrancaSinReloj(ctx)
}

// Limpia: borra de Odoo las líneas de pgocode de las tareas del escenario (de una ejecución anterior).
func (b Banco) Limpia(e Escenario) error {
	s := &Sim{b: b, e: e}
	var ids []int
	for _, l := range s.Lineas() {
		ids = append(ids, l.ID)
	}
	if len(s.fallos) > 0 {
		return errors.New(strings.Join(s.fallos, "; "))
	}
	if len(ids) == 0 {
		return nil
	}
	var nada any
	return b.Odoo.CallKW(context.Background(), b.Sesion, "account.analytic.line", "unlink", []any{ids}, nil, &nada)
}

// Ejecuta un escenario: sus pasos y la comprobación final. Devuelve las líneas que quedan en Odoo y los fallos.
func (b Banco) Ejecuta(e Escenario) ([]LineaOdoo, []string) {
	y, m, d := b.Dia.Date()
	s := &Sim{b: b, e: e, reloj: time.Date(y, m, d-1, 0, 0, 0, 0, time.Local), odoo: &odooCortable{Odoo: b.Odoo}, sesion: b.Sesion,
		dir: filepath.Join(b.Dir, e.Clave), tareas: map[string]int{}}
	os.RemoveAll(s.dir)
	os.MkdirAll(s.dir, 0o700)
	s.nuevoGestor()
	e.Pasos(s)
	s.g.Espera()
	s.cancel()
	for _, f := range s.compara(e.Lineas) {
		s.falla("al acabar: %s", f)
	}
	return s.Lineas(), s.fallos
}

// arrancaSinReloj: como Arranca (retoma el fichero local) pero sin el reloj de 15 s: en los escenarios el tiempo
// lo lleva la simulación (Sim.Revisa).
func (g *Gestor) arrancaSinReloj(ctx context.Context) {
	g.arrancaCon(ctx, false)
}

// ---- los escenarios ----

const (
	min5  = 5.0 / 60
	min10 = 10.0 / 60
	min15 = 0.25
	min20 = 20.0 / 60
	min30 = 0.5
)

// Escenarios: todos los casos de imputación que se prueban.
func Escenarios() []Escenario {
	return []Escenario{
		{Clave: "E01", Nombre: "Turno del agente sin nadie delante",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:16")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10, Descripcion: "pgocode (agente Claude Code): «Sesión A»"}}},

		{Clave: "E02", Nombre: "Una sola intervención de la persona",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Persona("A", "")
				s.En(0, "09:20")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "h", Horas: min15, Descripcion: "pgocode (trabajo): «Sesión A»"}}},

		{Clave: "E03", Nombre: "Presencia continua de 40 minutos",
			Pasos: func(s *Sim) {
				for m := 0; m <= 40; m += 4 {
					s.En(0, fmt.Sprintf("09:%02d", m))
					s.Persona("A", "")
					s.Revisa()
				}
				s.En(0, "10:00")
				s.Revisa()
			},
			// De 09:00 a 09:45 (5 min tras la última intervención): 45 min.
			Lineas: []LineaEsperada{{Clase: "h", Horas: 0.75}}},

		{Clave: "E04", Nombre: "Dos tramos de presencia con una pausa",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Persona("A", "")
				s.En(0, "09:30")
				s.Revisa() // la presencia acabó a las 09:15 y el periodo se cierra
				s.Persona("A", "")
				s.En(0, "10:00")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "h", Horas: min30}}},

		{Clave: "E05", Nombre: "Persona y agente a la vez",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				for m := 0; m <= 28; m += 4 {
					s.En(0, fmt.Sprintf("09:%02d", m))
					s.Persona("A", "")
				}
				s.En(0, "09:30")
				fin()
				s.En(0, "10:00")
				s.Revisa()
			},
			// Máquina 30 min; hombre de 09:00 a 09:33 → 45 min (bloques de 15).
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min30}, {Clase: "h", Horas: 0.75}}},

		{Clave: "E06", Nombre: "La misma persona en dos sesiones a la vez",
			Pasos: func(s *Sim) {
				for m := 0; m <= 30; m += 5 {
					s.En(0, fmt.Sprintf("09:%02d", m))
					if m <= 20 {
						s.Persona("A", "")
					}
					if m >= 10 {
						s.Persona("B", "")
					}
				}
				s.En(0, "10:00")
				s.Revisa()
			},
			// A de 09:00 a 09:25 y B de 09:10 a 09:35: la persona estuvo 35 min (no 50) → 45 min.
			Lineas: []LineaEsperada{{Clase: "h", Horas: 0.75, Descripcion: "«Sesión A» + «Sesión B»"}}},

		{Clave: "E07", Nombre: "Dos agentes distintos en la misma tarea",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				a := s.Turno("A", "Claude Code", "")
				b := s.Turno("B", "OpenCode", "")
				s.En(0, "09:10")
				a()
				s.En(0, "09:20")
				b()
				s.En(0, "09:30")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}, {Clase: "m", Agente: "OpenCode", Horas: min20}}},

		{Clave: "E08", Nombre: "El mismo agente en dos sesiones solapadas",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				a := s.Turno("A", "Claude Code", "")
				s.En(0, "09:05")
				b := s.Turno("B", "Claude Code", "")
				s.En(0, "09:10")
				a()
				s.En(0, "09:15")
				b()
				s.En(0, "09:30")
				s.Revisa()
			},
			// El tiempo de máquina es el de cada instancia del agente: 10 + 10 min.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20, Descripcion: "«Sesión A» + «Sesión B»"}}},

		{Clave: "E09", Nombre: "Varias sesiones seguidas el mismo día",
			Pasos: func(s *Sim) {
				for i, h := range []string{"09", "10", "11"} {
					s.En(0, h+":00")
					fin := s.Turno(string(rune('A'+i)), "Claude Code", "")
					s.En(0, h+":06")
					fin()
					s.En(0, h+":20")
					s.Revisa()
				}
			},
			// 3 × 6 min = 18 min → 20 min (bloques de 5): una sola línea con las tres sesiones.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20, Descripcion: "«Sesión A» + «Sesión B» + «Sesión C»"}}},

		{Clave: "E10", Nombre: "Cambio de tarea en la misma sesión",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "#uno")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:12")
				fin = s.Turno("A", "Claude Code", "#dos")
				s.En(0, "09:22")
				fin()
				s.En(0, "09:30")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Tarea: "#uno", Clase: "m", Agente: "Claude Code", Horas: min10},
				{Tarea: "#dos", Clase: "m", Agente: "Claude Code", Horas: min10}}},

		{Clave: "E11", Nombre: "Tarea por defecto Desarrollo",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "-")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:20")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Tarea: TareaDefecto, Clase: "m", Agente: "Claude Code", Horas: min10}}},

		{Clave: "E12", Nombre: "Tarea elegida en la sesión",
			Pasos: func(s *Sim) {
				s.CreaTarea("#elegida")
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "#elegida")
				s.Persona("A", "#elegida")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:30")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Tarea: "#elegida", Clase: "m", Agente: "Claude Code", Horas: min10}, {Tarea: "#elegida", Clase: "h", Horas: min15}}},

		{Clave: "E13", Nombre: "Turno que pasa la medianoche",
			Pasos: func(s *Sim) {
				s.En(-1, "23:50")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "00:01")
				s.Revisa()
				s.En(0, "00:20")
				fin()
				s.En(0, "00:30")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Dia: -1, Clase: "m", Agente: "Claude Code", Horas: min10}, {Clase: "m", Agente: "Claude Code", Horas: min20}}},

		{Clave: "E14", Nombre: "Presencia que pasa la medianoche",
			Pasos: func(s *Sim) {
				s.En(-1, "23:55")
				s.Persona("A", "")
				s.En(0, "00:02")
				s.Persona("A", "")
				s.En(0, "00:30")
				s.Revisa()
			},
			// Ayer de 23:55 a 00:00 (5 min → 15); hoy de 00:02 a 00:17 (15).
			Lineas: []LineaEsperada{{Dia: -1, Clase: "h", Horas: min15}, {Clase: "h", Horas: min15}}},

		{Clave: "E15", Nombre: "Actualización de pgocode con la persona presente",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:11")
				s.Persona("A", "")
				s.En(0, "09:12")
				s.Reinicia(true, false)
				s.En(0, "09:20")
				s.Persona("A", "")
				s.En(0, "09:40")
				s.Revisa()
			},
			// Hombre de 09:11 a 09:26 (15 min); máquina 10.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}, {Clase: "h", Horas: min15}}},

		{Clave: "E16", Nombre: "Cierre inesperado con un turno en marcha",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Turno("A", "Claude Code", "")
				for m := 1; m <= 10; m++ { // el reloj de pgocode sigue revisando mientras trabaja el agente
					s.En(0, fmt.Sprintf("09:%02d", m))
					s.Revisa()
				}
				s.Reinicia(false, false) // se cae a las 09:10: el turno no acaba
				s.En(0, "09:30")
				s.Revisa()
				fin := s.Turno("A", "Claude Code", "") // y después se sigue trabajando
				s.En(0, "09:40")
				fin()
				s.En(0, "09:50")
				s.Revisa()
			},
			// El turno cortado cuenta hasta la última revisión antes de caerse (09:10): 10 min; más los 10 de después.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20}}},

		{Clave: "E17", Nombre: "Se sale de pgocode con trabajo abierto",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Turno("A", "Claude Code", "")
				s.Persona("A", "")
				s.En(0, "09:10")
				s.CierraTodo()
			},
			// Máquina hasta la salida (10 min); la presencia cuenta su mínimo de 15.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}, {Clase: "h", Horas: min15}}},

		{Clave: "E18", Nombre: "Odoo caído y vuelve",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.OdooCae()
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:20")
				fin()
				s.En(0, "09:30")
				s.Revisa()
				s.OdooVuelve()
				s.En(0, "09:31")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20}}},

		{Clave: "E19", Nombre: "Sesión de Odoo caducada y se vuelve a entrar",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Caduca()
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:20")
				s.Revisa() // el periodo se cierra sin poder resolver su tarea
				s.ReEntra()
				s.En(0, "09:21")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}}},

		{Clave: "E20", Nombre: "El usuario borra la línea del día en Odoo",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:16")
				s.Revisa()
				s.Comprueba("antes de borrar", []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}})
				s.BorraLinea("m", "Claude Code", "")
				s.En(0, "09:30")
				fin = s.Turno("A", "Claude Code", "")
				s.En(0, "09:40")
				fin()
				s.En(0, "09:50")
				s.Revisa()
			},
			// Se crea otra línea con todo lo del día (lo borrado y lo nuevo).
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20}}},

		{Clave: "E21", Nombre: "Se pierde el estado local",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:16")
				s.Revisa()
				s.Reinicia(false, true)
				s.En(0, "09:30")
				fin = s.Turno("A", "Claude Code", "")
				s.En(0, "09:40")
				fin()
				s.En(0, "09:50")
				s.Revisa()
			},
			// La línea de Odoo se encuentra y se sigue sumando sobre ella.
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min20}}},

		{Clave: "E22", Nombre: "Consumo de IA en la línea del agente",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:05")
				s.Uso("A", 120000, 3000, 0.42)
				fin()
				s.En(0, "09:15")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min5, Tokens: 123000}}},

		{Clave: "E23", Nombre: "La línea crece mientras trabaja el agente",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:01")
				s.Revisa()
				s.Comprueba("recién empezado", []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min5}})
				s.En(0, "09:13")
				s.Revisa()
				s.Comprueba("a los 13 min", []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min15}})
				s.En(0, "09:30")
				fin()
				s.En(0, "09:40")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min30}}},

		{Clave: "E24", Nombre: "Muchas sesiones en la descripción",
			Pasos: func(s *Sim) {
				for i := 0; i < 8; i++ {
					s.En(0, fmt.Sprintf("%02d:00", 9+i))
					fin := s.Turno(string(rune('A'+i)), "Claude Code", "")
					s.En(0, fmt.Sprintf("%02d:01", 9+i))
					fin()
					s.En(0, fmt.Sprintf("%02d:10", 9+i))
					s.Revisa()
				}
			},
			// 8 × 1 min = 8 min → 10 min; en la descripción, las 6 primeras «y 2 más».
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10, Descripcion: "«Sesión F» y 2 más"}}},

		{Clave: "E25", Nombre: "La misma persona en dos tareas a la vez",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Persona("A", "#uno")
				s.En(0, "09:05")
				s.Persona("B", "#dos")
				s.En(0, "09:40")
				s.Revisa()
			},
			// A de 09:00 a 09:15 y B de 09:05 a 09:20: lo solapado se cuenta una vez (B suma 5 min → 15 al redondear).
			Lineas: []LineaEsperada{{Tarea: "#uno", Clase: "h", Horas: min15}, {Tarea: "#dos", Clase: "h", Horas: min15}}},

		{Clave: "E26", Nombre: "Se cierra la sesión antes del mínimo de presencia",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.Persona("A", "")
				s.En(0, "09:03")
				s.Cierra("A")
			},
			Lineas: []LineaEsperada{{Clase: "h", Horas: min15}}},

		{Clave: "E27", Nombre: "Turno de 11 minutos y presencia de 16",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				fin := s.Turno("A", "Antigravity", "")
				s.Persona("A", "")
				s.En(0, "09:11")
				fin()
				s.Persona("A", "")
				s.En(0, "09:40")
				s.Revisa()
			},
			// Máquina 11 min → 15 (bloques de 5); hombre de 09:00 a 09:16 → 30 (bloques de 15).
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Antigravity", Horas: min15}, {Clase: "h", Horas: min30}}},

		{Clave: "E28", Nombre: "Odoo caído, se cierra el periodo y pgocode se actualiza",
			Pasos: func(s *Sim) {
				s.En(0, "09:00")
				s.OdooCae()
				fin := s.Turno("A", "Claude Code", "")
				s.En(0, "09:10")
				fin()
				s.En(0, "09:20")
				s.Revisa() // se cierra sin haber podido resolver su tarea
				s.Reinicia(true, false)
				s.OdooVuelve()
				s.En(0, "09:25")
				s.Revisa()
			},
			Lineas: []LineaEsperada{{Clase: "m", Agente: "Claude Code", Horas: min10}}},
	}
}
