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
	TotalHours           float64
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

	log.Printf("Iniciando PlanesGo v%s - Odoo Timesheets...", Version)

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
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// 1. Google OAuth 2.0 Iniciar Flujo
	http.HandleFunc("/auth/google", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		googleCfg := state.cfg.GoogleAuth
		state.mu.RUnlock()

		if googleCfg.ClientID == "" || googleCfg.ClientSecret == "" {
			http.Redirect(w, r, "/login?error=Google+OAuth+no+está+configurado.+Configura+client_id+y+client_secret+en+config.yml", http.StatusSeeOther)
			return
		}

		googleService := auth.NewGoogleOAuthService(googleCfg)
		oauthState := auth.GenerateStateToken()
		redirectURI := googleService.ResolveRedirectURI(r)

		// Guardar state y redirectURI en cookies HttpOnly temporales
		http.SetCookie(w, &http.Cookie{
			Name:     oauthStateCookieName,
			Value:    oauthState,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   300, // 5 minutos
			SameSite: http.SameSiteLaxMode,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "planesgo_oauth_redirect_uri",
			Value:    redirectURI,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   300,
			SameSite: http.SameSiteLaxMode,
		})

		authURL := googleService.GetAuthURL(oauthState, redirectURI)
		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	})

	// 2. Google OAuth 2.0 Callback
	http.HandleFunc("/auth/google/callback", func(w http.ResponseWriter, r *http.Request) {
		// Validar posible error devuelto por Google
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			log.Printf("[GOOGLE_AUTH] Error recibido de Google: %s", errParam)
			http.Redirect(w, r, "/login?error=Acceso+cancelado+por+el+usuario", http.StatusSeeOther)
			return
		}

		// Validar State CSRF
		stateParam := r.URL.Query().Get("state")
		stateCookie, err := r.Cookie(oauthStateCookieName)
		if err != nil || stateCookie.Value == "" || stateCookie.Value != stateParam {
			log.Printf("[GOOGLE_AUTH] Error de validación CSRF State")
			http.Redirect(w, r, "/login?error=Error+de+seguridad+CSRF+en+Google+Auth", http.StatusSeeOther)
			return
		}

		// Obtener redirectURI usada en la solicitud
		redirectURI := ""
		if redirectCookie, err := r.Cookie("planesgo_oauth_redirect_uri"); err == nil && redirectCookie.Value != "" {
			redirectURI = redirectCookie.Value
		}

		// Limpiar cookies de state
		http.SetCookie(w, &http.Cookie{
			Name:     oauthStateCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})
		http.SetCookie(w, &http.Cookie{
			Name:     "planesgo_oauth_redirect_uri",
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})

		code := r.URL.Query().Get("code")
		if code == "" {
			http.Redirect(w, r, "/login?error=Código+de+autorización+no+recibido", http.StatusSeeOther)
			return
		}

		state.mu.RLock()
		googleCfg := state.cfg.GoogleAuth
		savedOdooCfg := state.cfg.Odoo
		state.mu.RUnlock()

		googleService := auth.NewGoogleOAuthService(googleCfg)
		if redirectURI == "" {
			redirectURI = googleService.ResolveRedirectURI(r)
		}

		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()

		tokenResp, err := googleService.ExchangeCode(ctx, code, redirectURI)
		if err != nil {
			log.Printf("[GOOGLE_AUTH] Error intercambiando código: %v", err)
			http.Redirect(w, r, fmt.Sprintf("/login?error=%s", "Error al validar token de Google"), http.StatusSeeOther)
			return
		}

		userInfo, err := googleService.GetUserInfo(ctx, tokenResp.AccessToken)
		if err != nil {
			log.Printf("[GOOGLE_AUTH] Error obteniendo perfil de Google: %v", err)
			http.Redirect(w, r, fmt.Sprintf("/login?error=%s", "Error al obtener perfil de Google"), http.StatusSeeOther)
			return
		}

		log.Printf("[GOOGLE_AUTH] Usuario autenticado con éxito vía Google: %s (%s)", userInfo.Name, userInfo.Email)

		// Crear sesión validada con Google OAuth
		sess := SessionData{
			URL:         savedOdooCfg.URL,
			DB:          savedOdooCfg.DB,
			Username:    savedOdooCfg.Username,
			Password:    savedOdooCfg.Password,
			AuthMethod:  "google",
			UserEmail:   userInfo.Email,
			UserName:    userInfo.Name,
			UserPicture: userInfo.Picture,
		}

		// Si no hay usuario de Odoo configurado, usar el email de Google
		if sess.Username == "" {
			sess.Username = userInfo.Email
		}
		if sess.URL == "" {
			sess.URL = DefaultOdooURL
		}
		if sess.DB == "" {
			sess.DB = DefaultOdooDB
		}

		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    encodeSession(sess),
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 30, // 30 días
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// 3. Manejador de la página de Login
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		defaultCfg := *state.cfg
		state.mu.RUnlock()

		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error cargando plantilla: %v", err), http.StatusInternalServerError)
			return
		}

		urlError := r.URL.Query().Get("error")

		isGoogleConfigured := defaultCfg.GoogleAuth.ClientID != "" && defaultCfg.GoogleAuth.ClientSecret != ""

		if r.Method == http.MethodGet {
			data := LoginPageData{
				Version:           Version,
				URL:               DefaultOdooURL,
				DB:                DefaultOdooDB,
				Username:          defaultCfg.Odoo.Username,
				Password:          defaultCfg.Odoo.Password,
				GoogleAuthEnabled: defaultCfg.GoogleAuth.Enabled || isGoogleConfigured,
				GoogleConfigured:  isGoogleConfigured,
				Error:             urlError,
			}
			tmpl.Execute(w, data)
			return
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Error procesando formulario", http.StatusBadRequest)
				return
			}

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
					Error:             "Por favor, introduce tu usuario y contraseña.",
				}
				tmpl.Execute(w, data)
				return
			}

			// Validar contra Odoo
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
					Error:             fmt.Sprintf("No se pudo iniciar sesión en Odoo: %v", authErr),
				}
				tmpl.Execute(w, data)
				return
			}

			log.Printf("[AUTH] Inicio de sesión exitoso en Odoo para %s (UID: %d)", usernameInput, uid)

			// Guardar siempre el último login en config.yml automáticamente
			state.mu.Lock()
			state.cfg.Odoo = testCfg
			if err := config.SaveConfig(state.configPath, state.cfg); err != nil {
				log.Printf("[ADVERTENCIA] No se pudo actualizar %s: %v", state.configPath, err)
			} else {
				log.Printf("[CONFIG] Guardado último login en %s para %s", state.configPath, usernameInput)
			}
			state.mu.Unlock()

			// Establecer cookie de sesión
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
				MaxAge:   86400 * 30, // 30 días
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

		// Si no hay sesión en cookie, comprobar si config.yml tiene credenciales de Odoo
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
			URL:      DefaultOdooURL,
			DB:       DefaultOdooDB,
			Username: odooUser,
			Password: odooPass,
			Limit:    200,
		}

		activeCfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Odoo:   currentOdooCfg,
		}

		var entries []odoo.TimesheetEntry
		var fetchErr error

		if currentOdooCfg.Password != "" {
			client := odoo.NewClient(currentOdooCfg)
			ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
			defer cancel()
			entries, fetchErr = client.GetTimesheets(ctx, nil)
		}

		// Calcular métricas
		var totalHours float64
		projectMap := make(map[string]bool)
		employeeMap := make(map[string]bool)

		for _, entry := range entries {
			totalHours += entry.UnitAmount
			if entry.ProjectID.Name != "" {
				projectMap[entry.ProjectID.Name] = true
			}
			emp := entry.DisplayEmployee()
			if emp != "" && emp != "Sin asignar" {
				employeeMap[emp] = true
			}
		}

		var projectsList []string
		for p := range projectMap {
			projectsList = append(projectsList, p)
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
			TotalHours:           totalHours,
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

	// 6. API JSON
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
