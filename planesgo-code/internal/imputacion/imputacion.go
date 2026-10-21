// Package imputacion lleva el tiempo de las sesiones de pgocode directamente a Odoo (partes de horas en
// account.analytic.line), con la sesión de Odoo del usuario y sin pasar por PlanesGo ni por el agente.
//
// Regla:
//   - En Odoo hay, por día, proyecto y tarea, una línea de «Hora Hombre» y una de «Hora Máquina» por agente
//     (Claude Code, OpenCode, Antigravity…). Todas las sesiones de ese día, proyecto y tarea suman en ellas: «hoy,
//     en este proyecto y esta tarea, he invertido tanto». Las líneas se ponen al día cada 3 minutos.
//   - Cada sesión imputa a su proyecto y su tarea (la de sus ajustes; por defecto la canónica «Desarrollo», que se
//     crea si no existe). Lleva periodos de trabajo: hay trabajo cuando el agente está con un turno (cuenta como
//     máquina) o cuando la persona usa la sesión con la ventana en primer plano (cuenta como hombre). Tras el
//     último trabajo hay una espera activa de 5 minutos que también cuenta (hombre); después el periodo se cierra.
//   - Los periodos, los latidos y los totales del día se guardan en un fichero local (no en Odoo), así que Odoo no
//     se llena de líneas pequeñas. Tras actualizarse o un cierre inesperado, pgocode lo retoma. Cada apertura y
//     cierre de periodo queda en un diario (imputacion.log) para poder auditarlo.
package imputacion

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

const (
	TareaDefecto = "Desarrollo" // tarea canónica de PSF / PlanesGo
	Espera       = 5 * time.Minute
	Escritura    = 3 * time.Minute // cada cuánto se ponen al día en Odoo las líneas del día
	activoHumano = 60 * time.Second
	// Redondeo hacia arriba del total del día de cada línea (como Odoo: un segundo cuenta como un bloque entero).
	BloqueHombre  = 15 * time.Minute
	BloqueMaquina = 5 * time.Minute
)

// Redondea hacia arriba los segundos de una línea del día al bloque de su clase; devuelve horas.
func Redondea(seg float64, clase string) float64 {
	if seg <= 0 {
		return 0
	}
	b := BloqueHombre.Seconds()
	if clase == "m" {
		b = BloqueMaquina.Seconds()
	}
	return math.Ceil(seg/b) * b / 3600
}

// Odoo: lo que necesita del cliente de Odoo (call_kw con la sesión del usuario).
type Odoo interface {
	CallKW(ctx context.Context, sesion, modelo, metodo string, args []any, kwargs map[string]any, res any) error
}

// Trabajo: a quién y dónde se imputa el trabajo de una sesión.
type Trabajo struct {
	Sesion     string `json:"sesion"`
	Titulo     string `json:"titulo"`
	Proyecto   int    `json:"proyecto"`
	NombreProy string `json:"nombre_proyecto"`
	TareaID    int    `json:"tarea_id"`
	Tarea      string `json:"tarea"`
	UID        int    `json:"uid"`
	OdooSesion string `json:"odoo_sesion"`
	Modelo     string `json:"modelo"`
	Desc       string `json:"desc"`
}

// Periodo: trabajo continuo de una sesión (en memoria y en el fichero local, nunca en Odoo).
type Periodo struct {
	Sesion     string        `json:"sesion"`
	T          Trabajo       `json:"t"`
	TareaID    int           `json:"tarea_id"` // resuelta en Odoo
	Inicio     time.Time     `json:"inicio"`
	Ultima     time.Time     `json:"ultima"`
	UltimaHum  time.Time     `json:"ultima_hum"`
	Maquina    time.Duration `json:"maquina"`
	TurnoDesde time.Time     `json:"turno_desde"`
	Entrada    int           `json:"entrada"`
	Salida     int           `json:"salida"`
	Coste      float64       `json:"coste"`
	Estado     string        `json:"estado"` // trabajando | activo | en-espera | error
	Error      string        `json:"error,omitempty"`
	turnos     int
}

// Dia: lo acumulado de un día, proyecto, tarea y clase (h: hombre; m: máquina de un agente) = una línea en Odoo.
type Dia struct {
	Fecha      string    `json:"fecha"`
	Proyecto   int       `json:"proyecto"`
	NombreProy string    `json:"nombre_proyecto"`
	TareaID    int       `json:"tarea_id"`
	Tarea      string    `json:"tarea"`
	Clase      string    `json:"clase"`
	Agente     string    `json:"agente,omitempty"`
	Modelo     string    `json:"modelo,omitempty"`
	LineaID    int       `json:"linea_id"`
	Cerrado    float64   `json:"cerrado"` // segundos de periodos ya cerrados
	Entrada    int       `json:"entrada"`
	Salida     int       `json:"salida"`
	Coste      float64   `json:"coste"`
	Sesiones   []string  `json:"sesiones"`
	UID        int       `json:"uid"`
	OdooSesion string    `json:"odoo_sesion"`
	Escrito    float64   `json:"escrito"` // horas escritas en Odoo la última vez
	Sucio      bool      `json:"sucio"`   // hay que escribirla
	version    int       // cambia cada vez que se ensucia: una escritura que empezó antes no la da por escrita
	Error      string    `json:"error,omitempty"`
	EscritoEn  time.Time `json:"escrito_en"`
}

// Linea: lo que ve la interfaz de cada línea del día.
type Linea struct {
	Dia
	Horas  float64   `json:"horas"` // en este momento
	Ritmo  int       `json:"ritmo"` // periodos que la están haciendo crecer ahora
	Calcul time.Time `json:"calculado"`
}

// Abierto: lo que ve la interfaz de cada sesión con un periodo abierto.
type Abierto struct {
	Sesion     string    `json:"sesion"`
	Titulo     string    `json:"titulo"`
	Proyecto   int       `json:"proyecto"`
	NombreProy string    `json:"nombre_proyecto"`
	TareaID    int       `json:"tarea_id"`
	Tarea      string    `json:"tarea"`
	Estado     string    `json:"estado"`
	Error      string    `json:"error,omitempty"`
	Horas      float64   `json:"horas"` // del periodo
	Inicio     time.Time `json:"inicio"`
	Ultima     time.Time `json:"ultima"`
	Agente     bool      `json:"agente"`  // el agente está con un turno (el periodo sigue creciendo)
	Maquina    float64   `json:"maquina"` // horas de máquina del periodo (en el momento del cálculo)
	Calculado  time.Time `json:"calculado"`
}

