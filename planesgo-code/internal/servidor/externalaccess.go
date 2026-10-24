package servidor

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/sesiones"
)

// External access: the person lets an agent that runs outside pgocode (another Claude Code session, say) use «pgo»
// on one of their work sessions: its agent desktop, its SQL Server connections, its remote PowerShell… the same
// orders as the session's own agents (agentEndpoints), and the time of what it does goes to that session, as the
// person's. The person gives it from the session's properties, for a while, and can take it back at any moment.
//
// The access is kept with the session's properties: they show its token and the variables to give the external agent
// (copy them, or load the file below), and it lasts, across restarts of pgocode, until its time is over or the
// person takes it back (Servidor.ExternalFile, in pgocode's data folder: agents cannot read it). pgocode also leaves it
// in ExternalAccessFile, in the session's folder (out of git, permissions 600): the external agent loads it
// («. .pgocode/acceso-externo.env») and uses the «pgo» command from that folder, so its screenshots go to the folder's
// .pgocode/salidas as the agents'. pgocode's own agents are kept from reading it (internal/llm/odooguard.go).

// ExternalAccessFile: where the access is left, relative to the session's folder.
const ExternalAccessFile = ".pgocode/acceso-externo.env"

// externalMax: the longest an access can be given for.
const externalMax = 24 * time.Hour

