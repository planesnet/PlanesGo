package main

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"pasigo/config"
	"pasigo/odoo"
)

func TestIndexTemplateParsingAndRendering(t *testing.T) {
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		t.Fatalf("Error al parsear templates/index.html: %v", err)
	}
	if _, err := tmpl.ParseGlob("templates/partials/*.html"); err != nil {
		t.Fatalf("Error al parsear templates/partials/*.html: %v", err)
	}

	cfg := &config.Config{}
	cfg.Odoo.DB = "ap113"
	cfg.Odoo.URL = "https://planesnet.autopyme.com"

	data := PageData{
		Version:      "1.1.0",
		Config:       cfg,
		Session:      &SessionData{Username: "test@planesnet.com", UserName: "Test User"},
		HasOdooToken: true,
		Entries: []odoo.TimesheetEntry{
			{
				ID:         1,
				Date:       "2026-09-09",
				EmployeeID: odoo.Many2One{ID: 5, Name: "Test User"},
				ProjectID:  odoo.Many2One{ID: 10, Name: "Proyecto Test"},
				TaskID:     odoo.Many2One{ID: 20, Name: "Tarea Test"},
				Name:       "Desarrollo modular",
				UnitAmount: 2.5,
			},
		},
		Projects: []odoo.Project{
			{
				ID:   10,
				Name: "Proyecto Test",
			},
		},
		RecentProjects: []WorkerRecentProject{
			{
				ID:         10,
				Name:       "Proyecto Test",
				TotalHours: 2.5,
				EntryCount: 1,
				LastTask:   "Tarea Test",
			},
		},
		TotalHours:           2.5,
		UniqueProjectsCount:  1,
		UniqueEmployeesCount: 1,
		ProjectsList:         []string{"Proyecto Test"},
		EmployeesList:        []string{"Test User"},
		CurrentWorker:        "Test User",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("Error al ejecutar plantilla index: %v", err)
	}
	rendered := buf.String()
	if bytes.Contains([]byte(rendered), []byte("Todos los trabajadores")) {
		t.Fatalf("La opción 'Todos los trabajadores' no debe estar presente en la lista de trabajadores")
	}
	if !bytes.Contains([]byte(rendered), []byte("Test User")) {
		t.Fatalf("El trabajador 'Test User' debería estar presente en el HTML renderizado")
	}
	if !bytes.Contains([]byte(rendered), []byte("col-project-header")) {
		t.Fatalf("La cabecera de la columna proyecto debe contener la clase 'col-project-header'")
	}
	if !bytes.Contains([]byte(rendered), []byte("col-project-cell")) {
		t.Fatalf("La celda de la columna proyecto debe contener la clase 'col-project-cell'")
	}
}

func TestSettingsTemplateRendering(t *testing.T) {
	tmpl, err := template.ParseFiles("templates/settings.html")
	if err != nil {
		t.Fatalf("Error al parsear templates/settings.html: %v", err)
	}

	cfg := &config.Config{}
	cfg.Odoo.DB = "ap113"
	cfg.Odoo.URL = "https://planesnet.autopyme.com"

	// Probar renderizado con sesión anónima (cuando Odoo no conecta y el usuario no ha iniciado sesión)
	anonSession := &SessionData{
		Username:   "Configuración",
		UserEmail:  "",
		URL:        "https://planesnet.autopyme.com",
		DB:         "ap113",
		AuthMethod: "local",
	}

	dataAnon := SettingsPageData{
		Version: "1.1.0",
		Config:  cfg,
		Session: anonSession,
	}

	var bufAnon bytes.Buffer
	if err := tmpl.Execute(&bufAnon, dataAnon); err != nil {
		t.Fatalf("Error al renderizar settings con sesión anónima: %v", err)
	}
	if bufAnon.Len() == 0 {
		t.Fatalf("El contenido renderizado para sesión anónima está vacío")
	}

	// Probar renderizado con sesión normal
	userSession := &SessionData{
		Username:   "luis@planesnet.com",
		UserEmail:  "luis@planesnet.com",
		Password:   "secret-token",
		URL:        "https://planesnet.autopyme.com",
		DB:         "ap113",
		AuthMethod: "local",
	}

	dataUser := SettingsPageData{
		Version: "1.1.0",
		Config:  cfg,
		Session: userSession,
	}

	var bufUser bytes.Buffer
	if err := tmpl.Execute(&bufUser, dataUser); err != nil {
		t.Fatalf("Error al renderizar settings con sesión de usuario: %v", err)
	}
	if bufUser.Len() == 0 {
		t.Fatalf("El contenido renderizado para usuario está vacío")
	}
}

func TestGetIndexTemplateCache(t *testing.T) {
	tmpl1, err := getIndexTemplate()
	if err != nil {
		t.Fatalf("Error al obtener plantilla index: %v", err)
	}
	if tmpl1 == nil {
		t.Fatalf("Plantilla obtenida es nil")
	}

	tmpl2, err := getIndexTemplate()
	if err != nil {
		t.Fatalf("Error en segunda obtención de plantilla: %v", err)
	}
	if tmpl1 != tmpl2 {
		t.Fatalf("Se esperaba la misma instancia de plantilla desde la caché")
	}
}

func TestLoggingAndRecoveryMiddleware(t *testing.T) {
	// 1. Probar ruta normal (200 OK)
	normalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	wrappedNormal := loggingAndRecoveryMiddleware(normalHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	wrappedNormal.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("Se esperaba status 200, obtenido %d", rr.Code)
	}

	// 2. Probar recuperación de pánico (500 Internal Server Error)
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("error inesperado de prueba")
	})
	wrappedPanic := loggingAndRecoveryMiddleware(panicHandler)
	reqPanic := httptest.NewRequest("GET", "/panic", nil)
	rrPanic := httptest.NewRecorder()
	wrappedPanic.ServeHTTP(rrPanic, reqPanic)
	if rrPanic.Code != http.StatusInternalServerError {
		t.Fatalf("Se esperaba status 500 tras pánico recuperado, obtenido %d", rrPanic.Code)
	}
}