// Gestor lleva los periodos de las sesiones y las líneas del día.
type Gestor struct {
	Odoo                  Odoo
	Aviso                 func()
	FicheroEstado, Diario string
	Ahora                 func() time.Time
	// Tiempos: registro detallado de todo lo que interviene en la imputación (registro.go), para validarla.
	Tiempos *Registro

	mu        sync.Mutex
	periodos  map[string]*Periodo
	dias      map[string]*Dia
	tareas    map[string]int  // tareas ya resueltas en Odoo (proyecto|nombre → id)
	sinTarea  []*Periodo      // periodos cerrados antes de resolver su tarea: se suman al resolverla
	unificado map[int]bool    // usuarios a los que ya se les han unificado las líneas sueltas en esta ejecución
	campos    map[string]bool // campos de IA que tiene account.analytic.line en este Odoo (nil: sin consultar)
	escribe   sync.Mutex      // una escritura en Odoo a la vez
	// humanos: los tramos de tiempo humano ya imputados por usuario y día (uid|fecha). Una persona solo está en un
	// sitio: si varias sesiones se solapan, ese rato se imputa como humano una sola vez (a la primera).
	humanos   map[string][]Tramo
	arrancado bool
}

func (g *Gestor) ahora() time.Time {
	if g.Ahora != nil {
		return g.Ahora()
	}
	return time.Now()
}

func (g *Gestor) avisa() {
	if g.Aviso != nil {
		go g.Aviso()
	}
}

func (g *Gestor) inicia() {
	if g.periodos == nil {
		g.periodos = map[string]*Periodo{}
	}
	if g.dias == nil {
		g.dias = map[string]*Dia{}
	}
	if g.tareas == nil {
		g.tareas = map[string]int{}
	}
}

func claveTarea(t Trabajo) string { return fmt.Sprintf("%d|%s", t.Proyecto, tareaDe(t)) }

func tareaDe(t Trabajo) string {
	n := strings.TrimSpace(t.Tarea)
	if n == "" || n == "pgocode - Sesión IA" {
		return TareaDefecto
	}
	return n
}

// agenteDe: el agente de un modelo («Claude Code opus» → «Claude Code»).
func agenteDe(modelo string) string {
	for _, a := range []string{"Claude Code", "OpenCode", "Antigravity", "Gemini CLI"} {
		if strings.HasPrefix(modelo, a) {
			return a
		}
	}
	if f := strings.Fields(modelo); len(f) > 0 {
		return f[0]
	}
	return "Agente"
}

var noAlfa = regexp.MustCompile(`[^a-z0-9]+`)

// marca de la línea en ai_session_id: «pgocode-h» o «pgocode-m-<agente>».
func (d *Dia) marca() string {
	if d.Clase == "h" {
		return "pgocode-h"
	}
	return "pgocode-m-" + strings.Trim(noAlfa.ReplaceAllString(strings.ToLower(d.Agente), "-"), "-")
}

func claveDe(fecha string, proyecto, tarea int, clase, agente string) string {
	return fmt.Sprintf("%s|%d|%d|%s|%s", fecha, proyecto, tarea, clase, agente)
}

// ---- tiempos de un periodo ----

// fin del tiempo que cuenta: último trabajo + espera, sin pasar de ahora ni de la medianoche del inicio.
func (g *Gestor) finDe(p *Periodo) time.Time {
	fin := p.Ultima.Add(Espera)
	if p.turnos > 0 || fin.After(g.ahora()) {
		fin = g.ahora()
	}
	y, m, d := p.Inicio.Date()
	if medianoche := time.Date(y, m, d+1, 0, 0, 0, 0, p.Inicio.Location()); fin.After(medianoche) {
		fin = medianoche
	}
	return fin
}

// partes de un periodo: segundos de máquina y de hombre.
func (g *Gestor) partes(p *Periodo) (maq, hom float64) {
	fin := g.finDe(p)
	total := fin.Sub(p.Inicio)
	m := p.Maquina
	if !p.TurnoDesde.IsZero() {
		m += fin.Sub(p.TurnoDesde)
	}
	if m > total {
		m = total
	}
	if total < 0 {
		total = 0
	}
	return m.Seconds(), (total - m).Seconds()
}

// creciendo: a qué clase está sumando ahora mismo el periodo ("m", "h" o "" si ya no cuenta).
func (g *Gestor) creciendo(p *Periodo) string {
	if p.turnos > 0 {
		return "m"
	}
	if g.ahora().Before(p.Ultima.Add(Espera)) {
		return "h"
	}
	return ""
}

// ---- trabajo ----

// Arranca: retoma el estado del fichero local y pone en marcha el reloj (cierre de periodos, escritura de las
// líneas del día cada 3 minutos y reintentos).
func (g *Gestor) Arranca(ctx context.Context) {
	g.mu.Lock()
	if g.arrancado {
		g.mu.Unlock()
		return
	}
	g.arrancado = true
	g.inicia()
	if g.FicheroEstado != "" {
		var e struct {
			Periodos []*Periodo
			Dias     []*Dia
			Humanos  map[string][]Tramo
		}
		if b, err := os.ReadFile(g.FicheroEstado); err == nil && json.Unmarshal(b, &e) == nil {
			hoy := g.ahora().Format("2006-01-02")
			for _, d := range e.Dias {
				if d.Fecha == hoy || d.Sucio {
					g.dias[claveDe(d.Fecha, d.Proyecto, d.TareaID, d.Clase, d.Agente)] = d
				}
			}
			if e.Humanos != nil {
				g.humanos = e.Humanos
			}
			for _, p := range e.Periodos {
				turno := !p.TurnoDesde.IsZero()
				if turno { // el turno en curso termina donde terminó la ejecución anterior
					p.Maquina += p.Ultima.Sub(p.TurnoDesde)
					p.TurnoDesde = time.Time{}
				}
				g.periodos[p.Sesion] = p
				g.estado(p)
				g.anota("periodo-recupera", p, map[string]any{"inicio": p.Inicio.Format(time.RFC3339Nano), "ultima": p.Ultima.Format(time.RFC3339Nano),
					"maquina_s": seg(p.Maquina), "turno_cortado": turno})
			}
		}
	}
	n := len(g.periodos)
	g.mu.Unlock()
	if g.Tiempos != nil {
		g.Tiempos.Anota("arranca", map[string]any{"periodos": n})
	}
	g.revisa() // cierra lo que ya pasó la espera y escribe
	go func() {
		tic := time.NewTicker(15 * time.Second)
		defer tic.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tic.C:
				g.revisa()
			}
		}
	}()
}

