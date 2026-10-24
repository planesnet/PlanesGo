package servidor

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/procesos"
)

// Programs launched from a work session (pgosage, say: «Programas» of the session, or «pgo ejecutar» of its agent)
// use that session's services: its SQL Server connections, its remote PowerShell, its agent's desktop and the
// imputation of its time. They get, in their environment, a one-use code (EnvProgramCode) that they exchange for
// their token as they start (POST /api/pgo/programa/token with the code in X-PGO-Programa); every other request
// carries the token in X-PGO-Programa. The token is bound to the project, the session and the program, only opens
// programEndpoints, and is revoked when the program ends.
//
// Why a code and not the token itself: the environment of a process can be read by any process of the same user (an
// agent among them, with «ps e» or /proc). The code is worth nothing once used, and the token never leaves the
// program's memory. A request with a program token from an agent's process is refused too (where the system says
// whose process it is).

const (
	EnvProgramCode    = "PGO_CODIGO_PROGRAMA" // the one-use code
	EnvProgramSession = "PGO_SESION"          // the work session the program belongs to
	EnvProgramName    = "PGO_PROGRAMA"        // its name in «Programas»
	programHeader     = "X-PGO-Programa"
	programCodeTTL    = 10 * time.Minute // a program that has not started by then does not get in
)

// programEndpoints: what a program can use (/api/pgo/<orden>) and with which method. Nothing that launches programs,
// reads Google or opens pages: that is the agent's and the person's.
var programEndpoints = map[string]string{
	"sql":            http.MethodPost, // a batch on one of the session's connections («conexion»)
	"sql-flujo":      http.MethodPost, // the same, all its rows as they come (JSON lines)
	"sql-conexiones": http.MethodGet,  // the session's connections and their mode
	"ps":             http.MethodPost,
	"ps-download":    http.MethodPost,
	"ps-upload":      http.MethodPost,
	"escritorio":     http.MethodPost,
	"actividad":      http.MethodPost, // the person is using the program: its time goes to the session
}

// programGrant: whose a program token is. uid and odooSession: the person who launched it from the interface (an
// agent's program has none: it cannot log time).
type programGrant struct {
	project     int
	projectName string
	session     string
	program     string
	login       string
	uid         int
	odooSession string
}

type programTokens struct {
	mu     sync.Mutex
	codes  map[string]programCode // one-use code → its token
	grants map[string]programGrant
}

type programCode struct {
	token   string
	expires time.Time
}

func randomHex() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// issue creates the token of a program and its one-use code; revoke ends both.
func (t *programTokens) issue(g programGrant) (code string, revoke func()) {
	token, code := randomHex(), randomHex()
	if token == "" || code == "" {
		return "", func() {}
	}
	t.mu.Lock()
	if t.codes == nil {
		t.codes, t.grants = map[string]programCode{}, map[string]programGrant{}
	}
	now := time.Now()
	for k, c := range t.codes {
		if now.After(c.expires) {
			delete(t.codes, k)
		}
	}
	t.codes[code] = programCode{token: token, expires: now.Add(programCodeTTL)}
	t.grants[token] = g
	t.mu.Unlock()
	return code, func() {
		t.mu.Lock()
		delete(t.codes, code)
		delete(t.grants, token)
		t.mu.Unlock()
	}
}

// exchange: the token of a code, once.
func (t *programTokens) exchange(code string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	c, ok := t.codes[code]
	delete(t.codes, code)
	if !ok || time.Now().After(c.expires) {
		return "", false
	}
	_, alive := t.grants[c.token]
	return c.token, alive
}

