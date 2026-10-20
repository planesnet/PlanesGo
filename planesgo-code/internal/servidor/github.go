package servidor

import (
	"net/http"
	"strconv"

	"planesgo-code/internal/github"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

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
