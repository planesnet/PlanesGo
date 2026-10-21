package servidor

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/sesiones"
)

// Mensajes a mitad de turno: con un agente que los admite (Claude Code), lo que el usuario manda mientras trabaja
// le llega ya (lo lee antes de acabar el turno), sin esperar a la cola.

// inyector: cómo pasar un mensaje al turno en curso de una sesión de pgocode.
type inyector struct {
	uid, sesionImp, turno string
	enviar                func(string) error
}

var (
	muInyectores sync.Mutex
	inyectores   = map[string]*inyector{} // sesión de pgocode → su turno en curso
)

// alInyectar: lo que recibe el agente para apuntar (y quitar) su turno como destino de mensajes.
func (s *Servidor) alInyectar(ses *sesiones.Sesion, sesion, sesionImp, turno string) func(func(string) error) {
	if sesion == "" {
		return nil
	}
	uid := ses.Login
	return func(f func(string) error) {
		muInyectores.Lock()
		defer muInyectores.Unlock()
		if f == nil {
			if x := inyectores[sesion]; x != nil && x.turno == turno {
				delete(inyectores, sesion)
			}
			return
		}
		inyectores[sesion] = &inyector{uid: uid, sesionImp: sesionImp, turno: turno, enviar: f}
	}
}

// inyecta: POST /api/chat/inyecta {sesion, texto}. 409 si la sesión no tiene un turno que lo admita (o acaba de
// terminar): la interfaz lo deja entonces en la cola.
func (s *Servidor) inyecta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Sesion string `json:"sesion"`
		Texto  string `json:"texto"`
	}
	if err := lee(r, &p); err != nil || strings.TrimSpace(p.Texto) == "" {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	muInyectores.Lock()
	x := inyectores[p.Sesion]
	muInyectores.Unlock()
	if x == nil || x.uid != ses.Login {
		jsonError(w, http.StatusConflict, "el agente no está en un turno que admita mensajes")
		return
	}
	if err := x.enviar(p.Texto); err != nil {
		if errors.Is(err, llm.ErrEntradaCerrada) {
			jsonError(w, http.StatusConflict, err.Error())
		} else {
			jsonError(w, http.StatusConflict, "no se pudo pasar el mensaje al agente: "+err.Error())
		}
		return
	}
	if s.Imputacion != nil {
		s.Imputacion.AnotaAgente(x.sesionImp, "inyecta", map[string]any{"turno": x.turno, "caracteres": len(p.Texto)})
	}
	jsonOK(w, map[string]bool{"ok": true})
}
