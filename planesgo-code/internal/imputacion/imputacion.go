// Package imputacion lleva el tiempo de las sesiones de pgocode directamente a Odoo (partes de horas en
// account.analytic.line), con la sesión de Odoo del usuario y sin pasar por PlanesGo ni por el agente.
//
// Regla (la misma para todo el tiempo imputado):
//   - Cada sesión imputa a su proyecto y a su tarea (la de sus ajustes; por defecto «pgocode - Sesión IA», que se
//     crea si no existe). Un parte es un periodo de trabajo continuo de una sesión.
//   - Hay trabajo cuando el agente está con un turno o cuando la persona usa la sesión (teclado, ratón, desplazarse,
//     con la ventana en primer plano). El primer trabajo abre el parte (línea en Odoo con su cronómetro).
//   - Tras el último trabajo hay una espera activa de 5 minutos que también cuenta. Si en ese plazo no hay más
//     trabajo, el parte se cierra con las horas de inicio → último trabajo + 5 minutos. Mientras tanto no se
//     envía nada a Odoo.
//   - Los cronómetros los tiene Odoo: cada línea lleva su sesión (ai_session_id = pgocode-<sesión>) desde que se
//     crea. Al arrancar (por ejemplo tras actualizarse), pgocode lee de Odoo las líneas suyas con el cronómetro en
//     marcha y las retoma (misma línea, mismo inicio). Al salir para actualizarse no se cierra nada; al salir de
//     verdad, sí. Si ha pasado más de la espera sin trabajo, se cierran con su último trabajo.
//   - Fiabilidad: los partes abiertos se guardan también en disco; si Odoo falla al cerrar, se reintenta hasta
//     lograrlo. Cada apertura, reanudación y cierre queda en un diario (imputacion.log) para poder auditarlo.
package imputacion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

const (
	TareaDefecto = "pgocode - Sesión IA"
	Espera       = 5 * time.Minute  // espera activa tras el último trabajo (cuenta)
	minimo       = time.Minute      // un parte cuenta al menos un minuto
	activoHumano = 60 * time.Second // la actividad humana se ve «activa» (verde) este tiempo
)

// Odoo: lo que necesita del cliente de Odoo (call_kw con la sesión del usuario).
type Odoo interface {
	CallKW(ctx context.Context, sesion, modelo, metodo string, args []any, kwargs map[string]any, res any) error
}

// Trabajo: a quién y dónde se imputa el trabajo de una sesión.
type Trabajo struct {
	Sesion     string `json:"sesion"` // id de la sesión de pgocode
	Titulo     string `json:"titulo"`
	Proyecto   int    `json:"proyecto"`
	NombreProy string `json:"nombre_proyecto"`
	TareaID    int    `json:"tarea_id"` // 0: se busca o crea por nombre
	Tarea      string `json:"tarea"`
	UID        int    `json:"uid"`
	OdooSesion string `json:"odoo_sesion"`
	Modelo     string `json:"modelo"`
	Desc       string `json:"desc"`
}

// Parte es lo que se ve en la interfaz y lo que se guarda.
type Parte struct {
	Sesion     string    `json:"sesion"`
	Titulo     string    `json:"titulo"`
	Proyecto   int       `json:"proyecto"`
	NombreProy string    `json:"nombre_proyecto"`
	TareaID    int       `json:"tarea_id"`
	Tarea      string    `json:"tarea"`
	LineaID    int       `json:"linea_id"`
	Inicio     time.Time `json:"inicio"`
	Ultima     time.Time `json:"ultima"`     // último trabajo (agente o persona)
	UltimaHum  time.Time `json:"ultima_hum"` // última actividad de la persona
	Fin        time.Time `json:"fin,omitzero"`
	Horas      float64   `json:"horas"`
	Tokens     int       `json:"tokens"`
	Coste      float64   `json:"coste"`
	Estado     string    `json:"estado"` // trabajando (agente) | activo (persona) | en-espera | cerrado | pendiente | error
	Error      string    `json:"error,omitempty"`
	Desc       string    `json:"desc"`
}

type parte struct {
	Parte
	T        Trabajo `json:"t"`
	Turnos   int     `json:"-"`
	Entrada  int     `json:"entrada"`
	Salida   int     `json:"salida"`
	abriendo chan struct{}
}

