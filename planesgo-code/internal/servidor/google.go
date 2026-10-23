package servidor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"planesgo-code/internal/gbrowser"
)

// Google (Drive, Docs, Sheets, Slides y Gmail), solo lectura, a través del navegador integrado de pgocode: el agente
// pide con «pgo drive …» y «pgo gmail …»; el backend le pasa cada página a la app de escritorio (Electron), que la
// carga con la sesión de Google que el usuario haya abierto en el panel Web y devuelve su texto (internal/gbrowser).

// esElectron: la petición viene del proceso principal de la app de escritorio (con su secreto). Con -navegador
// tampoco vale otra cosa: el canal de Google es solo para Electron.
func (s *Servidor) esElectron(r *http.Request) bool {
	c, err := r.Cookie(cookieEscritorio)
	return s.TokenEscritorio != "" && err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.TokenEscritorio)) == 1
}

// GET /api/escritorio/google/trabajo: Electron recoge la siguiente página que cargar (espera hasta 25 s; 204 si no hay).
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

// maxResultadoGoogle: lo que puede ocupar un resultado (el texto ya viene recortado).
const maxResultadoGoogle = 16 << 20

// POST /api/escritorio/google/resultado: Electron entrega lo leído.
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

// driveOrden: buscar | listar | leer (pgo drive), con los parámetros de la URL. Responde el texto compacto (el que
// ve el agente) y, en búsquedas y listados, los ficheros.
func (s *Servidor) driveOrden(w http.ResponseWriter, r *http.Request, orden string) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, gbrowser.ErrNoDesktop.Error())
		return
	}
	q := r.URL.Query()
	proyecto, _ := strconv.Atoi(q.Get("proyecto"))
	max, _ := strconv.Atoi(q.Get("max"))
	var fs []gbrowser.File
	var err error
	switch orden {
	case "buscar":
		fs, err = s.Google.Search(r.Context(), proyecto, gbrowser.SearchParams{Text: q.Get("q"), Folder: q.Get("carpeta"), Type: q.Get("tipo"), Max: max})
	case "listar":
		fs, err = s.Google.List(r.Context(), proyecto, q.Get("carpeta"), max)
	case "leer":
		var c gbrowser.Content
		c, err = s.Google.Read(r.Context(), proyecto, gbrowser.ReadParams{Ref: q.Get("ref"), Sheet: q.Get("hoja"), Range: q.Get("rango"), Max: max})
		if err == nil {
			jsonOK(w, map[string]any{"texto": gbrowser.FormatContent(c), "recortado": c.Truncated})
			return
		}
	default:
		jsonError(w, http.StatusNotFound, "orden desconocida")
		return
	}
	if err != nil {
		s.googleFalla(w, proyecto, err)
		return
	}
	jsonOK(w, map[string]any{"texto": gbrowser.FormatFiles(fs), "ficheros": fs})
}

// gmailOrden: buscar (q) | listar (lo reciente de la bandeja de entrada) | leer (ref: un hilo), solo lectura. En el
// registro, solo la orden, cuántos correos y cuánto ha tardado: nunca la consulta, los identificadores ni el contenido.
func (s *Servidor) gmailOrden(w http.ResponseWriter, r *http.Request, orden string) {
	if s.Google == nil {
		jsonError(w, http.StatusServiceUnavailable, gbrowser.ErrNoDesktop.Error())
		return
	}
	q := r.URL.Query()
	proyecto, _ := strconv.Atoi(q.Get("proyecto"))
	max, _ := strconv.Atoi(q.Get("max"))
	t0 := time.Now()
	var res map[string]any
	var n int
	var err error
	switch orden {
	case "buscar", "listar":
		var l gbrowser.MailList
		if orden == "buscar" {
			l, err = s.Google.SearchMail(r.Context(), proyecto, q.Get("q"), max)
		} else {
			l, err = s.Google.RecentMail(r.Context(), proyecto, max)
		}
		n, res = len(l.Mails), map[string]any{"texto": gbrowser.FormatMails(l), "correos": l.Mails, "hay_mas": l.More}
	case "leer":
		var c gbrowser.MailContent
		c, err = s.Google.ReadMail(r.Context(), proyecto, q.Get("ref"), max)
		n, res = len(c.Messages), map[string]any{"texto": gbrowser.FormatMailContent(c), "recortado": c.Truncated}
	default:
		jsonError(w, http.StatusNotFound, "orden desconocida")
		return
	}
	if err != nil {
		log.Printf("gmail: %s falla tras %s", orden, time.Since(t0).Round(time.Millisecond))
		s.googleFalla(w, proyecto, err)
		return
	}
	log.Printf("gmail: %s · %d correos · %s", orden, n, time.Since(t0).Round(time.Millisecond))
	jsonOK(w, res)
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
	case errors.Is(err, gbrowser.ErrNoAccess):
		return http.StatusForbidden
	case errors.Is(err, gbrowser.ErrNotFound), errors.Is(err, gbrowser.ErrMailNotFound):
		return http.StatusNotFound
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	}
	return http.StatusBadRequest
}
