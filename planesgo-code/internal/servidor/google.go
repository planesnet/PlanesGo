package servidor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"planesgo-code/internal/gbrowser"
	"planesgo-code/internal/procesos"
)

// Google (Drive, Docs, Sheets, Slides y Gmail), solo lectura y solo por enlace, a través del navegador integrado de
// pgocode: el agente pide con «pgo drive leer» y «pgo gmail leer»; el backend le pasa a la app de escritorio
// (Electron) el enlace de DESCARGA del fichero (la exportación de Docs/Sheets/Slides, la descarga de Drive, el
// mensaje original de Gmail), que lo descarga con la sesión de Google que el usuario haya abierto en el panel Web; el
// fichero se lee aquí (internal/gbrowser). Nunca se lee una página de Google.

// esElectron: la petición viene del proceso principal de la app de escritorio (con su secreto). Con -navegador
// tampoco vale otra cosa: el canal de Google es solo para Electron.
func (s *Servidor) esElectron(r *http.Request) bool {
	c, err := r.Cookie(cookieEscritorio)
	return s.TokenEscritorio != "" && err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.TokenEscritorio)) == 1
}

// GET /api/escritorio/google/trabajo: Electron recoge la siguiente descarga (espera hasta 25 s; 204 si no hay).
func (s *Servidor) googleTrabajo(w http.ResponseWriter, r *http.Request) {
	if !s.esElectron(r) || s.Google == nil {
		jsonError(w, http.StatusForbidden, "solo para la app de escritorio")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	j, ok := s.Google.Bridge.Next(ctx)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	jsonOK(w, j)
}

// maxResultadoGoogle: lo que puede ocupar un resultado (el fichero descargado, en base64, y poco más).
const maxResultadoGoogle = gbrowser.MaxDownload*4/3 + 1<<20

// POST /api/escritorio/google/resultado: Electron entrega lo descargado.
func (s *Servidor) googleResultado(w http.ResponseWriter, r *http.Request) {
	if !s.esElectron(r) || s.Google == nil {
		jsonError(w, http.StatusForbidden, "solo para la app de escritorio")
		return
	}
	var res gbrowser.Result
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxResultadoGoogle)).Decode(&res); err != nil {
		jsonError(w, http.StatusBadRequest, "resultado no válido")
		return
	}
	jsonOK(w, map[string]bool{"esperado": s.Google.Bridge.Deliver(res)})
}

// driveOrden: leer (pgo drive leer <enlace o id>), con los parámetros de la URL. Responde el texto compacto (el que
// ve el agente). Lo que no es texto (un PDF, una imagen…) se guarda en .pgocode/salidas de la carpeta del agente
// (si es del proyecto) y se le da la ruta.
func (s *Servidor) driveOrden(w http.ResponseWriter, r *http.Request, orden string) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, gbrowser.ErrNoDesktop.Error())
		return
	}
	if orden != "leer" {
		jsonError(w, http.StatusNotFound, "orden desconocida")
		return
	}
	q := r.URL.Query()
	proyecto, _ := strconv.Atoi(q.Get("proyecto"))
	max, _ := strconv.Atoi(q.Get("max"))
	p := gbrowser.ReadParams{Ref: q.Get("ref"), Sheet: q.Get("hoja"), Range: q.Get("rango"), Max: max}
	if dir := q.Get("carpeta"); filepath.IsAbs(dir) && s.dentroDelProyecto(proyecto, dir) {
		p.SaveDir = procesos.DirRegistros(dir)
	}
	c, err := s.Google.Read(r.Context(), proyecto, p)
	if err != nil {
		s.googleFalla(w, proyecto, err)
		return
	}
	if c.Path != "" {
		excluyeDeGit(q.Get("carpeta"))
	}
	jsonOK(w, map[string]any{"texto": gbrowser.FormatContent(c), "recortado": c.Truncated})
}

// gmailOrden: leer (ref: el enlace de un correo), solo lectura. En el registro, solo la orden y cuánto ha tardado:
// nunca el enlace ni el contenido.
func (s *Servidor) gmailOrden(w http.ResponseWriter, r *http.Request, orden string) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, gbrowser.ErrNoDesktop.Error())
		return
	}
	if orden != "leer" {
		jsonError(w, http.StatusNotFound, "orden desconocida")
		return
	}
	q := r.URL.Query()
	proyecto, _ := strconv.Atoi(q.Get("proyecto"))
	max, _ := strconv.Atoi(q.Get("max"))
	t0 := time.Now()
	c, err := s.Google.ReadMail(r.Context(), proyecto, q.Get("ref"), max)
	if err != nil {
		log.Printf("gmail: leer falla tras %s", time.Since(t0).Round(time.Millisecond))
		s.googleFalla(w, proyecto, err)
		return
	}
	log.Printf("gmail: leer · %s", time.Since(t0).Round(time.Millisecond))
	jsonOK(w, map[string]any{"texto": gbrowser.FormatMailContent(c), "recortado": c.Truncated})
}

// googleFalla responde el error; si falta la sesión de Google, abre accounts.google.com en el panel Web del
// proyecto (como mucho cada 2 minutos) para que el usuario la inicie.
func (s *Servidor) googleFalla(w http.ResponseWriter, proyecto int, err error) {
	if errors.Is(err, gbrowser.ErrLogin) && loginGoogle.toca(proyecto) {
		s.Eventos.Emite(map[string]any{"tipo": "abrir", "url": "https://accounts.google.com/", "proyecto": proyecto})
	}
	jsonError(w, googleStatus(err), err.Error())
}

// loginGoogle: cuándo se abrió la página de inicio de sesión de Google en cada proyecto.
var loginGoogle = &avisoLogin{ultimo: map[int]time.Time{}}

type avisoLogin struct {
	mu     sync.Mutex
	ultimo map[int]time.Time
}

func (a *avisoLogin) toca(proyecto int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if time.Since(a.ultimo[proyecto]) < 2*time.Minute {
		return false
	}
	a.ultimo[proyecto] = time.Now()
	return true
}

// googleStatus: el código HTTP de cada error (Drive y Gmail).
func googleStatus(err error) int {
	switch {
	case errors.Is(err, gbrowser.ErrNoDesktop), errors.Is(err, gbrowser.ErrLogin):
		return http.StatusServiceUnavailable
	case errors.Is(err, gbrowser.ErrNoDownload):
		return http.StatusBadGateway
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	}
	return http.StatusBadRequest
}