// Gestor lleva los partes de todas las sesiones.
type Gestor struct {
	Odoo Odoo
	// Aviso se llama cuando cambia algún parte (para refrescar la interfaz).
	Aviso func()
	// Estado: fichero de los partes abiertos y pendientes (para sobrevivir a un cierre inesperado);
	// Diario: registro de aperturas y cierres. Vacíos: sin persistencia (pruebas).
	FicheroEstado, Diario string
	// Ahora se puede sustituir en las pruebas.
	Ahora func() time.Time

	mu            sync.Mutex
	sincronizados map[int]bool // usuarios cuyos cronómetros de Odoo ya se han leído
	partes        map[string]*parte
	pendientes    []*parte // cierres que Odoo no aceptó: se reintentan
	cerrados      []Parte
	arrancado     bool
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

func tareaDe(t Trabajo) string {
	if strings.TrimSpace(t.Tarea) == "" {
		return TareaDefecto
	}
	return strings.TrimSpace(t.Tarea)
}

// Arranca: recupera lo que quedó de una ejecución anterior (cierra sus partes con su último trabajo) y pone en
// marcha el reloj que cierra los partes tras la espera y reintenta los cierres pendientes.
func (g *Gestor) Arranca(ctx context.Context) {
	g.mu.Lock()
	if g.arrancado {
		g.mu.Unlock()
		return
	}
	g.arrancado = true
	if g.partes == nil {
		g.partes = map[string]*parte{}
	}
	var viejos []*parte
	if g.FicheroEstado != "" {
		var e struct {
			Abiertos, Pendientes []*parte
		}
		if b, err := os.ReadFile(g.FicheroEstado); err == nil && json.Unmarshal(b, &e) == nil {
			viejos = append(e.Abiertos, e.Pendientes...)
		}
	}
	g.mu.Unlock()
	usuarios := map[int]string{}
	for _, p := range viejos {
		p.abriendo = make(chan struct{})
		close(p.abriendo)
		if p.T.UID > 0 && p.T.OdooSesion != "" {
			usuarios[p.T.UID] = p.T.OdooSesion
		}
		// Reciente (p. ej. una actualización): se retoma tal cual. Si no, se cierra con su último trabajo.
		if p.Estado != "pendiente" && g.ahora().Sub(p.Ultima) < Espera && p.LineaID > 0 {
			g.mu.Lock()
			p.Turnos = 0
			g.estado(p)
			g.partes[p.Sesion] = p
			g.mu.Unlock()
			g.diario("retoma", p, "parte abierto antes de reiniciar pgocode")
			continue
		}
		g.diario("recupera", p, "parte de una ejecución anterior de pgocode")
		g.cierraParte(p, "pgocode se cerró sin cerrarlo")
	}
	// Y lo que diga Odoo: cronómetros de pgocode en marcha que no estén en el fichero.
	for uid, ses := range usuarios {
		g.Sincroniza(ctx, uid, ses)
	}
	g.guarda()
	go func() {
		tic := time.NewTicker(15 * time.Second)
		defer tic.Stop()
		reintento := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tic.C:
				g.revisa()
				if time.Since(reintento) > time.Minute {
					reintento = time.Now()
					g.reintenta()
				}
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
			p.Turnos--
			p.Ultima = g.ahora()
			g.estado(p)
			g.mu.Unlock()
			g.guarda()
			g.avisa()
		})
	}
}

// Actividad: la persona está usando la sesión (teclado, ratón… con la ventana en primer plano).
func (g *Gestor) Actividad(t Trabajo) { g.trabajo(t, false, true) }

