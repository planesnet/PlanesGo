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

	grupo   int    // su grupo de procesos
	marca   string // PGO_EJECUCION de sus procesos (también los que se van a otro grupo, p. ej. el Odoo de «odt run»)
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
	marca := fmt.Sprintf("prog-%d-%s-%d", os.Getpid(), nombre, time.Now().UnixNano()%1e9)
	cmd.Env = append(append(os.Environ(), entorno...), proceso.VarMarca+"="+marca)
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 10 * time.Second
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	p := &Proceso{Nombre: nombre, Proyecto: proyecto, Carpeta: carpeta, Orden: orden, Estado: "curso", Inicio: time.Now(),
		Lanzado: lanzado, cancela: cancela, hecho: make(chan struct{}), marca: marca}
	if err := cmd.Start(); err != nil {
		cancela()
		return nil, err
	}
	p.grupo = cmd.Process.Pid
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
	enCurso := p != nil && p.Estado == "curso"
	g.mu.Unlock()
	if !enCurso {
		return false
	}
	// Lo que el programa lanzó en otros grupos (lleva su marca) también se para: primero que termine.
	otros := gruposDe(p)
	for _, g := range otros {
		proceso.SenalGrupo(g, false)
	}
	p.cancela()
	select {
	case <-p.hecho:
	case <-time.After(15 * time.Second):
	}
	go func() {
		time.Sleep(5 * time.Second)
		for _, g := range otros {
			if proceso.GrupoVivo(g) {
				proceso.SenalGrupo(g, true)
			}
		}
	}()
	return true
}

// gruposDe: los otros grupos de procesos con la marca del programa (no el suyo ni el de pgocode).
func gruposDe(p *Proceso) []int {
	if p.marca == "" {
		return nil
	}
	var l []int
	for g := range proceso.GruposConMarca(p.marca) {
		if g != p.grupo && g != proceso.MiGrupo() && g > 1 {
			l = append(l, g)
		}
	}
	return l
}

// DeMarca: el programa en marcha al que pertenece una marca (PGO_EJECUCION), si lo hay.
func (g *Gestor) DeMarca(marca string) (string, bool) {
	if marca == "" {
		return "", false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for n, p := range g.ps {
		if p.Estado == "curso" && p.marca == marca {
			return n, true
		}
	}
	return "", false
}

// PonURL: la dirección de un programa, si aún no la tiene (p. ej. el puerto de su proceso en otro grupo).
func (g *Gestor) PonURL(nombre, url string) {
	g.mu.Lock()
	p := g.ps[nombre]
	nueva := p != nil && p.URL == ""
	if nueva {
		p.URL = url
	}
	g.mu.Unlock()
	if nueva {
		g.avisa()
	}
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

// Grupos: los grupos de procesos de los programas en marcha de pgocode.
func (g *Gestor) Grupos() map[int]bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	m := map[int]bool{}
	for _, p := range g.ps {
		if p.Estado == "curso" && p.grupo > 0 {
			m[p.grupo] = true
		}
	}
	return m
}
