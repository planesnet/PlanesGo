package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	mu        sync.RWMutex
	cfg       *config.Config
	userStore *store.UserSettingsStore
}

type WorkerRecentProject struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	LastDate   string  `json:"last_date"`
	TotalHours float64 `json:"total_hours"`
	EntryCount int     `json:"entry_count"`
	LastTask   string  `json:"last_task"`
	Employee   string  `json:"employee"`
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
	Config               *config.Config
	Session              *SessionData
	HasOdooToken         bool
	Entries              []odoo.TimesheetEntry
	Projects             []odoo.Project
	TotalHours           float64
	TotalProjectsCount   int
	UniqueProjectsCount  int
	UniqueEmployeesCount int
	ProjectsList         []string
	EmployeesList        []string
	CurrentWorker        string
	RecentProjects       []WorkerRecentProject
	Error                string
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

	if sess != nil {
		userEmail := sess.Username
		if sess.UserEmail != "" {
			userEmail = sess.UserEmail
		}
		odooCfg.Username = userEmail

		// 1. Prioridad: Almacén persistente del usuario (independiente de la sesión de Google)
		if state.userStore != nil && userEmail != "" {
			if uSettings, ok := state.userStore.GetSettings(userEmail); ok {
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
		if odooCfg.DB == "" && state.userStore != nil {
			if sharedDB := state.userStore.GetSharedOdooDB(); sharedDB != "" && !strings.EqualFold(strings.TrimSpace(sharedDB), "pasi") {
				odooCfg.DB = sharedDB
			}
		}

		// 3. Fallback: Base de datos o contraseña en sesión de cookie
		if odooCfg.DB == "" && sess.DB != "" && !strings.EqualFold(strings.TrimSpace(sess.DB), "pasi") {
			odooCfg.DB = sess.DB
		}
		if odooCfg.Password == "" && sess.Password != "" {
			odooCfg.Password = sess.Password
		}
	} else {
		// Sesión anónima: comprobar si hay base de datos guardada en los ajustes
		if odooCfg.DB == "" && state.userStore != nil {
			if sharedDB := state.userStore.GetSharedOdooDB(); sharedDB != "" && !strings.EqualFold(strings.TrimSpace(sharedDB), "pasi") {
				odooCfg.DB = sharedDB
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(odooCfg.DB), "pasi") {
		odooCfg.DB = ""
	}

	// 4. Fallback: Variables del sistema si aún estuvieran vacías
	if odooCfg.Password == "" && defaultCfg.Password != "" {
		odooCfg.Password = defaultCfg.Password
		if odooCfg.Username == "" {
			odooCfg.Username = defaultCfg.Username
		}
	}

	return odooCfg
}
