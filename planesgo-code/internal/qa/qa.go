// Package qa da acceso a las campañas de pruebas de un proyecto (<carpeta>/.psf/qa/<campaña>/): sus casos
// (visor/casos.json), sus vídeos (visor/media), sus pruebas ejecutables (*.py) y los comentarios de cada caso.
// Lanza las pruebas en este equipo, con el navegador visible: el grabador de la campaña emite al Studio TV local.
package qa

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/proceso"
)

var (
	nombreValido = regexp.MustCompile(`^[A-Za-z0-9._-]{1,80}$`)
	scriptValido = regexp.MustCompile(`^(pruebas/)?[A-Za-z0-9._-]{1,80}\.py$`)
	archivoMedia = regexp.MustCompile(`^media/[A-Za-z0-9._-]{1,120}\.(mp4|webm|jpg|jpeg|png)$`)
	ErrNoExiste  = errors.New("campaña no encontrada")
	ErrNoValido  = errors.New("nombre no válido")
)

// Campana es una campaña de pruebas de una carpeta del proyecto.
type Campana struct {
	ID          string `json:"id"` // <índice de la carpeta>:<nombre>
	Nombre      string `json:"nombre"`
	Carpeta     string `json:"carpeta"`
	Casos       int    `json:"casos"`
	Fase        string `json:"fase,omitempty"`
	Actualizado string `json:"actualizado,omitempty"`
}

type resumen struct {
	Fase        string            `json:"fase"`
	Actualizado string            `json:"actualizado"`
	Casos       []json.RawMessage `json:"casos"`
}

// Busca las campañas de las carpetas del proyecto.
func Busca(carpetas []string) []Campana {
	l := []Campana{}
	for i, c := range carpetas {
		dirs, _ := filepath.Glob(filepath.Join(c, ".psf", "qa", "*"))
		sort.Strings(dirs)
		for _, d := range dirs {
			n := filepath.Base(d)
			if st, err := os.Stat(d); err != nil || !st.IsDir() || !nombreValido.MatchString(n) {
				continue
			}
			cp := Campana{ID: fmt.Sprintf("%d:%s", i, n), Nombre: n, Carpeta: c}
			if b, err := os.ReadFile(filepath.Join(d, "visor", "casos.json")); err == nil {
				var r resumen
				if json.Unmarshal(b, &r) == nil {
					cp.Casos, cp.Fase, cp.Actualizado = len(r.Casos), r.Fase, r.Actualizado
				}
			}
			l = append(l, cp)
		}
	}
	return l
}

// Dir devuelve la carpeta de una campaña a partir de su id (comprobado: no se sale del proyecto).
func Dir(carpetas []string, id string) (string, error) {
	i, nombre, ok := strings.Cut(id, ":")
	n, err := strconv.Atoi(i)
	if !ok || err != nil || n < 0 || n >= len(carpetas) || !nombreValido.MatchString(nombre) {
		return "", ErrNoValido
	}
	d := filepath.Join(carpetas[n], ".psf", "qa", nombre)
	if st, err := os.Stat(d); err != nil || !st.IsDir() {
		return "", ErrNoExiste
	}
	return d, nil
}

// Casos devuelve visor/casos.json tal cual (o un documento vacío).
func Casos(dir string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, "visor", "casos.json"))
	if errors.Is(err, os.ErrNotExist) {
		return []byte(`{"casos":[]}`), nil
	}
	return b, err
}

// Media devuelve la ruta de un vídeo o imagen del visor (solo media/…).
func Media(dir, archivo string) (string, error) {
	if !archivoMedia.MatchString(archivo) {
		return "", ErrNoValido
	}
	p := filepath.Join(dir, "visor", filepath.FromSlash(archivo))
	if _, err := os.Stat(p); err != nil {
		return "", ErrNoExiste
	}
	return p, nil
}

// Pruebas lista los scripts que se pueden lanzar: *.py de la campaña y de pruebas/ (sin bibliotecas como rpc.py).
func Pruebas(dir string) []string {
	l := []string{}
	for _, patron := range []string{"*.py", "pruebas/*.py"} {
		fs, _ := filepath.Glob(filepath.Join(dir, patron))
		for _, f := range fs {
			rel, _ := filepath.Rel(dir, f)
			rel = filepath.ToSlash(rel)
			if b := filepath.Base(rel); b == "rpc.py" || strings.HasPrefix(b, "_") {
				continue
			}
			l = append(l, rel)
		}
	}
	sort.Strings(l)
	return l
}

// Comentario es una nota sobre un caso en un segundo concreto del vídeo.
type Comentario struct {
	Caso  string    `json:"caso"`
	T     float64   `json:"t"`
	Texto string    `json:"texto"`
	Autor string    `json:"autor"`
	Fecha time.Time `json:"fecha"`
}

var muComentarios sync.Mutex

// Comentarios lee visor/comentarios.json.
func Comentarios(dir string) []Comentario {
	muComentarios.Lock()
	defer muComentarios.Unlock()
	return leeComentarios(dir)
}

