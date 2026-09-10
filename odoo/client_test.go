package odoo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"pasigo/config"
	"sync/atomic"
	"testing"
)

func TestClientPooling(t *testing.T) {
	cfg1 := config.OdooConfig{
		URL:      "https://odoo.example.com",
		DB:       "testdb",
		Username: "user1@example.com",
		Password: "secretpassword",
	}
	cfg2 := config.OdooConfig{
		URL:      "https://odoo.example.com",
		DB:       "testdb",
		Username: "user1@example.com",
		Password: "secretpassword",
	}

	c1 := GetClient(cfg1)
	c2 := GetClient(cfg2)

	if c1 != c2 {
		t.Fatalf("Esperado que GetClient devuelva el mismo puntero para idéntica configuración")
	}

	InvalidateClient(cfg1)
	c3 := GetClient(cfg1)
	if c1 == c3 {
		t.Fatalf("Esperado que InvalidateClient remueva la instancia del pool")
	}
}

func TestClientAuthenticateCache(t *testing.T) {
	var authCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if req.Method == "call" {
			params, _ := req.Params.(map[string]interface{})
			if params["service"] == "common" && params["method"] == "authenticate" {
				atomic.AddInt32(&authCalls, 1)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  42,
				})
				return
			}
		}
	}))
	defer server.Close()

	cfg := config.OdooConfig{
		URL:      server.URL,
		DB:       "testdb",
		Username: "user@example.com",
		Password: "secretpassword",
	}

	client := &Client{
		config:       cfg,
		httpClient:   server.Client(),
		userUIDCache: make(map[string]int),
	}

	ctx := context.Background()

	// 1. Primera autenticación: debe llamar al servidor
	uid1, err := client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Error autenticando: %v", err)
	}
	if uid1 != 42 {
		t.Fatalf("Esperado UID 42, obtenido %d", uid1)
	}
	if calls := atomic.LoadInt32(&authCalls); calls != 1 {
		t.Fatalf("Esperado 1 llamada a Odoo, obtenidas %d", calls)
	}

	// 2. Segunda autenticación: debe usar la caché sin llamar al servidor
	uid2, err := client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Error en segunda autenticación: %v", err)
	}
	if uid2 != 42 {
		t.Fatalf("Esperado UID 42, obtenido %d", uid2)
	}
	if calls := atomic.LoadInt32(&authCalls); calls != 1 {
		t.Fatalf("Esperado 1 sola llamada por caché, pero hubo %d", calls)
	}
}

func TestGetProjectsCache(t *testing.T) {
	var executeCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		json.NewDecoder(r.Body).Decode(&req)
		params, _ := req.Params.(map[string]interface{})

		if params["service"] == "common" && params["method"] == "authenticate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  7,
			})
			return
		}

		if params["service"] == "object" && params["method"] == "execute_kw" {
			atomic.AddInt32(&executeCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			projects := []Project{
				{ID: 1, Name: "Proyecto Alpha", Active: true},
				{ID: 2, Name: "Proyecto Beta", Active: true},
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  projects,
			})
			return
		}
	}))
	defer server.Close()

	cfg := config.OdooConfig{
		URL:      server.URL,
		DB:       "testdb",
		Username: "user@example.com",
		Password: "secretpassword",
	}

	client := &Client{
		config:       cfg,
		httpClient:   server.Client(),
		userUIDCache: make(map[string]int),
	}

	ctx := context.Background()

	// 1. Primera consulta: hace petición de red y almacena en caché
	projs1, err := client.GetProjects(ctx, nil)
	if err != nil {
		t.Fatalf("Error obteniendo proyectos: %v", err)
	}
	if len(projs1) != 2 {
		t.Fatalf("Esperados 2 proyectos, obtenidos %d", len(projs1))
	}
	if calls := atomic.LoadInt32(&executeCalls); calls != 1 {
		t.Fatalf("Esperado 1 execute_kw, obtenidos %d", calls)
	}

	// 2. Segunda consulta sin filtro: debe responder desde la caché
	projs2, err := client.GetProjects(ctx, nil)
	if err != nil {
		t.Fatalf("Error obteniendo proyectos: %v", err)
	}
	if len(projs2) != 2 {
		t.Fatalf("Esperados 2 proyectos, obtenidos %d", len(projs2))
	}
	if calls := atomic.LoadInt32(&executeCalls); calls != 1 {
		t.Fatalf("Esperado que se sirviera desde la caché (1 llamada), pero hubo %d", calls)
	}

	// 3. Invalidación explícita de caché
	client.InvalidateProjectsCache()
	projs3, err := client.GetProjects(ctx, nil)
	if err != nil {
		t.Fatalf("Error tras invalidar caché: %v", err)
	}
	if len(projs3) != 2 {
		t.Fatalf("Esperados 2 proyectos, obtenidos %d", len(projs3))
	}
	if calls := atomic.LoadInt32(&executeCalls); calls != 2 {
		t.Fatalf("Esperada segunda llamada tras invalidación, pero hubo %d", calls)
	}
}