// trabajo registra trabajo en la sesión: abre el parte si no lo hay (o si cambia el proyecto o la tarea).
func (g *Gestor) trabajo(t Trabajo, turno, humano bool) *parte {
	g.mu.Lock()
	if g.partes == nil {
		g.partes = map[string]*parte{}
	}
	p := g.partes[t.Sesion]
	if p != nil && (p.T.Proyecto != t.Proyecto || tareaDe(p.T) != tareaDe(t) || p.T.TareaID != t.TareaID) {
		ant := p
		delete(g.partes, t.Sesion)
		go g.cierraParte(ant, "cambio de tarea o de proyecto")
		p = nil
	}
	ahora := g.ahora()
	nuevo := p == nil
	if nuevo {
		p = &parte{T: t, abriendo: make(chan struct{})}
		p.Parte = Parte{Sesion: t.Sesion, Titulo: t.Titulo, Proyecto: t.Proyecto, NombreProy: t.NombreProy, TareaID: t.TareaID,
			Tarea: tareaDe(t), Inicio: ahora, Desc: t.Desc}
		g.partes[t.Sesion] = p
	}
	if t.Desc != "" {
		p.T.Desc, p.Desc = t.Desc, t.Desc
	}
	if t.Modelo != "" {
		p.T.Modelo = t.Modelo
	}
	p.T.Titulo, p.Titulo, p.T.OdooSesion = t.Titulo, t.Titulo, t.OdooSesion
	if turno {
		p.Turnos++
	}
	if humano {
		p.UltimaHum = ahora
	}
	p.Ultima = ahora
	g.estado(p)
	g.mu.Unlock()
	if nuevo {
		go g.abre(p)
	}
	g.guarda()
	g.avisa()
	if t.UID > 0 && t.OdooSesion != "" {
		g.mu.Lock()
		hecho := g.sincronizados[t.UID]
		g.mu.Unlock()
		if !hecho {
			go g.Sincroniza(context.Background(), t.UID, t.OdooSesion)
		}
	}
	return p
}

