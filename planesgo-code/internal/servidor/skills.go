package servidor

import (
	"net/http"
	"os"
	"strconv"

	"planesgo-code/internal/sesiones"
	"planesgo-code/internal/skills"
)

// skillsCarpeta comprueba proyecto y carpeta (la carpeta puede faltar: solo las skills del usuario).
func (s *Servidor) skillsCarpeta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) (string, bool) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return "", false
	}
	c := r.URL.Query().Get("carpeta")
	if c != "" && !s.Indice.Pertenece(id, c) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return "", false
	}
	return c, true
}

// skillsInventario: las skills que ven los agentes en la carpeta del proyecto.
func (s *Servidor) skillsInventario(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	c, ok := s.skillsCarpeta(w, r, ses)
	if !ok {
		return
	}
	jsonOK(w, skills.Inventario(c))
}

// skillsTexto: el SKILL.md de una skill del inventario (solo esas rutas).
func (s *Servidor) skillsTexto(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	c, ok := s.skillsCarpeta(w, r, ses)
	if !ok {
		return
	}
	ruta := r.URL.Query().Get("ruta")
	for _, k := range skills.Inventario(c) {
		if k.Ruta == ruta && !k.Rota {
			b, err := os.ReadFile(ruta)
			if err != nil {
				jsonError(w, http.StatusNotFound, err.Error())
				return
			}
			jsonOK(w, map[string]string{"texto": string(b)})
			return
		}
	}
	jsonError(w, http.StatusNotFound, "esa skill no está en el inventario")
}
