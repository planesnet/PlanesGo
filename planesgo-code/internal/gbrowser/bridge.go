// Package gbrowser: lectura de Google (Drive, Docs, Sheets, Slides y Gmail) A TRAVÉS del navegador integrado de
// pgocode, como la haría el usuario con «Archivo → Descargar»: la app de escritorio (Electron,
// electron/googleReader.js) DESCARGA el fichero por su enlace de descarga o exportación, con la misma sesión de
// navegador que el panel Web, y devuelve el fichero descargado; aquí se convierte en texto (Markdown, CSV, el correo
// .eml…). Nunca se carga ni se lee una página de Google (ni su texto ni su DOM): solo descargas. pgocode no lee, copia
// ni reutiliza cookies, tokens ni cabeceras.
//
// El backend no habla con Google: le pide cada descarga a Electron por un canal local (Bridge: Electron pregunta con
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

// Job: un enlace de descarga que Electron descarga.
type Job struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Project   int    `json:"project,omitempty"` // proyecto del agente (la sesión del navegador es la de su panel Web)
	TimeoutMs int    `json:"timeoutMs"`
}

// Result: lo que ha descargado Electron.
type Result struct {
	ID          string `json:"id"`
	URL         string `json:"url"`                   // la URL final (tras las redirecciones)
	Download    bool   `json:"download,omitempty"`    // ha habido descarga completa
	FileName    string `json:"fileName,omitempty"`    // el nombre que da Google al fichero
	ContentType string `json:"contentType,omitempty"` // el tipo de la respuesta
	Disposition string `json:"disposition,omitempty"` // su Content-Disposition (attachment en las descargas de verdad)
	Data        []byte `json:"data,omitempty"`        // el fichero (en JSON, base64)
	Login       bool   `json:"login,omitempty"`       // Google pide iniciar sesión (la descarga lleva a accounts.google.com)
	Error       string `json:"error,omitempty"`
}

// ErrNoDesktop: no hay app de escritorio que descargue (backend con -navegador, o Electron cerrado).
var ErrNoDesktop = errors.New("Google se lee a través del navegador integrado de la aplicación de escritorio de pgocode, " +
	"y no está abierta (en modo navegador, -navegador, no está disponible)")

// errBusy: demasiadas lecturas en cola.
var errBusy = errors.New("el navegador de pgocode tiene demasiadas descargas pendientes: espera un poco")

// MaxDownload: lo más grande que se descarga (Electron no pasa de ahí).
const MaxDownload = 20 << 20

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

// Do pide una descarga a Electron y espera su resultado.
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
