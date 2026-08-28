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

	"pasigo/config"
	"pasigo/odoo"
)

const Version = "1.0.1"
const sessionCookieName = "planesgo_session"
const DefaultOdooURL = "https://www.planesnet.com"
const DefaultOdooDB = "pasi"

type SessionData struct {
	URL      string `json:"url"`
	DB       string `json:"db"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type AppState struct {
	mu         sync.RWMutex
	cfg        *config.Config
	configPath string
}

type PageData struct {
	Version              string
	Config               *config.Config
	Entries              []odoo.TimesheetEntry
	TotalHours           float64
	UniqueProjectsCount  int
	UniqueEmployeesCount int
	ProjectsList         []string
	EmployeesList        []string
	Error                string
}

type LoginPageData struct {
	Version  string
	URL      string
	DB       string
	Username string
	Password string
	Error    string
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
				URL:      "https://www.planesnet.com",
				DB:       "pasi",
				Username: "",
				Password: "",
				Limit:    200,
			},
		}
	}

	state := &AppState{
		cfg:        cfg,
		configPath: *configPath,
	}

	// 1. Manejador de la página de Login
	http.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		defaultCfg := *state.cfg
		state.mu.RUnlock()

		tmpl, err := template.ParseFiles("templates/login.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Error cargando plantilla: %v", err), http.StatusInternalServerError)
			return
		}

		if r.Method == http.MethodGet {
			// Si ya hay sesión válida por cookie o config completa, pre-llenar campos
			urlVal := defaultCfg.Odoo.URL
			if urlVal == "" {
				urlVal = "https://www.planesnet.com"
			}

			data := LoginPageData{
				Version:  Version,
				URL:      urlVal,
				DB:       defaultCfg.Odoo.DB,
				Username: defaultCfg.Odoo.Username,
				Password: defaultCfg.Odoo.Password,
			}
			tmpl.Execute(w, data)
			return
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Error procesando formulario", http.StatusBadRequest)
				return
			}

			urlInput := strings.TrimRight(strings.TrimSpace(r.FormValue("url")), "/")
			if urlInput == "" {
				urlInput = defaultCfg.Odoo.URL
				if urlInput == "" {
					urlInput = "https://www.planesnet.com"
				}
			}

			dbInput := strings.TrimSpace(r.FormValue("db"))
			if dbInput == "" {
				dbInput = defaultCfg.Odoo.DB
				if dbInput == "" {
					dbInput = "pasi"
				}
			}

			usernameInput := strings.TrimSpace(r.FormValue("username"))
			passwordInput := r.FormValue("password")
			saveConfig := r.FormValue("save_config") == "true"

			if usernameInput == "" || passwordInput == "" {
				data := LoginPageData{
					Version:  Version,
					URL:      urlInput,
					DB:       dbInput,
					Username: usernameInput,
					Password: passwordInput,
					Error:    "Por favor, introduce tu usuario y contraseña.",
				}
				tmpl.Execute(w, data)
				return
			}

			// Validar contra Odoo
			testCfg := config.OdooConfig{
				URL:      urlInput,
				DB:       dbInput,
				Username: usernameInput,
				Password: passwordInput,
				Limit:    200,
			}

			client := odoo.NewClient(testCfg)
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			uid, authErr := client.Authenticate(ctx)
			if authErr != nil {
				log.Printf("[AUTH] Error de login para %s en %s: %v", usernameInput, urlInput, authErr)
				data := LoginPageData{
					Version:  Version,
					URL:      urlInput,
					DB:       dbInput,
					Username: usernameInput,
					Password: passwordInput,
					Error:    fmt.Sprintf("No se pudo iniciar sesión: %v", authErr),
				}
				tmpl.Execute(w, data)
				return
			}

			log.Printf("[AUTH] Inicio de sesión exitoso para %s (UID: %d)", usernameInput, uid)

			// Guardar en config.yml si se solicitó
			if saveConfig {
				state.mu.Lock()
				state.cfg.Odoo = testCfg
				if err := config.SaveConfig(state.configPath, state.cfg); err != nil {
					log.Printf("[ADVERTENCIA] No se pudo guardar config.yml: %v", err)
				}
				state.mu.Unlock()
			}

			// Establecer cookie de sesión
			sess := SessionData{
				URL:      urlInput,
				DB:       dbInput,
				Username: usernameInput,
				Password: passwordInput,
			}
			http.SetCookie(w, &http.Cookie{
				Name:     sessionCookieName,
				Value:    encodeSession(sess),
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400 * 7, // 7 días
			})

			http.Redirect(w, r, "/", http.StatusSeeOther)
		}
	})

	// 2. Cerrar sesión
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

	// 3. Página Principal (Protegida)
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

		// Si no hay sesión en cookie, comprobar si config.yml tiene credenciales
		if session == nil {
			state.mu.RLock()
			savedCfg := state.cfg.Odoo
			state.mu.RUnlock()
			if savedCfg.URL != "" && savedCfg.DB != "" && savedCfg.Username != "" && savedCfg.Password != "" {
				session = &SessionData{
					URL:      savedCfg.URL,
					DB:       savedCfg.DB,
					Username: savedCfg.Username,
					Password: savedCfg.Password,
				}
			}
		}

		if session == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		currentOdooCfg := config.OdooConfig{
			URL:      session.URL,
			DB:       session.DB,
			Username: session.Username,
			Password: session.Password,
			Limit:    200,
		}

		activeCfg := &config.Config{
			Server: config.ServerConfig{Port: 8080},
			Odoo:   currentOdooCfg,
		}

		client := odoo.NewClient(currentOdooCfg)
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()

		entries, fetchErr := client.GetTimesheets(ctx, nil)

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

	// 4. API JSON
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
				URL:      savedCfg.URL,
				DB:       savedCfg.DB,
				Username: savedCfg.Username,
				Password: savedCfg.Password,
			}
		}

		client := odoo.NewClient(config.OdooConfig{
			URL:      session.URL,
			DB:       session.DB,
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

	// 5. Servidor HTTP
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