// Empieza: empieza un turno del agente (devuelve la función de fin de turno).
func (g *Gestor) Empieza(t Trabajo) (fin func()) {
	p := g.trabajo(t, true, false)
	var una sync.Once
	return func() {
		una.Do(func() {
			g.mu.Lock()
			ahora := g.ahora()
			dur := time.Duration(0)
			if p.turnos--; p.turnos <= 0 {
				p.turnos = 0
				if !p.TurnoDesde.IsZero() {
					dur = ahora.Sub(p.TurnoDesde)
					p.Maquina += dur
					p.TurnoDesde = time.Time{}
				}
			}
			g.anota("turno-acaba", p, map[string]any{"turno_s": seg(dur), "turnos_vivos": p.turnos, "maquina_s": seg(p.Maquina)})
			p.Ultima = ahora
			g.estado(p)
			g.marcaSucias(p)
			g.mu.Unlock()
			go g.escribeSucias() // fin de turno: las líneas del día al día
			g.guarda()
			g.avisa()
		})
	}
}

// Actividad: la persona está usando la sesión (teclado, ratón… con la ventana en primer plano).
func (g *Gestor) Actividad(t Trabajo) { g.trabajo(t, false, true) }

func (g *Gestor) trabajo(t Trabajo, turno, humano bool) *Periodo {
	g.mu.Lock()
	g.inicia()
	ahora := g.ahora()
	p := g.periodos[t.Sesion]
	// Otra tarea, otro proyecto u otro día: se cierra el periodo y empieza otro.
	if p != nil && (p.T.Proyecto != t.Proyecto || tareaDe(p.T) != tareaDe(t) || (t.TareaID != 0 && p.TareaID != 0 && p.TareaID != t.TareaID) ||
		ahora.Format("2006-01-02") != p.Inicio.Format("2006-01-02")) {
		g.cierraPeriodo(p, "cambio de tarea, de proyecto o de día")
		p = nil
	}
	nuevo := p == nil
	if nuevo {
		p = &Periodo{Sesion: t.Sesion, T: t, TareaID: t.TareaID, Inicio: ahora}
		if p.TareaID == 0 {
			p.TareaID = g.tareas[claveTarea(t)]
		}
		g.periodos[t.Sesion] = p
	}
	for _, c := range []struct {
		dst *string
		v   string
	}{{&p.T.Titulo, t.Titulo}, {&p.T.Desc, t.Desc}, {&p.T.Modelo, t.Modelo}, {&p.T.OdooSesion, t.OdooSesion}, {&p.T.NombreProy, t.NombreProy}} {
		if c.v != "" {
			*c.dst = c.v
		}
	}
	if nuevo {
		g.anota("periodo-abre", p, map[string]any{"titulo": p.T.Titulo, "por": map[bool]string{true: "turno", false: "humano"}[turno]})
	}
	if turno {
		if p.turnos == 0 {
			p.TurnoDesde = ahora
		}
		p.turnos++
		g.anota("turno-empieza", p, map[string]any{"turnos_vivos": p.turnos, "modelo": p.T.Modelo})
	}
	if humano {
		if toca, desde := g.Tiempos.humanoToca(p.Sesion); toca {
			g.anota("humano", p, map[string]any{"desde_anterior_s": seg(desde), "sin_actividad_s": seg(ahora.Sub(p.UltimaHum))})
		}
		p.UltimaHum = ahora
	}
	p.Ultima = ahora
	antes := p.Estado
	g.estado(p)
	if !nuevo && antes != p.Estado {
		g.anota("estado", p, map[string]any{"de": antes, "a": p.Estado})
	}
	g.mu.Unlock()
	if nuevo {
		g.diario("abre", p, "")
		go g.preparaYEscribe(p)
	} else if turno && !g.tieneMaquina(p) {
		go g.preparaYEscribe(p)
	}
	g.guarda()
	g.avisa()
	if t.UID > 0 && t.OdooSesion != "" {
		go g.unificaUnaVez(t.UID, t.OdooSesion)
	}
	return p
}

// unificaUnaVez: la primera vez (por usuario y equipo) se unifican las líneas sueltas de versiones anteriores.
func (g *Gestor) unificaUnaVez(uid int, ses string) {
	g.mu.Lock()
	if g.unificado == nil {
		g.unificado = map[int]bool{}
	}
	hecho := g.unificado[uid]
	g.unificado[uid] = true
	g.mu.Unlock()
	marca := ""
	if g.FicheroEstado != "" {
		marca = fmt.Sprintf("%s.unificado-%d", g.FicheroEstado, uid)
		if _, err := os.Stat(marca); err == nil {
			return
		}
	}
	if hecho {
		return
	}
	ctx, cancela := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancela()
	r, err := g.UnificaSueltas(ctx, uid, ses)
	if err != nil {
		depuracion.Log("imputacion", "unificar las líneas sueltas: %v", err) // se puede repetir con el botón del panel
		return
	}
	if marca != "" && r.Fallos == 0 {
		os.WriteFile(marca, []byte(r.Mensaje+"\n"), 0o600)
	}
	g.avisa()
}

func (g *Gestor) tieneMaquina(p *Periodo) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	d := g.dias[claveDe(p.Inicio.Format("2006-01-02"), p.T.Proyecto, p.TareaID, "m", agenteDe(p.T.Modelo))]
	return d != nil && d.LineaID > 0
}

// preparaYEscribe resuelve la tarea del periodo (si hace falta) y escribe sus líneas del día.
func (g *Gestor) preparaYEscribe(p *Periodo) {
	g.mu.Lock()
	t, tarea := p.T, p.TareaID
	g.mu.Unlock()
	if tarea == 0 {
		ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
		id, err := g.tarea(ctx, t)
		cancela()
		if err != nil {
			g.mu.Lock()
			p.Estado, p.Error = "error", "tarea de Odoo: "+err.Error()
			g.mu.Unlock()
			g.diario("error", p, p.Error)
			g.anota("error", p, map[string]any{"error": p.Error})
			g.avisa()
			return
		}
		g.mu.Lock()
		p.TareaID = id
		g.tareas[claveTarea(t)] = id
		g.mu.Unlock()
	}
	g.mu.Lock()
	g.marcaSucias(p)
	g.mu.Unlock()
	g.escribeSucias()
}

// resuelvePendientes: los periodos cerrados sin tarea resuelta se suman en cuanto se resuelve.
func (g *Gestor) resuelvePendientes() {
	g.mu.Lock()
	l := g.sinTarea
	g.sinTarea = nil
	g.mu.Unlock()
	for _, p := range l {
		g.mu.Lock()
		id := g.tareas[claveTarea(p.T)]
		g.mu.Unlock()
		if id == 0 {
			ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
			var err error
			id, err = g.tarea(ctx, p.T)
			cancela()
			if err != nil {
				g.mu.Lock()
				g.sinTarea = append(g.sinTarea, p)
				g.mu.Unlock()
				continue
			}
		}
		g.mu.Lock()
		g.tareas[claveTarea(p.T)] = id
		p.TareaID = id
		g.sumaPeriodo(p)
		g.mu.Unlock()
	}
}

