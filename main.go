package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pasigo/config"
	"pasigo/store"
)

var Version = "1.1.0"

func setupRoutes(mux *http.ServeMux, state *AppState) {
	// Archivos estáticos
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Dashboard principal
	mux.HandleFunc("/", state.handleDashboard)

	// Autenticación
	mux.HandleFunc("/login", state.handleLogin)
	mux.HandleFunc("/logout", state.handleLogout)
	mux.HandleFunc("/auth/google", state.handleGoogleAuth)
	mux.HandleFunc("/auth/google/callback", state.handleGoogleCallback)

	// Configuración y ajustes
	mux.HandleFunc("/settings", state.handleSettings)
	mux.HandleFunc("/api/settings/test-connection", state.handleTestConnection)

	// API JSON
	mux.HandleFunc("/api/timesheets", state.handleAPITimesheets)
	mux.HandleFunc("/api/timesheets/update", state.handleAPITimesheetsUpdate)
	mux.HandleFunc("/api/timesheets/delete", state.handleAPITimesheetsDelete)
	mux.HandleFunc("/api/tasks", state.handleAPITasks)
	mux.HandleFunc("/api/projects", state.handleAPIProjects)
	mux.HandleFunc("/api/tickets", state.handleAPITickets)
	mux.HandleFunc("/api/timer/active", state.handleAPITimerActive)
	mux.HandleFunc("/api/timer/start", state.handleAPITimerStart)
	mux.HandleFunc("/api/timer/pause", state.handleAPITimerPause)
	mux.HandleFunc("/api/timer/resume", state.handleAPITimerResume)
	mux.HandleFunc("/api/timer/stop", state.handleAPITimerStop)

	// Health check y ping
	mux.HandleFunc("/health", state.handleHealth)
	mux.HandleFunc("/ping", state.handlePing)
}

func main() {
	portFlag := flag.Int("port", 0, "Puerto del servidor HTTP (opcional, sobrescribe PORT de entorno)")
	versionFlag := flag.Bool("version", false, "Muestra la versión de la aplicación y termina")
	vFlag := flag.Bool("v", false, "Muestra la versión de la aplicación y termina")
	flag.Parse()

	if *versionFlag || *vFlag {
		fmt.Printf("PlanesGo v%s\n", Version)
		return
	}

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

	mux := http.NewServeMux()
	setupRoutes(mux, state)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      loggingAndRecoveryMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
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
