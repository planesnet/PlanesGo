package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"pasigo/auth"
	"pasigo/config"
	"pasigo/odoo"
	"pasigo/store"
)

const Version = "1.0.1"
const sessionCookieName = "planesgo_session"
const oauthStateCookieName = "planesgo_oauth_state"
const DefaultOdooURL = "https://www.planesnet.com"
const DefaultOdooDB = "pasi"

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
	return &data, nil
}

func (state *AppState) resolveUserOdooConfig(sess *SessionData) config.OdooConfig {
	state.mu.RLock()
	defaultCfg := state.cfg.Odoo
	state.mu.RUnlock()

	odooCfg := config.OdooConfig{
		URL:      DefaultOdooURL,
		DB:       DefaultOdooDB,
		Username: "",
		Password: "",
		Limit:    200,
	}

	if defaultCfg.URL != "" {
		odooCfg.URL = defaultCfg.URL
	}
	if defaultCfg.DB != "" {
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
					odooCfg.URL = uSettings.OdooURL
				}
				if uSettings.OdooDB != "" {
					odooCfg.DB = uSettings.OdooDB
				}
				if uSettings.PageLimit > 0 {
					odooCfg.Limit = uSettings.PageLimit
				}
			}
		}

		// 2. Fallback: Contraseña en sesión de cookie
		if odooCfg.Password == "" && sess.Password != "" {
			odooCfg.Password = sess.Password
		}
	}

	// 3. Fallback: Variables del sistema si aún estuvieran vacías
	if odooCfg.Password == "" && defaultCfg.Password != "" {
		odooCfg.Password = defaultCfg.Password
		if odooCfg.Username == "" {
			odooCfg.Username = defaultCfg.Username
		}
	}

	return odooCfg
}