// externalView: one access, as the session's properties show it: with its token and the variables to give the
// external agent (only to the person, logged in, in their own app).
type externalView struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Login   string    `json:"login"`
	File    string    `json:"file"`
	Token   string    `json:"token"`
	Env     string    `json:"env"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

type externalRequest struct {
	sessionRef
	Folder string `json:"folder"`
	Label  string `json:"label"`
	Hours  int    `json:"hours"`
	ID     string `json:"id"`
}

func (s *Servidor) externalList(project int, session string) []externalView {
	l := []externalView{}
	for token, g := range s.agentes.externalTokens() {
		if g.project == project && g.session == session {
			l = append(l, externalView{ID: g.id, Label: g.label, Login: g.login, File: g.file, Token: token,
				Env: s.externalEnv(project, session, token), Created: g.created, Expires: g.expires})
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].Created.Before(l[j].Created) })
	return l
}

// GET /api/acceso-externo?project=&session=: the session's external accesses.
func (s *Servidor) externalState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p externalRequest
	if s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		jsonOK(w, s.externalList(p.Project, p.Session))
	}
}

// POST /api/acceso-externo {project, session, folder, label, hours}: gives an external agent access to the session.
func (s *Servidor) externalGrant(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p externalRequest
	if !s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		return
	}
	name, ok := s.proyecto(w, r, ses, p.Project)
	if !ok {
		return
	}
	if !filepath.IsAbs(p.Folder) || !s.dentroDelProyecto(p.Project, p.Folder) {
		jsonError(w, http.StatusForbidden, "la carpeta tiene que ser la de la sesión, del proyecto")
		return
	}
	if s.TokenAgente == "" {
		jsonError(w, http.StatusConflict, "este pgocode no da la orden «pgo» a los agentes")
		return
	}
	label := strings.TrimSpace(p.Label)
	if label == "" {
		label = "agente externo"
	}
	label, ok = cleanText(label, 80)
	if !ok {
		jsonError(w, http.StatusBadRequest, "nombre no válido")
		return
	}
	hours := min(max(p.Hours, 1), int(externalMax/time.Hour))
	id := randomHex()[:12]
	file := filepath.Join(p.Folder, ExternalAccessFile)
	g := agentGrant{project: p.Project, session: p.Session, external: true, id: id, label: label, login: ses.Login,
		uid: ses.UID, projectName: name, file: file, created: time.Now()}
	token, revoke := s.agentes.issueGrant(g, time.Duration(hours)*time.Hour)
	if token == "" {
		jsonError(w, http.StatusInternalServerError, "no se ha podido crear el acceso")
		return
	}
	if err := s.writeExternalFile(file, p.Project, p.Session, token, label, time.Now().Add(time.Duration(hours)*time.Hour)); err != nil {
		revoke()
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	excluyeDeGit(p.Folder)
	s.saveExternal()
	log.Printf("acceso externo «%s» a la sesión %s (proyecto %d) dado por %s durante %d h", label, p.Session, p.Project, ses.Login, hours)
	jsonOK(w, s.externalList(p.Project, p.Session))
}

// POST /api/acceso-externo/retirar {project, session, id}: takes the access back (its file goes too).
func (s *Servidor) externalRevoke(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p externalRequest
	if !s.readSessionRequest(w, r, ses, &p, &p.sessionRef) {
		return
	}
	g, ok := s.agentes.revokeExternal(p.Project, p.Session, p.ID)
	if !ok {
		jsonError(w, http.StatusNotFound, "ese acceso ya no está")
		return
	}
	removeExternalFile(g.file)
	s.saveExternal()
	log.Printf("acceso externo «%s» a la sesión %s retirado por %s", g.label, p.Session, ses.Login)
	jsonOK(w, s.externalList(p.Project, p.Session))
}

// writeExternalFile leaves the access in file (600), replacing the one there was.
func (s *Servidor) writeExternalFile(file string, project int, session, token, label string, until time.Time) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	if st, err := os.Lstat(file); err == nil && !st.Mode().IsRegular() {
		return fmt.Errorf("%s no es un fichero normal: no se escribe", file)
	}
	text := fmt.Sprintf("# Acceso de «%s» a la sesión %s del proyecto %d de pgocode (lo da la persona en las propiedades de la\n"+
		"# sesión y lo puede retirar). Vale hasta %s. Se carga con «. %s» en esta carpeta y se usa con «pgo».\n%s",
		label, session, project, until.Format("02/01/2006 15:04"), ExternalAccessFile, s.externalEnv(project, session, token))
	f, err := os.CreateTemp(filepath.Dir(file), ".acceso-externo-*")
	if err != nil {
		return err
	}
	_, err = f.WriteString(text)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), file) // CreateTemp makes it 600
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// externalEnv: the variables the external agent needs («export …» lines).
func (s *Servidor) externalEnv(project int, session, token string) string {
	path := ""
	if s.DirBin != "" {
		path = "export PATH=" + strconv.Quote(s.DirBin) + ":\"$PATH\"\n"
	}
	return fmt.Sprintf("export PGO_API=%s\nexport PGO_TOKEN=%s\nexport PGO_PROYECTO=%d\nexport PGO_SESION=%s\n%s",
		"http://127.0.0.1:"+s.Puerto, token, project, session, path)
}

// externalKept: an access as it is kept in ExternalFile.
type externalKept struct {
	Token       string    `json:"token"`
	Project     int       `json:"project"`
	ProjectName string    `json:"project_name"`
	Session     string    `json:"session"`
	ID          string    `json:"id"`
	Label       string    `json:"label"`
	Login       string    `json:"login"`
	UID         int       `json:"uid"`
	File        string    `json:"file"`
	Created     time.Time `json:"created"`
	Expires     time.Time `json:"expires"`
}

// saveExternal keeps the valid accesses in ExternalFile (600; written whole, atomically).
func (s *Servidor) saveExternal() {
	if s.ExternalFile == "" {
		return
	}
	l := []externalKept{}
	for token, g := range s.agentes.externalTokens() {
		l = append(l, externalKept{Token: token, Project: g.project, ProjectName: g.projectName, Session: g.session, ID: g.id,
			Label: g.label, Login: g.login, UID: g.uid, File: g.file, Created: g.created, Expires: g.expires})
	}
	b, _ := json.MarshalIndent(l, "", " ")
	tmp := s.ExternalFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		err = os.Rename(tmp, s.ExternalFile)
		if err != nil {
			log.Printf("accesos externos: %v", err)
		}
	}
}

// LoadExternal puts back, when pgocode starts, the accesses kept that are still valid.
func (s *Servidor) LoadExternal() {
	b, err := os.ReadFile(s.ExternalFile)
	if err != nil {
		return
	}
	var l []externalKept
	if json.Unmarshal(b, &l) != nil {
		return
	}
	now := time.Now()
	for _, k := range l {
		if now.Before(k.Expires) && k.Token != "" && safeSession.MatchString(k.Session) {
			s.agentes.restore(k.Token, agentGrant{project: k.Project, session: k.Session, expires: k.Expires, external: true, id: k.ID,
				label: k.Label, login: k.Login, uid: k.UID, projectName: k.ProjectName, file: k.File, created: k.Created})
		} else {
			removeExternalFile(k.File)
		}
	}
	s.saveExternal()
}

func removeExternalFile(file string) {
	if st, err := os.Lstat(file); err == nil && st.Mode().IsRegular() {
		os.Remove(file)
	}
}

// externalActivity: an order of an external agent counts as work in its session, the person's who gave the access.
func (s *Servidor) externalActivity(g agentGrant) {
	if !g.external || g.uid == 0 || s.Imputacion == nil {
		return
	}
	what := "Agente externo: " + g.label
	odoo := ""
	if s.Sesiones != nil {
		odoo = s.Sesiones.OdooSesionDe(g.uid) // the person's current pgocode session (none: the service waits for one)
	}
	s.Imputacion.Actividad(imputacion.Trabajo{Sesion: g.session, Titulo: what, Proyecto: g.project, NombreProy: g.projectName,
		UID: g.uid, OdooSesion: odoo, Desc: imputacion.Descripcion(what, "trabajo de un agente externo")})
}
