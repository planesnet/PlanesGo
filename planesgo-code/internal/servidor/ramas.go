package servidor

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// repoProyectoDir: la carpeta del repositorio principal del proyecto (la primera con git: con .planesgo.json o
// registrada), de la que salen las copias de trabajo de cada rama.
func (s *Servidor) repoProyectoDir(r *http.Request, proyecto int) string {
	for _, c := range append(s.Indice.Carpetas(false)[proyecto], s.Indice.Registradas(proyecto)...) {
		if i := repo.Lee(r.Context(), c); i.Repositorio != "" {
			return c
		}
	}
	return ""
}

// proyectoRamas: ramas del repositorio, en qué carpeta está abierta cada una y el directorio principal sugerido.
func (s *Servidor) proyectoRamas(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	dir := s.repoProyectoDir(r, id)
	if dir == "" {
		jsonOK(w, map[string]any{"repo": "", "ramas": []string{}, "copias": map[string]string{}, "principal": ""})
		return
	}
	i := repo.Lee(r.Context(), dir)
	jsonOK(w, map[string]any{"repo": dir, "repositorio": i.Repositorio, "ramas": i.Ramas, "actual": i.Rama,
		"copias": repo.Copias(r.Context(), dir), "principal": filepath.Dir(dir), "tipo": i.Tipo})
}

// proyectoInfoCarpeta: el repositorio de una carpeta (sin registrarla), para el diálogo de la sesión.
func (s *Servidor) proyectoInfoCarpeta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	c := filepath.Clean(r.URL.Query().Get("carpeta"))
	if !filepath.IsAbs(c) {
		jsonError(w, http.StatusBadRequest, "la carpeta tiene que ser una ruta completa")
		return
	}
	i := repo.Lee(r.Context(), c)
	_, err := os.Stat(c)
	jsonOK(w, map[string]any{"git": i.Git, "repositorio": i.Repositorio, "remoto": i.Remoto, "rama": i.Rama, "ramas": i.Ramas, "tipo": i.Tipo, "existe": err == nil})
}

// proyectoCarpeta: registra para el proyecto una carpeta elegida con el diálogo del sistema.
func (s *Servidor) proyectoCarpeta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if err := s.Indice.Registra(p.Proyecto, p.Carpeta); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, repo.Lee(r.Context(), filepath.Clean(p.Carpeta)))
}

// proyectoPreparar: deja lista la carpeta de la rama de una sesión (la crea como copia de trabajo si no existe) y
// la registra para el proyecto.
func (s *Servidor) proyectoPreparar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Rama     string `json:"rama"`
		Carpeta  string `json:"carpeta"`
		Remoto   string `json:"remoto"` // carpeta sin repositorio: el que se clona en ella (SSH)
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	c := filepath.Clean(p.Carpeta)
	if !filepath.IsAbs(c) {
		jsonError(w, http.StatusBadRequest, "la carpeta tiene que ser una ruta completa")
		return
	}
	// La carpeta de la sesión manda: si ya es un repositorio, el suyo; si no, el que se indique (se clona en ella);
	// si no se indica ninguno, una copia de trabajo del repositorio del proyecto en esa rama.
	switch info := repo.Lee(r.Context(), c); {
	case info.Git:
		if p.Rama != "" && info.Rama != p.Rama {
			jsonError(w, http.StatusBadRequest, fmt.Sprintf("esa carpeta es un repositorio en la rama «%s»: elige esa rama o otra carpeta", info.Rama))
			return
		}
	case p.Remoto != "":
		if err := repo.Clona(r.Context(), strings.TrimSpace(p.Remoto), p.Rama, c); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	default:
		dir := s.repoProyectoDir(r, p.Proyecto)
		if dir == "" {
			jsonError(w, http.StatusBadRequest, "esa carpeta no tiene repositorio y el proyecto tampoco: indica el repositorio (SSH) para clonarlo en ella")
			return
		}
		if err := repo.PreparaRama(r.Context(), dir, p.Rama, c); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := s.Indice.Registra(p.Proyecto, c); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, repo.Lee(r.Context(), c))
}
