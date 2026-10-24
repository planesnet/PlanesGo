package servidor

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"planesgo-code/internal/anydesk"
	"planesgo-code/internal/sesiones"
)

// AnyDesk: the client's desktop helper (internal/desktop) opens the installed AnyDesk and follows its traces; each
// session it sees is a remote session (remotesessions.go). What stays here is the accounting: the project of each
// remote computer, the default project of incoming sessions and the user who logs them (AnyDeskFile), and giving a
// project to a session without one (the interface's «Sesión de AnyDesk sin proyecto»).

const anydeskMaxPeers = 300 // projects remembered per remote computer

type anydeskProject struct {
	Project int    `json:"project"`
	Name    string `json:"name"`
}

// anydeskSettings (AnyDeskFile): the default project for incoming AnyDesk sessions and the last project of each remote
// computer. Nothing secret. (The time is logged to the user with a pgocode session in that app: anydeskProjectFor.)
type anydeskSettings struct {
	Default     int                       `json:"default_project"`
	DefaultName string                    `json:"default_project_name"`
	Peers       map[string]anydeskProject `json:"peers"`
}

type anydeskSessions struct {
	mu       sync.Mutex
	settings *anydeskSettings
}

// anydeskSettingsLocked: the settings, read from AnyDeskFile the first time (s.anydesk.mu held).
func (s *Servidor) anydeskSettingsLocked() *anydeskSettings {
	a := &s.anydesk
	if a.settings == nil {
		a.settings = &anydeskSettings{}
		if s.AnyDeskFile != "" {
			if b, err := os.ReadFile(s.AnyDeskFile); err == nil {
				if err := json.Unmarshal(b, a.settings); err != nil {
					log.Printf("anydesk: %s: %v", s.AnyDeskFile, err)
				}
			}
		}
		if a.settings.Peers == nil {
			a.settings.Peers = map[string]anydeskProject{}
		}
	}
	return a.settings
}

// anydeskSaveLocked writes the settings (s.anydesk.mu held).
func (s *Servidor) anydeskSaveLocked() {
	if s.AnyDeskFile == "" {
		return
	}
	st := s.anydeskSettingsLocked()
	for p := range st.Peers { // a bound on what is remembered (map order: any of them)
		if len(st.Peers) <= anydeskMaxPeers {
			break
		}
		delete(st.Peers, p)
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	os.MkdirAll(filepath.Dir(s.AnyDeskFile), 0o700)
	tmp := s.AnyDeskFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil {
		err = os.Rename(tmp, s.AnyDeskFile)
		if err != nil {
			log.Printf("anydesk: %v", err)
		}
	}
}

// anydeskProjectFor completes a new AnyDesk session (not listed yet). With a project (opened from pgocode), it is
// remembered for that computer; without one: the computer's last project or, incoming, the default one, but only for
// the user with a pgocode session in that app (ses) and only if they still have access to it in Odoo. With nobody
// there it gets no project: nobody's time is logged on a guess, and the person gives it one in the interface.
func (s *Servidor) anydeskProjectFor(ctx context.Context, c *remoteSession, ses *sesiones.Sesion) {
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	if c.Project > 0 {
		defer a.mu.Unlock()
		if c.Peer != "" {
			st.Peers[c.Peer] = anydeskProject{Project: c.Project, Name: c.ProjectName}
		}
		s.anydeskSaveLocked()
		return
	}
	project := 0
	if p, ok := st.Peers[c.Peer]; ok && c.Peer != "" {
		project = p.Project
	} else if c.Direction == string(anydesk.Incoming) && st.Default > 0 {
		project = st.Default
	}
	a.mu.Unlock()
	if project == 0 || ses == nil {
		return
	}
	name, err := s.projectAccess(ctx, ses, project)
	if err != nil {
		log.Printf("anydesk: sesión %s sin proyecto: %v", c.Peer, err)
		return
	}
	c.Project, c.ProjectName, c.uid, c.odooSession = project, name, ses.UID, ses.OdooSesion
}

// anydeskState: GET /api/anydesk. The open AnyDesk sessions and the default project of incoming ones.
func (s *Servidor) anydeskState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	def, defName := st.Default, st.DefaultName
	a.mu.Unlock()
	client, _ := s.clientOf(r)
	jsonOK(w, map[string]any{"sessions": s.remote.list(remoteAnyDesk, true, ses.UID, client), "default_project": def, "default_project_name": defName})
}

// anydeskAssign: POST /api/anydesk/assign {id, project, default}. Gives an open session a project (and remembers it
// for that computer; default: also for every incoming session without one).
func (s *Servidor) anydeskAssign(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		ID      string `json:"id"`
		Project int    `json:"project"`
		Default bool   `json:"default"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	projectName, ok := s.proyecto(w, r, ses, p.Project)
	if !ok {
		return
	}
	client, _ := s.clientOf(r)
	rs := &s.remote
	rs.mu.Lock()
	c := rs.m[p.ID]
	if c == nil || c.Kind != remoteAnyDesk || !c.open() || !c.visibleTo(ses.UID, client) {
		rs.mu.Unlock()
		jsonError(w, http.StatusNotFound, "esa sesión de AnyDesk ya no está abierta")
		return
	}
	old := c.Project
	c.Project, c.ProjectName, c.uid, c.odooSession = p.Project, projectName, ses.UID, ses.OdooSesion
	snap := *c
	rs.mu.Unlock()
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	if snap.Peer != "" {
		st.Peers[snap.Peer] = anydeskProject{Project: p.Project, Name: projectName}
	}
	if p.Default {
		st.Default, st.DefaultName = p.Project, projectName
	}
	s.anydeskSaveLocked()
	a.mu.Unlock()
	if old > 0 && old != p.Project && s.Imputacion != nil {
		s.Imputacion.Cierra(remoteAnyDesk+"-"+snap.ID, "proyecto cambiado") // what was logged stays in the old project
	}
	s.Eventos.Emite(map[string]string{"tipo": "remoto"})
	jsonOK(w, snap)
}

// anydeskDefault: POST /api/anydesk/default {project}. The project of incoming sessions (0: none, ask each time).
func (s *Servidor) anydeskDefault(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Project int `json:"project"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	name := ""
	if p.Project != 0 {
		n, ok := s.proyecto(w, r, ses, p.Project)
		if !ok {
			return
		}
		name = n
	}
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	st.Default, st.DefaultName = p.Project, name
	s.anydeskSaveLocked()
	a.mu.Unlock()
	s.Eventos.Emite(map[string]string{"tipo": "remoto"})
	jsonOK(w, map[string]any{"default_project": p.Project, "default_project_name": name})
}
