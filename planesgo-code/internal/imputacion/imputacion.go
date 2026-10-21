// Package imputacion lleva el tiempo de las sesiones de pgocode directamente a Odoo (partes de horas en
// account.analytic.line), con la sesión de Odoo del usuario y sin pasar por PlanesGo ni por el agente.
//
// Regla (la misma para todo el tiempo imputado):
//   - Cada sesión imputa a su proyecto y a su tarea (la de sus ajustes; por defecto la canónica «Desarrollo»,
//     que se crea si no existe). Un parte es un periodo de trabajo continuo de una sesión dentro de un mismo día.
//   - Hay trabajo cuando el agente está con un turno o cuando la persona usa la sesión (teclado, ratón,
//     desplazarse, con la ventana en primer plano). Tras el último trabajo hay una espera activa de 5 minutos que
//     también cuenta; si no hay más trabajo, el parte se cierra con inicio → último trabajo + 5 minutos.
//   - Dos líneas por parte: «Hora Máquina» con el tiempo de los turnos del agente (con ai_model, tokens y coste) y
//     «Hora Hombre» con el resto (uso de la persona y esperas), sin ai_model.
//   - Sin el cronómetro de Odoo (es uno por usuario y chocaría con PlanesGo y con otras sesiones): las líneas son
//     «vivas», con las horas acumuladas escritas cada 3 minutos, y marcadas con ai_session_id =
//     pgocode-abierto-<sesión>-m|h mientras están abiertas (pgocode-<sesión>-m|h al cerrarse). Al arrancar (por
//     ejemplo tras actualizarse), pgocode lee de Odoo sus líneas abiertas y continúa. Al salir para actualizarse no
//     se cierra nada; al salir de verdad, sí.
//   - Fiabilidad: también se guarda el estado en disco; si Odoo falla al escribir, se reintenta (salvo que la línea
//     ya no exista o esté validada). Cada apertura, reanudación y cierre queda en un diario (imputacion.log).
package imputacion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

const (
	TareaDefecto = "Desarrollo" // tarea canónica de PSF / PlanesGo
	Espera       = 5 * time.Minute
	Sincroniza   = 3 * time.Minute // cada cuánto se escriben en Odoo las horas de una línea viva
	activoHumano = 60 * time.Second
	marcaAbierta = "pgocode-abierto-"
)

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

// Parte es lo que se ve en la interfaz.
type Parte struct {
	Sesion       string    `json:"sesion"`
	Titulo       string    `json:"titulo"`
	Proyecto     int       `json:"proyecto"`
	NombreProy   string    `json:"nombre_proyecto"`
	TareaID      int       `json:"tarea_id"`
	Tarea        string    `json:"tarea"`
	LineaMaquina int       `json:"linea_maquina"`
	LineaHombre  int       `json:"linea_hombre"`
	Inicio       time.Time `json:"inicio"`
	Ultima       time.Time `json:"ultima"`
	Fin          time.Time `json:"fin,omitzero"`
	Horas        float64   `json:"horas"`
	HorasMaquina float64   `json:"horas_maquina"`
	HorasHombre  float64   `json:"horas_hombre"`
	Tokens       int       `json:"tokens"`
	Coste        float64   `json:"coste"`
	Estado       string    `json:"estado"` // trabajando | activo | en-espera | cerrado | pendiente | error
	Error        string    `json:"error,omitempty"`
	Desc         string    `json:"desc"`
}

type parte struct {
	Parte
	T          Trabajo       `json:"t"`
	UltimaHum  time.Time     `json:"ultima_hum"`
	Maquina    time.Duration `json:"maquina"`     // turnos del agente ya terminados
	TurnoDesde time.Time     `json:"turno_desde"` // inicio del turno en curso (cero si no hay)
	Turnos     int           `json:"-"`
	Entrada    int           `json:"entrada"`
	Salida     int           `json:"salida"`
	Escrito    time.Time     `json:"escrito"` // última escritura de las horas en Odoo
	Cerrado    bool          `json:"cerrado"` // ya cerrado en Odoo: no se vuelve a escribir
	mu         sync.Mutex    // serializa las escrituras en Odoo de este parte
}

