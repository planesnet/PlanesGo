// Package proyectos encuentra las carpetas locales de cada proyecto de Odoo.
//
// Una carpeta pertenece a un proyecto si tiene un .pgo.json con su odoo_project_id (link.go; antes .planesgo.json,
// que se sigue leyendo). Un proyecto puede tener varias carpetas.
package proyectos

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Profundidad máxima de búsqueda bajo cada raíz.
const Profundidad = 4

var saltar = map[string]bool{".git": true, "node_modules": true, ".venv": true, "venv": true, "__pycache__": true,
	"dist": true, "build": true, "vendor": true, ".cache": true, "target": true}

// Indice relaciona proyectos de Odoo con carpetas locales.
type Indice struct {
	Raices []string
	Vida   time.Duration

	// Registro: carpetas que el usuario ha elegido para un proyecto (p. ej. la de la rama de una sesión), guardadas
	// en este fichero. Cuentan como del proyecto igual que las que tienen .pgo.json.
	Registro string

	mu          sync.Mutex
	carpetas    map[int][]string
	hecho       time.Time
	buscando    bool
	registradas map[int][]string
}

// NuevoIndice crea un índice que busca bajo las raíces indicadas.
func NuevoIndice(raices []string) *Indice {
	return &Indice{Raices: raices, Vida: 2 * time.Minute}
}

// Carpetas devuelve las carpetas locales de cada proyecto (se vuelve a buscar si el índice es viejo o forzar).
// Con un índice viejo (y sin forzar) se devuelve el que hay y se rehace en segundo plano: recorrer el disco no
// bloquea las peticiones, que casi todas preguntan por las carpetas.
func (x *Indice) Carpetas(forzar bool) map[int][]string {
	x.mu.Lock()
	if forzar || x.carpetas == nil {
		x.mu.Unlock()
		m := busca(x.Raices)
		x.mu.Lock()
		x.carpetas, x.hecho = m, time.Now()
	} else if time.Since(x.hecho) > x.Vida && !x.buscando {
		x.buscando = true
		go func() {
			m := busca(x.Raices)
			x.mu.Lock()
			x.carpetas, x.hecho, x.buscando = m, time.Now(), false
			x.mu.Unlock()
		}()
	}
	c := x.carpetas
	x.mu.Unlock()
	return c
}

// Pertenece dice si la carpeta es una de las del proyecto (con .pgo.json o registrada por el usuario).
func (x *Indice) Pertenece(proyecto int, carpeta string) bool {
	for _, c := range x.Carpetas(false)[proyecto] {
		if c == carpeta {
			return true
		}
	}
	for _, c := range x.Registradas(proyecto) {
		if c == carpeta {
			return true
		}
	}
	return false
}

// Registradas devuelve las carpetas que el usuario ha registrado para el proyecto.
func (x *Indice) Registradas(proyecto int) []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cargaRegistro()
	return append([]string(nil), x.registradas[proyecto]...)
}

func (x *Indice) cargaRegistro() {
	if x.registradas != nil {
		return
	}
	x.registradas = map[int][]string{}
	if x.Registro != "" {
		if b, err := os.ReadFile(x.Registro); err == nil {
			json.Unmarshal(b, &x.registradas)
		}
	}
}

// Registra una carpeta para el proyecto: tiene que ser una carpeta que exista dentro de la carpeta personal.
func (x *Indice) Registra(proyecto int, carpeta string) error {
	casa, _ := os.UserHomeDir()
	c := filepath.Clean(carpeta)
	if !filepath.IsAbs(c) || c == casa || !strings.HasPrefix(c, casa+string(filepath.Separator)) {
		return errors.New("la carpeta tiene que estar dentro de " + casa)
	}
	if st, err := os.Stat(c); err != nil || !st.IsDir() {
		return errors.New("la carpeta no existe")
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cargaRegistro()
	for _, y := range x.registradas[proyecto] {
		if y == c {
			return nil
		}
	}
	x.registradas[proyecto] = append(x.registradas[proyecto], c)
	if x.Registro == "" {
		return nil
	}
	b, _ := json.MarshalIndent(x.registradas, "", " ")
	tmp := x.Registro + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, x.Registro)
}

func busca(raices []string) map[int][]string {
	m := map[int][]string{}
	for _, dir := range linkDirs(raices, LinkFile, legacyLinkFile) {
		if l, ok := ReadLink(dir); ok {
			m[l.ProjectID] = append(m[l.ProjectID], dir)
		}
	}
	for _, l := range m {
		sort.Strings(l)
	}
	return m
}

// Olvida una carpeta registrada para el proyecto (p. ej. una copia de trabajo que se ha quitado).
func (x *Indice) Olvida(proyecto int, carpeta string) error {
	c := filepath.Clean(carpeta)
	x.mu.Lock()
	defer x.mu.Unlock()
	x.cargaRegistro()
	l := x.registradas[proyecto][:0:0]
	for _, y := range x.registradas[proyecto] {
		if y != c {
			l = append(l, y)
		}
	}
	if len(l) == len(x.registradas[proyecto]) {
		return nil
	}
	x.registradas[proyecto] = l
	if x.Registro == "" {
		return nil
	}
	b, _ := json.MarshalIndent(x.registradas, "", " ")
	tmp := x.Registro + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, x.Registro)
}
