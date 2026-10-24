package servidor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/desktop"
	"planesgo-code/internal/freerdp"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/sesiones"
)

// Remote sessions (RDP and AnyDesk): the viewers run in the client (its desktop helper, internal/desktop), and the
// server only keeps the accounting. The helper reports each session it opens (POST /api/remote/sessions), a beat
// every 20 s with whether someone is at the keyboard (…/beat) and its end (…/end). The time goes to the project with
// the imputation service while the session is open and in use, as with the rest of pgocode. Passwords never reach
// the server.
//
// Who the time belongs to: the helper only carries its client's token, not a user session. The user is the one with
// a pgocode session open in that same client: the request's own session cookie if it has one, otherwise the user
// session last seen from that client (every request of its window brings both cookies: noteClientUser). A client is
// one desktop app with one cookie jar, so it has one user at a time; logged out, a session with a project is refused
// and an incoming AnyDesk one is registered without a project.

const (
	remoteBeat      = 20 * time.Second // how often the client reports activity
	remoteFirstBeat = 10 * time.Second // connected this long before counting (a failed login logs nothing)
	remoteSilence   = 60 * time.Second // without beats this long, the client is gone: the period is closed
	remoteKeepEnded = 10 * time.Minute // ended sessions stay listed this long (the interface shows their error)
	remoteMaxText   = 200              // names, hosts and users
	remoteMaxError  = 2000
)

// Kinds and states of a remote session.
const (
	remoteRDP     = "rdp"
	remoteAnyDesk = "anydesk"

	remoteConnecting = "connecting"
	remoteConnected  = "connected"
	remoteEnded      = "ended"
)