// Sincroniza lee de Odoo los cronómetros en marcha de pgocode del usuario (líneas con ai_session_id «pgocode-…» y
// timer_start) y retoma los que pgocode no tenga abiertos: los cronómetros los tiene Odoo.
func (g *Gestor) Sincroniza(ctx context.Context, uid int, sesionOdoo string) {
	g.mu.Lock()
	if g.sincronizados == nil {
		g.sincronizados = map[int]bool{}
	}
	if g.sincronizados[uid] {
		g.mu.Unlock()
		return
	}
	g.sincronizados[uid] = true
	g.mu.Unlock()
	ctx, cancela := context.WithTimeout(ctx, 30*time.Second)
	defer cancela()
	var l []struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`
		Sesion     any    `json:"ai_session_id"`
		TimerStart any    `json:"timer_start"`
		Project    []any  `json:"project_id"`
		Task       any    `json:"task_id"`
	}
	dominio := []any{[]any{"user_id", "=", uid}, []any{"timer_start", "!=", false}, []any{"ai_session_id", "=like", "pgocode-%"}}
	campos := map[string]any{"fields": []string{"id", "name", "ai_session_id", "timer_start", "project_id", "task_id"}, "limit": 50}
	if err := g.Odoo.CallKW(ctx, sesionOdoo, "account.analytic.line", "search_read", []any{dominio}, campos, &l); err != nil {
		depuracion.Log("imputacion", "leer cronómetros de Odoo (usuario %d): %v", uid, err)
		return
	}
	for _, x := range l {
		s, _ := x.Sesion.(string)
		sesion := strings.TrimPrefix(s, "pgocode-")
		ts, _ := x.TimerStart.(string)
		inicio, err := time.ParseInLocation("2006-01-02 15:04:05", ts, time.UTC)
		if sesion == "" || err != nil || len(x.Project) < 2 {
			continue
		}
		g.mu.Lock()
		conocido := false
		for _, p := range g.partes {
			conocido = conocido || p.LineaID == x.ID || p.Sesion == sesion
		}
		for _, p := range g.pendientes {
			conocido = conocido || p.LineaID == x.ID
		}
		if conocido {
			g.mu.Unlock()
			continue
		}
		proy, _ := x.Project[0].(float64)
		nombreProy, _ := x.Project[1].(string)
		tareaID, tarea := 0, TareaDefecto
		if t, ok := x.Task.([]any); ok && len(t) == 2 {
			f, _ := t[0].(float64)
			tareaID = int(f)
			tarea, _ = t[1].(string)
		}
		p := &parte{T: Trabajo{Sesion: sesion, Proyecto: int(proy), NombreProy: nombreProy, TareaID: tareaID, Tarea: tarea, UID: uid,
			OdooSesion: sesionOdoo, Desc: x.Name}, abriendo: make(chan struct{})}
		close(p.abriendo)
		// No se sabe cuándo fue el último trabajo: se cuenta desde ahora con su espera (si no hay más, se cierra en 5 min).
		p.Parte = Parte{Sesion: sesion, Proyecto: int(proy), NombreProy: nombreProy, TareaID: tareaID, Tarea: tarea, LineaID: x.ID,
			Inicio: inicio.Local(), Ultima: g.ahora(), Desc: x.Name}
		g.estado(p)
		if g.partes == nil {
			g.partes = map[string]*parte{}
		}
		g.partes[sesion] = p
		g.mu.Unlock()
		g.diario("retoma", p, "cronómetro en marcha en Odoo")
	}
	g.guarda()
	g.avisa()
}

// estado (con el cerrojo): trabajando si hay turno del agente; activo si la persona lo usó hace poco; si no, en
// espera (que cuenta).
func (g *Gestor) estado(p *parte) {
	if p.Estado == "error" {
		return
	}
	switch {
	case p.Turnos > 0:
		p.Estado = "trabajando"
	case g.ahora().Sub(p.UltimaHum) < activoHumano:
		p.Estado = "activo"
	default:
		p.Estado = "en-espera"
	}
}

// revisa cierra los partes cuya espera ha pasado y actualiza los estados.
func (g *Gestor) revisa() {
	g.mu.Lock()
	var cerrar []*parte
	cambio := false
	for s, p := range g.partes {
		antes := p.Estado
		g.estado(p)
		cambio = cambio || antes != p.Estado
		if p.Turnos == 0 && g.ahora().Sub(p.Ultima) >= Espera {
			delete(g.partes, s)
			cerrar = append(cerrar, p)
		}
	}
	g.mu.Unlock()
	for _, p := range cerrar {
		g.cierraParte(p, "5 minutos sin trabajo")
	}
	if cambio {
		g.avisa()
	}
}

// SumaUso añade el consumo de IA de un turno al parte abierto de la sesión.
func (g *Gestor) SumaUso(sesion string, entrada, salida int, coste float64) {
	g.mu.Lock()
	if p := g.partes[sesion]; p != nil {
		p.Entrada += entrada
		p.Salida += salida
		p.Coste += coste
		p.Tokens = p.Entrada + p.Salida
	}
	g.mu.Unlock()
	g.guarda()
}

// ActualizaSesionOdoo: tras volver a entrar en pgocode, los partes y cierres pendientes del usuario usan la nueva
// sesión de Odoo (la anterior puede haber caducado).
func (g *Gestor) ActualizaSesionOdoo(uid int, sesion string) {
	g.mu.Lock()
	for _, p := range g.partes {
		if p.T.UID == uid {
			p.T.OdooSesion = sesion
		}
	}
	for _, p := range g.pendientes {
		if p.T.UID == uid {
			p.T.OdooSesion = sesion
		}
	}
	g.mu.Unlock()
	g.guarda()
}

// abre crea la línea del parte en Odoo (y la tarea, si no existe) y pone en marcha su cronómetro.
func (g *Gestor) abre(p *parte) {
	defer close(p.abriendo)
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancela()
	g.mu.Lock()
	t := p.T
	inicio := p.Inicio
	g.mu.Unlock()
	tarea := t.TareaID
	if tarea == 0 {
		id, err := g.tarea(ctx, t)
		if err != nil {
			g.falla(p, "tarea: "+err.Error())
			return
		}
		tarea = id
	}
	vals := map[string]any{"name": t.Desc, "date": inicio.Format("2006-01-02"), "project_id": t.Proyecto, "task_id": tarea, "unit_amount": 0}
	if t.UID > 0 {
		vals["user_id"] = t.UID
	}
	// La sesión va en la línea desde el principio: así se puede retomar su cronómetro tras reiniciar pgocode.
	vals["ai_session_id"] = "pgocode-" + t.Sesion
	var id int
	err := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "create", []any{vals}, nil, &id)
	if err != nil {
		delete(vals, "ai_session_id") // Odoo sin el módulo de PlanesGo (campos de IA)
		err = g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "create", []any{vals}, nil, &id)
	}
	if err != nil {
		g.falla(p, "crear el parte: "+err.Error())
		return
	}
	var nada any
	g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "action_timer_start", []any{[]int{id}}, nil, &nada)
	g.mu.Lock()
	p.LineaID, p.TareaID, p.T.TareaID = id, tarea, tarea
	g.mu.Unlock()
	g.diario("abre", p, "")
	g.guarda()
	g.avisa()
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
		vals["user_id"] = t.UID // Odoo 14; en versiones nuevas, user_ids
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

func (g *Gestor) falla(p *parte, e string) {
	g.mu.Lock()
	p.Estado, p.Error = "error", e
	g.mu.Unlock()
	g.diario("error", p, e)
	g.avisa()
}

// Cierra el parte de una sesión ya (si no hay turnos del agente en curso).
func (g *Gestor) Cierra(sesion, motivo string) {
	g.mu.Lock()
	p := g.partes[sesion]
	if p == nil || p.Turnos > 0 {
		g.mu.Unlock()
		return
	}
	delete(g.partes, sesion)
	g.mu.Unlock()
	g.cierraParte(p, motivo)
}

// horasDe: del inicio al último trabajo más la espera activa (sin pasar de ahora), redondeado al minuto.
func (g *Gestor) horasDe(p *parte) (float64, time.Time) {
	fin := p.Ultima.Add(Espera)
	if ahora := g.ahora(); fin.After(ahora) {
		fin = ahora
	}
	dur := fin.Sub(p.Inicio)
	if dur < minimo {
		dur = minimo
	}
	return float64(int(dur.Minutes()+0.5)) / 60, fin
}

// cierraParte para el cronómetro y escribe las horas, la descripción y los datos de IA. Si Odoo falla, el parte
// queda pendiente y se reintenta.
func (g *Gestor) cierraParte(p *parte, motivo string) {
	select {
	case <-p.abriendo:
	case <-time.After(40 * time.Second):
	}
	g.mu.Lock()
	t, id := p.T, p.LineaID
	horas, fin := g.horasDe(p)
	entrada, salida, coste := p.Entrada, p.Salida, p.Coste
	g.mu.Unlock()
	var err error
	if id == 0 {
		err = errors.New("el parte no llegó a crearse en Odoo")
	} else {
		ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancela()
		var nada any
		g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "action_timer_stop", []any{[]int{id}}, nil, &nada)
		// Las horas se escriben después de parar el cronómetro: así mandan las de pgocode, no las del redondeo de Odoo.
		err = g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, map[string]any{"unit_amount": horas, "name": t.Desc}}, nil, &nada)
		if err == nil {
			ia := map[string]any{"ai_model": t.Modelo, "ai_session_id": "pgocode-" + t.Sesion, "ai_tokens_input": entrada,
				"ai_tokens_output": salida, "ai_tokens_total": entrada + salida, "ai_cost": coste}
			if e := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, ia}, nil, &nada); e != nil {
				depuracion.Log("imputacion", "parte %d: campos de IA no guardados: %v", id, e)
			}
		}
	}
	g.mu.Lock()
	p.Fin, p.Horas = fin, horas
	switch {
	case err == nil:
		p.Estado, p.Error = "cerrado", ""
	case id == 0:
		p.Estado, p.Error = "error", err.Error()
	default:
		// Odoo no lo aceptó (caído, sesión caducada…): pendiente, se reintenta.
		p.Estado, p.Error = "pendiente", err.Error()
		g.pendientes = append(g.pendientes, p)
	}
	g.cerrados = append([]Parte{p.Parte}, g.cerrados...)
	if len(g.cerrados) > 100 {
		g.cerrados = g.cerrados[:100]
	}
	g.mu.Unlock()
	if err != nil {
		g.diario("cierre-fallido", p, motivo+": "+err.Error())
	} else {
		g.diario("cierra", p, motivo)
	}
	g.guarda()
	g.avisa()
}

// reintenta los cierres pendientes.
func (g *Gestor) reintenta() {
	g.mu.Lock()
	l := g.pendientes
	g.pendientes = nil
	// Se quitan de la lista de cerrados: cierraParte los vuelve a poner con su resultado.
	var quedan []Parte
	for _, c := range g.cerrados {
		pend := false
		for _, p := range l {
			if p.LineaID == c.LineaID && c.Estado == "pendiente" {
				pend = true
			}
		}
		if !pend {
			quedan = append(quedan, c)
		}
	}
	g.cerrados = quedan
	g.mu.Unlock()
	for _, p := range l {
		g.cierraParte(p, "reintento")
	}
}

// CierraTodo cierra todos los partes (al salir de pgocode).
func (g *Gestor) CierraTodo() {
	g.mu.Lock()
	var l []*parte
	for s, p := range g.partes {
		l = append(l, p)
		delete(g.partes, s)
	}
	g.mu.Unlock()
	var wg sync.WaitGroup
	for _, p := range l {
		wg.Add(1)
		go func(p *parte) { defer wg.Done(); g.cierraParte(p, "pgocode se cierra") }(p)
	}
	wg.Wait()
}

// Estado: partes abiertos y los cerrados hoy.
func (g *Gestor) Estado() (abiertos, cerrados []Parte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	abiertos, cerrados = []Parte{}, []Parte{}
	for _, p := range g.partes {
		x := p.Parte
		x.Horas, _ = g.horasDe(p)
		abiertos = append(abiertos, x)
	}
	sort.Slice(abiertos, func(i, j int) bool { return abiertos[i].Inicio.Before(abiertos[j].Inicio) })
	hoy := g.ahora().Format("2006-01-02")
	for _, c := range g.cerrados {
		if c.Fin.Format("2006-01-02") == hoy {
			cerrados = append(cerrados, c)
		}
	}
	return
}

// guarda los partes abiertos y pendientes en disco (con la sesión de Odoo: el fichero es solo del usuario).
func (g *Gestor) guarda() {
	if g.FicheroEstado == "" {
		return
	}
	g.mu.Lock()
	e := struct {
		Abiertos, Pendientes []*parte
	}{Pendientes: g.pendientes}
	for _, p := range g.partes {
		e.Abiertos = append(e.Abiertos, p)
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

// diario añade una línea al registro de auditoría de la imputación.
func (g *Gestor) diario(evento string, p *parte, motivo string) {
	g.mu.Lock()
	r := map[string]any{"fecha": g.ahora().Format(time.RFC3339), "evento": evento, "sesion": p.Sesion, "titulo": p.Titulo,
		"proyecto": p.Proyecto, "nombre_proyecto": p.NombreProy, "tarea_id": p.TareaID, "tarea": p.Tarea, "linea": p.LineaID,
		"inicio": p.Inicio.Format(time.RFC3339), "ultimo_trabajo": p.Ultima.Format(time.RFC3339), "horas": p.Horas, "tokens": p.Tokens, "motivo": motivo}
	if !p.Fin.IsZero() {
		r["fin"] = p.Fin.Format(time.RFC3339)
	}
	g.mu.Unlock()
	depuracion.Log("imputacion", "%s parte %d (sesión %s, proyecto %d, tarea %s): %.2f h %s", evento, p.LineaID, p.Sesion, p.Proyecto, p.Tarea, p.Horas, motivo)
	if g.Diario == "" {
		return
	}
	if f, err := os.OpenFile(g.Diario, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		b, _ := json.Marshal(r)
		f.Write(append(b, '\n'))
		f.Close()
	}
}

// Descripcion: lo que se imputa (en español): la sesión y lo último que se ha pedido.
func Descripcion(titulo, pedido string) string {
	pedido = strings.Join(strings.Fields(pedido), " ")
	if r := []rune(pedido); len(r) > 200 {
		pedido = string(r[:197]) + "…"
	}
	if titulo == "" {
		return "Sesión de pgocode: " + pedido
	}
	return fmt.Sprintf("Sesión de pgocode «%s»: %s", titulo, pedido)
}

// Suspende: pgocode sale para actualizarse. No se cierra nada (los cronómetros siguen en Odoo): solo se guarda el
// estado para retomarlo al arrancar la versión nueva.
func (g *Gestor) Suspende() {
	g.mu.Lock()
	for _, p := range g.partes {
		p.Turnos = 0
	}
	g.mu.Unlock()
	g.guarda()
	g.mu.Lock()
	n := len(g.partes)
	g.mu.Unlock()
	depuracion.Log("imputacion", "pgocode se actualiza: %d partes siguen abiertos en Odoo", n)
}