// estado (con el cerrojo).
func (g *Gestor) estado(p *Periodo) {
	if p.Estado == "error" {
		return
	}
	switch {
	case p.turnos > 0:
		p.Estado = "trabajando"
	case g.ahora().Sub(p.UltimaHum) < activoHumano:
		p.Estado = "activo"
	default:
		p.Estado = "en-espera"
	}
}

// dia (con el cerrojo): la línea del día de una clase para un periodo (la crea en memoria si no existe).
func (g *Gestor) dia(p *Periodo, clase string) *Dia {
	if p.TareaID == 0 {
		return nil
	}
	fecha := p.Inicio.Format("2006-01-02")
	agente := ""
	if clase == "m" {
		agente = agenteDe(p.T.Modelo)
	}
	k := claveDe(fecha, p.T.Proyecto, p.TareaID, clase, agente)
	d := g.dias[k]
	if d == nil {
		d = &Dia{Fecha: fecha, Proyecto: p.T.Proyecto, NombreProy: p.T.NombreProy, TareaID: p.TareaID, Tarea: tareaDe(p.T),
			Clase: clase, Agente: agente}
		g.dias[k] = d
	}
	d.UID, d.OdooSesion = p.T.UID, p.T.OdooSesion
	if p.T.NombreProy != "" {
		d.NombreProy = p.T.NombreProy
	}
	if clase == "m" && p.T.Modelo != "" {
		d.Modelo = p.T.Modelo
	}
	if tit := strings.TrimSpace(p.T.Titulo); tit != "" && !contiene(d.Sesiones, tit) {
		d.Sesiones = append(d.Sesiones, tit)
	}
	return d
}

func contiene(l []string, x string) bool {
	for _, v := range l {
		if v == x {
			return true
		}
	}
	return false
}

// marcaSucias (con el cerrojo): las líneas del día de un periodo hay que escribirlas.
func (g *Gestor) marcaSucias(p *Periodo) {
	if d := g.dia(p, "h"); d != nil {
		d.ensucia()
	}
	if m, _ := g.partes(p); m > 0 || p.turnos > 0 {
		if d := g.dia(p, "m"); d != nil {
			d.ensucia()
		}
	}
}

// cierraPeriodo (con el cerrojo): suma el periodo a sus líneas del día y lo quita.
func (g *Gestor) cierraPeriodo(p *Periodo, motivo string) {
	maq, hom := g.partes(p)
	if p.TareaID == 0 {
		p.TareaID = g.tareas[claveTarea(p.T)]
	}
	// Se congela el tiempo del periodo (ya no crece) para sumarlo ahora o cuando se resuelva su tarea.
	p.Maquina, p.TurnoDesde, p.turnos = time.Duration(maq*float64(time.Second)), time.Time{}, 0
	fin := p.Inicio.Add(time.Duration((maq + hom) * float64(time.Second)))
	p.Ultima = fin.Add(-Espera)
	if p.TareaID == 0 {
		g.sinTarea = append(g.sinTarea, p)
	} else {
		g.sumaPeriodo(p)
	}
	delete(g.periodos, p.Sesion)
	g.anota("periodo-cierra", p, map[string]any{"motivo": motivo, "inicio": p.Inicio.Format(time.RFC3339Nano), "fin": fin.Format(time.RFC3339Nano),
		"maquina_s": math.Round(maq*1000) / 1000, "hombre_s": math.Round(hom*1000) / 1000, "sin_tarea": p.TareaID == 0,
		"tokens": p.Entrada + p.Salida, "coste": p.Coste})
	go g.diario("cierra", p, fmt.Sprintf("%s (máquina %.0f s, hombre %.0f s)", motivo, maq, hom))
}

// sumaPeriodo (con el cerrojo): añade un periodo cerrado a sus líneas del día.
func (g *Gestor) sumaPeriodo(p *Periodo) {
	maq, hom := g.partesCerrado(p)
	// Tiempo humano que ya se imputó en otra sesión a la vez: no se cuenta otra vez.
	fin := p.Ultima.Add(Espera)
	if hom > 0 {
		k := claveHumano(p)
		if sol := math.Min(hom, solape(g.humanos[k], p.Inicio, fin)); sol > 0 {
			hom -= sol
			g.anota("periodo-solape", p, map[string]any{"solapado_s": math.Round(sol*1000) / 1000, "inicio": p.Inicio.Format(time.RFC3339Nano), "fin": fin.Format(time.RFC3339Nano)})
		}
		if g.humanos == nil {
			g.humanos = map[string][]Tramo{}
		}
		g.humanos[k] = append(g.humanos[k], Tramo{p.Inicio, fin})
	}
	{
		if d := g.dia(p, "h"); d != nil {
			d.Cerrado += hom
			d.ensucia()
			g.anota("dia-suma", p, map[string]any{"fecha": d.Fecha, "clase": "h", "mas_s": math.Round(hom*1000) / 1000, "cerrado_s": math.Round(d.Cerrado*1000) / 1000})
		}
		if maq > 0 || p.Entrada+p.Salida > 0 {
			if d := g.dia(p, "m"); d != nil {
				d.Cerrado += maq
				g.anota("dia-suma", p, map[string]any{"fecha": d.Fecha, "clase": "m", "linea_agente": d.Agente, "mas_s": math.Round(maq*1000) / 1000, "cerrado_s": math.Round(d.Cerrado*1000) / 1000})
				d.Entrada += p.Entrada
				d.Salida += p.Salida
				d.Coste += p.Coste
				d.ensucia()
			}
		}
	}
}

// partesCerrado: máquina y hombre de un periodo ya congelado (Ultima = fin - espera).
func (g *Gestor) partesCerrado(p *Periodo) (maq, hom float64) {
	total := p.Ultima.Add(Espera).Sub(p.Inicio).Seconds()
	maq = math.Min(p.Maquina.Seconds(), total)
	return maq, math.Max(0, total-maq)
}

// revisa: cierra los periodos que pasaron la espera (o la medianoche) y escribe las líneas del día pendientes
// (cada 3 minutos las que siguen creciendo).
func (g *Gestor) revisa() {
	g.mu.Lock()
	g.inicia()
	ahora := g.ahora()
	cambio := false
	for _, p := range g.periodos {
		antes := p.Estado
		g.estado(p)
		if antes != p.Estado {
			g.anota("estado", p, map[string]any{"de": antes, "a": p.Estado})
		}
		cambio = cambio || antes != p.Estado
		y, m, d := p.Inicio.Date()
		medianoche := ahora.After(time.Date(y, m, d+1, 0, 0, 0, 0, p.Inicio.Location()))
		if (p.turnos == 0 && !ahora.Before(p.Ultima.Add(Espera))) || medianoche {
			g.cierraPeriodo(p, map[bool]string{true: "fin del día", false: "5 minutos sin trabajo"}[medianoche])
			cambio = true
		}
	}
	// Las líneas que crecen se ponen al día cada 3 minutos.
	for _, p := range g.periodos {
		if c := g.creciendo(p); c != "" {
			if d := g.dia(p, c); d != nil && ahora.Sub(d.EscritoEn) >= Escritura {
				d.ensucia()
			}
		}
	}
	g.mu.Unlock()
	g.resuelvePendientes()
	g.escribeSucias()
	g.guarda()
	if cambio {
		g.avisa()
	}
}

