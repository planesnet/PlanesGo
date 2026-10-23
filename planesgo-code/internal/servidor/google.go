package servidor

import (
	"errors"
	"net/http"
	"strconv"

	"planesgo-code/internal/gworkspace"
	"planesgo-code/internal/sesiones"
)

// Google Workspace (Drive, Docs y Sheets), solo lectura, con las credenciales de gcloud del usuario del equipo
// (internal/gworkspace). La interfaz ve el estado; el agente busca, lista y lee con «pgo drive …».

// GET /api/google[?probar=1]: el estado del acceso (cuenta, ámbitos, qué ejecutar si falta).
func (s *Servidor) googleEstado(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	jsonOK(w, s.Google.Status(r.Context(), r.URL.Query().Get("probar") == "1"))
}

// GET /api/google/{orden}: buscar, listar o leer (desde la interfaz).
func (s *Servidor) googleDrive(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	s.driveOrden(w, r, r.PathValue("orden"))
}

// driveOrden: buscar | listar | leer, con los parámetros de la URL. Responde el texto compacto (el que ve el
// agente) y, en búsquedas y listados, los ficheros.
func (s *Servidor) driveOrden(w http.ResponseWriter, r *http.Request, orden string) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	q := r.URL.Query()
	max, _ := strconv.Atoi(q.Get("max"))
	var fs []gworkspace.File
	var err error
	switch orden {
	case "buscar":
		fs, err = s.Google.Search(r.Context(), gworkspace.SearchParams{Text: q.Get("q"), Folder: q.Get("carpeta"), Type: q.Get("tipo"), Max: max})
	case "listar":
		fs, err = s.Google.List(r.Context(), q.Get("carpeta"), max)
	case "leer":
		var c gworkspace.Content
		c, err = s.Google.Read(r.Context(), gworkspace.ReadParams{Ref: q.Get("ref"), Sheet: q.Get("hoja"), Range: q.Get("rango"), Max: max})
		if err == nil {
			jsonOK(w, map[string]any{"texto": gworkspace.FormatContent(c), "recortado": c.Truncated})
			return
		}
	default:
		jsonError(w, http.StatusNotFound, "orden desconocida")
		return
	}
	if err != nil {
		jsonError(w, driveStatus(err), err.Error())
		return
	}
	jsonOK(w, map[string]any{"texto": gworkspace.FormatFiles(fs), "ficheros": fs})
}

// driveStatus: el código HTTP de cada error.
func driveStatus(err error) int {
	var ae *gworkspace.APIError
	switch {
	case errors.Is(err, gworkspace.ErrNoAccess), errors.Is(err, gworkspace.ErrNoGcloud):
		return http.StatusServiceUnavailable
	case errors.Is(err, gworkspace.ErrNotFound):
		return http.StatusNotFound
	case errors.As(err, &ae):
		return http.StatusBadGateway
	}
	return http.StatusBadRequest
}