// remoteSession: a remote session a client reports. Nothing secret: it goes to the interface as is.
type remoteSession struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`      // rdp, anydesk
	Direction   string    `json:"direction"` // outgoing, incoming
	Project     int       `json:"project"`   // 0: without a project (AnyDesk), not logged until it gets one
	ProjectName string    `json:"project_name,omitempty"`
	Name        string    `json:"name,omitempty"`
	Host        string    `json:"host,omitempty"` // RDP
	User        string    `json:"user,omitempty"` // RDP: «DOMINIO\usuario» or «usuario»
	Peer        string    `json:"peer,omitempty"` // AnyDesk: the other side's ID or alias
	PeerName    string    `json:"peer_name,omitempty"`
	Started     time.Time `json:"started"`
	State       string    `json:"state"` // connecting, connected, ended
	Error       string    `json:"error,omitempty"`
	client      string    // the client that reports it (only it can beat or end it)
	uid         int
	odooSession string
	lastBeat    time.Time
	ended       time.Time
}

func (c *remoteSession) open() bool { return c.State != remoteEnded }

// visibleTo: whether a user, from a client app, may see (and give a project to) the session: theirs, or one nobody owns
// yet that their own app reported (an incoming AnyDesk one without a pgocode session there). Other users' hosts,
// Windows users and projects are not shown.
func (c *remoteSession) visibleTo(uid int, client string) bool {
	return c.uid == uid && uid != 0 || c.uid == 0 && client != "" && c.client == client
}

// work: what the imputation service logs for the session.
func (c *remoteSession) work() imputacion.Trabajo {
	var what string
	switch c.Kind {
	case remoteRDP:
		what = "Escritorio remoto en " + c.Name + " (" + c.Host + ") como " + c.User
	default:
		who := c.Peer
		if c.PeerName != "" {
			who = c.PeerName + " (" + c.Peer + ")"
		}
		if who == "" {
			who = "equipo desconocido"
		}
		what = "AnyDesk en " + who
		if c.Direction == string(anydesk.Incoming) {
			what = "AnyDesk entrante desde " + who
		}
	}
	title, model := "Escritorio remoto: "+c.Name, "Escritorio remoto"
	if c.Kind == remoteAnyDesk {
		title, model = what, "AnyDesk"
	}
	return imputacion.Trabajo{Sesion: c.Kind + "-" + c.ID, Titulo: title, Proyecto: c.Project, NombreProy: c.ProjectName,
		UID: c.uid, OdooSesion: c.odooSession, Modelo: model, Desc: imputacion.Descripcion(what, "trabajo en el escritorio remoto")}
}

type remoteSessions struct {
	mu  sync.Mutex
	m   map[string]*remoteSession
	now func() time.Time // tests
}

func (rs *remoteSessions) clock() time.Time {
	if rs.now != nil {
		return rs.now()
	}
	return time.Now()
}

// logging: the open sessions whose time is being logged (with a project).
func (rs *remoteSessions) logging() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	n := 0
	for _, c := range rs.m {
		if c.open() && c.Project > 0 {
			n++
		}
	}
	return n
}

// list: a copy of the sessions of a kind ("" all) that user uid may see from that client, the oldest first.
func (rs *remoteSessions) list(kind string, onlyOpen bool, uid int, client string) []remoteSession {
	rs.mu.Lock()
	l := []remoteSession{}
	for _, c := range rs.m {
		if (kind == "" || c.Kind == kind) && (!onlyOpen || c.open()) && c.visibleTo(uid, client) {
			l = append(l, *c)
		}
	}
	rs.mu.Unlock()
	sort.Slice(l, func(i, j int) bool { return l[i].Started.Before(l[j].Started) })
	return l
}

// clientUsers: the user session last seen from each client (see the package comment above).
type clientUsers struct {
	mu sync.Mutex
	m  map[string]string // client id → session token
}

// noteClientUser remembers the user session of a request that comes from a client.
func (s *Servidor) noteClientUser(r *http.Request, ses *sesiones.Sesion) {
	id, ok := s.clientOf(r)
	if !ok {
		return
	}
	s.clientUser.mu.Lock()
	if s.clientUser.m == nil {
		s.clientUser.m = map[string]string{}
	}
	s.clientUser.m[id] = ses.Token
	s.clientUser.mu.Unlock()
}

// userOf: the user session of a client's request: its own cookie or the one last seen from that client.
func (s *Servidor) userOf(r *http.Request, client string) (*sesiones.Sesion, bool) {
	if s.Sesiones == nil {
		return nil, false
	}
	if c, err := r.Cookie(s.cookieSesion()); err == nil {
		if ses, ok := s.Sesiones.Busca(c.Value); ok {
			return ses, true
		}
	}
	s.clientUser.mu.Lock()
	token := s.clientUser.m[client]
	s.clientUser.mu.Unlock()
	if token == "" {
		return nil, false
	}
	return s.Sesiones.Busca(token)
}

// fromClient: the client of a request to the helper's routes (403 if it is not a paired one).
func (s *Servidor) fromClient(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := s.clientOf(r)
	if !ok {
		jsonError(w, http.StatusForbidden, "solo para la app de escritorio")
	}
	return id, ok
}

// cleanText: a short single-line text from a client (ok false if it has control characters).
func cleanText(s string, max int) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsFunc(s, func(r rune) bool { return r < ' ' || r == 0x7f }) || !utf8.ValidString(s) {
		return "", false
	}
	if len(s) > max {
		s = s[:max]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s, true
}

// validateRemote checks and tidies what a client reports when a session opens; the error is for the person.
func validateRemote(p *desktop.SessionInfo) string {
	var ok [5]bool
	p.Name, ok[0] = cleanText(p.Name, remoteMaxText)
	p.Host, ok[1] = cleanText(p.Host, remoteMaxText)
	p.User, ok[2] = cleanText(p.User, remoteMaxText)
	p.PeerName, ok[3] = cleanText(p.PeerName, remoteMaxText)
	p.Peer, ok[4] = cleanText(p.Peer, remoteMaxText)
	if ok != [5]bool{true, true, true, true, true} {
		return "datos de la sesión no válidos"
	}
	if p.Direction == "" {
		p.Direction = string(anydesk.Outgoing)
	}
	switch p.Kind {
	case remoteRDP:
		if err := freerdp.ValidateTarget(p.Host, p.User); err != nil {
			return err.Error()
		}
		if p.Project <= 0 {
			return "elige el proyecto al que se imputa el tiempo"
		}
		if p.Direction != string(anydesk.Outgoing) {
			return "dirección no válida"
		}
		if p.Name == "" {
			p.Name = p.Host
		}
		p.Peer, p.PeerName = "", ""
	case remoteAnyDesk:
		if p.Direction != string(anydesk.Outgoing) && p.Direction != string(anydesk.Incoming) {
			return "dirección no válida"
		}
		if p.Peer != "" {
			peer, ok := anydesk.ValidID(p.Peer)
			if !ok {
				return "el ID de AnyDesk no es válido"
			}
			p.Peer = peer
		}
		p.Host, p.User = "", ""
	default:
		return "tipo de sesión no válido"
	}
	if p.Project < 0 {
		return "proyecto no válido"
	}
	return ""
}

// remoteCreate: POST /api/remote/sessions. A client opens a session; the answer is the session (its id).
func (s *Servidor) remoteCreate(w http.ResponseWriter, r *http.Request) {
	client, ok := s.fromClient(w, r)
	if !ok {
		return
	}
	var p desktop.SessionInfo
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if msg := validateRemote(&p); msg != "" {
		jsonError(w, http.StatusBadRequest, msg)
		return
	}
	ses, haveUser := s.userOf(r, client)
	c := &remoteSession{Kind: p.Kind, Direction: p.Direction, Name: p.Name, Host: p.Host, User: p.User, Peer: p.Peer,
		PeerName: p.PeerName, client: client}
	if haveUser {
		c.uid, c.odooSession = ses.UID, ses.OdooSesion
	}
	if p.Project > 0 {
		if !haveUser {
			jsonError(w, http.StatusUnauthorized, "no hay ninguna sesión de pgocode abierta en esta app: entra para imputar el tiempo")
			return
		}
		name, ok := s.proyecto(w, r, ses, p.Project)
		if !ok {
			return
		}
		c.Project, c.ProjectName = p.Project, name
	}
	if c.Kind == remoteAnyDesk {
		var who *sesiones.Sesion
		if haveUser {
			who = ses
		}
		s.anydeskProjectFor(r.Context(), c, who)
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.ID = hex.EncodeToString(b)
	rs := &s.remote
	rs.mu.Lock()
	now := rs.clock()
	c.Started, c.lastBeat, c.State = now, now, remoteConnecting
	if rs.m == nil {
		rs.m = map[string]*remoteSession{}
	}
	rs.m[c.ID] = c
	snap := *c
	rs.mu.Unlock()
	if c.Project == 0 {
		log.Printf("%s: sesión %s %s sin proyecto: no se imputa hasta que se le asigne uno", c.Kind, c.Direction, c.Peer)
	} else {
		log.Printf("%s: sesión %s %s%s en el proyecto %d (cliente %s)", c.Kind, c.Direction, c.Host, c.Peer, c.Project, client)
	}
	s.Eventos.Emite(map[string]string{"tipo": "remoto"})
	jsonOK(w, snap)
}

// ownSession: the open session id of the request's client (404 if it is not its own: nobody else's is revealed).
func (s *Servidor) ownSession(w http.ResponseWriter, r *http.Request, client string) (*remoteSession, bool) {
	rs := &s.remote
	rs.mu.Lock()
	c := rs.m[r.PathValue("id")]
	rs.mu.Unlock()
	if c == nil || c.client != client {
		jsonError(w, http.StatusNotFound, "no hay esa sesión remota")
		return nil, false
	}
	return c, true
}

// remoteBeatHandler: POST /api/remote/sessions/{id}/beat {active}. The session is still open; active: someone used
// the keyboard or mouse lately (measured in the client). 410 if it already ended (the client stops beating).
func (s *Servidor) remoteBeatHandler(w http.ResponseWriter, r *http.Request) {
	client, ok := s.fromClient(w, r)
	if !ok {
		return
	}
	var p struct {
		Active bool `json:"active"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	c, ok := s.ownSession(w, r, client)
	if !ok {
		return
	}
	rs := &s.remote
	rs.mu.Lock()
	if !c.open() {
		rs.mu.Unlock()
		jsonError(w, http.StatusGone, "esa sesión remota ya ha terminado")
		return
	}
	now := rs.clock()
	c.lastBeat = now
	counts := now.Sub(c.Started) >= remoteFirstBeat
	changed := counts && c.State == remoteConnecting
	if changed {
		c.State = remoteConnected
	}
	var work *imputacion.Trabajo
	if counts && p.Active && c.Project > 0 && c.uid > 0 {
		x := c.work()
		work = &x
	}
	rs.mu.Unlock()
	if work != nil && s.Imputacion != nil {
		s.Imputacion.Actividad(*work)
	}
	if changed {
		s.Eventos.Emite(map[string]string{"tipo": "remoto"})
	}
	jsonOK(w, map[string]bool{"ok": true, "logged": work != nil})
}