// SumaUso añade el consumo de IA de un turno al periodo abierto de la sesión.
func (g *Gestor) SumaUso(sesion string, entrada, salida int, coste float64) {
	g.mu.Lock()
	if p := g.periodos[sesion]; p != nil {
		p.Entrada += entrada
		p.Salida += salida
		p.Coste += coste
		g.anota("uso", p, map[string]any{"entrada": entrada, "salida": salida, "coste": coste})
	} else {
		g.Tiempos.Anota("uso-sin-periodo", map[string]any{"sesion": sesion, "entrada": entrada, "salida": salida, "coste": coste})
	}
	g.mu.Unlock()
	g.guarda()
}

// ActualizaSesionOdoo: tras volver a entrar, se escribe con la nueva sesión de Odoo del usuario.
func (g *Gestor) ActualizaSesionOdoo(uid int, sesion string) {
	g.mu.Lock()
	for _, p := range g.periodos {
		if p.T.UID == uid {
			p.T.OdooSesion = sesion
		}
	}
	for _, d := range g.dias {
		if d.UID == uid {
			d.OdooSesion = sesion
		}
	}
	g.mu.Unlock()
	g.guarda()
}

// totales (con el cerrojo): segundos de una línea del día ahora mismo (cerrado + periodos abiertos) y uso de IA.
func (g *Gestor) totales(d *Dia) (seg float64, ent, sal int, coste float64, ritmo int) {
	seg, ent, sal, coste = d.Cerrado, d.Entrada, d.Salida, d.Coste
	for _, p := range g.periodos {
		if p.TareaID != d.TareaID || p.T.Proyecto != d.Proyecto || p.Inicio.Format("2006-01-02") != d.Fecha {
			continue
		}
		maq, hom := g.partes(p)
		switch {
		case d.Clase == "h":
			seg += math.Max(0, hom-g.solapeAbierto(p))
		case agenteDe(p.T.Modelo) == d.Agente:
			seg += maq
			ent, sal, coste = ent+p.Entrada, sal+p.Salida, coste+p.Coste
		default:
			continue
		}
		if c := g.creciendo(p); (c == "h" && d.Clase == "h") || (c == "m" && d.Clase == "m" && agenteDe(p.T.Modelo) == d.Agente) {
			ritmo++
		}
	}
	return
}

// prefijo de la descripción de la línea del día (la identifica).
func (d *Dia) prefijo() string {
	if d.Clase == "m" {
		return "pgocode (agente " + d.Agente + ")"
	}
	return "pgocode (trabajo)"
}

// descripción de la línea del día.
func (d *Dia) descripcion() string {
	tipo := "trabajo"
	if d.Clase == "m" {
		tipo = "agente " + d.Agente
	}
	if len(d.Sesiones) == 0 {
		return fmt.Sprintf("pgocode (%s)", tipo)
	}
	s := d.Sesiones
	resto := ""
	if len(s) > 6 {
		resto = fmt.Sprintf(" y %d más", len(s)-6)
		s = s[:6]
	}
	return fmt.Sprintf("pgocode (%s): «%s»%s", tipo, strings.Join(s, "», «"), resto)
}

// escribeSucias pone al día en Odoo las líneas del día pendientes (creándolas si hace falta).
func (g *Gestor) escribeSucias() {
	g.escribe.Lock()
	defer g.escribe.Unlock()
	g.mu.Lock()
	var l []*Dia
	for _, d := range g.dias {
		if d.Sucio && d.OdooSesion != "" {
			l = append(l, d)
		}
	}
	g.mu.Unlock()
	for _, d := range l {
		g.escribeDia(d)
	}
	if len(l) > 0 {
		g.guarda()
		g.avisa()
	}
}

