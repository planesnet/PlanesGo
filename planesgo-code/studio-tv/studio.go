// Package studiotv es el estudio de TV de PSFCode: retransmite en directo lo que hacen los navegadores de las
// pruebas automáticas (QA de Odoo en backoffice y en móvil, o cualquier otra prueba del pipeline).
//
// Cada emisor envía fotogramas JPEG y el estado del caso a un canal; el estudio los reparte a los espectadores
// como MJPEG (vídeo en directo en un <img>) y eventos SSE. Hay varios canales a la vez (uno por sesión o agente)
// y una parrilla con todos. Emitir y ver usan tokens distintos.
package studiotv

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"sync"
	"time"
)

//go:embed web/estudio.html
var pagina []byte

const (
	// CanalPorDefecto recibe lo que se emite sin indicar canal: así siguen valiendo los emisores anteriores.
	CanalPorDefecto = "principal"

	maxFrame   = 8 << 20
	maxEstado  = 1 << 20
	maxCanales = 64
	// enDirecto: un canal se da por en directo si ha recibido algo en este tiempo.
	enDirecto = 15 * time.Second
	// caducidad: un canal sin espectadores ni señal durante este tiempo se borra.
	caducidad = time.Hour
)

var (
	nombreValido    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	errDemasiados   = errors.New("demasiados canales abiertos")
	errNoEncontrado = errors.New("canal sin señal")
)

type canal struct {
	frame   []byte
	estado  []byte
	ultima  time.Time
	frames  map[chan []byte]struct{}
	estados map[chan []byte]struct{}
}

// Studio guarda los canales y atiende a emisores y espectadores. Se crea con Nuevo y se sirve con Handler.
type Studio struct {
	emitTok, viewTok string
	ahora            func() time.Time

	mu      sync.Mutex
	canales map[string]*canal
}

// Nuevo crea un estudio con el token de emitir y el de ver (deben ser distintos y largos).
func Nuevo(emitTok, viewTok string) *Studio {
	return &Studio{emitTok: emitTok, viewTok: viewTok, ahora: time.Now, canales: map[string]*canal{}}
}

// emitir entrega a cada suscriptor el último valor; si va lento, se le descarta el anterior (nunca se bloquea).
func emitir(subs map[chan []byte]struct{}, b []byte) {
	for c := range subs {
		select {
		case c <- b:
		default:
			select {
			case <-c:
			default:
			}
			select {
			case c <- b:
			default:
			}
		}
	}
}

// canalLocked devuelve el canal, creándolo si no existe. Se llama con s.mu tomado.
func (s *Studio) canalLocked(nombre string) (*canal, error) {
	if c, ok := s.canales[nombre]; ok {
		return c, nil
	}
	s.purgaLocked()
	if len(s.canales) >= maxCanales {
		return nil, errDemasiados
	}
	c := &canal{estado: []byte(`{}`), ultima: s.ahora(), frames: map[chan []byte]struct{}{}, estados: map[chan []byte]struct{}{}}
	s.canales[nombre] = c
	return c, nil
}

// purgaLocked borra los canales caducados. Se llama con s.mu tomado.
func (s *Studio) purgaLocked() {
	for n, c := range s.canales {
		if len(c.frames)+len(c.estados) == 0 && s.ahora().Sub(c.ultima) > caducidad {
			delete(s.canales, n)
		}
	}
}

// publica guarda y reparte un fotograma o un estado (el que no sea nil) en el canal.
func (s *Studio) publica(nombre string, frame, estado []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.canalLocked(nombre)
	if err != nil {
		return err
	}
	c.ultima = s.ahora()
	if frame != nil {
		c.frame = frame
		emitir(c.frames, frame)
	}
	if estado != nil {
		c.estado = estado
		emitir(c.estados, estado)
	}
	return nil
}

// suscribe da de alta a un espectador (vídeo o estados) y le entrega el último valor. Devuelve la función de baja.
func (s *Studio) suscribe(nombre string, video bool) (chan []byte, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := s.canalLocked(nombre)
	if err != nil {
		return nil, nil, err
	}
	m, ult := c.estados, c.estado
	if video {
		m, ult = c.frames, c.frame
	}
	ch := make(chan []byte, 1)
	if ult != nil {
		ch <- ult
	}
	m[ch] = struct{}{}
	return ch, func() {
		s.mu.Lock()
		delete(m, ch)
		s.mu.Unlock()
	}, nil
}

// ultimoFrame devuelve el último fotograma del canal sin crearlo.
func (s *Studio) ultimoFrame(nombre string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.canales[nombre]; ok && c.frame != nil {
		return c.frame, nil
	}
	return nil, errNoEncontrado
}

// InfoCanal es una fila de la parrilla de canales.
type InfoCanal struct {
	Canal        string          `json:"canal"`
	EnDirecto    bool            `json:"en_directo"`
	Ultima       time.Time       `json:"ultima"`
	Espectadores int             `json:"espectadores"`
	ConImagen    bool            `json:"con_imagen"`
	Estado       json.RawMessage `json:"estado"`
}

// Canales lista los canales abiertos, ordenados por nombre.
func (s *Studio) Canales() []InfoCanal {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgaLocked()
	l := make([]InfoCanal, 0, len(s.canales))
	for n, c := range s.canales {
		l = append(l, InfoCanal{Canal: n, EnDirecto: s.ahora().Sub(c.ultima) < enDirecto, Ultima: c.ultima,
			Espectadores: len(c.frames), ConImagen: c.frame != nil, Estado: c.estado})
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Canal < l[j].Canal })
	return l
}