// Gestor lleva los partes de todas las sesiones.
type Gestor struct {
	Odoo                  Odoo
	Aviso                 func()
	FicheroEstado, Diario string
	Ahora                 func() time.Time

	mu            sync.Mutex
	partes        map[string]*parte
	pendientes    []*parte
	cerrados      []Parte
	sincronizados map[int]bool
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
	n := strings.TrimSpace(t.Tarea)
	if n == "" || n == "pgocode - Sesión IA" { // la tarea propia de versiones anteriores pasa a la canónica
		return TareaDefecto
	}
	return n
}

// ---- tiempos ----

// fin del tiempo que cuenta: último trabajo + espera, sin pasar de ahora ni de la medianoche del día del inicio.
func (g *Gestor) finDe(p *parte) time.Time {
	fin := p.Ultima.Add(Espera)
	if p.Turnos > 0 || fin.After(g.ahora()) {
		fin = g.ahora()
	}
	y, m, d := p.Inicio.Date()
	if medianoche := time.Date(y, m, d+1, 0, 0, 0, 0, p.Inicio.Location()); fin.After(medianoche) {
		fin = medianoche
	}
	return fin
}

func minutos(d time.Duration) float64 { return float64(int(d.Minutes()+0.5)) / 60 }

// horas de máquina y de hombre (redondeadas al minuto) hasta fin.
func (g *Gestor) horasDe(p *parte) (maq, hom float64, fin time.Time) {
	fin = g.finDe(p)
	total := fin.Sub(p.Inicio)
	m := p.Maquina
	if !p.TurnoDesde.IsZero() {
		m += fin.Sub(p.TurnoDesde)
	}
	if m > total {
		m = total
	}
	maq, hom = minutos(m), minutos(total-m)
	if maq+hom == 0 {
		hom = 1.0 / 60 // al menos un minuto
	}
	return
}

// ---- trabajo ----

// Arranca: recupera el estado (del disco y de Odoo), y pone en marcha el reloj que escribe las horas, cierra tras
// la espera y reintenta lo pendiente.
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
		var e struct{ Abiertos, Pendientes []*parte }
		if b, err := os.ReadFile(g.FicheroEstado); err == nil && json.Unmarshal(b, &e) == nil {
			viejos = append(e.Abiertos, e.Pendientes...)
		}
	}
	g.mu.Unlock()
	usuarios := map[int]string{}
	for _, p := range viejos {
		if p.T.UID > 0 && p.T.OdooSesion != "" {
			usuarios[p.T.UID] = p.T.OdooSesion
		}
		p.Turnos = 0
		if !p.TurnoDesde.IsZero() { // el turno que estaba en curso termina donde terminó la ejecución anterior
			p.Maquina += p.Ultima.Sub(p.TurnoDesde)
			p.TurnoDesde = time.Time{}
		}
		if p.Estado != "pendiente" && g.ahora().Sub(p.Ultima) < Espera {
			g.mu.Lock()
			g.estado(p)
			g.partes[p.Sesion] = p
			g.mu.Unlock()
			g.diario("retoma", p, "parte abierto antes de reiniciar pgocode")
			continue
		}
		g.diario("recupera", p, "parte de una ejecución anterior de pgocode")
		g.cierraParte(p, "pgocode se cerró sin cerrarlo")
	}
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
			ahora := g.ahora()
			if p.Turnos--; p.Turnos == 0 && !p.TurnoDesde.IsZero() {
				p.Maquina += ahora.Sub(p.TurnoDesde)
				p.TurnoDesde = time.Time{}
			}
			p.Ultima = ahora
			g.estado(p)
			g.mu.Unlock()
			go g.escribe(p) // fin de turno: las horas al día en Odoo
			g.guarda()
			g.avisa()
		})
	}
}

// Actividad: la persona está usando la sesión (teclado, ratón… con la ventana en primer plano).
func (g *Gestor) Actividad(t Trabajo) { g.trabajo(t, false, true) }

