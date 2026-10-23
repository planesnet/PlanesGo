// Package gbrowser: lectura de Google (Drive, Docs, Sheets, Slides y Gmail) A TRAVÉS del navegador integrado de
// pgocode, como la haría el usuario: la app de escritorio (Electron, electron/googleReader.js) carga la página en
// una ventana oculta con la misma sesión de navegador que el panel Web y devuelve el texto de la página (o el del
// fichero que Google descarga al exportar). pgocode no lee, copia ni reutiliza cookies, tokens ni cabeceras: solo
// navega y lee lo que la página enseña, en solo lectura (nunca hace clic ni escribe en las páginas de Google).
//
// El backend no habla con Google: le pide cada carga a Electron por un canal local (Bridge: Electron pregunta con
// su secreto de escritorio si hay trabajo y entrega el resultado).
package gbrowser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// Job: una página que Electron carga y lee.
type Job struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Mode      string `json:"mode"`              // text | resolve | sheets | drive | gmail-list | gmail-print
	Project   int    `json:"project,omitempty"` // proyecto del agente (la sesión del navegador es la de su panel Web)
	MaxChars  int    `json:"maxChars"`          // texto como mucho (Electron recorta)
	TimeoutMs int    `json:"timeoutMs"`
}

// Modos de lectura (los guiones de cada uno están en electron/googleReader.js).
const (
	ModeText       = "text"        // el texto de la página (o del fichero descargado)
	ModeResolve    = "resolve"     // solo a dónde lleva la URL (qué tipo de fichero es)
	ModeSheets     = "sheets"      // las hojas de una hoja de cálculo (pestañas de htmlview)
	ModeDrive      = "drive"       // las filas de una búsqueda o carpeta de Drive
	ModeGmailList  = "gmail-list"  // las filas de la bandeja o de una búsqueda de Gmail
	ModeGmailPrint = "gmail-print" // los mensajes de un hilo (vista de impresión)
)

// Item: una fila leída de la página (un fichero de Drive, un correo, una hoja).
type Item struct {
	ID      string   `json:"id"`
	Name    string   `json:"name,omitempty"`
	Lines   []string `json:"lines,omitempty"`
	Labels  []string `json:"labels,omitempty"`
	Href    string   `json:"href,omitempty"`
	From    string   `json:"from,omitempty"`
	Date    string   `json:"date,omitempty"`
	Snippet string   `json:"snippet,omitempty"`
	Unread  bool     `json:"unread,omitempty"`
	Text    string   `json:"text,omitempty"`
}

// Result: lo que ha leído Electron.
type Result struct {
	ID          string `json:"id"`
	URL         string `json:"url"`    // la URL final (tras las redirecciones)
	Status      int    `json:"status"` // el código HTTP de la página
	Title       string `json:"title,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Download    bool   `json:"download,omitempty"` // el texto es el del fichero que ha descargado la página
	FileName    string `json:"fileName,omitempty"`
	Text        string `json:"text,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Total       int    `json:"total,omitempty"` // caracteres que había (si se conoce) cuando se recorta
	Items       []Item `json:"items,omitempty"`
	Login       bool   `json:"login,omitempty"` // Google pide iniciar sesión
	Error       string `json:"error,omitempty"`
}

// ErrNoDesktop: no hay app de escritorio que cargue las páginas (backend con -navegador, o Electron cerrado).
var ErrNoDesktop = errors.New("Google se lee a través del navegador integrado de la aplicación de escritorio de pgocode, " +
	"y no está abierta (en modo navegador, -navegador, no está disponible)")

// errBusy: demasiadas lecturas en cola.
var errBusy = errors.New("el navegador de pgocode tiene demasiadas lecturas pendientes: espera un poco")

const (
	queueSize      = 16
	defaultTimeout = 45 * time.Second
	// presence: cuánto vale la última consulta de Electron para dar por abierta la app de escritorio.
	presence = 60 * time.Second
)

type pending struct {
	job  Job
	done chan Result
}

// Bridge: el canal entre el backend y Electron. El backend encola (Do) y Electron recoge (Next) y entrega (Deliver).
type Bridge struct {
	mu       sync.Mutex
	jobs     chan *pending
	waiting  map[string]*pending
	polling  int
	lastPoll time.Time
	now      func() time.Time
}

// NewBridge: un canal vacío.
func NewBridge() *Bridge {
	return &Bridge{jobs: make(chan *pending, queueSize), waiting: map[string]*pending{}, now: time.Now}
}

// Available: Electron está recogiendo trabajo (ahora o hace poco).
func (b *Bridge) Available() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.polling > 0 || (!b.lastPoll.IsZero() && b.now().Sub(b.lastPoll) < presence)
}

// Do pide una carga a Electron y espera su resultado.
func (b *Bridge) Do(ctx context.Context, j Job) (Result, error) {
	if !b.Available() {
		return Result{}, ErrNoDesktop
	}
	if j.TimeoutMs <= 0 {
		j.TimeoutMs = int(defaultTimeout / time.Millisecond)
	}
	j.ID = newID()
	p := &pending{job: j, done: make(chan Result, 1)}
	b.mu.Lock()
	b.waiting[j.ID] = p
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		delete(b.waiting, j.ID)
		b.mu.Unlock()
	}()
	select {
	case b.jobs <- p:
	default:
		return Result{}, errBusy
	}
	// Lo que tarda Electron más un margen (la cola, abrir la ventana).
	t := time.NewTimer(time.Duration(j.TimeoutMs)*time.Millisecond*2 + 15*time.Second)
	defer t.Stop()
	select {
	case r := <-p.done:
		return r, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-t.C:
		return Result{}, errors.New("el navegador de pgocode no ha respondido a tiempo")
	}
}

// Next: el siguiente trabajo para Electron (espera hasta que lo haya o se acabe ctx). Salta los que ya nadie espera.
func (b *Bridge) Next(ctx context.Context) (Job, bool) {
	b.mu.Lock()
	b.polling++
	b.lastPoll = b.now()
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.polling--
		b.lastPoll = b.now()
		b.mu.Unlock()
	}()
	for {
		select {
		case p := <-b.jobs:
			b.mu.Lock()
			_, ok := b.waiting[p.job.ID]
			b.mu.Unlock()
			if ok {
				return p.job, true
			}
		case <-ctx.Done():
			return Job{}, false
		}
	}
}

// Deliver: el resultado de un trabajo (false si ya nadie lo espera).
func (b *Bridge) Deliver(r Result) bool {
	b.mu.Lock()
	p, ok := b.waiting[r.ID]
	b.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case p.done <- r:
		return true
	default:
		return false
	}
}

func newID() string {
	x := make([]byte, 12)
	rand.Read(x)
	return hex.EncodeToString(x)
}
