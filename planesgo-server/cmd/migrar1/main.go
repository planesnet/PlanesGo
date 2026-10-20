package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"pasigo/cmd/migrar1/db"
	"pasigo/cmd/migrar1/ui"
)

const (
	AppName    = "Migrar1"
	AppVersion = "1.0.0"
)

func openBrowser(url string) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		// En Linux, intentar abrir en modo app de Chrome/Chromium si está disponible para experiencia de escritorio nativa
		chromePaths := []string{"google-chrome", "chromium", "chromium-browser", "brave-browser"}
		for _, browser := range chromePaths {
			if path, err := exec.LookPath(browser); err == nil {
				cmd = exec.Command(path, fmt.Sprintf("--app=%s", url))
				if err := cmd.Start(); err == nil {
					return
				}
			}
		}
		// Fallback a xdg-open
		cmd = exec.Command("xdg-open", url)
	}

	_ = cmd.Start()
}

func main() {
	portFlag := flag.Int("port", 8990, "Puerto HTTP local para la aplicación de escritorio")
	dbFlag := flag.String("db", "migrar1.db", "Ruta de la base de datos SQLite de trazabilidad")
	noBrowserFlag := flag.Bool("no-browser", false, "No abrir automáticamente el navegador")
	flag.Parse()

	fmt.Println("==================================================================")
	fmt.Printf("🚀 %s v%s - Sistema de Migración Idempotente de Odoo\n", AppName, AppVersion)
	fmt.Println("   Origen:  https://planesnet.autopyme.com")
	fmt.Println("   Destino: https://pasi-test.autopyme.com")
	fmt.Println("==================================================================")

	// 1. Inicializar base de datos SQLite
	database, err := db.Open(*dbFlag)
	if err != nil {
		log.Fatalf("❌ Error crítico abriendo base de datos SQLite (%s): %v\n", *dbFlag, err)
	}
	defer database.Close()
	fmt.Printf("✓ Base de datos SQLite inicializada: %s\n", *dbFlag)

	// 2. Iniciar servidor de interfaz de escritorio
	server := ui.NewServer(database, *portFlag)
	actualPort, err := server.Start()
	if err != nil {
		log.Fatalf("❌ Error iniciando servidor UI: %v\n", err)
	}

	appURL := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Printf("✓ Panel de control disponible en: %s\n", appURL)

	// 3. Abrir ventana de escritorio / navegador
	if !*noBrowserFlag {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(appURL)
		}()
	}

	fmt.Println("\n📌 Presiona Ctrl+C para detener la aplicación.")

	// 4. Captura de señales para cierre limpio
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	<-stopChan
	fmt.Println("\nCerrando Migrar1 de forma segura...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		log.Printf("Advertencia al apagar servidor: %v\n", err)
	}
	fmt.Println("✓ Migrar1 finalizado correctamente.")
}
