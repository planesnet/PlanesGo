package main

import (
	"bytes"
	"html/template"
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
	cfg.Odoo.DB = "pasi"
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
	if buf.Len() == 0 {
		t.Fatalf("El contenido renderizado está vacío")
	}
}
