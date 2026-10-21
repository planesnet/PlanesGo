// Package imputacion lleva el tiempo de las sesiones de pgocode a PlanesGo (Odoo) sin que el agente intervenga:
// pgocode sabe el proyecto de cada sesión, así que vincula la carpeta (.planesgo.json) y envía los latidos con
// planesgo-mcp: uno al empezar cada turno del agente y otro cada 30 s mientras trabaja; si en 5 minutos no hay
// otro turno, cierra el parte (se imputa en Odoo con su descripción).
package imputacion

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"planesgo-code/internal/depuracion"
)

const (
	Tarea       = "pgocode - Sesión IA"
	TipoTarea   = "Desarrollo"
	Intervalo   = 30 * time.Second
	CerrarTras  = 5 * time.Minute
	URLPlanesGo = "https://planesgo.autopyme.com"
)

// Trabajo es lo que se imputa de una sesión.
type Trabajo struct {
	Sesion   string // id de la sesión de pgocode (va como ai_session_id)
	Proyecto int    // proyecto de Odoo
	Nombre   string // nombre del proyecto (para .planesgo.json)
	Carpeta  string
	Modelo   string // agente y modelo
	Desc     string // qué se está haciendo (en español)
}

type parte struct {
	t        Trabajo
	abierto  bool      // Odoo confirmó el primer latido
	ultimo   time.Time // último latido enviado
	enTurnos int       // turnos de agente en curso
	cierre   *time.Timer
}

// Gestor lleva los partes abiertos (uno por sesión).
type Gestor struct {
	Bin string // planesgo-mcp (vacío: se busca)

	mu     sync.Mutex
	partes map[string]*parte
}

func (g *Gestor) bin() string {
	if g.Bin != "" {
		return g.Bin
	}
	if p, err := exec.LookPath("planesgo-mcp"); err == nil {
		return p
	}
	casa, _ := os.UserHomeDir()
	return filepath.Join(casa, ".local", "bin", "planesgo-mcp")
}

// Vincula escribe .planesgo.json en la carpeta si no lo tiene (el mismo vínculo que crea set_project).
func Vincula(carpeta string, proyecto int, nombre string) error {
	f := filepath.Join(carpeta, ".planesgo.json")
	if _, err := os.Stat(f); err == nil {
		return nil
	}
	b, _ := json.MarshalIndent(map[string]any{"odoo_project_id": proyecto, "odoo_project_name": nombre, "planesgo_url": URLPlanesGo}, "", "  ")
	if err := os.WriteFile(f, append(b, '\n'), 0o644); err != nil {
		return err
	}
	// Que no acabe en un commit: es un vínculo local.
	excluye(carpeta, ".planesgo.json")
	return nil
}

// excluye añade un patrón a .git/info/exclude del repositorio de la carpeta (también en copias de trabajo).
func excluye(carpeta, patron string) {
	out, err := exec.Command("git", "-C", carpeta, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return
	}
	dir := string(out[:len(out)-1])
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(carpeta, dir)
	}
	f := filepath.Join(dir, "info", "exclude")
	b, _ := os.ReadFile(f)
	for _, l := range splitLineas(string(b)) {
		if l == patron {
			return
		}
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(fh, patron)
		fh.Close()
	}
}

func splitLineas(s string) []string {
	var l []string
	ini := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			l = append(l, s[ini:i])
			ini = i + 1
		}
	}
	return append(l, s[ini:])
}

// Empieza: un turno del agente empieza. Primer latido (abre el parte si hace falta) y latidos cada 30 s hasta
// que se llame a la función devuelta (fin del turno), que programa el cierre del parte.
func (g *Gestor) Empieza(t Trabajo) (fin func()) {
	g.mu.Lock()
	if g.partes == nil {
		g.partes = map[string]*parte{}
	}
	p := g.partes[t.Sesion]
	if p == nil {
		p = &parte{}
		g.partes[t.Sesion] = p
	}
	if p.cierre != nil {
		p.cierre.Stop()
		p.cierre = nil
	}
	// Otra carpeta o proyecto en la misma sesión: se cierra antes el parte anterior.
	if p.abierto && (p.t.Proyecto != t.Proyecto || p.t.Carpeta != t.Carpeta) {
		ant := p.t
		p.abierto = false
		go g.ejecuta("--stop", ant)
	}
	p.t = t
	p.enTurnos++
	g.mu.Unlock()

	g.late(t.Sesion)
	ctx, cancela := context.WithCancel(context.Background())
	go func() {
		tic := time.NewTicker(Intervalo)
		defer tic.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tic.C:
				g.late(t.Sesion)
			}
		}
	}()
	var una sync.Once
	return func() {
		una.Do(func() {
			cancela()
			g.mu.Lock()
			defer g.mu.Unlock()
			if p.enTurnos--; p.enTurnos == 0 {
				p.cierre = time.AfterFunc(CerrarTras, func() { g.cierra(t.Sesion) })
			}
		})
	}
}

// late envía un latido si toca (el primero, en primer plano: solo se da el parte por abierto si Odoo lo confirma).
func (g *Gestor) late(sesion string) {
	g.mu.Lock()
	p := g.partes[sesion]
	if p == nil || (p.abierto && time.Since(p.ultimo) < Intervalo-time.Second) {
		g.mu.Unlock()
		return
	}
	t := p.t
	p.ultimo = time.Now()
	g.mu.Unlock()
	if err := g.ejecuta("--beat", t); err == nil {
		g.mu.Lock()
		p.abierto = true
		g.mu.Unlock()
	}
}

// cierra el parte de una sesión (lo imputa en Odoo).
func (g *Gestor) cierra(sesion string) {
	g.mu.Lock()
	p := g.partes[sesion]
	if p == nil || p.enTurnos > 0 {
		g.mu.Unlock()
		return
	}
	delete(g.partes, sesion)
	t, abierto := p.t, p.abierto
	g.mu.Unlock()
	if abierto {
		g.ejecuta("--stop", t)
	}
}

// CierraTodo cierra los partes abiertos (al cerrar pgocode).
func (g *Gestor) CierraTodo() {
	g.mu.Lock()
	var ss []string
	for s, p := range g.partes {
		if p.cierre != nil {
			p.cierre.Stop()
		}
		p.enTurnos = 0
		ss = append(ss, s)
	}
	g.mu.Unlock()
	for _, s := range ss {
		g.cierra(s)
	}
}

func (g *Gestor) ejecuta(accion string, t Trabajo) error {
	args := []string{accion, "--path", t.Carpeta, "--project-id", strconv.Itoa(t.Proyecto), "--desc", t.Desc,
		"--task", Tarea, "--type", TipoTarea, "--model", t.Modelo, "--session-id", "pgocode-" + t.Sesion}
	ctx, cancela := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancela()
	cmd := exec.CommandContext(ctx, g.bin(), args...)
	cmd.Dir = t.Carpeta
	cmd.Env = append(os.Environ(), "PLANESGO_GESTOR=pgocode")
	out, err := cmd.CombinedOutput()
	depuracion.Log("imputacion", "%s proyecto %d sesión %s: err=%v %s", accion, t.Proyecto, t.Sesion, err, recorta(string(out), 300))
	return err
}

func recorta(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
