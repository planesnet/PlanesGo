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
const pairPath = clients.PairPath

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
	me, _ := s.clientOf(r)
	l := []clients.Client{}
	if s.Clients != nil {
		l = s.Clients.List()
	}
	jsonOK(w, map[string]any{"clients": l, "me": me})
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
	if me, _ := s.clientOf(r); me == p.ID {
		jsonError(w, http.StatusBadRequest, "es esta misma app: revócala desde otra o borra sus datos")
		return
	}
	if err := s.Clients.Revoke(p.ID); err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("cliente revocado: %s (%s)", p.ID, ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
}

// POST /api/servidor/parar: the person stops the server (Propiedades → Servidor). It closes as on any shutdown: what
// is running stops and the periods close. The answer goes first (Shutdown waits for it).
func (s *Servidor) serverStop(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Stop == nil {
		jsonError(w, http.StatusConflict, "este servidor no se puede parar desde la app")
		return
	}
	log.Printf("%s para el servidor", ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
	go s.Stop()
}

// proofPath: where a client checks, before sending anything secret, that this is the server it paired with (or the
// one of this computer's user, to pair). Like pairPath, open to clients that are not paired yet.
const proofPath = "/api/clientes/prueba"

// GET /api/clientes/prueba?id=&nonce= (or ?emparejar=1&nonce=): the server's proof (internal/clients: Proof).
func (s *Servidor) clientsProof(w http.ResponseWriter, r *http.Request) {
	if s.Clients == nil {
		jsonError(w, http.StatusServiceUnavailable, "este servidor no admite clientes")
		return
	}
	q := r.URL.Query()
	var proof string
	var ok bool
	if q.Get("emparejar") != "" {
		proof, ok = s.Clients.SecretProof(q.Get("nonce"))
	} else {
		proof, ok = s.Clients.Proof(q.Get("id"), q.Get("nonce"))
	}
	if !ok {
		jsonError(w, http.StatusNotFound, "no hay ese cliente")
		return
	}
	jsonOK(w, map[string]string{"proof": proof})
}