func (g *Gestor) escribeDia(d *Dia) {
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancela()
	g.mu.Lock()
	seg, ent, sal, coste, ritmo := g.totales(d)
	dd := *d
	g.mu.Unlock()
	if seg == 0 && ritmo > 0 { // trabajo recién empezado: ya cuenta su primer bloque
		seg = 1
	}
	horas := Redondea(seg, dd.Clase)
	vals := map[string]any{"name": dd.descripcion(), "unit_amount": horas, "ai_session_id": dd.marca()}
	if dd.Clase == "m" {
		vals["ai_model"] = dd.Modelo
		vals["ai_tokens_input"], vals["ai_tokens_output"], vals["ai_tokens_total"], vals["ai_cost"] = ent, sal, ent+sal, coste
	}
	id := dd.LineaID
	if id == 0 {
		id = g.buscaLinea(ctx, &dd)
	}
	var err error
	if id == 0 {
		if horas == 0 { // nada que imputar todavía: no se crea una línea vacía
			g.mu.Lock()
			d.Sucio = d.version != dd.version
			g.mu.Unlock()
			return
		}
		v := map[string]any{"date": dd.Fecha, "project_id": dd.Proyecto, "task_id": dd.TareaID}
		if dd.UID > 0 {
			v["user_id"] = dd.UID
		}
		for k, x := range vals {
			v[k] = x
		}
		id, err = g.crea(ctx, dd.OdooSesion, v)
	} else {
		err = g.escribeLinea(ctx, dd.OdooSesion, id, vals)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	reg := map[string]any{"fecha": dd.Fecha, "proyecto": dd.Proyecto, "tarea_id": dd.TareaID, "clase": dd.Clase, "linea_agente": dd.Agente,
		"linea": id, "nueva": dd.LineaID == 0, "segundos": seg, "horas": horas, "antes": dd.Escrito, "ritmo": ritmo}
	if err != nil {
		reg["error"] = err.Error()
	}
	if g.Tiempos != nil {
		g.Tiempos.Anota("odoo", reg)
	}
	if err != nil {
		d.Error = err.Error()
		if definitivo(err) { // la línea ya no existe o está validada: se crea otra con lo que falte la próxima vez
			d.LineaID = 0
		}
		depuracion.Log("imputacion", "línea del día %s/%d/%s: %v", dd.Fecha, dd.Proyecto, dd.marca(), err)
		return
	}
	// Si mientras se escribía cambió (se cerró otro periodo…), sigue pendiente: lo escrito ya no es lo último.
	d.LineaID, d.Escrito, d.EscritoEn, d.Sucio, d.Error = id, horas, g.ahora(), d.version != dd.version, ""
}

// buscaLinea: la línea del día ya existente en Odoo (p. ej. si se perdió el fichero local). Si se encuentra y no
// había nada acumulado, sus horas pasan a ser lo ya cerrado.
func (g *Gestor) buscaLinea(ctx context.Context, d *Dia) int {
	var l []struct {
		ID    int     `json:"id"`
		Horas float64 `json:"unit_amount"`
	}
	// La línea del día se reconoce por el principio de su descripción («pgocode (trabajo)…», «pgocode (agente X)…»),
	// que existe en cualquier Odoo (ai_session_id no siempre).
	dominio := []any{[]any{"date", "=", d.Fecha}, []any{"project_id", "=", d.Proyecto}, []any{"task_id", "=", d.TareaID},
		[]any{"name", "=like", d.prefijo() + "%"}}
	if d.UID > 0 {
		dominio = append(dominio, []any{"user_id", "=", d.UID})
	}
	if err := g.Odoo.CallKW(ctx, d.OdooSesion, "account.analytic.line", "search_read", []any{dominio},
		map[string]any{"fields": []string{"id", "unit_amount"}, "limit": 1}, &l); err != nil || len(l) == 0 {
		return 0
	}
	g.mu.Lock()
	for _, x := range g.dias {
		if x.Fecha == d.Fecha && x.Proyecto == d.Proyecto && x.TareaID == d.TareaID && x.Clase == d.Clase && x.Agente == d.Agente {
			if x.LineaID == 0 && x.Cerrado == 0 {
				x.Cerrado = l[0].Horas * 3600
				d.Cerrado = x.Cerrado
			}
			x.LineaID = l[0].ID
		}
	}
	g.mu.Unlock()
	return l[0].ID
}

// CamposIA: los campos ai_* que existen en account.analytic.line (se pregunta a Odoo una vez). No todos los Odoo
// los tienen todos: p. ej. ai_model sí y ai_session_id no.
func (g *Gestor) CamposIA(ctx context.Context, ses string) map[string]bool {
	g.mu.Lock()
	c := g.campos
	g.mu.Unlock()
	if c != nil {
		return c
	}
	var r map[string]any
	c = map[string]bool{}
	if err := g.Odoo.CallKW(ctx, ses, "account.analytic.line", "fields_get", []any{}, map[string]any{"attributes": []string{"type"}}, &r); err != nil {
		depuracion.Log("imputacion", "fields_get: %v", err)
		return c // sin saberlo, no se envía ningún campo de IA (y se vuelve a preguntar la próxima vez)
	}
	for k := range r {
		if strings.HasPrefix(k, "ai_") {
			c[k] = true
		}
	}
	g.mu.Lock()
	g.campos = c
	g.mu.Unlock()
	depuracion.Log("imputacion", "campos de IA en Odoo: %v", c)
	return c
}

// soloExistentes quita de vals los campos de IA que este Odoo no tiene.
func (g *Gestor) soloExistentes(ctx context.Context, ses string, v map[string]any) map[string]any {
	c := g.CamposIA(ctx, ses)
	x := map[string]any{}
	for k, val := range v {
		if !strings.HasPrefix(k, "ai_") || c[k] {
			x[k] = val
		}
	}
	return x
}

func sinIA(v map[string]any) map[string]any {
	x := map[string]any{}
	for k, val := range v {
		if !strings.HasPrefix(k, "ai_") {
			x[k] = val
		}
	}
	return x
}

func (g *Gestor) crea(ctx context.Context, ses string, v map[string]any) (int, error) {
	v = g.soloExistentes(ctx, ses, v)
	var id int
	err := g.Odoo.CallKW(ctx, ses, "account.analytic.line", "create", []any{v}, nil, &id)
	if err != nil {
		if e2 := g.Odoo.CallKW(ctx, ses, "account.analytic.line", "create", []any{sinIA(v)}, nil, &id); e2 == nil {
			depuracion.Log("imputacion", "línea %d creada sin los campos de IA: %v", id, err)
			return id, nil
		}
	}
	return id, err
}

func (g *Gestor) escribeLinea(ctx context.Context, ses string, id int, v map[string]any) error {
	v = g.soloExistentes(ctx, ses, v)
	var nada any
	err := g.Odoo.CallKW(ctx, ses, "account.analytic.line", "write", []any{[]int{id}, v}, nil, &nada)
	if err != nil && !definitivo(err) {
		if e2 := g.Odoo.CallKW(ctx, ses, "account.analytic.line", "write", []any{[]int{id}, sinIA(v)}, nil, &nada); e2 == nil {
			return nil
		}
	}
	return err
}

// definitivo: errores de Odoo que no se arreglan reintentando (la línea ya no existe o está validada).
func definitivo(err error) bool {
	s := strings.ToLower(err.Error())
	for _, k := range []string{"missingerror", "does not exist", "no existe", "has been deleted", "validad", "validated"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// tarea busca la tarea por nombre en el proyecto y, si no existe, la crea (asignada al usuario).
func (g *Gestor) tarea(ctx context.Context, t Trabajo) (int, error) {
	var l []struct {
		ID int `json:"id"`
	}
	dominio := []any{[]any{"project_id", "=", t.Proyecto}, []any{"name", "=", tareaDe(t)}}
	if err := g.Odoo.CallKW(ctx, t.OdooSesion, "project.task", "search_read", []any{dominio}, map[string]any{"fields": []string{"id"}, "limit": 1}, &l); err != nil {
		return 0, err
	}
	if len(l) > 0 {
		return l[0].ID, nil
	}
	vals := map[string]any{"name": tareaDe(t), "project_id": t.Proyecto}
	if t.UID > 0 {
		vals["user_id"] = t.UID
	}
	var id int
	err := g.Odoo.CallKW(ctx, t.OdooSesion, "project.task", "create", []any{vals}, nil, &id)
	if err != nil && t.UID > 0 {
		delete(vals, "user_id")
		vals["user_ids"] = []any{[]any{6, 0, []int{t.UID}}}
		err = g.Odoo.CallKW(ctx, t.OdooSesion, "project.task", "create", []any{vals}, nil, &id)
	}
	if err == nil {
		depuracion.Log("imputacion", "tarea «%s» creada en el proyecto %d: %d", tareaDe(t), t.Proyecto, id)
	}
	return id, err
}

// Cierra el periodo de una sesión ya (si el agente no está con un turno).
func (g *Gestor) Cierra(sesion, motivo string) {
	g.mu.Lock()
	if p := g.periodos[sesion]; p != nil && p.turnos == 0 {
		g.cierraPeriodo(p, motivo)
	}
	g.mu.Unlock()
	g.escribeSucias()
	g.guarda()
	g.avisa()
}

// CierraTodo cierra todos los periodos y escribe (al salir de pgocode de verdad).
func (g *Gestor) CierraTodo() {
	if g.Tiempos != nil {
		g.Tiempos.Anota("cierra-todo", nil)
	}
	g.mu.Lock()
	for _, p := range g.periodos {
		if !p.TurnoDesde.IsZero() {
			p.Maquina += g.ahora().Sub(p.TurnoDesde)
			p.TurnoDesde = time.Time{}
		}
		p.turnos = 0
		g.cierraPeriodo(p, "pgocode se cierra")
	}
	g.mu.Unlock()
	g.escribeSucias()
	g.guarda()
}

// Suspende: pgocode sale para actualizarse. No se cierra nada: se escriben las líneas del día y se guarda el
// estado para que la versión nueva continúe.
func (g *Gestor) Suspende() {
	g.mu.Lock()
	for _, p := range g.periodos {
		g.marcaSucias(p)
	}
	n := len(g.periodos)
	g.mu.Unlock()
	g.escribeSucias()
	g.guarda()
	depuracion.Log("imputacion", "pgocode se actualiza: %d periodos siguen abiertos", n)
	if g.Tiempos != nil {
		g.Tiempos.Anota("suspende", map[string]any{"periodos": n})
	}
}

// Estado: las líneas de hoy (con sus horas en este momento) y las sesiones con un periodo abierto.
func (g *Gestor) Estado() (hoy []Linea, abiertos []Abierto) {
	g.mu.Lock()
	defer g.mu.Unlock()
	hoy, abiertos = []Linea{}, []Abierto{}
	fecha := g.ahora().Format("2006-01-02")
	for _, d := range g.dias {
		if d.Fecha != fecha {
			continue
		}
		seg, ent, sal, coste, ritmo := g.totales(d)
		x := Linea{Dia: *d, Horas: seg / 3600, Ritmo: ritmo, Calcul: g.ahora()}
		x.Entrada, x.Salida, x.Coste = ent, sal, coste
		hoy = append(hoy, x)
	}
	sort.Slice(hoy, func(i, j int) bool {
		if hoy[i].NombreProy != hoy[j].NombreProy {
			return hoy[i].NombreProy < hoy[j].NombreProy
		}
		return hoy[i].Clase > hoy[j].Clase
	})
	for _, p := range g.periodos {
		maq, hom := g.partes(p)
		abiertos = append(abiertos, Abierto{Sesion: p.Sesion, Titulo: p.T.Titulo, Proyecto: p.T.Proyecto, NombreProy: p.T.NombreProy,
			TareaID: p.TareaID, Tarea: tareaDe(p.T), Estado: p.Estado, Error: p.Error, Horas: (maq + hom) / 3600,
			Inicio: p.Inicio, Ultima: p.Ultima, Agente: p.turnos > 0, Maquina: maq / 3600, Calculado: g.ahora()})
	}
	sort.Slice(abiertos, func(i, j int) bool { return abiertos[i].Sesion < abiertos[j].Sesion })
	return
}

func (g *Gestor) guarda() {
	if g.FicheroEstado == "" {
		return
	}
	g.mu.Lock()
	e := struct {
		Periodos []*Periodo
		Dias     []*Dia
		Humanos  map[string][]Tramo
	}{Humanos: map[string][]Tramo{}}
	hoy := g.ahora().Format("2006-01-02")
	for k, v := range g.humanos {
		if strings.HasSuffix(k, "|"+hoy) {
			e.Humanos[k] = v
		}
	}
	for _, p := range g.periodos {
		e.Periodos = append(e.Periodos, p)
	}
	for _, d := range g.dias {
		e.Dias = append(e.Dias, d)
	}
	b, err := json.Marshal(e)
	g.mu.Unlock()
	if err != nil {
		return
	}
	tmp := g.FicheroEstado + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, g.FicheroEstado)
	}
}

// diario: registro local de auditoría (apertura y cierre de cada periodo).
func (g *Gestor) diario(evento string, p *Periodo, motivo string) {
	g.mu.Lock()
	maq, hom := g.partes(p)
	r := map[string]any{"fecha": g.ahora().Format(time.RFC3339), "evento": evento, "sesion": p.Sesion, "titulo": p.T.Titulo,
		"proyecto": p.T.Proyecto, "tarea_id": p.TareaID, "tarea": tareaDe(p.T), "agente": agenteDe(p.T.Modelo),
		"inicio": p.Inicio.Format(time.RFC3339), "ultimo_trabajo": p.Ultima.Format(time.RFC3339),
		"segundos_maquina": math.Round(maq), "segundos_hombre": math.Round(hom), "tokens": p.Entrada + p.Salida, "motivo": motivo}
	g.mu.Unlock()
	depuracion.Log("imputacion", "%s periodo sesión %s (proyecto %d, %s): máquina %.0f s, hombre %.0f s %s", evento, p.Sesion, p.T.Proyecto, tareaDe(p.T), maq, hom, motivo)
	if g.Diario == "" {
		return
	}
	if f, err := os.OpenFile(g.Diario, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
		f.Close()
	}
}

// Descripcion: lo que se pide en una sesión (para el diario y la depuración).
func Descripcion(titulo, pedido string) string {
	pedido = strings.Join(strings.Fields(pedido), " ")
	if r := []rune(pedido); len(r) > 200 {
		pedido = string(r[:197]) + "…"
	}
	switch {
	case titulo == "" && pedido == "":
		return "Sesión de pgocode"
	case titulo == "":
		return "Sesión de pgocode: " + pedido
	case pedido == "":
		return fmt.Sprintf("Sesión de pgocode «%s»", titulo)
	}
	return fmt.Sprintf("Sesión de pgocode «%s»: %s", titulo, pedido)
}

// ---- unificar líneas sueltas ----

// UmbralSueltas: las líneas de pgocode por debajo de esta duración se unifican en la línea del día.
const UmbralSueltas = 5 * time.Minute

// Unificado: resultado de unificar las líneas sueltas.
type Unificado struct {
	Lineas  int     `json:"lineas"`  // líneas sueltas unificadas (y borradas)
	Horas   float64 `json:"horas"`   // su tiempo, sumado a las líneas del día
	Fallos  int     `json:"fallos"`  // no se pudieron borrar (siguen como estaban)
	Mensaje string  `json:"mensaje"` // resumen
}

// UnificaSueltas suma a la línea del día de su fecha, proyecto y tarea las líneas creadas por versiones anteriores
// de pgocode (una por periodo) de menos de 5 minutos, y las borra. Solo toca líneas de pgocode (ai_session_id
// «pgocode-…» que no sea una línea del día, o descripción «Sesión de pgocode…»); si Odoo no deja borrar una, su
// tiempo no se suma.
func (g *Gestor) UnificaSueltas(ctx context.Context, uid int, sesionOdoo string) (Unificado, error) {
	var r Unificado
	var l []struct {
		ID        int     `json:"id"`
		Fecha     string  `json:"date"`
		Horas     float64 `json:"unit_amount"`
		Project   []any   `json:"project_id"`
		Task      any     `json:"task_id"`
		Sesion    any     `json:"ai_session_id"`
		Modelo    any     `json:"ai_model"`
		TokensIn  any     `json:"ai_tokens_input"`
		TokensOut any     `json:"ai_tokens_output"`
		Coste     any     `json:"ai_cost"`
		Name      string  `json:"name"`
	}
	ci := g.CamposIA(ctx, sesionOdoo)
	dominio := []any{[]any{"user_id", "=", uid}, []any{"unit_amount", "<", UmbralSueltas.Hours()}, []any{"name", "=like", "Sesión de pgocode%"}}
	if ci["ai_session_id"] {
		dominio = []any{[]any{"user_id", "=", uid}, []any{"unit_amount", "<", UmbralSueltas.Hours()},
			"|", []any{"ai_session_id", "=like", "pgocode-%"}, []any{"name", "=like", "Sesión de pgocode%"}}
	}
	nombres := []string{"id", "date", "unit_amount", "project_id", "task_id", "name"}
	for _, k := range []string{"ai_session_id", "ai_model", "ai_tokens_input", "ai_tokens_output", "ai_cost"} {
		if ci[k] {
			nombres = append(nombres, k)
		}
	}
	campos := map[string]any{"fields": nombres, "limit": 2000}
	if err := g.Odoo.CallKW(ctx, sesionOdoo, "account.analytic.line", "search_read", []any{dominio}, campos, &l); err != nil {
		return r, err
	}
	num := func(v any) float64 { f, _ := v.(float64); return f }
	texto := func(v any) string { s, _ := v.(string); return s }
	for _, x := range l {
		marca := texto(x.Sesion)
		if marca == "pgocode-h" || strings.HasPrefix(marca, "pgocode-m-") || strings.HasPrefix(x.Name, "pgocode (") || len(x.Project) < 2 {
			continue // ya es una línea del día
		}
		t, ok := x.Task.([]any)
		if !ok || len(t) < 2 {
			continue // sin tarea: no hay línea del día a la que sumarla
		}
		clase, agente, modelo := "h", "", texto(x.Modelo)
		if modelo != "" {
			clase, agente = "m", agenteDe(modelo)
		}
		proy, nombreProy := int(num(x.Project[0])), texto(x.Project[1])
		tarea, nombreTarea := int(num(t[0])), texto(t[1])
		k := claveDe(x.Fecha, proy, tarea, clase, agente)
		g.mu.Lock()
		g.inicia()
		d := g.dias[k]
		if d == nil {
			d = &Dia{Fecha: x.Fecha, Proyecto: proy, NombreProy: nombreProy, TareaID: tarea, Tarea: nombreTarea, Clase: clase, Agente: agente, Modelo: modelo}
			g.dias[k] = d
		}
		d.UID, d.OdooSesion = uid, sesionOdoo
		dd := *d
		g.mu.Unlock()
		// La línea del día existente (si la hay) se busca antes de sumar, para partir de lo que ya tiene.
		if dd.LineaID == 0 {
			g.buscaLinea(ctx, &dd)
		}
		var nada any
		if err := g.Odoo.CallKW(ctx, sesionOdoo, "account.analytic.line", "unlink", []any{[]int{x.ID}}, nil, &nada); err != nil {
			r.Fallos++
			depuracion.Log("imputacion", "unificar: no se pudo borrar la línea %d: %v", x.ID, err)
			continue
		}
		g.mu.Lock()
		d.Cerrado += x.Horas * 3600
		d.Entrada += int(num(x.TokensIn))
		d.Salida += int(num(x.TokensOut))
		d.Coste += num(x.Coste)
		d.ensucia()
		g.mu.Unlock()
		r.Lineas++
		r.Horas += x.Horas
		if g.Diario != "" {
			if f, err := os.OpenFile(g.Diario, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				b, _ := json.Marshal(map[string]any{"fecha": g.ahora().Format(time.RFC3339), "evento": "unifica", "linea_borrada": x.ID,
					"fecha_parte": x.Fecha, "proyecto": proy, "tarea_id": tarea, "clase": clase, "agente": agente, "horas": x.Horas, "descripcion": x.Name})
				f.Write(append(b, '\n'))
				f.Close()
			}
		}
	}
	g.escribeSucias()
	g.guarda()
	r.Mensaje = fmt.Sprintf("%d líneas sueltas unificadas (%.0f min) en las líneas del día", r.Lineas, r.Horas*60)
	if r.Fallos > 0 {
		r.Mensaje += fmt.Sprintf("; %d no se pudieron borrar y siguen como estaban", r.Fallos)
	}
	depuracion.Log("imputacion", "unificar (usuario %d): %s", uid, r.Mensaje)
	return r, nil
}

// Tramo de tiempo.
type Tramo struct{ Ini, Fin time.Time }

func claveHumano(p *Periodo) string {
	return fmt.Sprintf("%d|%s", p.T.UID, p.Inicio.Format("2006-01-02"))
}

// solape: los segundos de [ini, fin] que caen dentro de alguno de los tramos (contando cada instante una vez).
func solape(tramos []Tramo, ini, fin time.Time) float64 {
	var l []Tramo
	for _, t := range tramos {
		a, b := maxT(t.Ini, ini), minT(t.Fin, fin)
		if b.After(a) {
			l = append(l, Tramo{a, b})
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Ini.Before(l[j].Ini) })
	var total time.Duration
	var hasta time.Time
	for _, t := range l {
		if t.Ini.Before(hasta) {
			t.Ini = hasta
		}
		if t.Fin.After(t.Ini) {
			total += t.Fin.Sub(t.Ini)
			hasta = t.Fin
		}
	}
	return total.Seconds()
}

// solapeAbierto (con el cerrojo): el tiempo de un periodo abierto que ya cuenta como humano en otro sitio (los ya
// imputados y los otros periodos abiertos del mismo usuario y día que empezaron antes).
func (g *Gestor) solapeAbierto(p *Periodo) float64 {
	k := claveHumano(p)
	tramos := append([]Tramo(nil), g.humanos[k]...)
	for _, o := range g.periodos {
		if o == p || claveHumano(o) != k {
			continue
		}
		if o.Inicio.Before(p.Inicio) || (o.Inicio.Equal(p.Inicio) && o.Sesion < p.Sesion) {
			if _, h := g.partes(o); h > 0 {
				tramos = append(tramos, Tramo{o.Inicio, g.finDe(o)})
			}
		}
	}
	return solape(tramos, p.Inicio, g.finDe(p))
}

func maxT(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// ensucia (con el cerrojo): la línea hay que escribirla (otra vez).
func (d *Dia) ensucia() {
	d.Sucio = true
	d.version++
}
