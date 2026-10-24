package servidor

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"

	"planesgo-code/internal/clients"
	"planesgo-code/internal/sesiones"
)

// The client apps of this server (internal/clients): a client pairs once with the pairing secret (a file only the
// server's system user can read) and then brings its own token in the cookieEscritorio cookie. The person sees and
// revokes them in the application properties.

// clientOf: the client a request comes from (the id of its token), and whether it is one.
func (s *Servidor) clientOf(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieEscritorio)
	if err != nil || c.Value == "" {
		return "", false
	}
	// The secret of a desktop app that started its own server (PGO_TOKEN_ESCRITORIO): still accepted.
	if s.TokenEscritorio != "" && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.TokenEscritorio)) == 1 {
		return "local", true
	}
	if s.Clients == nil {
		return "", false
	}
	return s.Clients.Check(c.Value)
}

// pairPath: the only route a client that is not paired yet can call.
const pairPath = "/api/clientes/emparejar"

// POST /api/clientes/emparejar {secret, name}: a new client, if it knows the pairing secret. The token goes back once.
func (s *Servidor) clientsPair(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Secret string `json:"secret"`
		Name   string `json:"name"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if s.Clients == nil {
		jsonError(w, http.StatusServiceUnavailable, "este servidor no admite clientes")
		return
	}
	id, token, err := s.Clients.Pair(p.Secret, p.Name)
	if err != nil {
		jsonError(w, http.StatusForbidden, err.Error())
		return
	}
	log.Printf("cliente emparejado: %s (%s)", id, strings.TrimSpace(p.Name))
	jsonOK(w, map[string]string{"id": id, "token": token})
}

// GET /api/clientes/yo: whether the request's client is paired (the desktop app checks its token with it).
func (s *Servidor) clientsMe(w http.ResponseWriter, r *http.Request) {
	id, ok := s.clientOf(r)
	if !ok {
		jsonError(w, http.StatusUnauthorized, "cliente no emparejado")
		return
	}
	jsonOK(w, map[string]string{"id": id})
}

// GET /api/clientes: the paired clients.
func (s *Servidor) clientsList(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	if s.Clients == nil {
		jsonOK(w, []clients.Client{})
		return
	}
	me, _ := s.clientOf(r)
	jsonOK(w, map[string]any{"clients": s.Clients.List(), "me": me})
}

// POST /api/clientes/revocar {id}: a client stops having access at once.
func (s *Servidor) clientsRevoke(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		ID string `json:"id"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if s.Clients == nil {
		jsonError(w, http.StatusNotFound, "no hay ese cliente")
		return
	}
	if err := s.Clients.Revoke(p.ID); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("cliente revocado: %s (%s)", p.ID, ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
}
