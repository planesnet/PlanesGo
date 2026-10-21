// Package imputacion lleva el tiempo de las sesiones de pgocode directamente a Odoo (partes de horas en
// account.analytic.line), con la sesión de Odoo del usuario y sin pasar por PlanesGo ni por el agente.
//
// Regla: cada sesión imputa a su proyecto y a su tarea (la de sus ajustes; por defecto «pgocode - Sesión IA», que
// se crea si no existe). Un parte es un periodo de trabajo: se crea en Odoo (con su cronómetro) al empezar el
// primer turno del agente; mientras se trabaja no se envía nada (pgocode anota la última actividad); se cierra a
// los 5 minutos sin turnos, al cambiar de tarea o al salir, con las horas reales (inicio → última actividad), la
// descripción y los datos de IA (tokens, coste, modelo y sesión).
package imputacion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

const (
	TareaDefecto = "pgocode - Sesión IA"
	CerrarTras   = 5 * time.Minute
	minimo       = time.Minute // un parte cuenta al menos un minuto
)

// Odoo: lo que necesita del cliente de Odoo (call_kw con la sesión del usuario).
type Odoo interface {
	CallKW(ctx context.Context, sesion, modelo, metodo string, args []any, kwargs map[string]any, res any) error
}

// Trabajo: un turno del agente en una sesión.
type Trabajo struct {
	Sesion     string // id de la sesión de pgocode
	Titulo     string
	Proyecto   int
	NombreProy string
	TareaID    int    // 0: se busca o crea por nombre
	Tarea      string // nombre de la tarea (vacío: la de por defecto)
	UID        int    // usuario de Odoo
	OdooSesion string
	Modelo     string // agente y modelo
	Desc       string
}

// Uso de IA acumulado de un parte.
type Uso struct {
	Entrada, Salida int
	Coste           float64
}

// Parte es lo que se ve en la interfaz.
type Parte struct {
	Sesion     string    `json:"sesion"`
	Titulo     string    `json:"titulo"`
	Proyecto   int       `json:"proyecto"`
	NombreProy string    `json:"nombre_proyecto"`
	TareaID    int       `json:"tarea_id"`
	Tarea      string    `json:"tarea"`
	LineaID    int       `json:"linea_id"`
	Inicio     time.Time `json:"inicio"`
	Ultima     time.Time `json:"ultima"`
	Fin        time.Time `json:"fin,omitzero"`
	Horas      float64   `json:"horas"`
	Tokens     int       `json:"tokens"`
	Coste      float64   `json:"coste"`
	Estado     string    `json:"estado"` // trabajando | en-espera | cerrado | error
	Error      string    `json:"error,omitempty"`
	Desc       string    `json:"desc"`
}

type parte struct {
	Parte
	t        Trabajo
	turnos   int
	uso      Uso
	cierre   *time.Timer
	abriendo chan struct{} // se cierra cuando la línea ya está creada en Odoo (o falló)
}

// Gestor lleva los partes de todas las sesiones.
type Gestor struct {
	Odoo Odoo
	// Aviso se llama cuando cambia algún parte (para refrescar la interfaz).
	Aviso func()

	mu       sync.Mutex
	partes   map[string]*parte
	cerrados []Parte // los últimos cerrados (para la interfaz)
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

// Empieza: empieza un turno del agente. Abre el parte si no lo hay (en segundo plano: el agente no espera a Odoo)
// y devuelve la función de fin de turno.
func (g *Gestor) Empieza(t Trabajo) (fin func()) {
	g.mu.Lock()
	if g.partes == nil {
		g.partes = map[string]*parte{}
	}
	p := g.partes[t.Sesion]
	// Otra tarea u otro proyecto en la misma sesión: se cierra el parte anterior y se abre otro.
	if p != nil && (p.t.Proyecto != t.Proyecto || tareaDe(p.t) != tareaDe(t) || p.t.TareaID != t.TareaID) {
		ant := p
		delete(g.partes, t.Sesion)
		go g.cierraParte(ant)
		p = nil
	}
	ahora := time.Now()
	if p == nil {
		p = &parte{t: t, abriendo: make(chan struct{})}
		p.Parte = Parte{Sesion: t.Sesion, Titulo: t.Titulo, Proyecto: t.Proyecto, NombreProy: t.NombreProy, TareaID: t.TareaID,
			Tarea: tareaDe(t), Inicio: ahora, Ultima: ahora, Estado: "trabajando", Desc: t.Desc}
		g.partes[t.Sesion] = p
		go g.abre(p)
	}
	if p.cierre != nil {
		p.cierre.Stop()
		p.cierre = nil
	}
	p.t.Desc, p.Desc, p.t.Modelo, p.t.Titulo, p.Titulo = t.Desc, t.Desc, t.Modelo, t.Titulo, t.Titulo
	p.t.OdooSesion = t.OdooSesion
	p.turnos++
	p.Ultima, p.Estado = ahora, "trabajando"
	g.mu.Unlock()
	g.avisa()

	ctx, cancela := context.WithCancel(context.Background())
	// Mientras el agente trabaja solo se anota la actividad (sin red).
	go func() {
		tic := time.NewTicker(30 * time.Second)
		defer tic.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tic.C:
				g.mu.Lock()
				p.Ultima = time.Now()
				g.mu.Unlock()
			}
		}
	}()
	var una sync.Once
	return func() {
		una.Do(func() {
			cancela()
			g.mu.Lock()
			p.Ultima = time.Now()
			if p.turnos--; p.turnos == 0 {
				p.Estado = "en-espera"
				p.cierre = time.AfterFunc(CerrarTras, func() { g.Cierra(t.Sesion) })
			}
			g.mu.Unlock()
			g.avisa()
		})
	}
}