func (g *Gestor) trabajo(t Trabajo, turno, humano bool) *parte {
	g.mu.Lock()
	if g.partes == nil {
		g.partes = map[string]*parte{}
	}
	ahora := g.ahora()
	p := g.partes[t.Sesion]
	// Otra tarea, otro proyecto u otro día: se cierra el parte y se abre otro.
	if p != nil && (p.T.Proyecto != t.Proyecto || tareaDe(p.T) != tareaDe(t) || (t.TareaID != 0 && p.T.TareaID != t.TareaID) ||
		ahora.Format("2006-01-02") != p.Inicio.Format("2006-01-02")) {
		ant := p
		delete(g.partes, t.Sesion)
		go g.cierraParte(ant, "cambio de tarea, de proyecto o de día")
		p = nil
	}
	nuevo := p == nil
	if nuevo {
		p = &parte{T: t}
		p.Parte = Parte{Sesion: t.Sesion, Titulo: t.Titulo, Proyecto: t.Proyecto, NombreProy: t.NombreProy, TareaID: t.TareaID,
			Tarea: tareaDe(t), Inicio: ahora}
		g.partes[t.Sesion] = p
	}
	if t.Desc != "" {
		p.T.Desc, p.Desc = t.Desc, t.Desc
	}
	if p.T.Desc == "" {
		p.T.Desc, p.Desc = Descripcion(t.Titulo, ""), Descripcion(t.Titulo, "")
	}
	if t.Modelo != "" {
		p.T.Modelo = t.Modelo
	}
	if t.Titulo != "" {
		p.T.Titulo, p.Titulo = t.Titulo, t.Titulo
	}
	if t.OdooSesion != "" {
		p.T.OdooSesion = t.OdooSesion
	}
	if turno {
		if p.Turnos == 0 {
			p.TurnoDesde = ahora
		}
		p.Turnos++
	}
	if humano {
		p.UltimaHum = ahora
	}
	p.Ultima = ahora
	g.estado(p)
	maquinaFalta := turno && p.LineaMaquina == 0
	hecho := g.sincronizados[t.UID]
	g.mu.Unlock()
	if nuevo || maquinaFalta {
		go g.escribe(p) // crea la(s) línea(s) que falten
	}
	g.guarda()
	g.avisa()
	if t.UID > 0 && t.OdooSesion != "" && !hecho {
		go g.Sincroniza(context.Background(), t.UID, t.OdooSesion)
	}
	return p
}

// estado (con el cerrojo).
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

