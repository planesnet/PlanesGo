// Package procesos arranca y vigila los programas del proyecto que se ejecutan en local (un Odoo, un servicio,
// el programa en desarrollo…): los lanza el usuario desde pgocode o el agente con la orden «pgo», guarda las
// últimas líneas de su salida y detecta la URL en la que escuchan para abrirla en el navegador integrado.
package procesos

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/proceso"
)

var (
	nombreValido = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	// Una URL local en la salida (p. ej. «HTTP service (werkzeug) running on 0.0.0.0:8069» o «http://localhost:5173/»).
	urlLocal  = regexp.MustCompile(`https?://(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1?\])(?::\d+)?[^\s"'<>]*`)
	puerto    = regexp.MustCompile(`(?:running on|listening on|escuchando en)\s+(?:https?://)?(?:[\w.-]+|\[[:\w]+\]):(\d{2,5})`)
	ErrNombre = errors.New("nombre no válido (letras, números, punto, guion y guion bajo)")
	ErrNoHay  = errors.New("no hay ningún proceso con ese nombre")
)

const maxLineas = 1000

// Proceso es un programa lanzado (en curso o terminado).
type Proceso struct {
	Nombre   string    `json:"nombre"`
	Proyecto int       `json:"proyecto"`
	Carpeta  string    `json:"carpeta"`
	Orden    string    `json:"orden"`
	URL      string    `json:"url,omitempty"`
	Estado   string    `json:"estado"` // curso | terminado | error | parado
	Codigo   int       `json:"codigo"`
	Inicio   time.Time `json:"inicio"`
	Fin      time.Time `json:"fin,omitzero"`
	Lanzado  string    `json:"lanzado"` // usuario | agente

	lineas  []string
	total   int
	cancela context.CancelFunc
	hecho   chan struct{}
}

// Gestor guarda los procesos por nombre. Al lanzar uno con el nombre de otro en curso, el anterior se para.
type Gestor struct {
	mu    sync.Mutex
	ps    map[string]*Proceso
	Aviso func() // se llama cuando cambia algo (para avisar a la interfaz)
}

func Nuevo() *Gestor { return &Gestor{ps: map[string]*Proceso{}} }

func (g *Gestor) avisa() {
	if g.Aviso != nil {
		go g.Aviso()
	}
}

// Lanza ejecuta orden (con bash -c) en carpeta, en su propio grupo de procesos.
func (g *Gestor) Lanza(nombre string, proyecto int, carpeta, orden, lanzado string, entorno []string) (*Proceso, error) {
	if !nombreValido.MatchString(nombre) {
		return nil, ErrNombre
	}
	if strings.TrimSpace(orden) == "" {
		return nil, errors.New("falta la orden")
	}
	if st, err := os.Stat(carpeta); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("la carpeta %q no existe", carpeta)
	}
	g.Para(nombre) // mismo nombre: se sustituye
	ctx, cancela := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "bash", "-c", orden)
	cmd.Dir = carpeta
	cmd.Env = append(os.Environ(), entorno...)
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 10 * time.Second
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	p := &Proceso{Nombre: nombre, Proyecto: proyecto, Carpeta: carpeta, Orden: orden, Estado: "curso", Inicio: time.Now(),
		Lanzado: lanzado, cancela: cancela, hecho: make(chan struct{})}
	if err := cmd.Start(); err != nil {
		cancela()
		return nil, err
	}
	g.mu.Lock()
	g.ps[nombre] = p
	g.mu.Unlock()
	g.avisa()
	go g.lee(p, pr)
	go func() {
		err := cmd.Wait()
		pw.Close()
		g.mu.Lock()
		p.Fin = time.Now()
		switch {
		case ctx.Err() != nil:
			p.Estado, p.Codigo = "parado", -1
		case err != nil:
			p.Estado, p.Codigo = "error", -1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				p.Codigo = ee.ExitCode()
			}
		default:
			p.Estado = "terminado"
		}
		g.mu.Unlock()
		cancela()
		close(p.hecho)
		g.avisa()
	}()
	return p, nil
}

func (g *Gestor) lee(p *Proceso, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		l := sc.Text()
		g.mu.Lock()
		p.lineas = append(p.lineas, l)
		p.total++
		if len(p.lineas) > maxLineas {
			p.lineas = p.lineas[len(p.lineas)-maxLineas:]
		}
		nueva := false
		if p.URL == "" {
			if u := urlDe(l); u != "" {
				p.URL, nueva = u, true
			}
		}
		g.mu.Unlock()
		if nueva {
			g.avisa()
		}
	}
	io.Copy(io.Discard, r)
}

// urlDe saca de una línea de salida la URL local en la que escucha el programa.
func urlDe(l string) string {
	if u := urlLocal.FindString(l); u != "" {
		u = strings.TrimRight(u, ".,;:)]")
		return strings.Replace(strings.Replace(u, "0.0.0.0", "localhost", 1), "[::]", "localhost", 1)
	}
	if m := puerto.FindStringSubmatch(l); m != nil {
		return "http://localhost:" + m[1] + "/"
	}
	return ""
}

// Para detiene un proceso (todo su grupo) y espera a que acabe.
func (g *Gestor) Para(nombre string) bool {
	g.mu.Lock()
	p := g.ps[nombre]
	g.mu.Unlock()
	if p == nil || p.Estado != "curso" {
		return false
	}
	p.cancela()
	select {
	case <-p.hecho:
	case <-time.After(15 * time.Second):
	}
	return true
}

// Quita un proceso terminado de la lista.
func (g *Gestor) Quita(nombre string) {
	g.mu.Lock()
	if p := g.ps[nombre]; p != nil && p.Estado != "curso" {
		delete(g.ps, nombre)
	}
	g.mu.Unlock()
	g.avisa()
}

// ParaTodos detiene todos los procesos (al cerrar la app).
func (g *Gestor) ParaTodos() {
	g.mu.Lock()
	var ns []string
	for n := range g.ps {
		ns = append(ns, n)
	}
	g.mu.Unlock()
	for _, n := range ns {
		g.Para(n)
	}
}

// Lista devuelve copias de los procesos de un proyecto (0: todos), los más recientes primero.
func (g *Gestor) Lista(proyecto int) []Proceso {
	g.mu.Lock()
	defer g.mu.Unlock()
	l := []Proceso{}
	for _, p := range g.ps {
		if proyecto == 0 || p.Proyecto == proyecto {
			c := *p
			c.lineas = nil
			l = append(l, c)
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Inicio.After(l[j].Inicio) })
	return l
}

// Busca devuelve una copia de un proceso.
func (g *Gestor) Busca(nombre string) (Proceso, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.ps[nombre]
	if p == nil {
		return Proceso{}, false
	}
	c := *p
	c.lineas = nil
	return c, true
}

// Salida devuelve las últimas n líneas de un proceso y el total de líneas que lleva.
func (g *Gestor) Salida(nombre string, n int) ([]string, int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := g.ps[nombre]
	if p == nil {
		return nil, 0, ErrNoHay
	}
	if n <= 0 || n > len(p.lineas) {
		n = len(p.lineas)
	}
	return append([]string(nil), p.lineas[len(p.lineas)-n:]...), p.total, nil
}

// EnCurso cuenta los procesos en marcha.
func (g *Gestor) EnCurso() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, p := range g.ps {
		if p.Estado == "curso" {
			n++
		}
	}
	return n
}
