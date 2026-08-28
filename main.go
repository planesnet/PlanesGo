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
	"strings"
	"sync"
	"syscall"
	"time"

	"pasigo/auth"
	"pasigo/config"
	"pasigo/odoo"
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
	mu         sync.RWMutex
	cfg        *config.Config
	configPath string
}

type PageData struct {
	Version              string
	Config               *config.Config
	Session              *SessionData
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

func main() {
	configPath := flag.String("config", "config.yml", "Ruta al archivo de configuración YAML")
	flag.Parse()

	log.Printf("Iniciando PlanesGo v%s - Odoo Timesheets & Projects...", Version)

	// Cargar archivo de configuración
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Printf("[ADVERTENCIA] No se pudo cargar %s: %v", *configPath, err)
		cfg = &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Odoo: config.OdooConfig{
				URL:      DefaultOdooURL,
				DB:       DefaultOdooDB,
				Username: "",
				Password: "",
				Limit:    200,
			},
			GoogleAuth: config.GoogleAuthConfig{
				Enabled:     false,
				RedirectURL: "http://localhost:8080/auth/google/callback",
			},
		}
	}

	state := &AppState{
		cfg:        cfg,
		configPath: *configPath,
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

		sess := SessionData{
			URL:         DefaultOdooURL,
			DB:          DefaultOdooDB,
			Username:    googleUser.Email,
			Password:    "",
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

			state.mu.Lock()
			state.cfg.Odoo = testCfg
			if err := config.SaveConfig(state.configPath, state.cfg); err != nil {
				log.Printf("[ADVERTENCIA] No se pudo actualizar %s: %v", state.configPath, err)
			} else {
				log.Printf("[CONFIG] Guardado último login en %s para %s", state.configPath, usernameInput)
			}
			state.mu.Unlock()

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

	// 5. Página Principal (Protegida)
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
			state.mu.RLock()
			savedCfg := state.cfg.Odoo
			state.mu.RUnlock()
			if savedCfg.Username != "" && savedCfg.Password != "" {
				session = &SessionData{
					URL:        DefaultOdooURL,
					DB:         DefaultOdooDB,
					Username:   savedCfg.Username,
					Password:   savedCfg.Password,
					AuthMethod: "odoo",
					UserName:   savedCfg.Username,
				}
			}
		}

		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		state.mu.RLock()
		savedCfg := state.cfg.Odoo
		state.mu.RUnlock()

		odooUser := session.Username
		odooPass := session.Password
		if odooPass == "" && savedCfg.Password != "" {
			odooPass = savedCfg.Password
			if odooUser == "" {
				odooUser = savedCfg.Username
			}
		}

		currentOdooCfg := config.OdooConfig{
			URL:      savedCfg.URL,
			DB:       savedCfg.DB,
			Username: odooUser,
			Password: odooPass,
			Limit:    savedCfg.Limit,
		}
		if currentOdooCfg.URL == "" {
			currentOdooCfg.URL = DefaultOdooURL
		}
		if currentOdooCfg.DB == "" {
			currentOdooCfg.DB = DefaultOdooDB
		}
		if currentOdooCfg.Limit <= 0 {
			currentOdooCfg.Limit = 200
		}

		activeCfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Odoo:   currentOdooCfg,
		}

		var entries []odoo.TimesheetEntry
		var projects []odoo.Project
		var fetchErr error

		if currentOdooCfg.Password != "" {
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

	// 6. API JSON Partes de horas
	http.HandleFunc("/api/timesheets", func(w http.ResponseWriter, r *http.Request) {
		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
		if session == nil {
			state.mu.RLock()
			savedCfg := state.cfg.Odoo
			state.mu.RUnlock()
			session = &SessionData{
				URL:      DefaultOdooURL,
				DB:       DefaultOdooDB,
				Username: savedCfg.Username,
				Password: savedCfg.Password,
			}
		}

		client := odoo.NewClient(config.OdooConfig{
			URL:      DefaultOdooURL,
			DB:       DefaultOdooDB,
			Username: session.Username,
			Password: session.Password,
			Limit:    200,
		})

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

	// 7. API JSON Proyectos Odoo
	http.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		var session *SessionData
		cookie, err := r.Cookie(sessionCookieName)
		if err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
		if session == nil {
			state.mu.RLock()
			savedCfg := state.cfg.Odoo
			state.mu.RUnlock()
			session = &SessionData{
				URL:      DefaultOdooURL,
				DB:       DefaultOdooDB,
				Username: savedCfg.Username,
				Password: savedCfg.Password,
			}
		}

		client := odoo.NewClient(config.OdooConfig{
			URL:      DefaultOdooURL,
			DB:       DefaultOdooDB,
			Username: session.Username,
			Password: session.Password,
			Limit:    200,
		})

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

	// 7. Servidor HTTP
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
		log.Printf(" Archivo de configuración activo: %s", *configPath)
		if cfg.GoogleAuth.ClientID != "" {
			log.Printf(" Google OAuth 2.0: ACTIVO (Client ID: %s...)", cfg.GoogleAuth.ClientID[:min(len(cfg.GoogleAuth.ClientID), 12)])
		} else {
			log.Printf(" Google OAuth 2.0: Disponible (configurar en config.yml o variables de entorno)")
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