func main() {
	portFlag := flag.Int("port", 0, "Puerto del servidor HTTP (opcional, sobrescribe PORT de entorno)")
	flag.Parse()

	log.Printf("Iniciando PlanesGo v%s - Odoo Timesheets & Projects...", Version)

	// Cargar configuración desde variables de entorno y .env
	cfg := config.LoadConfig()
	if *portFlag > 0 {
		cfg.Server.Port = *portFlag
	}

	// Inicializar almacén persistente de ajustes de usuario en data/user_settings.json
	userStore, err := store.NewUserSettingsStore("data/user_settings.json")
	if err != nil {
		log.Printf("[ADVERTENCIA] No se pudo inicializar store/user_settings: %v", err)
	}

	state := &AppState{
		cfg:       cfg,
		userStore: userStore,
	}

	// 0. Servir archivos estáticos (Logo corporativo, assets)
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// 1. Google OAuth 2.0 Iniciar Flujo
	http.HandleFunc("/auth/google", func(w http.ResponseWriter, r *http.Request) {
		clientID, clientSecret := state.cfg.GetGoogleAuthCredentials()
		if clientID == "" || clientSecret == "" {
			http.Error(w, "Google OAuth no configurado. Faltan las credenciales GOOGLE_CLIENT_ID y GOOGLE_CLIENT_SECRET.", http.StatusBadRequest)
			return
		}

		googleCfg := state.cfg.GoogleAuth
		googleCfg.ClientID = clientID
		googleCfg.ClientSecret = clientSecret
		googleService := auth.NewGoogleOAuthService(googleCfg)

		redirectURI := auth.CalculateRedirectURI(r)
		stateToken := auth.GenerateStateToken()

		http.SetCookie(w, &http.Cookie{
			Name:     oauthStateCookieName,
			Value:    stateToken,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   300,
			SameSite: http.SameSiteLaxMode,
		})

		url := googleService.GetAuthURL(stateToken, redirectURI)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	// 2. Google OAuth 2.0 Callback
	http.HandleFunc("/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
		stateCookie, err := r.Cookie(oauthStateCookieName)
		if err != nil || stateCookie.Value == "" {
			http.Redirect(w, r, "/login?error="+url.QueryEscape("Estado de seguridad no encontrado o sesión expirada."), http.StatusSeeOther)
			return
		}

		queryState := r.URL.Query().Get("state")
		if queryState == "" || queryState != stateCookie.Value {
			http.Redirect(w, r, "/login?error="+url.QueryEscape("Validación de seguridad OAuth inválida (CSRF detectado)."), http.StatusSeeOther)
			return
		}

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Redirect(w, r, "/login?error="+url.QueryEscape("No se recibió código de autorización de Google."), http.StatusSeeOther)
			return
		}

		clientID, clientSecret := state.cfg.GetGoogleAuthCredentials()
		googleCfg := state.cfg.GoogleAuth
		googleCfg.ClientID = clientID
		googleCfg.ClientSecret = clientSecret
		googleService := auth.NewGoogleOAuthService(googleCfg)

		redirectURI := auth.CalculateRedirectURI(r)

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		tokenResp, err := googleService.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			log.Printf("[OAUTH] Error al intercambiar código: %v", err)
			http.Redirect(w, r, "/login?error="+url.QueryEscape("No se pudo intercambiar el token con Google: "+err.Error()), http.StatusSeeOther)
			return
		}

		googleUser, err := googleService.GetUserInfo(ctx, tokenResp.AccessToken)
		if err != nil {
			log.Printf("[OAUTH] Error al obtener info de usuario: %v", err)
			http.Redirect(w, r, "/login?error="+url.QueryEscape("No se pudo obtener información de Google: "+err.Error()), http.StatusSeeOther)
			return
		}

		allowedDomain := state.cfg.GoogleAuth.AllowedDomain
		if allowedDomain != "" && !strings.HasSuffix(strings.ToLower(googleUser.Email), "@"+strings.ToLower(allowedDomain)) {
			log.Printf("[OAUTH] Dominio no autorizado: %s (requerido: %s)", googleUser.Email, allowedDomain)
			http.Redirect(w, r, "/login?error="+url.QueryEscape(fmt.Sprintf("Acceso restringido a cuentas del dominio @%s", allowedDomain)), http.StatusSeeOther)
			return
		}

		log.Printf("[OAUTH] Autenticación Google exitosa para: %s (%s)", googleUser.Email, googleUser.Name)

		// Recuperar token persistente previo del usuario si existe
		var savedToken string
		if state.userStore != nil {
			if uSettings, ok := state.userStore.GetSettings(googleUser.Email); ok {
				savedToken = uSettings.OdooToken
			}
		}

		sess := SessionData{
			URL:         DefaultOdooURL,
			DB:          DefaultOdooDB,
			Username:    googleUser.Email,
			Password:    savedToken,
			AuthMethod:  "google",
			UserEmail:   googleUser.Email,
			UserName:    googleUser.Name,
			UserPicture: googleUser.Picture,
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    encodeSession(sess),
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 30,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// 3. Manejador de la página de Login
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error cargando login.html: %v", err), http.StatusInternalServerError)
			return
		}

		state.mu.RLock()
		defaultCfg := state.cfg
		state.mu.RUnlock()

		isGoogleConfigured := defaultCfg.IsGoogleConfigured()
		var googleConfigError string
		if !isGoogleConfigured {
			googleConfigError = "Configuración OAuth no válida. No se han encontrado las variables de entorno GOOGLE_CLIENT_ID ni GOOGLE_CLIENT_SECRET en el sistema ni en .env."
		}

		if r.Method == http.MethodGet {
			errorQuery := r.URL.Query().Get("error")
			data := LoginPageData{
				Version:           Version,
				URL:               DefaultOdooURL,
				DB:                DefaultOdooDB,
				Username:          defaultCfg.Odoo.Username,
				Password:          "",
				GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
				GoogleConfigured:  isGoogleConfigured,
				GoogleConfigError: googleConfigError,
				Error:             errorQuery,
			}
			tmpl.Execute(w, data)
			return
		}

		if r.Method == http.MethodPost {
			usernameInput := strings.TrimSpace(r.FormValue("username"))
			passwordInput := r.FormValue("password")

			if usernameInput == "" || passwordInput == "" {
				data := LoginPageData{
					Version:           Version,
					URL:               DefaultOdooURL,
					DB:                DefaultOdooDB,
					Username:          usernameInput,
					Password:          passwordInput,
					GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
					GoogleConfigured:  isGoogleConfigured,
					GoogleConfigError: googleConfigError,
					Error:             "Por favor, introduce usuario y contraseña de Odoo.",
				}
				tmpl.Execute(w, data)
				return
			}

			testCfg := config.OdooConfig{
				URL:      DefaultOdooURL,
				DB:       DefaultOdooDB,
				Username: usernameInput,
				Password: passwordInput,
				Limit:    200,
			}

			client := odoo.NewClient(testCfg)
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			uid, authErr := client.Authenticate(ctx)
			if authErr != nil {
				log.Printf("[AUTH] Error de login para %s: %v", usernameInput, authErr)
				data := LoginPageData{
					Version:           Version,
					URL:               DefaultOdooURL,
					DB:                DefaultOdooDB,
					Username:          usernameInput,
					Password:          passwordInput,
					GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
					GoogleConfigured:  isGoogleConfigured,
					GoogleConfigError: googleConfigError,
					Error:             fmt.Sprintf("No se pudo iniciar sesión en Odoo: %v", authErr),
				}
				tmpl.Execute(w, data)
				return
			}

			log.Printf("[AUTH] Inicio de sesión exitoso en Odoo para %s (UID: %d)", usernameInput, uid)

			// Guardar también en almacén de usuario persistente
			if state.userStore != nil {
				_ = state.userStore.SaveSettings(store.UserSettings{
					Email:     usernameInput,
					OdooUser:  usernameInput,
					OdooToken: passwordInput,
					OdooURL:   DefaultOdooURL,
					OdooDB:    DefaultOdooDB,
					PageLimit: 200,
				})
			}

			sess := SessionData{
				URL:        DefaultOdooURL,
				DB:         DefaultOdooDB,
				Username:   usernameInput,
				Password:   passwordInput,
				AuthMethod: "odoo",
				UserName:   usernameInput,
			}

			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    encodeSession(sess),
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 30,
				SameSite: http.SameSiteLaxMode,
			})

			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	})

	// 4. Logout
	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// 5. Ajustes de Usuario (Layout 2 Columnas)
	http.HandleFunc("/settings", func(w http.ResponseWriter, r *http.Request) {
		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}

		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		userEmail := session.Username
		if session.UserEmail != "" {
			userEmail = session.UserEmail
		}

		// Obtener o inicializar ajustes persistentes del usuario
		var userSettings store.UserSettings
		if state.userStore != nil {
			if s, ok := state.userStore.GetSettings(userEmail); ok {
				userSettings = s
			}
		}

		if userSettings.Email == "" {
			userSettings.Email = userEmail
		}
		if userSettings.OdooUser == "" {
			userSettings.OdooUser = userEmail
		}
		if userSettings.OdooURL == "" {
			userSettings.OdooURL = DefaultOdooURL
		}
		if userSettings.OdooDB == "" {
			userSettings.OdooDB = DefaultOdooDB
		}
		if userSettings.PageLimit <= 0 {
			userSettings.PageLimit = 200
		}
		if userSettings.OdooToken == "" && session.Password != "" {
			userSettings.OdooToken = session.Password
		}

		tmpl, err := template.ParseFiles("templates/settings.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error al cargar plantilla settings.html: %v", err), http.StatusInternalServerError)
			return
		}

		state.mu.RLock()
		defaultCfg := state.cfg
		state.mu.RUnlock()

		if r.Method == http.MethodGet {
			data := SettingsPageData{
				Version:  Version,
				Config:   defaultCfg,
				Session:  session,
				Settings: userSettings,
			}
			tmpl.Execute(w, data)
			return
		}

		if r.Method == http.MethodPost {
			odooUser := strings.TrimSpace(r.FormValue("odoo_user"))
			odooToken := strings.TrimSpace(r.FormValue("odoo_token"))
			odooURL := strings.TrimSpace(r.FormValue("odoo_url"))
			odooDB := strings.TrimSpace(r.FormValue("odoo_db"))
			limitStr := strings.TrimSpace(r.FormValue("page_limit"))

			if odooUser == "" {
				odooUser = userEmail
			}
			if odooURL == "" {
				odooURL = userSettings.OdooURL
			}
			if odooDB == "" {
				odooDB = userSettings.OdooDB
			}
			limit := userSettings.PageLimit
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}

			updatedSettings := store.UserSettings{
				Email:     userEmail,
				OdooUser:  odooUser,
				OdooToken: odooToken,
				OdooURL:   odooURL,
				OdooDB:    odooDB,
				PageLimit: limit,
			}

			var errMsg string
			var successMsg string

			if state.userStore != nil {
				if err := state.userStore.SaveSettings(updatedSettings); err != nil {
					errMsg = fmt.Sprintf("Error guardando ajustes: %v", err)
				} else {
					successMsg = "Ajustes de usuario y token de Odoo guardados de forma persistente."
					log.Printf("[SETTINGS] Ajustes actualizados de forma persistente para %s", userEmail)
				}
			} else {
				successMsg = "Ajustes actualizados en la sesión actual."
			}

			// Actualizar cookie de sesión activa con el nuevo token
			session.Password = odooToken
			session.Username = odooUser
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    encodeSession(*session),
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 30,
				SameSite: http.SameSiteLaxMode,
			})

			data := SettingsPageData{
				Version:        Version,
				Config:         defaultCfg,
				Session:        session,
				Settings:       updatedSettings,
				SuccessMessage: successMsg,
				ErrorMessage:   errMsg,
			}
			tmpl.Execute(w, data)
		}
	})

	// 6. API JSON para Probar Conexión con Odoo
	http.HandleFunc("/api/settings/test-connection", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			OdooUser  string `json:"odoo_user"`
			OdooToken string `json:"odoo_token"`
			OdooURL   string `json:"odoo_url"`
			OdooDB    string `json:"odoo_db"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Formato JSON inválido"})
			return
		}

		if payload.OdooToken == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "El token de acceso no puede estar vacío."})
			return
		}

		if payload.OdooURL == "" {
			payload.OdooURL = DefaultOdooURL
		}
		if payload.OdooDB == "" {
			payload.OdooDB = DefaultOdooDB
		}

		testCfg := config.OdooConfig{
			URL:      payload.OdooURL,
			DB:       payload.OdooDB,
			Username: payload.OdooUser,
			Password: payload.OdooToken,
			Limit:    10,
		}

		client := odoo.NewClient(testCfg)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		uid, err := client.Authenticate(ctx)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "uid": uid})
	})

	// 7. Página Principal (Protegida)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}

		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Resolver credenciales personalizadas de Odoo (persistent store > session > fallback)
		currentOdooCfg := state.resolveUserOdooConfig(session)

		activeCfg := &config.Config{
			Server: config.ServerConfig{Port: cfg.Server.Port},
			Odoo:   currentOdooCfg,
		}

		var entries []odoo.TimesheetEntry
		var projects []odoo.Project
		var fetchErr error
		hasOdooToken := (currentOdooCfg.Password != "")

		if hasOdooToken {
			client := odoo.NewClient(currentOdooCfg)
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()

			projList, pErr := client.GetProjects(ctx, nil)
			if pErr != nil {
				log.Printf("[ADVERTENCIA] Error al obtener proyectos de Odoo: %v", pErr)
			} else {
				projects = projList
			}

			tsEntries, tsErr := client.GetTimesheets(ctx, nil)
			if tsErr != nil {
				log.Printf("[ADVERTENCIA] Error al obtener partes de horas: %v", tsErr)
				if fetchErr == nil && pErr != nil {
					fetchErr = tsErr
				}
			} else {
				entries = tsEntries
			}
		}

		projectHoursMap := make(map[int]float64)
		projectCountMap := make(map[int]int)
		projectNameHoursMap := make(map[string]float64)
		projectNameCountMap := make(map[string]int)

		var totalHours float64
		projectMap := make(map[string]bool)
		employeeMap := make(map[string]bool)

		for _, entry := range entries {
			totalHours += entry.UnitAmount
			if entry.ProjectID.ID > 0 {
				projectHoursMap[entry.ProjectID.ID] += entry.UnitAmount
				projectCountMap[entry.ProjectID.ID]++
			}
			if entry.ProjectID.Name != "" {
				projectMap[entry.ProjectID.Name] = true
				projectNameHoursMap[entry.ProjectID.Name] += entry.UnitAmount
				projectNameCountMap[entry.ProjectID.Name]++
			}
			emp := entry.DisplayEmployee()
			if emp != "" && emp != "Sin asignar" {
				employeeMap[emp] = true
			}
		}

		for i := range projects {
			pName := projects[i].DisplayNameOrName()
			if pName != "" {
				projectMap[pName] = true
			}
			if h, ok := projectHoursMap[projects[i].ID]; ok {
				projects[i].TotalHours = h
				projects[i].TimesheetCount = projectCountMap[projects[i].ID]
			} else if h, ok := projectNameHoursMap[pName]; ok {
				projects[i].TotalHours = h
				projects[i].TimesheetCount = projectNameCountMap[pName]
			}
		}

		var projectsList []string
		for p := range projectMap {
			if p != "" && p != "-" {
				projectsList = append(projectsList, p)
			}
		}
		sort.Strings(projectsList)

		var employeesList []string
		for e := range employeeMap {
			employeesList = append(employeesList, e)
		}
		sort.Strings(employeesList)

		errMsg := ""
		if fetchErr != nil {
			errMsg = fetchErr.Error()
			log.Printf("[ERROR] Consulta Odoo: %v", fetchErr)
		}

		data := PageData{
			Version:              Version,
			Config:               activeCfg,
			Session:              session,
			HasOdooToken:         hasOdooToken,
			Entries:              entries,
			Projects:             projects,
			TotalHours:           totalHours,
			TotalProjectsCount:   len(projects),
			UniqueProjectsCount:  len(projectMap),
			UniqueEmployeesCount: len(employeeMap),
			ProjectsList:         projectsList,
			EmployeesList:        employeesList,
			Error:                errMsg,
		}

		tmpl, err := template.ParseFiles("templates/index.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error al cargar plantilla: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("[ERROR] Renderizado de plantilla: %v", err)
		}
	})

	// 8. API JSON Partes de horas
	http.HandleFunc("/api/timesheets", func(w http.ResponseWriter, r *http.Request) {
		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}

		odooCfg := state.resolveUserOdooConfig(session)
		client := odoo.NewClient(odooCfg)

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		entries, err := client.GetTimesheets(ctx, nil)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(entries)
	})

	// 9. API JSON Proyectos Odoo
	http.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}

		odooCfg := state.resolveUserOdooConfig(session)
		client := odoo.NewClient(odooCfg)

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		projects, err := client.GetProjects(ctx, nil)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(projects)
	})

	// 10. Servidor HTTP
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("=======================================================")
		log.Printf(" Servidor PlanesGo v%s listo en: http://localhost:%d", Version, cfg.Server.Port)
		log.Printf(" Configuración: Variables de entorno del sistema / .env")
		if cfg.GoogleAuth.ClientID != "" {
			log.Printf(" Google OAuth 2.0: ACTIVO (Client ID: %s...)", cfg.GoogleAuth.ClientID[:min(len(cfg.GoogleAuth.ClientID), 12)])
		} else {
			log.Printf(" Google OAuth 2.0: Inactivo (configurar GOOGLE_CLIENT_ID en .env o entorno)")
		}
		log.Printf("=======================================================")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error al iniciar servidor: %v", err)
		}
	}()

	// Manejo de apagado elegante (Ctrl+C / SIGTERM)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Cerrando servidor...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error durante el cierre del servidor: %v", err)
	}
	log.Println("PlanesGo detenido correctamente.")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
