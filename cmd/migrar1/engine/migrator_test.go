package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pasigo/cmd/migrar1/db"
	"pasigo/cmd/migrar1/odoo"
)

func TestMigratorIdempotencyAndDryRun(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "migrator_test_*")
	defer os.RemoveAll(tempDir)
	database, err := db.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatalf("error db: %v", err)
	}
	defer database.Close()

	// Mock Odoo Server Origen & Destino
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Params struct {
				Service string        `json:"service"`
				Method  string        `json:"method"`
				Args    []interface{} `json:"args"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")

		if req.Params.Service == "common" && req.Params.Method == "authenticate" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":2}`))
			return
		}

		if req.Params.Service == "object" {
			method := req.Params.Method
			model := ""
			if len(req.Params.Args) >= 4 {
				model, _ = req.Params.Args[3].(string)
			}

			if method == "execute_kw" {
				subMethod := ""
				if len(req.Params.Args) >= 5 {
					subMethod, _ = req.Params.Args[4].(string)
				}

				if subMethod == "fields_get" {
					w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"name":{},"description":{},"partner_id":{},"user_id":{},"project_id":{}}}`))
					return
				}

				if subMethod == "search_read" {
					if model == "res.partner" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[{"id":20,"name":"Acme SL","vat":"B12345678","email":"test@acme.com","active":true}]}`))
						return
					}
					if model == "res.users" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[{"id":5,"name":"Pepe Técnico","login":"pepe","email":"pepe@empresa.com","active":true}]}`))
						return
					}
					if model == "project.project" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[{"id":100,"name":"Proyecto Soporte","display_name":"Proyecto Soporte","active":true}]}`))
						return
					}
					if model == "helpdesk.ticket" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[{"id":501,"name":"Error en servidor","create_date":"2025-02-10 10:00:00","partner_id":[20,"Acme SL"],"user_id":[5,"Pepe Técnico"],"project_id":[100,"Proyecto Soporte"]}]}`))
						return
					}
					if model == "account.analytic.line" {
						w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[{"id":901,"name":"Revisión de logs","date":"2025-02-10","unit_amount":2.5,"helpdesk_ticket_id":[501,"Error en servidor"]}]}`))
						return
					}
				}

				if subMethod == "create" {
					w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":9999}`))
					return
				}
				if subMethod == "write" {
					w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":true}`))
					return
				}
			}
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":[]}`))
	}))
	defer mockServer.Close()

	srcClient := odoo.NewClient(odoo.ConnectionConfig{URL: mockServer.URL, DB: "pasi", Username: "u", Password: "p"})
	tgtClient := odoo.NewClient(odoo.ConnectionConfig{URL: mockServer.URL, DB: "pasi", Username: "u", Password: "p"})

	mig := NewMigrator(database, srcClient, tgtClient, MigrationConfig{
		StartDate: "2025-01-01",
	})

	ctx := context.Background()

	// 1. Test Dry-Run
	report, err := mig.DryRun(ctx)
	if err != nil {
		t.Fatalf("error dry-run: %v", err)
	}
	if report.TotalSourceTickets != 1 || report.ReadyToMigrateTickets != 1 {
		t.Fatalf("resultado dry-run inesperado: %+v", report)
	}

	// 2. Primera migración del ticket 501 -> Debe crearlo (ID: 9999)
	rec1, err := mig.MigrateTicket(ctx, 501)
	if err != nil {
		t.Fatalf("error migrando ticket: %v", err)
	}
	if rec1.Status != "synced" || rec1.TargetID != 9999 {
		t.Fatalf("estado de rec1 inesperado: %+v", rec1)
	}

	// 3. Segunda migración del MISMO ticket -> Idempotencia: no debe recrear, se omite o actualiza
	rec2, err := mig.MigrateTicket(ctx, 501)
	if err != nil {
		t.Fatalf("error en segunda migración: %v", err)
	}
	if rec2.TargetID != 9999 || rec2.Status != "synced" {
		t.Fatalf("fallo de idempotencia en rec2: %+v", rec2)
	}
}
