package servidor

import (
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

// anydeskSettings (AnyDeskFile): who logs the AnyDesk sessions, their default project for incoming sessions and the
// last project of each remote computer. Nothing secret.
type anydeskSettings struct {
	UID         int                       `json:"uid"`
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
// remembered for that computer; without one: the computer's last project or, incoming, the default one.
func (s *Servidor) anydeskProjectFor(c *remoteSession) {
	a := &s.anydesk
	a.mu.Lock()
	defer a.mu.Unlock()
	st := s.anydeskSettingsLocked()
	if c.Project > 0 {
		st.UID = c.uid
		if c.Peer != "" {
			st.Peers[c.Peer] = anydeskProject{Project: c.Project, Name: c.ProjectName}
		}
		s.anydeskSaveLocked()
		return
	}
	if p, ok := st.Peers[c.Peer]; ok && c.Peer != "" {
		c.Project, c.ProjectName = p.Project, p.Name
	} else if c.Direction == string(anydesk.Incoming) && st.Default > 0 {
		c.Project, c.ProjectName = st.Default, st.DefaultName
	}
	if c.uid == 0 {
		c.uid = st.UID
	}
	if c.uid == 0 {
		c.Project, c.ProjectName = 0, "" // nobody has used AnyDesk from pgocode: whose time would it be?
	}
}

// anydeskState: GET /api/anydesk. The open AnyDesk sessions and the default project of incoming ones.
func (s *Servidor) anydeskState(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	a := &s.anydesk
	a.mu.Lock()
	st := s.anydeskSettingsLocked()
	def, defName := st.Default, st.DefaultName
	a.mu.Unlock()
	jsonOK(w, map[string]any{"sessions": s.remote.list(remoteAnyDesk, true), "default_project": def, "default_project_name": defName})
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
	rs := &s.remote
	rs.mu.Lock()
	c := rs.m[p.ID]
	if c == nil || c.Kind != remoteAnyDesk || !c.open() {
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
	st.UID = ses.UID
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
	st.UID = ses.UID
	st.Default, st.DefaultName = p.Project, name
	s.anydeskSaveLocked()
	a.mu.Unlock()
	s.Eventos.Emite(map[string]string{"tipo": "remoto"})
	jsonOK(w, map[string]any{"default_project": p.Project, "default_project_name": name})
}
