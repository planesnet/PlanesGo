// Package proyectos encuentra las carpetas locales de cada proyecto de Odoo.
//
// Una carpeta pertenece a un proyecto si tiene un .planesgo.json con su odoo_project_id (el mismo vínculo que usa
// PlanesGo para imputar horas). Un proyecto puede tener varias carpetas.
package proyectos

import (
	"encoding/json"
	"io/fs"
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

	mu       sync.Mutex
	carpetas map[int][]string
	hecho    time.Time
}

// NuevoIndice crea un índice que busca bajo las raíces indicadas.
func NuevoIndice(raices []string) *Indice {
	return &Indice{Raices: raices, Vida: 2 * time.Minute}
}

// Carpetas devuelve las carpetas locales de cada proyecto (se vuelve a buscar si el índice es viejo o forzar).
func (x *Indice) Carpetas(forzar bool) map[int][]string {
	x.mu.Lock()
	defer x.mu.Unlock()
	if forzar || x.carpetas == nil || time.Since(x.hecho) > x.Vida {
		x.carpetas = busca(x.Raices)
		x.hecho = time.Now()
	}
	return x.carpetas
}

// Pertenece dice si la carpeta es una de las del proyecto.
func (x *Indice) Pertenece(proyecto int, carpeta string) bool {
	for _, c := range x.Carpetas(false)[proyecto] {
		if c == carpeta {
			return true
		}
	}
	return false
}

func busca(raices []string) map[int][]string {
	m := map[int][]string{}
	for _, r := range raices {
		r = filepath.Clean(r)
		base := strings.Count(r, string(filepath.Separator))
		filepath.WalkDir(r, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if p != r && (saltar[d.Name()] || strings.Count(p, string(filepath.Separator))-base > Profundidad) {
					return fs.SkipDir
				}
				return nil
			}
			if d.Name() != ".planesgo.json" {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			var cfg struct {
				ID int `json:"odoo_project_id"`
			}
			if json.Unmarshal(b, &cfg) == nil && cfg.ID > 0 {
				m[cfg.ID] = append(m[cfg.ID], filepath.Dir(p))
			}
			return nil
		})
	}
	for _, l := range m {
		sort.Strings(l)
	}
	return m
}