// revisa: cierra lo que ha pasado la espera (o la medianoche), escribe las horas de las líneas vivas cada 3
// minutos y actualiza los estados.
func (g *Gestor) revisa() {
	g.mu.Lock()
	var cerrar, escribir []*parte
	cambio := false
	ahora := g.ahora()
	for s, p := range g.partes {
		antes := p.Estado
		g.estado(p)
		cambio = cambio || antes != p.Estado
		y, m, d := p.Inicio.Date()
		pasada := ahora.After(time.Date(y, m, d+1, 0, 0, 0, 0, p.Inicio.Location()))
		if (p.Turnos == 0 && ahora.Sub(p.Ultima) >= Espera) || pasada {
			delete(g.partes, s)
			cerrar = append(cerrar, p)
		} else if ahora.Sub(p.Escrito) >= Sincroniza {
			escribir = append(escribir, p)
		}
	}
	g.mu.Unlock()
	for _, p := range cerrar {
		motivo := "5 minutos sin trabajo"
		if g.ahora().Format("2006-01-02") != p.Inicio.Format("2006-01-02") {
			motivo = "fin del día"
		}
		g.cierraParte(p, motivo)
	}
	for _, p := range escribir {
		g.escribe(p)
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

// ActualizaSesionOdoo: tras volver a entrar, los partes y pendientes del usuario usan la nueva sesión de Odoo.
func (g *Gestor) ActualizaSesionOdoo(uid int, sesion string) {
	g.mu.Lock()
	for _, l := range [][]*parte{g.pendientes} {
		for _, p := range l {
			if p.T.UID == uid {
				p.T.OdooSesion = sesion
			}
		}
	}
	for _, p := range g.partes {
		if p.T.UID == uid {
			p.T.OdooSesion = sesion
		}
	}
	g.mu.Unlock()
	g.guarda()
}

// ---- Odoo ----

// escribe crea las líneas que falten y pone al día sus horas (y la descripción y los datos de IA).
func (g *Gestor) escribe(p *parte) error { return g.escribeFin(p, false) }

func (g *Gestor) escribeFin(p *parte, cerrar bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Una escritura «al día» que llega tarde no puede reabrir un parte ya cerrado.
	if p.Cerrado {
		return nil
	}
	ctx, cancela := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancela()
	g.mu.Lock()
	t := p.T
	maq, hom, _ := g.horasDe(p)
	turnos := p.Turnos > 0 || p.Maquina > 0 || !p.TurnoDesde.IsZero()
	lm, lh, tareaID := p.LineaMaquina, p.LineaHombre, p.TareaID
	entrada, salida, coste := p.Entrada, p.Salida, p.Coste
	fecha := p.Inicio.Format("2006-01-02")
	g.mu.Unlock()

	if tareaID == 0 {
		id, err := g.tarea(ctx, t)
		if err != nil {
			g.falla(p, "tarea: "+err.Error())
			return err
		}
		tareaID = id
		g.mu.Lock()
		p.TareaID, p.T.TareaID = id, id
		g.mu.Unlock()
	}
	marca := func(clase string) string {
		if cerrar {
			return "pgocode-" + t.Sesion + "-" + clase
		}
		return marcaAbierta + t.Sesion + "-" + clase
	}
	nombre := t.Desc
	if strings.TrimSpace(nombre) == "" {
		nombre = Descripcion(t.Titulo, "")
	}
	base := func(horas float64, clase string) map[string]any {
		v := map[string]any{"name": nombre, "unit_amount": horas, "ai_session_id": marca(clase)}
		if clase == "m" {
			v["ai_model"] = t.Modelo
			v["ai_tokens_input"], v["ai_tokens_output"], v["ai_tokens_total"], v["ai_cost"] = entrada, salida, entrada+salida, coste
		}
		return v
	}
	// Hora Hombre: siempre; Hora Máquina: si ha habido turnos del agente.
	var err error
	if lh, err = g.linea(ctx, t, lh, tareaID, fecha, base(hom, "h")); err != nil {
		return g.errorEscritura(p, err)
	}
	if turnos {
		if lm, err = g.linea(ctx, t, lm, tareaID, fecha, base(maq, "m")); err != nil {
			return g.errorEscritura(p, err)
		}
	}
	g.mu.Lock()
	nuevaH, nuevaM := p.LineaHombre == 0 && lh > 0, p.LineaMaquina == 0 && lm > 0
	p.LineaHombre, p.LineaMaquina, p.Escrito = lh, lm, g.ahora()
	p.Cerrado = cerrar
	if p.Estado == "error" {
		p.Estado, p.Error = "en-espera", ""
		g.estado(p)
	}
	g.mu.Unlock()
	if nuevaH || nuevaM {
		g.diario("abre", p, "")
	}
	g.guarda()
	g.avisa()
	return nil
}

// linea crea (id 0) o actualiza una línea de parte. Si Odoo no tiene los campos de IA, se escribe sin ellos.
func (g *Gestor) linea(ctx context.Context, t Trabajo, id, tarea int, fecha string, vals map[string]any) (int, error) {
	sinIA := func(v map[string]any) map[string]any {
		x := map[string]any{}
		for k, val := range v {
			if !strings.HasPrefix(k, "ai_") {
				x[k] = val
			}
		}
		return x
	}
	var nada any
	if id > 0 {
		err := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, vals}, nil, &nada)
		if err != nil && !definitivo(err) {
			if e2 := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "write", []any{[]int{id}, sinIA(vals)}, nil, &nada); e2 == nil {
				depuracion.Log("imputacion", "línea %d escrita sin los campos de IA: %v", id, err)
				return id, nil
			}
		}
		return id, err
	}
	v := map[string]any{"date": fecha, "project_id": t.Proyecto, "task_id": tarea}
	if t.UID > 0 {
		v["user_id"] = t.UID
	}
	for k, x := range vals {
		v[k] = x
	}
	var nuevo int
	err := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "create", []any{v}, nil, &nuevo)
	if err != nil {
		if e2 := g.Odoo.CallKW(ctx, t.OdooSesion, "account.analytic.line", "create", []any{sinIA(v)}, nil, &nuevo); e2 == nil {
			depuracion.Log("imputacion", "línea %d creada sin los campos de IA: %v", nuevo, err)
			return nuevo, nil
		}
	}
	return nuevo, err
}

