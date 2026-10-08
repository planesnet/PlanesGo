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

func TestHasField(t *testing.T) {
	var fieldsGetCalls int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		params, _ := req.Params.(map[string]interface{})
		if params["service"] == "common" && params["method"] == "authenticate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  10,
			})
			return
		}

		if params["service"] == "object" && params["method"] == "execute_kw" {
			args, _ := params["args"].([]interface{})
			if len(args) >= 6 && args[4] == "fields_get" {
				atomic.AddInt32(&fieldsGetCalls, 1)
				res := map[string]interface{}{}
				if posArgs, ok := args[5].([]interface{}); ok && len(posArgs) > 0 {
					if fieldList, ok := posArgs[0].([]interface{}); ok {
						for _, f := range fieldList {
							if fStr, ok := f.(string); ok && fStr == "existing_field" {
								res[fStr] = map[string]interface{}{"type": "char"}
							}
						}
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result":  res,
				})
				return
			}
		}

		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := config.OdooConfig{
		URL:      server.URL,
		DB:       "testdb",
		Username: "user@example.com",
		Password: "password",
	}

	client := &Client{
		config:           cfg,
		httpClient:       server.Client(),
		userUIDCache:     make(map[string]int),
		modelFieldsCache: make(map[string]map[string]bool),
	}

	ctx := context.Background()

	// 1. Campo existente
	if !client.HasField(ctx, "project.task", "existing_field") {
		t.Fatalf("Esperado que HasField devuelva true para 'existing_field'")
	}
	if calls := atomic.LoadInt32(&fieldsGetCalls); calls != 1 {
		t.Fatalf("Esperada 1 llamada a fields_get, obtenidas %d", calls)
	}

	// 2. Campo existente de nuevo (debe responder desde caché en memoria)
	if !client.HasField(ctx, "project.task", "existing_field") {
		t.Fatalf("Esperado que HasField devuelva true desde caché")
	}
	if calls := atomic.LoadInt32(&fieldsGetCalls); calls != 1 {
		t.Fatalf("Esperado que se sirva desde caché (1 llamada), pero hubo %d", calls)
	}

	// 3. Campo inexistente
	if client.HasField(ctx, "project.task", "is_timer_running") {
		t.Fatalf("Esperado que HasField devuelva false para campo inexistente")
	}
	if calls := atomic.LoadInt32(&fieldsGetCalls); calls != 2 {
		t.Fatalf("Esperadas 2 llamadas a fields_get en total, obtenidas %d", calls)
	}

	// 4. Campo inexistente de nuevo (debe responder false desde caché sin llamar a la red)
	if client.HasField(ctx, "project.task", "is_timer_running") {
		t.Fatalf("Esperado que HasField devuelva false desde caché")
	}
	if calls := atomic.LoadInt32(&fieldsGetCalls); calls != 2 {
		t.Fatalf("Esperado que se sirva desde caché (2 llamadas), pero hubo %d", calls)
	}
}

// TestGetPartnersCompanyFilter comprueba que, cuando se pasa companyID, el dominio
// enviado a Odoo restringe a esa empresa y a sus contactos hijos (parent_id), en vez
// de listar todos los contactos de la base de datos (el bug que reportó el usuario:
// el desplegable de "Contacto" del ticket mostraba toda la base de contactos).
func TestGetPartnersCompanyFilter(t *testing.T) {
	var lastDomain []interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		params, _ := req.Params.(map[string]interface{})
		if params["service"] == "common" && params["method"] == "authenticate" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": 10})
			return
		}
		if params["service"] == "object" && params["method"] == "execute_kw" {
			args, _ := params["args"].([]interface{})
			if len(args) >= 6 && args[3] == "res.partner" && args[4] == "search_read" {
				if posArgs, ok := args[5].([]interface{}); ok && len(posArgs) > 0 {
					if domain, ok := posArgs[0].([]interface{}); ok {
						lastDomain = domain
					}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      req.ID,
					"result": []map[string]interface{}{
						{"id": 332, "name": "MADERAS PLANES S.L.", "display_name": "MADERAS PLANES S.L.", "email": "esteban@maderasplanes.com"},
						{"id": 898, "name": "laura@maderasplanes.com", "display_name": "MADERAS PLANES S.L., laura@maderasplanes.com", "email": "laura@maderasplanes.com"},
					},
				})
				return
			}
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer server.Close()

	cfg := config.OdooConfig{URL: server.URL, DB: "testdb", Username: "user@example.com", Password: "password"}
	client := &Client{config: cfg, httpClient: server.Client(), userUIDCache: make(map[string]int)}
	ctx := context.Background()

	partners, err := client.GetPartners(ctx, "", 332)
	if err != nil {
		t.Fatalf("GetPartners con companyID falló: %v", err)
	}
	if len(partners) != 2 {
		t.Fatalf("esperados 2 contactos, obtenidos %d", len(partners))
	}

	// Comparación ESTRUCTURAL exacta, no de subcadenas: el dominio de Odoo es una
	// lista plana ["|", cond1, cond2], no una lista anidada [["|", cond1, cond2]].
	// Un simple "contiene estas subcadenas" no habría detectado el bug real (el "|"
	// y sus dos condiciones quedaban envueltos en una lista de más), ya que todas
	// las subcadenas seguían apareciendo igual en el JSON mal anidado.
	domainJSON, _ := json.Marshal(lastDomain)
	wantDomain := `["|",["id","=",332],["parent_id","=",332]]`
	if string(domainJSON) != wantDomain {
		t.Fatalf("dominio esperado %s, obtenido %s", wantDomain, string(domainJSON))
	}

	// Sin companyID, no debe mandar ninguna condición de empresa (lista general)
	lastDomain = nil
	if _, err := client.GetPartners(ctx, "", 0); err != nil {
		t.Fatalf("GetPartners sin companyID falló: %v", err)
	}
	domainJSON2, _ := json.Marshal(lastDomain)
	if string(domainJSON2) != "[]" {
		t.Fatalf("esperado dominio vacío sin companyID, obtenido: %s", string(domainJSON2))
	}
}

