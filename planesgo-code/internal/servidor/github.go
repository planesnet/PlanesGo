package servidor

import (
	"log"
	"net/http"
	"strconv"

	"planesgo-code/internal/github"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

// repoGitHub comprueba proyecto y carpeta y devuelve «dueño/repo».
func (s *Servidor) repoGitHub(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, id int, carpeta string) (string, bool) {
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return "", false
	}
	if carpeta == "" || !s.Indice.Pertenece(id, carpeta) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return "", false
	}
	nombre := github.DeRemoto(repo.Lee(r.Context(), carpeta).Remoto)
	if nombre == "" {
		jsonError(w, http.StatusBadRequest, "el repositorio de esta carpeta no está en GitHub")
		return "", false
	}
	return nombre, true
}

// githubCrea: nueva issue en el repositorio de la carpeta.
func (s *Servidor) githubCrea(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		github.Nueva
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	nombre, ok := s.repoGitHub(w, r, ses, p.Proyecto, p.Carpeta)
	if !ok {
		return
	}
	n, u, err := s.GitHub.Crea(r.Context(), nombre, p.Nueva)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	log.Printf("issue creada por %s en %s: #%d", ses.Login, nombre, n)
	jsonOK(w, map[string]any{"numero": n, "url": u})
}

// githubEstado: issues y pull requests del repositorio de la carpeta (solo GitHub, de momento).
func (s *Servidor) githubEstado(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	carpeta := r.URL.Query().Get("carpeta")
	if carpeta == "" || !s.Indice.Pertenece(id, carpeta) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return
	}
	info := repo.Lee(r.Context(), carpeta)
	nombre := github.DeRemoto(info.Remoto)
	if nombre == "" {
		jsonError(w, http.StatusBadRequest, "el repositorio de esta carpeta no está en GitHub")
		return
	}
	e, err := s.GitHub.Lee(r.Context(), nombre, r.URL.Query().Get("recargar") == "1")
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, e)
}