// definitivo: errores de Odoo que no se arreglan reintentando (la línea ya no existe o está validada).
func definitivo(err error) bool {
	s := strings.ToLower(err.Error())
	for _, k := range []string{"missingerror", "does not exist", "no existe", "has been deleted", "validad", "validated", "accesserror"} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

func (g *Gestor) errorEscritura(p *parte, err error) error {
	if definitivo(err) {
		g.falla(p, "Odoo: "+err.Error())
	} else {
		depuracion.Log("imputacion", "sesión %s: escribir en Odoo: %v (se reintentará)", p.Sesion, err)
	}
	return err
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

// cierraParte escribe las horas finales y quita la marca de abierta. Si Odoo falla (y no es definitivo), queda
// pendiente y se reintenta.
func (g *Gestor) cierraParte(p *parte, motivo string) {
	err := g.escribeFin(p, true)
	g.mu.Lock()
	maq, hom, fin := g.horasDe(p)
	p.Fin, p.HorasMaquina, p.HorasHombre, p.Horas = fin, maq, hom, maq+hom
	switch {
	case err == nil:
		p.Estado, p.Error = "cerrado", ""
	case definitivo(err):
		p.Estado = "error"
	default:
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

func (g *Gestor) reintenta() {
	g.mu.Lock()
	l := g.pendientes
	g.pendientes = nil
	var quedan []Parte
	for _, c := range g.cerrados {
		pend := false
		for _, p := range l {
			pend = pend || (c.Sesion == p.Sesion && c.Inicio.Equal(p.Inicio) && c.Estado == "pendiente")
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

// CierraTodo cierra todos los partes (al salir de pgocode de verdad).
func (g *Gestor) CierraTodo() {
	g.mu.Lock()
	var l []*parte
	for s, p := range g.partes {
		if !p.TurnoDesde.IsZero() {
			p.Maquina += g.ahora().Sub(p.TurnoDesde)
			p.TurnoDesde = time.Time{}
		}
		p.Turnos = 0
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

// Suspende: pgocode sale para actualizarse. No se cierra nada: las líneas siguen abiertas en Odoo con sus horas al
// día, y la versión nueva las retoma.
func (g *Gestor) Suspende() {
	g.mu.Lock()
	var l []*parte
	for _, p := range g.partes {
		l = append(l, p)
	}
	g.mu.Unlock()
	for _, p := range l {
		g.escribe(p)
	}
	g.guarda()
	depuracion.Log("imputacion", "pgocode se actualiza: %d partes siguen abiertos en Odoo", len(l))
}

// Sincroniza lee de Odoo las líneas abiertas de pgocode del usuario (ai_session_id pgocode-abierto-…) y retoma las
// que pgocode no tenga: las líneas vivas de Odoo mandan.
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
		ID        int     `json:"id"`
		Name      string  `json:"name"`
		Sesion    any     `json:"ai_session_id"`
		Model     any     `json:"ai_model"`
		Creada    string  `json:"create_date"`
		Fecha     string  `json:"date"`
		Horas     float64 `json:"unit_amount"`
		Project   []any   `json:"project_id"`
		Task      any     `json:"task_id"`
		TokensIn  any     `json:"ai_tokens_input"`
		TokensOut any     `json:"ai_tokens_output"`
		Coste     any     `json:"ai_cost"`
	}
	dominio := []any{[]any{"user_id", "=", uid}, []any{"ai_session_id", "=like", marcaAbierta + "%"}}
	campos := map[string]any{"fields": []string{"id", "name", "ai_session_id", "ai_model", "create_date", "date", "unit_amount",
		"project_id", "task_id", "ai_tokens_input", "ai_tokens_output", "ai_cost"}, "limit": 100}
	if err := g.Odoo.CallKW(ctx, sesionOdoo, "account.analytic.line", "search_read", []any{dominio}, campos, &l); err != nil {
		depuracion.Log("imputacion", "leer las líneas abiertas de Odoo (usuario %d): %v", uid, err)
		return
	}
	num := func(v any) float64 { f, _ := v.(float64); return f }
	for _, x := range l {
		s, _ := x.Sesion.(string)
		resto := strings.TrimPrefix(s, marcaAbierta)
		if len(resto) < 3 || len(x.Project) < 2 {
			continue
		}
		sesion, clase := resto[:len(resto)-2], resto[len(resto)-1:]
		creada, err := time.ParseInLocation("2006-01-02 15:04:05", x.Creada, time.UTC)
		if err != nil {
			continue
		}
		g.mu.Lock()
		p := g.partes[sesion]
		if p != nil && (p.LineaHombre == x.ID || p.LineaMaquina == x.ID) {
			g.mu.Unlock()
			continue
		}
		if p == nil {
			proy, _ := x.Project[0].(float64)
			nombreProy, _ := x.Project[1].(string)
			tareaID, tarea := 0, TareaDefecto
			if t, ok := x.Task.([]any); ok && len(t) == 2 {
				tareaID = int(num(t[0]))
				tarea, _ = t[1].(string)
			}
			p = &parte{T: Trabajo{Sesion: sesion, Proyecto: int(proy), NombreProy: nombreProy, TareaID: tareaID, Tarea: tarea,
				UID: uid, OdooSesion: sesionOdoo, Desc: x.Name}}
			// No se sabe cuándo fue el último trabajo: se cuenta desde ahora con su espera.
			p.Parte = Parte{Sesion: sesion, Proyecto: int(proy), NombreProy: nombreProy, TareaID: tareaID, Tarea: tarea,
				Inicio: creada.Local(), Ultima: g.ahora(), Desc: x.Name, Estado: "en-espera"}
			if g.partes == nil {
				g.partes = map[string]*parte{}
			}
			g.partes[sesion] = p
		}
		if creada.Local().Before(p.Inicio) {
			p.Inicio = creada.Local()
		}
		if clase == "m" {
			p.LineaMaquina = x.ID
			p.Maquina = time.Duration(x.Horas * float64(time.Hour))
			p.Entrada, p.Salida, p.Coste = int(num(x.TokensIn)), int(num(x.TokensOut)), num(x.Coste)
			p.Tokens = p.Entrada + p.Salida
			if m, _ := x.Model.(string); m != "" {
				p.T.Modelo = m
			}
		} else {
			p.LineaHombre = x.ID
		}
		g.estado(p)
		g.mu.Unlock()
		g.diario("retoma", p, "línea abierta en Odoo")
	}
	g.guarda()
	g.avisa()
}

// Estado: partes abiertos y los cerrados hoy.
func (g *Gestor) Estado() (abiertos, cerrados []Parte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	abiertos, cerrados = []Parte{}, []Parte{}
	for _, p := range g.partes {
		x := p.Parte
		x.HorasMaquina, x.HorasHombre, _ = g.horasDe(p)
		x.Horas = x.HorasMaquina + x.HorasHombre
		abiertos = append(abiertos, x)
	}
	sort.Slice(abiertos, func(i, j int) bool { return abiertos[i].Inicio.Before(abiertos[j].Inicio) })
	hoy := g.ahora().Format("2006-01-02")
	for _, c := range g.cerrados {
		if c.Fin.Format("2006-01-02") == hoy || c.Inicio.Format("2006-01-02") == hoy {
			cerrados = append(cerrados, c)
		}
	}
	return
}

func (g *Gestor) guarda() {
	if g.FicheroEstado == "" {
		return
	}
	g.mu.Lock()
	e := struct{ Abiertos, Pendientes []*parte }{Pendientes: g.pendientes}
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

func (g *Gestor) diario(evento string, p *parte, motivo string) {
	g.mu.Lock()
	maq, hom, _ := g.horasDe(p)
	r := map[string]any{"fecha": g.ahora().Format(time.RFC3339), "evento": evento, "sesion": p.Sesion, "titulo": p.Titulo,
		"proyecto": p.Proyecto, "nombre_proyecto": p.NombreProy, "tarea_id": p.TareaID, "tarea": p.Tarea,
		"linea_maquina": p.LineaMaquina, "linea_hombre": p.LineaHombre, "inicio": p.Inicio.Format(time.RFC3339),
		"ultimo_trabajo": p.Ultima.Format(time.RFC3339), "horas_maquina": maq, "horas_hombre": hom, "tokens": p.Tokens, "motivo": motivo}
	g.mu.Unlock()
	depuracion.Log("imputacion", "%s sesión %s (proyecto %d, %s): máquina %.2f h [%d], hombre %.2f h [%d] %s", evento, p.Sesion,
		p.Proyecto, p.Tarea, maq, p.LineaMaquina, hom, p.LineaHombre, motivo)
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