// SumaUso añade el consumo de IA de un turno al parte abierto de la sesión.
func (g *Gestor) SumaUso(sesion string, entrada, salida int, coste float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if p := g.partes[sesion]; p != nil {
		p.uso.Entrada += entrada
		p.uso.Salida += salida
		p.uso.Coste += coste
		p.Tokens, p.Coste = p.uso.Entrada+p.uso.Salida, p.uso.Coste
	}
}

// abre crea la línea del parte en Odoo (y la tarea, si no existe) y pone en marcha su cronómetro.
func (g *Gestor) abre(p *parte) {
	defer close(p.abriendo)
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancela()
	g.mu.Lock()
	t := p.t
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
	vals := map[string]any{"name": t.Desc, "date": time.Now().Format("2006-01-02"), "project_id": t.Proyecto,
		"task_id": tarea, "unit_amount": 0}
	if t.UID > 0 {
		vals["user_id"] = t.UID
	}
	var id int
	if err := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "create", []any{vals}, nil, &id); err != nil {
		g.falla(p, "crear el parte: "+err.Error())
		return
	}
	var nada any
	g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "action_timer_start", []any{[]int{id}}, nil, &nada)
	g.mu.Lock()
	p.LineaID, p.TareaID = id, tarea
	g.mu.Unlock()
	depuracion.Log("imputacion", "parte %d abierto: proyecto %d tarea %d sesión %s", id, t.Proyecto, tarea, t.Sesion)
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
	depuracion.Log("imputacion", "sesión %s: %s", p.Sesion, e)
	g.avisa()
}

// Cierra el parte de una sesión (si no hay turnos en curso).
func (g *Gestor) Cierra(sesion string) {
	g.mu.Lock()
	p := g.partes[sesion]
	if p == nil || p.turnos > 0 {
		g.mu.Unlock()
		return
	}
	delete(g.partes, sesion)
	g.mu.Unlock()
	g.cierraParte(p)
}

// cierraParte para el cronómetro y escribe las horas reales, la descripción y los datos de IA.
func (g *Gestor) cierraParte(p *parte) {
	select {
	case <-p.abriendo:
	case <-time.After(40 * time.Second):
	}
	g.mu.Lock()
	if p.cierre != nil {
		p.cierre.Stop()
	}
	t, id, uso := p.t, p.LineaID, p.uso
	dur := p.Ultima.Sub(p.Inicio)
	if dur < minimo {
		dur = minimo
	}
	horas := float64(int(dur.Minutes()+0.5)) / 60
	g.mu.Unlock()
	err := errors.New("el parte no llegó a crearse en Odoo")
	if id > 0 {
		ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancela()
		var nada any
		g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "action_timer_stop", []any{[]int{id}}, nil, &nada)
		err = g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, map[string]any{"unit_amount": horas, "name": t.Desc}}, nil, &nada)
		// Campos de IA (módulo de PlanesGo en Odoo): si no están, el parte vale igual.
		ia := map[string]any{"ai_model": t.Modelo, "ai_session_id": "pgocode-" + t.Sesion, "ai_tokens_input": uso.Entrada,
			"ai_tokens_output": uso.Salida, "ai_tokens_total": uso.Entrada + uso.Salida, "ai_cost": uso.Coste}
		if e := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, ia}, nil, &nada); e != nil {
			depuracion.Log("imputacion", "parte %d: campos de IA no guardados: %v", id, e)
		}
	}
	g.mu.Lock()
	p.Fin, p.Horas, p.Estado = time.Now(), horas, "cerrado"
	if err != nil && id > 0 {
		p.Estado, p.Error = "error", "cerrar el parte: "+err.Error()
	} else if id == 0 && p.Error == "" {
		p.Estado, p.Error = "error", err.Error()
	}
	g.cerrados = append([]Parte{p.Parte}, g.cerrados...)
	if len(g.cerrados) > 50 {
		g.cerrados = g.cerrados[:50]
	}
	g.mu.Unlock()
	depuracion.Log("imputacion", "parte %d cerrado: %.2f h, %d tokens (sesión %s) err=%v", id, horas, uso.Entrada+uso.Salida, t.Sesion, err)
	g.avisa()
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
		go func(p *parte) { defer wg.Done(); g.cierraParte(p) }(p)
	}
	wg.Wait()
}

// Estado: partes abiertos y los últimos cerrados de hoy.
func (g *Gestor) Estado() (abiertos, cerrados []Parte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	abiertos, cerrados = []Parte{}, []Parte{}
	for _, p := range g.partes {
		x := p.Parte
		x.Horas = time.Since(p.Inicio).Hours()
		abiertos = append(abiertos, x)
	}
	sort.Slice(abiertos, func(i, j int) bool { return abiertos[i].Inicio.Before(abiertos[j].Inicio) })
	hoy := time.Now().Format("2006-01-02")
	for _, c := range g.cerrados {
		if c.Fin.Format("2006-01-02") == hoy {
			cerrados = append(cerrados, c)
		}
	}
	return
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