// nombreCanal lee el canal de ?canal= o de la cabecera X-Canal; sin canal, el de por defecto.
func nombreCanal(r *http.Request) (string, bool) {
	n := r.URL.Query().Get("canal")
	if n == "" {
		n = r.Header.Get("X-Canal")
	}
	if n == "" {
		return CanalPorDefecto, true
	}
	return n, nombreValido.MatchString(n)
}

func sinCache(w http.ResponseWriter, tipo string) {
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func errorCanal(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errDemasiados):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, errNoEncontrado):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// protege comprueba el token y el nombre del canal antes de llamar a f.
func (s *Studio) protege(token string, f func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !autorizado(r, token) {
			http.Error(w, "Studio TV: token no válido o ausente", http.StatusUnauthorized)
			return
		}
		n, ok := nombreCanal(r)
		if !ok {
			http.Error(w, "nombre de canal no válido: letras, números, punto, guion o guion bajo, hasta 64", http.StatusBadRequest)
			return
		}
		f(w, r, n)
	}
}

// Handler devuelve el manejador HTTP del estudio.
//
//	POST /api/frame    JPEG del emisor                      (token de emitir)
//	POST /api/estado   JSON {caso,titulo,paso,estado,seg,marcadores[{t,txt}]}
//	GET  /api/canales  lista de canales en JSON              (token de ver)
//	GET  /snapshot     último fotograma del canal (JPEG)
//	GET  /stream       vídeo MJPEG del canal
//	GET  /events       estados del canal por SSE
//	GET  /             parrilla de canales, o un canal con ?canal=
//	GET  /healthz      sin token, para comprobar que está vivo
//
// El canal se indica con ?canal= o con la cabecera X-Canal; sin él se usa «principal». Los tokens van en
// Authorization: Bearer o en ?t=.
func (s *Studio) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		sinCache(w, "text/plain; charset=utf-8")
		io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("POST /api/frame", s.protege(s.emitTok, func(w http.ResponseWriter, r *http.Request, n string) {
		b, err := io.ReadAll(io.LimitReader(r.Body, maxFrame))
		if err != nil || len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
			http.Error(w, "se espera un JPEG", http.StatusBadRequest)
			return
		}
		if err := s.publica(n, b, nil); err != nil {
			errorCanal(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("POST /api/estado", s.protege(s.emitTok, func(w http.ResponseWriter, r *http.Request, n string) {
		b, err := io.ReadAll(io.LimitReader(r.Body, maxEstado))
		if err != nil || !json.Valid(b) {
			http.Error(w, "se espera JSON", http.StatusBadRequest)
			return
		}
		if err := s.publica(n, nil, b); err != nil {
			errorCanal(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("GET /api/canales", s.protege(s.viewTok, func(w http.ResponseWriter, r *http.Request, _ string) {
		sinCache(w, "application/json")
		json.NewEncoder(w).Encode(s.Canales())
	}))
	mux.HandleFunc("GET /snapshot", s.protege(s.viewTok, func(w http.ResponseWriter, r *http.Request, n string) {
		b, err := s.ultimoFrame(n)
		if err != nil {
			errorCanal(w, err)
			return
		}
		sinCache(w, "image/jpeg")
		w.Write(b)
	}))
	mux.HandleFunc("GET /stream", s.protege(s.viewTok, func(w http.ResponseWriter, r *http.Request, n string) {
		c, baja, err := s.suscribe(n, true)
		if err != nil {
			errorCanal(w, err)
			return
		}
		defer baja()
		// Los directos no tienen fin: sin límite de escritura aunque el servidor lo tenga (WriteTimeout).
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		fl, _ := w.(http.Flusher)
		sinCache(w, "multipart/x-mixed-replace; boundary=pco")
		if fl != nil {
			fl.Flush() // cabeceras ya, aunque el canal aún no tenga señal
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case b := <-c:
				if _, err := fmt.Fprintf(w, "--pco\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(b)); err != nil {
					return
				}
				if _, err := w.Write(b); err != nil {
					return
				}
				if _, err := io.WriteString(w, "\r\n"); err != nil {
					return
				}
				if fl != nil {
					fl.Flush()
				}
			}
		}
	}))
	mux.HandleFunc("GET /events", s.protege(s.viewTok, func(w http.ResponseWriter, r *http.Request, n string) {
		c, baja, err := s.suscribe(n, false)
		if err != nil {
			errorCanal(w, err)
			return
		}
		defer baja()
		// Los directos no tienen fin: sin límite de escritura aunque el servidor lo tenga (WriteTimeout).
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		fl, _ := w.(http.Flusher)
		sinCache(w, "text/event-stream")
		if fl != nil {
			fl.Flush() // cabeceras ya, aunque el canal aún no tenga señal
		}
		latido := time.NewTicker(15 * time.Second)
		defer latido.Stop()
		for {
			var err error
			select {
			case <-r.Context().Done():
				return
			case <-latido.C:
				_, err = io.WriteString(w, ": latido\n\n")
			case b := <-c:
				_, err = fmt.Fprintf(w, "data: %s\n\n", b)
			}
			if err != nil {
				return
			}
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	mux.HandleFunc("GET /{$}", s.protege(s.viewTok, func(w http.ResponseWriter, r *http.Request, _ string) {
		sinCache(w, "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Write(pagina)
	}))
	return mux
}
