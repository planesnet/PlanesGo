package servidor

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"planesgo-code/internal/gworkspace"
	"planesgo-code/internal/sesiones"
)

// Google Workspace (Drive, Docs y Sheets), solo lectura, con la cuenta de Google que cada usuario de pgocode conecta
// («Conectar con Google»: OAuth de escritorio, internal/gworkspace). La interfaz ve el estado, conecta y desconecta;
// el agente busca, lista y lee con «pgo drive …» (con la cuenta del usuario que usa la interfaz).

// GET /api/google[?probar=1]: el estado del acceso del usuario (cuenta, ámbitos, si falta el cliente…).
func (s *Servidor) googleEstado(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	jsonOK(w, s.Google.Status(r.Context(), ses.Login, r.URL.Query().Get("probar") == "1"))
}

// POST /api/google/conectar: empieza la conexión; responde la URL de Google que abre la interfaz.
func (s *Servidor) googleConecta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	u, err := s.Google.StartAuth(r.Context(), ses.Login)
	if err != nil {
		jsonError(w, http.StatusConflict, err.Error())
		return
	}
	jsonOK(w, map[string]string{"url": u})
}

// POST /api/google/desconectar: revoca el permiso en Google y borra la conexión del usuario.
func (s *Servidor) googleDesconecta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	res := map[string]string{}
	if err := s.Google.Disconnect(r.Context(), ses.Login); err != nil {
		res["aviso"] = err.Error()
	}
	log.Printf("google: %s desconecta su cuenta", ses.Login)
	jsonOK(w, res)
}

// POST /api/google/cliente {contenido}: guarda el fichero del cliente OAuth (el JSON de la consola de Google). No
// se devuelve ni se registra nada de su contenido.
func (s *Servidor) googleCliente(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, "Google Workspace no está disponible")
		return
	}
	var p struct {
		Contenido string `json:"contenido"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	c, err := s.Google.SaveClientFile([]byte(strings.TrimSpace(p.Contenido)))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("google: %s carga el cliente OAuth (proyecto %q)", ses.Login, c.ProjectID)
	jsonOK(w, map[string]string{"proyecto": c.ProjectID})
}

// GET /api/google/{orden}: buscar, listar o leer (desde la interfaz, con la cuenta del usuario).
func (s *Servidor) googleDrive(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	s.driveOrden(w, r.WithContext(gworkspace.WithUser(r.Context(), ses.Login)), r.PathValue("orden"))
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
	case errors.Is(err, gworkspace.ErrNoAccess), errors.Is(err, gworkspace.ErrNoClient):
		return http.StatusServiceUnavailable
	case errors.Is(err, gworkspace.ErrNotFound):
		return http.StatusNotFound
	case errors.As(err, &ae):
		return http.StatusBadGateway
	}
	return http.StatusBadRequest
}