// remoteEnd: POST /api/remote/sessions/{id}/end {error}. The session ended (error: why, explained, "" if normally).
func (s *Servidor) remoteEnd(w http.ResponseWriter, r *http.Request) {
	client, ok := s.fromClient(w, r)
	if !ok {
		return
	}
	var p struct {
		Error string `json:"error"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	c, ok := s.ownSession(w, r, client)
	if !ok {
		return
	}
	msg := strings.TrimSpace(p.Error)
	if len(msg) > remoteMaxError {
		msg = msg[:remoteMaxError]
	}
	s.endRemote(c, msg, "sesión remota terminada")
	jsonOK(w, map[string]bool{"ok": true})
}

// endRemote marks the session ended and closes its period (nothing if it had already ended).
func (s *Servidor) endRemote(c *remoteSession, msg, reason string) {
	rs := &s.remote
	rs.mu.Lock()
	if !c.open() {
		rs.mu.Unlock()
		return
	}
	c.State, c.Error, c.ended = remoteEnded, strings.ToValidUTF8(msg, ""), rs.clock()
	logged := c.Project > 0
	session := c.Kind + "-" + c.ID
	rs.mu.Unlock()
	if logged && s.Imputacion != nil {
		s.Imputacion.Cierra(session, reason)
	}
	log.Printf("%s: sesión %s %s%s terminada", c.Kind, c.Direction, c.Host, c.Peer)
	s.Eventos.Emite(map[string]string{"tipo": "remoto"})
}

// expireRemote closes the sessions whose client stopped beating (closed or fallen) and forgets the ended ones.
func (s *Servidor) expireRemote() {
	rs := &s.remote
	rs.mu.Lock()
	now := rs.clock()
	var silent []*remoteSession
	for id, c := range rs.m {
		switch {
		case c.open() && now.Sub(c.lastBeat) > remoteSilence:
			silent = append(silent, c)
		case !c.open() && now.Sub(c.ended) > remoteKeepEnded:
			delete(rs.m, id)
		}
	}
	rs.mu.Unlock()
	for _, c := range silent {
		s.endRemote(c, "La app de escritorio dejó de dar noticias de esta sesión: su tiempo se ha cerrado.", "sin noticias del cliente")
	}
}

// WatchRemoteSessions closes, until ctx ends, the remote sessions whose client stopped beating.
func (s *Servidor) WatchRemoteSessions(ctx context.Context) {
	t := time.NewTicker(remoteBeat / 2)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.expireRemote()
		}
	}
}

// remoteList: GET /api/remote/sessions. The user's remote sessions, open and recently ended (for the interface).
func (s *Servidor) remoteList(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	client, _ := s.clientOf(r)
	jsonOK(w, s.remote.list("", false, ses.UID, client))
}