func (t *programTokens) lookup(token string) (programGrant, bool) {
	if token == "" {
		return programGrant{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	g, ok := t.grants[token]
	return g, ok
}

// programEnv: the environment of a program launched from a session, with its code (revoked when the program ends:
// the caller calls watch once it is launched). Without a session, only what any program gets.
func (s *Servidor) programEnv(g programGrant) (env []string, watch func(*procesos.Proceso)) {
	env = s.entornoPrograma(g.project)
	if g.session == "" || !safeSession.MatchString(g.session) {
		return env, func(*procesos.Proceso) {}
	}
	code, revoke := s.programas.issue(g)
	if code == "" {
		return env, func(*procesos.Proceso) {}
	}
	env = append(env, EnvProgramCode+"="+code, EnvProgramSession+"="+g.session, EnvProgramName+"="+g.program)
	return env, func(p *procesos.Proceso) {
		if p == nil {
			revoke()
			return
		}
		go func() {
			<-p.Done()
			revoke()
		}()
	}
}

// pgoProgram serves the requests of a program (X-PGO-Programa): the exchange of its code, and then its orders.
func (s *Servidor) pgoProgram(w http.ResponseWriter, r *http.Request) {
	if s.fromAgentProcess(r) {
		jsonError(w, http.StatusForbidden, "un agente no puede usar el acceso de un programa")
		return
	}
	cred := strings.TrimSpace(r.Header.Get(programHeader))
	orden := r.PathValue("orden")
	if orden == "programa-token" && r.Method == http.MethodPost {
		token, ok := s.programas.exchange(cred)
		if !ok {
			jsonError(w, http.StatusForbidden, "código de programa no válido, ya usado o caducado")
			return
		}
		jsonOK(w, map[string]string{"token": token})
		return
	}
	g, ok := s.programas.lookup(cred)
	if !ok {
		jsonError(w, http.StatusForbidden, "token de programa no válido (o el programa ya ha terminado)")
		return
	}
	if m, ok := programEndpoints[orden]; !ok || m != r.Method {
		jsonError(w, http.StatusForbidden, "orden no permitida a los programas")
		return
	}
	var p struct {
		Conexion string               `json:"conexion"`
		SQL      string               `json:"sql"`
		N        int                  `json:"n"`
		Escribir bool                 `json:"escribir"`
		Script   string               `json:"script"`
		Path     string               `json:"path"`
		Data     string               `json:"data"`
		Carpeta  string               `json:"carpeta"`
		Desktop  *agentdesktop.Action `json:"desktop"`
		Titulo   string               `json:"titulo"`
	}
	if r.Method == http.MethodPost {
		if err := lee(r, &p); err != nil {
			errorLectura(w, err)
			return
		}
	}
	who := "programa " + g.program
	ref := sessionRef{g.project, g.session}
	switch orden {
	case "sql":
		s.sqlRun(w, r.Context(), g.login, who, g.project, g.session, p.Conexion, p.SQL, p.N, p.Escribir)
	case "sql-flujo":
		s.sqlStream(w, r, g.login, who, g.project, g.session, p.Conexion, p.SQL, p.Escribir)
	case "sql-conexiones":
		jsonOK(w, s.sqlConnections(g.project, g.session))
	case "ps":
		s.winrmRun(w, r.Context(), g.login, who, ref, p.Script, p.Escribir)
	case "ps-download":
		s.winrmDownload(w, r.Context(), ref, p.Path)
	case "ps-upload":
		s.winrmUpload(w, r.Context(), ref, p.Path, p.Data, p.Escribir)
	case "escritorio":
		s.agentDesktopRunAs(w, who, g.project, g.session, p.Carpeta, p.Desktop)
	case "actividad":
		s.programActivity(w, g, p.Titulo)
	}
}

// programActivity: the person is working in the program (it says so while there is keyboard or mouse in it, like
// the interface): its time goes to the session's period, as the person's.
func (s *Servidor) programActivity(w http.ResponseWriter, g programGrant, title string) {
	if g.uid == 0 || s.Imputacion == nil {
		jsonError(w, http.StatusConflict, "este programa no lo lanzó una persona desde la interfaz: su tiempo no se imputa")
		return
	}
	title = strings.TrimSpace(title)
	if len(title) > 200 {
		title = title[:200]
	}
	what := g.program
	if title != "" {
		what += ": " + title
	}
	s.Imputacion.Actividad(imputacion.Trabajo{Sesion: g.session, Titulo: what, Proyecto: g.project, NombreProy: g.projectName,
		UID: g.uid, OdooSesion: g.odooSession, Desc: imputacion.Descripcion(what, "trabajo en "+g.program)})
	w.WriteHeader(http.StatusNoContent)
}

// logProgram: one line when a session gives a program its access.
func logProgram(g programGrant) {
	log.Printf("programa %s con el acceso de la sesión %s (proyecto %d, lanzado por %s)", g.program, g.session, g.project, g.login)
}
