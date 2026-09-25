package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"pasigo/config"
	"pasigo/odoo"
	"pasigo/store"
)

const (
	sessionCookieName    = "planesgo_session"
	oauthStateCookieName = "planesgo_oauth_state"
	DefaultOdooURL       = "https://planesnet.autopyme.com"
	DefaultOdooDB        = "ap113"
)

type SessionData struct {
	URL         string `json:"url"`
	DB          string `json:"db"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	AuthMethod  string `json:"auth_method"` // "google" o "odoo"
	UserEmail   string `json:"user_email,omitempty"`
	UserName    string `json:"user_name,omitempty"`
	UserPicture string `json:"user_picture,omitempty"`
}

type AppState struct {
	mu                      sync.RWMutex
	cfg                     *config.Config
	userStore               *store.UserSettingsStore
	sseHub                  *SSEHub
	lastTimerConfirmMu      sync.RWMutex
	lastTimerConfirmedTimes map[int]int64
	activeTimersMu          sync.RWMutex
	activeTimers            map[int]map[string]*odoo.ActiveTimer // userUID -> timerKey -> timer
}

// timerKeyFor genera una clave única y estable para un temporizador activo.
func timerKeyFor(timer *odoo.ActiveTimer) string {
	if timer == nil {
		return "default"
	}
	if timer.TimerKey != "" {
		return timer.TimerKey
	}
	if timer.TaskID > 0 {
		return fmt.Sprintf("task_%d", timer.TaskID)
	}
	if timer.TimesheetID > 0 {
		return fmt.Sprintf("ts_%d", timer.TimesheetID)
	}
	if timer.ProjectID > 0 {
		return fmt.Sprintf("proj_%d", timer.ProjectID)
	}
	return "default"
}

func (state *AppState) setLastConfirmedAt(userUID int, ts int64) {
	state.lastTimerConfirmMu.Lock()
	defer state.lastTimerConfirmMu.Unlock()
	if state.lastTimerConfirmedTimes == nil {
		state.lastTimerConfirmedTimes = make(map[int]int64)
	}
	state.lastTimerConfirmedTimes[userUID] = ts
}

func (state *AppState) getLastConfirmedAt(userUID int) int64 {
	state.lastTimerConfirmMu.RLock()
	defer state.lastTimerConfirmMu.RUnlock()
	if state.lastTimerConfirmedTimes == nil {
		return 0
	}
	return state.lastTimerConfirmedTimes[userUID]
}

// setActiveTimer registra o actualiza un temporizador activo en memoria.
// Si timer es nil, limpia los temporizadores del usuario.
func (state *AppState) setActiveTimer(userUID int, timer *odoo.ActiveTimer) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		state.activeTimers = make(map[int]map[string]*odoo.ActiveTimer)
	}
	if timer == nil {
		delete(state.activeTimers, userUID)
		return
	}
	if timer.TimerKey == "" {
		timer.TimerKey = timerKeyFor(timer)
	}
	nowMs := time.Now().UnixMilli()
	if timer.LastHeartbeat == 0 {
		if timer.StartedAt > 0 {
			timer.LastHeartbeat = timer.StartedAt
		} else {
			timer.LastHeartbeat = nowMs
		}
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok || userMap == nil {
		userMap = make(map[string]*odoo.ActiveTimer)
		state.activeTimers[userUID] = userMap
	}
	userMap[timer.TimerKey] = timer
}

// getActiveTimer devuelve el temporizador activo principal o más recientemente utilizado del usuario.
// Para retrocompatibilidad con clientes monousuario.
func (state *AppState) getActiveTimer(userUID int) *odoo.ActiveTimer {
	state.activeTimersMu.RLock()
	defer state.activeTimersMu.RUnlock()
	if state.activeTimers == nil {
		return nil
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok || len(userMap) == 0 {
		return nil
	}
	var best *odoo.ActiveTimer
	for _, t := range userMap {
		if t == nil {
			continue
		}
		if best == nil {
			best = t
			continue
		}
		// Priorizar temporizador en ejecución
		if t.IsRunning && !best.IsRunning {
			best = t
			continue
		}
		if !t.IsRunning && best.IsRunning {
			continue
		}
		// Si ambos están en el mismo estado, priorizar el que tenga latido o inicio más reciente
		tTime := t.LastHeartbeat
		if tTime == 0 {
			tTime = t.StartedAt
		}
		bestTime := best.LastHeartbeat
		if bestTime == 0 {
			bestTime = best.StartedAt
		}
		if tTime > bestTime {
			best = t
		}
	}
	if best == nil {
		return nil
	}
	tCopy := *best
	return &tCopy
}

// getActiveTimers devuelve una lista de copias de todos los temporizadores activos del usuario.
func (state *AppState) getActiveTimers(userUID int) []*odoo.ActiveTimer {
	state.activeTimersMu.RLock()
	defer state.activeTimersMu.RUnlock()
	if state.activeTimers == nil {
		return nil
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok || len(userMap) == 0 {
		return nil
	}
	res := make([]*odoo.ActiveTimer, 0, len(userMap))
	for _, t := range userMap {
		if t != nil {
			tCopy := *t
			res = append(res, &tCopy)
		}
	}
	return res
}

// getActiveTimerByKey busca un temporizador por su clave única.
func (state *AppState) getActiveTimerByKey(userUID int, timerKey string) *odoo.ActiveTimer {
	state.activeTimersMu.RLock()
	defer state.activeTimersMu.RUnlock()
	if state.activeTimers == nil {
		return nil
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok {
		return nil
	}
	t, ok := userMap[timerKey]
	if !ok || t == nil {
		return nil
	}
	tCopy := *t
	return &tCopy
}

// clearActiveTimer elimina todos los temporizadores del usuario.
func (state *AppState) clearActiveTimer(userUID int) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers != nil {
		delete(state.activeTimers, userUID)
	}
}

// clearActiveTimerByKey elimina un temporizador concreto por clave.
func (state *AppState) clearActiveTimerByKey(userUID int, timerKey string) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers != nil {
		if userMap, ok := state.activeTimers[userUID]; ok {
			delete(userMap, timerKey)
			if len(userMap) == 0 {
				delete(state.activeTimers, userUID)
			}
		}
	}
}

// clearActiveTimerForTask elimina temporizadores que coincidan con la tarea o parte de horas indicado.
func (state *AppState) clearActiveTimerForTask(userUID int, taskID int, timesheetID int) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		return
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok {
		return
	}
	for key, t := range userMap {
		if t == nil {
			delete(userMap, key)
			continue
		}
		if (taskID > 0 && t.TaskID == taskID) || (timesheetID > 0 && t.TimesheetID == timesheetID) {
			delete(userMap, key)
		}
	}
	if len(userMap) == 0 {
		delete(state.activeTimers, userUID)
	}
}

// pauseActiveTimer pausa el temporizador en ejecución (o el principal si no se especifica clave).
func (state *AppState) pauseActiveTimer(userUID int, unitAmount float64) {
	state.pauseActiveTimerByKey(userUID, "", unitAmount)
}

// pauseActiveTimerForTask pausa el temporizador específico asociado a una tarea o parte de horas.
func (state *AppState) pauseActiveTimerForTask(userUID int, taskID int, timesheetID int, unitAmount float64) {
	if taskID == 0 && timesheetID == 0 {
		state.pauseActiveTimer(userUID, unitAmount)
		return
	}
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		return
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok {
		return
	}
	nowMs := time.Now().UnixMilli()
	for _, t := range userMap {
		if t == nil {
			continue
		}
		if (taskID > 0 && t.TaskID == taskID) || (timesheetID > 0 && t.TimesheetID == timesheetID) {
			if t.IsRunning && t.Source != "antigravity" && t.StartedAt > 0 {
				elapsed := nowMs - t.StartedAt
				if elapsed > 0 && elapsed <= 24*3600*1000 {
					t.AccumulatedMs += elapsed
				}
			}
			t.IsRunning = false
			t.StartedAt = 0
			if unitAmount > 0 {
				t.UnitAmount = unitAmount
				t.AccumulatedMs = int64(unitAmount * 3600 * 1000)
			} else {
				if t.AccumulatedMs < 0 {
					t.AccumulatedMs = 0
				}
				t.UnitAmount = float64(t.AccumulatedMs) / (3600 * 1000)
			}
			if t.UnitAmount > 24.0 {
				t.UnitAmount = 24.0
				t.AccumulatedMs = 24 * 3600 * 1000
			}
		}
	}
}

// pauseActiveTimerByKey pausa un temporizador específico por su clave (o todos si clave es vacía).
func (state *AppState) pauseActiveTimerByKey(userUID int, timerKey string, unitAmount float64) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		return
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok {
		return
	}
	nowMs := time.Now().UnixMilli()
	for key, t := range userMap {
		if t == nil {
			continue
		}
		if timerKey != "" && key != timerKey {
			continue
		}
		if t.IsRunning && t.Source != "antigravity" && t.StartedAt > 0 {
			elapsed := nowMs - t.StartedAt
			if elapsed > 0 && elapsed <= 24*3600*1000 {
				t.AccumulatedMs += elapsed
			}
		}
		t.IsRunning = false
		t.StartedAt = 0
		if unitAmount > 0 {
			t.UnitAmount = unitAmount
			t.AccumulatedMs = int64(unitAmount * 3600 * 1000)
		} else {
			if t.AccumulatedMs < 0 {
				t.AccumulatedMs = 0
			}
			t.UnitAmount = float64(t.AccumulatedMs) / (3600 * 1000)
		}
		if t.UnitAmount > 24.0 {
			t.UnitAmount = 24.0
			t.AccumulatedMs = 24 * 3600 * 1000
		}
	}
}

func (state *AppState) resumeActiveTimer(userUID int) {
	state.resumeActiveTimerWithTimesheet(userUID, 0, 0)
}

func (state *AppState) resumeActiveTimerWithTimesheet(userUID int, timesheetID int, taskID int) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		state.activeTimers = make(map[int]map[string]*odoo.ActiveTimer)
	}
	userMap, ok := state.activeTimers[userUID]
	if !ok || userMap == nil {
		userMap = make(map[string]*odoo.ActiveTimer)
		state.activeTimers[userUID] = userMap
	}
	nowMs := time.Now().UnixMilli()

	// Buscar temporizador existente que coincida con taskID o timesheetID
	var targetTimer *odoo.ActiveTimer
	for _, t := range userMap {
		if t == nil {
			continue
		}
		if (taskID > 0 && t.TaskID == taskID) || (timesheetID > 0 && t.TimesheetID == timesheetID) {
			targetTimer = t
			break
		}
	}

	if targetTimer != nil {
		targetTimer.IsRunning = true
		targetTimer.StartedAt = nowMs
		targetTimer.LastHeartbeat = nowMs
		if timesheetID > 0 {
			targetTimer.TimesheetID = timesheetID
		}
		if taskID > 0 {
			targetTimer.TaskID = taskID
		}
	} else {
		newTimer := &odoo.ActiveTimer{
			TimesheetID:   timesheetID,
			TaskID:        taskID,
			IsRunning:     true,
			StartedAt:     nowMs,
			LastHeartbeat: nowMs,
		}
		newTimer.TimerKey = timerKeyFor(newTimer)
		userMap[newTimer.TimerKey] = newTimer
	}
}

// StartTimerWatchdog inicia la supervisión de latidos periódicos para pausar tareas inactivas (umbral 15 min).
func (state *AppState) StartTimerWatchdog(ctx context.Context, idleTimeout time.Duration) {
	if idleTimeout <= 0 {
		idleTimeout = 15 * time.Minute
	}
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case now := <-ticker.C:
				state.checkIdleTimers(now, idleTimeout)
			}
		}
	}()
}

// checkIdleTimers inspecciona los temporizadores en curso y pausa aquellos sin latidos recientes.
func (state *AppState) checkIdleTimers(now time.Time, idleTimeout time.Duration) {
	state.activeTimersMu.Lock()
	defer state.activeTimersMu.Unlock()
	if state.activeTimers == nil {
		return
	}
	nowMs := now.UnixMilli()
	timeoutMs := idleTimeout.Milliseconds()

	for userUID, userMap := range state.activeTimers {
		for key, timer := range userMap {
			if timer == nil || !timer.IsRunning {
				continue
			}
			lastBeat := timer.LastHeartbeat
			if lastBeat == 0 {
				lastBeat = timer.StartedAt
			}
			if lastBeat > 0 && (nowMs-lastBeat) > timeoutMs {
				// Pausar temporizador por inactividad
				if timer.Source != "antigravity" && timer.StartedAt > 0 && lastBeat >= timer.StartedAt {
					elapsed := lastBeat - timer.StartedAt
					if elapsed > 0 && elapsed <= 24*3600*1000 {
						timer.AccumulatedMs += elapsed
					}
				}
				timer.IsRunning = false
				timer.StartedAt = 0
				if timer.AccumulatedMs < 0 {
					timer.AccumulatedMs = 0
				}
				timer.UnitAmount = float64(timer.AccumulatedMs) / (3600 * 1000)
				if timer.UnitAmount > 24.0 {
					log.Printf("[PlanesGo Watchdog] ADVERTENCIA: UnitAmount anómalo detectado (%f h) en temporizador '%s', ajustando a 24h", timer.UnitAmount, key)
					timer.UnitAmount = 24.0
					timer.AccumulatedMs = 24 * 3600 * 1000
				}
				log.Printf("[PlanesGo Watchdog] Temporizador '%s' (usuario %d, tarea %d) pausado automáticamente por inactividad (%v sin latidos)", key, userUID, timer.TaskID, idleTimeout)

				// Notificar al usuario mediante evento SSE
				if state.sseHub != nil {
					state.broadcastUserEvent(userUID, "timer_timeout", map[string]interface{}{
						"timer_key":     key,
						"task_id":       timer.TaskID,
						"task_name":     timer.TaskName,
						"unit_amount":   timer.UnitAmount,
						"accumulated_ms": timer.AccumulatedMs,
						"reason":        "inactivity_timeout",
					})
				}
			}
		}
	}
}

type WorkerRecentProject struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	LastDate        string  `json:"last_date"`
	TotalHours      float64 `json:"total_hours"`
	EntryCount      int     `json:"entry_count"`
	LastTask        string  `json:"last_task"`
	Employee        string  `json:"employee"`
	OpenTicketCount int     `json:"open_ticket_count,omitempty"`
	TicketTitle     string  `json:"ticket_title,omitempty"`
}

func (r WorkerRecentProject) FormattedHours() string {
	hours := int(r.TotalHours)
	minutes := int((r.TotalHours - float64(hours)) * 60)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %02dm", hours, minutes)
}

func (r WorkerRecentProject) FormattedDate() string {
	if r.LastDate == "" {
		return "-"
	}
	t, err := time.Parse("2006-01-02", r.LastDate)
	if err != nil {
		return r.LastDate
	}
	return t.Format("02/01/2006")
}

type PageData struct {
	Version              string
	CurrentView          string
	Config               *config.Config
	Session              *SessionData
	HasOdooToken         bool
	Entries              []odoo.TimesheetEntry
	Projects             []odoo.Project
	TotalHours           float64
	TotalHorasHombre     float64
	TotalHorasMaquina    float64
	TotalProjectsCount   int
	UniqueProjectsCount  int
	UniqueEmployeesCount int
	ProjectsList         []string
	EmployeesList        []string
	CurrentWorker        string
	RecentProjects       []WorkerRecentProject
	PendingTickets       []odoo.Ticket
	PendingTicketsCount  int
	OdooURL              string
	Today                 string
	ActiveTimer           *odoo.ActiveTimer
	ProjectPartnerMapJSON template.JS
	Error                 string
}

// FormatHoursToHHMM convierte un valor decimal de horas a formato Horas:Minutos (ej. 2.62 -> "2:37", 3.00 -> "3:00").
func FormatHoursToHHMM(h float64) string {
	if h <= 0 {
		return "0:00"
	}
	totalMinutes := int(math.Round(h * 60))
	hours := totalMinutes / 60
	mins := totalMinutes % 60
	return fmt.Sprintf("%d:%02d", hours, mins)
}

func (p PageData) TotalHoursHHMM() string {
	return FormatHoursToHHMM(p.TotalHours)
}

func (p PageData) TotalHorasHombreHHMM() string {
	return FormatHoursToHHMM(p.TotalHorasHombre)
}

func (p PageData) TotalHorasMaquinaHHMM() string {
	return FormatHoursToHHMM(p.TotalHorasMaquina)
}

type SettingsPageData struct {
	Version        string
	Config         *config.Config
	Session        *SessionData
	Settings       store.UserSettings
	SuccessMessage string
	ErrorMessage   string
}

type LoginPageData struct {
	Version           string
	URL               string
	DB                string
	Username          string
	Password          string
	GoogleAuthEnabled bool
	GoogleConfigured  bool
	GoogleConfigError string
	Error             string
	Next              string
}

func encodeSession(data SessionData) string {
	b, _ := json.Marshal(data)
	return base64.StdEncoding.EncodeToString(b)
}

func decodeSession(cookieVal string) (*SessionData, error) {
	b, err := base64.StdEncoding.DecodeString(cookieVal)
	if err != nil {
		return nil, err
	}
	var data SessionData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}
	// Sanitizar valor legacy "pasi" de cookies antiguas
	if strings.EqualFold(strings.TrimSpace(data.DB), "pasi") {
		data.DB = ""
	}
	return &data, nil
}

func (state *AppState) resolveUserOdooConfig(sess *SessionData) config.OdooConfig {
	state.mu.RLock()
	defaultCfg := state.cfg.Odoo
	state.mu.RUnlock()

	odooCfg := config.OdooConfig{
		URL:      DefaultOdooURL,
		DB:       "",
		Username: "",
		Password: "",
		Limit:    200,
	}

	if defaultCfg.URL != "" {
		odooCfg.URL = defaultCfg.URL
	}
	if defaultCfg.DB != "" && !strings.EqualFold(strings.TrimSpace(defaultCfg.DB), "pasi") {
		odooCfg.DB = defaultCfg.DB
	}

	foundInStore := false
	if sess != nil {
		userEmail := sess.Username
		if sess.UserEmail != "" {
			userEmail = sess.UserEmail
		}
		odooCfg.Username = userEmail

		// 1. Prioridad: Almacén persistente del usuario (independiente de la sesión de Google)
		if state.userStore != nil && userEmail != "" {
			if uSettings, ok := state.userStore.GetSettings(userEmail); ok {
				foundInStore = true
				if uSettings.OdooToken != "" {
					odooCfg.Password = uSettings.OdooToken
				}
				if uSettings.OdooUser != "" {
					odooCfg.Username = uSettings.OdooUser
				}
				if uSettings.OdooURL != "" {
					if uSettings.OdooURL == "https://www.planesnet.com" {
						odooCfg.URL = DefaultOdooURL
					} else {
						odooCfg.URL = uSettings.OdooURL
					}
				}
				if uSettings.OdooDB != "" && !strings.EqualFold(strings.TrimSpace(uSettings.OdooDB), "pasi") {
					odooCfg.DB = uSettings.OdooDB
				}
				if uSettings.PageLimit > 0 {
					odooCfg.Limit = uSettings.PageLimit
				}
			}
		}

		// 2. Si no tiene base de datos en su configuración individual, recuperar la configurada globalmente en los ajustes guardados
		if (odooCfg.DB == "" || strings.EqualFold(strings.TrimSpace(odooCfg.DB), "pasi")) && state.userStore != nil {
			if sharedDB := state.userStore.GetSharedOdooDB(); sharedDB != "" && !strings.EqualFold(strings.TrimSpace(sharedDB), "pasi") {
				odooCfg.DB = sharedDB
			}
		}

		// 3. Fallback: Base de datos, URL, usuario o contraseña en sesión de cookie
		if (odooCfg.DB == "" || strings.EqualFold(strings.TrimSpace(odooCfg.DB), "pasi")) && sess.DB != "" && !strings.EqualFold(strings.TrimSpace(sess.DB), "pasi") {
			odooCfg.DB = sess.DB
		}
		if odooCfg.Password == "" && sess.Password != "" {
			odooCfg.Password = sess.Password
		}
		if (odooCfg.URL == "" || odooCfg.URL == "https://www.planesnet.com") && sess.URL != "" && sess.URL != "https://www.planesnet.com" {
			odooCfg.URL = sess.URL
		}
		if sess.Username != "" && sess.Username != userEmail && !foundInStore {
			odooCfg.Username = sess.Username
		}

		// 4. Auto-rehidratación persistente: si la cookie contenía credenciales activas válidas y el almacén
		// no las tenía (ej. contenedor recién regenerado o volumen reiniciado), restaurarlas en userStore inmediatamente
		if !foundInStore && odooCfg.Password != "" && userEmail != "" && state.userStore != nil {
			effectiveDB := odooCfg.DB
			if strings.EqualFold(strings.TrimSpace(effectiveDB), "pasi") {
				effectiveDB = ""
			}
			if effectiveDB == "" {
				effectiveDB = DefaultOdooDB
			}
			_ = state.userStore.SaveSettings(store.UserSettings{
				Email:     userEmail,
				OdooUser:  odooCfg.Username,
				OdooToken: odooCfg.Password,
				OdooURL:   odooCfg.URL,
				OdooDB:    effectiveDB,
				PageLimit: odooCfg.Limit,
			})
			log.Printf("[AUTO-RESTORE] Configuración Odoo auto-restaurada desde la sesión para %s tras regeneración del contenedor", userEmail)
		}
	} else {
		// Sesión anónima: comprobar si hay base de datos guardada en los ajustes
		if (odooCfg.DB == "" || strings.EqualFold(strings.TrimSpace(odooCfg.DB), "pasi")) && state.userStore != nil {
			if sharedDB := state.userStore.GetSharedOdooDB(); sharedDB != "" && !strings.EqualFold(strings.TrimSpace(sharedDB), "pasi") {
				odooCfg.DB = sharedDB
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(odooCfg.DB), "pasi") {
		odooCfg.DB = ""
	}

	// 5. Fallback: Variables del sistema si aún estuvieran vacías
	if odooCfg.Password == "" && defaultCfg.Password != "" {
		odooCfg.Password = defaultCfg.Password
		if odooCfg.Username == "" {
			odooCfg.Username = defaultCfg.Username
		}
	}

	return odooCfg
}