func leeComentarios(dir string) []Comentario {
	var l []Comentario
	if b, err := os.ReadFile(filepath.Join(dir, "visor", "comentarios.json")); err == nil {
		json.Unmarshal(b, &l)
	}
	if l == nil {
		l = []Comentario{}
	}
	return l
}

// Comenta añade un comentario a visor/comentarios.json.
func Comenta(dir string, c Comentario) error {
	if strings.TrimSpace(c.Texto) == "" || len(c.Texto) > 4000 || len(c.Caso) > 80 {
		return ErrNoValido
	}
	muComentarios.Lock()
	defer muComentarios.Unlock()
	l := append(leeComentarios(dir), c)
	b, _ := json.MarshalIndent(l, "", " ")
	if err := os.MkdirAll(filepath.Join(dir, "visor"), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, "visor", "comentarios.json.tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "visor", "comentarios.json"))
}

// Ejecucion es una prueba lanzada desde la app.
type Ejecucion struct {
	ID      string    `json:"id"`
	Campana string    `json:"campana"`
	Script  string    `json:"script"`
	Inicio  time.Time `json:"inicio"`
	Fin     time.Time `json:"fin,omitzero"`
	Estado  string    `json:"estado"` // curso | ok | error | parada
	Codigo  int       `json:"codigo"`
	Salida  []string  `json:"salida"`

	cancela context.CancelFunc
}

const maxLineas = 300

// Ejecutor lanza las pruebas y guarda las últimas ejecuciones.
type Ejecutor struct {
	mu  sync.Mutex
	ejs []*Ejecucion
	sig int
}

// EnCurso dice cuántas pruebas se están ejecutando (para no actualizar la app mientras tanto).
func (e *Ejecutor) EnCurso() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, x := range e.ejs {
		if x.Estado == "curso" {
			n++
		}
	}
	return n
}

// Lista devuelve copias de las ejecuciones de una campaña (la más reciente primero).
func (e *Ejecutor) Lista(campana string) []Ejecucion {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := []Ejecucion{}
	for i := len(e.ejs) - 1; i >= 0; i-- {
		if x := e.ejs[i]; x.Campana == campana {
			c := *x
			c.Salida = append([]string(nil), x.Salida...)
			l = append(l, c)
		}
	}
	return l
}

// Para detiene una ejecución en curso.
func (e *Ejecutor) Para(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, x := range e.ejs {
		if x.ID == id && x.Estado == "curso" {
			x.cancela()
			return true
		}
	}
	return false
}

// Lanza ejecuta un script de la campaña con su Python (.venv si existe), el navegador visible y sin búfer, para
// ver la salida al momento. El grabador de la campaña emite solo al Studio TV local (~/.planesgo/studio_tv.env).
func (e *Ejecutor) Lanza(campana, dir, script string) (*Ejecucion, error) {
	if !scriptValido.MatchString(script) || strings.Contains(script, "..") {
		return nil, ErrNoValido
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(script))); err != nil {
		return nil, ErrNoExiste
	}
	python := filepath.Join(dir, ".venv", "bin", "python")
	if _, err := os.Stat(python); err != nil {
		python = "python3"
	}
	ctx, cancela := context.WithTimeout(context.Background(), 4*time.Hour)
	cmd := exec.CommandContext(ctx, python, "-u", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1")
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 5 * time.Second
	salida, err := cmd.StdoutPipe()
	if err != nil {
		cancela()
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	e.mu.Lock()
	e.sig++
	x := &Ejecucion{ID: fmt.Sprintf("%d-%d", time.Now().Unix(), e.sig), Campana: campana, Script: script,
		Inicio: time.Now(), Estado: "curso", Salida: []string{}, cancela: cancela}
	e.ejs = append(e.ejs, x)
	if len(e.ejs) > 30 {
		e.ejs = e.ejs[len(e.ejs)-30:]
	}
	e.mu.Unlock()
	if err := cmd.Start(); err != nil {
		cancela()
		e.termina(x, "error", -1, err.Error())
		return x, nil
	}
	go func() {
		defer cancela()
		e.lee(x, salida)
		err := cmd.Wait()
		switch {
		case ctx.Err() == context.Canceled:
			e.termina(x, "parada", -1, "Prueba detenida.")
		case err != nil:
			codigo := -1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				codigo = ee.ExitCode()
			}
			e.termina(x, "error", codigo, "")
		default:
			e.termina(x, "ok", 0, "")
		}
	}()
	return x, nil
}

func (e *Ejecutor) lee(x *Ejecucion, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		e.mu.Lock()
		x.Salida = append(x.Salida, sc.Text())
		if len(x.Salida) > maxLineas {
			x.Salida = x.Salida[len(x.Salida)-maxLineas:]
		}
		e.mu.Unlock()
	}
}

func (e *Ejecutor) termina(x *Ejecucion, estado string, codigo int, linea string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	x.Estado, x.Codigo, x.Fin = estado, codigo, time.Now()
	if linea != "" {
		x.Salida = append(x.Salida, linea)
	}
}
