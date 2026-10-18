package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCleanURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", DefaultServer},
		{"http://localhost:8089", DefaultServer},
		{"http://127.0.0.1:8089", DefaultServer},
		{"https://planesgo.autopyme.com/", "https://planesgo.autopyme.com"},
		{"https://custom.planesgo.com", "https://custom.planesgo.com"},
	}

	for _, tc := range tests {
		got := cleanURL(tc.input)
		if got != tc.expected {
			t.Errorf("cleanURL(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestToolsDefinition(t *testing.T) {
	tools := getToolsDefinition()
	if len(tools) != 9 {
		t.Fatalf("expected 9 tools defined, got %d", len(tools))
	}

	expectedNames := map[string]bool{
		"planesgo_check":           false,
		"planesgo_add_hours":       false,
		"planesgo_beat":            false,
		"planesgo_stop":            false,
		"planesgo_list_tasks":      false,
		"planesgo_status":          false,
		"planesgo_search_projects": false,
		"planesgo_set_project":     false,
		"planesgo_close_ticket":    false,
	}

	for _, tool := range tools {
		name, ok := tool["name"].(string)
		if !ok {
			t.Errorf("tool missing name")
			continue
		}
		if _, exists := expectedNames[name]; exists {
			expectedNames[name] = true
		} else {
			t.Errorf("unexpected tool name: %s", name)
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("tool %s was not found in definition", name)
		}
	}
}

func TestNormalizeTaskType(t *testing.T) {
	testCases := []struct {
		rawTitle  string
		realWork  string
		wantType  string
		wantClean string
	}{
		{
			rawTitle:  "Implementación de tag_ids y vistas en PlanesGo",
			realWork:  "Añadiendo soporte de tags y columnas en la interfaz",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "Desarrollo de nuevo endpoint REST",
			realWork:  "Creando handler y pruebas unitarias",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "[AGY] Implementación de vistas",
			realWork:  "",
			wantType:  "Desarrollo",
			wantClean: "Desarrollo",
		},
		{
			rawTitle:  "Análisis y diseño de la arquitectura de pasigo",
			realWork:  "Investigando patrones y diseño de endpoints",
			wantType:  "Análisis y diseño",
			wantClean: "Análisis y diseño",
		},
		{
			rawTitle:  "Batería de pruebas unitarias y de integración",
			realWork:  "Ejecutando tests de concurrencia y validación",
			wantType:  "Pruebas",
			wantClean: "Pruebas",
		},
		{
			rawTitle:  "[Pruebas] Verificación de suite Odoo",
			realWork:  "",
			wantType:  "Pruebas",
			wantClean: "Pruebas",
		},
	}

	for _, tc := range testCases {
		gotType, gotClean := NormalizeTaskType(tc.rawTitle, tc.realWork, "")
		if gotType != tc.wantType {
			t.Errorf("NormalizeTaskType(%q, %q) type = %q; want %q", tc.rawTitle, tc.realWork, gotType, tc.wantType)
		}
		if gotClean != tc.wantClean {
			t.Errorf("NormalizeTaskType(%q, %q) cleanName = %q; want %q", tc.rawTitle, tc.realWork, gotClean, tc.wantClean)
		}
	}
}

func TestDoWithRetry(t *testing.T) {
	var attempts int32

	// Servidor que falla con 502 en los 2 primeros intentos y responde 200 en el tercero
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		att := atomic.AddInt32(&attempts, 1)
		if att < 3 {
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`{"error":"bad gateway temporario"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	client := &PlanesGoClient{
		BaseURL:    ts.URL,
		Token:      "test-token",
		HTTPClient: ts.Client(),
		RetryDelay: 10 * time.Millisecond, // Delay mínimo para el test unitario
	}

	body, status, err := client.doWithRetry(http.MethodGet, ts.URL+"/test", nil, nil)
	if err != nil {
		t.Fatalf("doWithRetry falló inesperadamente: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status esperado 200, obtenido %d", status)
	}
	if string(body) != `{"status":"ok"}` {
		t.Fatalf("cuerpo inesperado: %s", string(body))
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Fatalf("se esperaban 3 intentos, pero hubo %d", attempts)
	}
}

func TestFindConfigStrictLocation(t *testing.T) {
	// 1. Probar que no busca en directorios padres
	parentDir := t.TempDir()
	childDir := filepath.Join(parentDir, "subproject")
	if err := os.MkdirAll(childDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Poner .planesgo.json en el padre
	parentConfig := []byte(`{"odoo_project_id": 999, "odoo_project_name": "PROYECTO PADRE"}`)
	if err := os.WriteFile(filepath.Join(parentDir, ".planesgo.json"), parentConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Buscar desde el subdirectorio NO debe encontrar el del padre
	_, _, err := findConfig(childDir)
	if err == nil {
		t.Errorf("findConfig(childDir) debería fallar y no heredar el .planesgo.json del padre")
	}

	// 2. Probar que busca estrictamente en el directorio raíz del proyecto
	projectDir := t.TempDir()
	projConfig := []byte(`{"odoo_project_id": 123, "odoo_project_name": "MI PROYECTO"}`)
	if err := os.WriteFile(filepath.Join(projectDir, ".planesgo.json"), projConfig, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, foundPath, err := findConfig(projectDir)
	if err != nil {
		t.Fatalf("findConfig(projectDir) falló: %v", err)
	}
	if cfg.OdooProjectID != 123 || cfg.OdooProjectName != "MI PROYECTO" {
		t.Errorf("configuración leída incorrecta: %+v", cfg)
	}
	if foundPath != filepath.Join(projectDir, ".planesgo.json") {
		t.Errorf("ruta encontrada inesperada: %s", foundPath)
	}

	// 3. Probar que no busca en directorios hijos (ej. custom/)
	noRootProj := t.TempDir()
	customSubdir := filepath.Join(noRootProj, "custom")
	if err := os.MkdirAll(customSubdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(customSubdir, ".planesgo.json"), projConfig, 0644); err != nil {
		t.Fatal(err)
	}

	// Si no está en la raíz, debe fallar
	_, _, err = findConfig(noRootProj)
	if err == nil {
		t.Errorf("findConfig(noRootProj) debería fallar cuando solo existe en el subdirectorio hijo 'custom'")
	}
}

// Sin project_path, un MCP lanzado en segundo plano (HOME) no debe adivinar el proyecto:
// cada sesión del arnés tiene que indicar el suyo.
func TestFindConfigBackgroundRequiresProjectPath(t *testing.T) {
	wsDir := t.TempDir()
	wsConfig := []byte(`{"odoo_project_id": 777, "odoo_project_name": "WORKSPACE ACTIVO"}`)
	if err := os.WriteFile(filepath.Join(wsDir, ".planesgo.json"), wsConfig, 0644); err != nil {
		t.Fatal(err)
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		t.Skip("sin HOME")
	}
	origWd, _ := os.Getwd()
	if err := os.Chdir(homeDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if _, _, err := findConfig(); err != errMissingProjectPath {
		t.Errorf("findConfig() desde HOME sin project_path: esperado errMissingProjectPath, obtenido %v", err)
	}
	if err := saveConfigFile(Config{OdooProjectID: 1}, ""); err != errMissingProjectPath {
		t.Errorf("saveConfigFile() desde HOME sin destino: esperado errMissingProjectPath, obtenido %v", err)
	}

	cfg, foundPath, err := findConfig(wsDir)
	if err != nil {
		t.Fatalf("findConfig(project_path) falló: %v", err)
	}
	if cfg.OdooProjectID != 777 || foundPath != filepath.Join(wsDir, ".planesgo.json") {
		t.Errorf("configuración inesperada: %+v en %s", cfg, foundPath)
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{50, "50"},
		{999, "999"},
		{1000, "1.000"},
		{15420, "15.420"},
		{1234567, "1.234.567"},
	}

	for _, c := range cases {
		result := formatTokens(c.input)
		if result != c.expected {
			t.Errorf("formatTokens(%d) = %s, esperado %s", c.input, result, c.expected)
		}
	}
}

func TestSendTaskActionWithTokens(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/antigravity/update_tasks" {
			_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &PlanesGoClient{
		BaseURL:    server.URL,
		Token:      "test-token",
		HTTPClient: server.Client(),
	}

	_, err := client.SendTaskActionWithTokens(
		"stop",
		"Desarrollo de feature",
		10,
		567,
		"PLANESGO",
		"Completado con éxito",
		"Desarrollo",
		"T123",
		15420,
		10000,
		5420,
		"Gemini 3.8 Flash",
		0.05,
		"session-abc",
	)
	if err != nil {
		t.Fatalf("SendTaskActionWithTokens falló: %v", err)
	}

	if receivedPayload["tokens_total"] != float64(15420) {
		t.Errorf("tokens_total esperado 15420, obtenido %v", receivedPayload["tokens_total"])
	}
	if receivedPayload["ai_model"] != "Gemini 3.8 Flash" {
		t.Errorf("ai_model esperado 'Gemini 3.8 Flash', obtenido %v", receivedPayload["ai_model"])
	}
	if receivedPayload["ai_session_id"] != "session-abc" {
		t.Errorf("ai_session_id esperado 'session-abc', obtenido %v", receivedPayload["ai_session_id"])
	}
}

func TestMultiSessionOption1AndOption2(t *testing.T) {
	sessDir := t.TempDir()
	t.Setenv("PLANESGO_SESSIONS_DIR", sessDir)

	var mu sync.Mutex
	var recordedPayloads []map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/antigravity/status":
			pName := strings.ToUpper(r.URL.Query().Get("project_name"))
			pID := r.URL.Query().Get("project_id")
			switch {
			case pName == "PRO1" || pID == "101":
				_, _ = w.Write([]byte(`{"status":"ok","project_id": 101, "project_name": "PRO1 - CUENTA CARGO 1"}`))
			case pName == "PRO2" || pID == "202":
				_, _ = w.Write([]byte(`{"status":"ok","project_id": 202, "project_name": "PRO2 - CUENTA CARGO 2"}`))
			case pName == "PRO3" || pID == "303":
				_, _ = w.Write([]byte(`{"status":"ok","project_id": 303, "project_name": "PRO3 - OVERRIDE EXPLICITO"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":"proyecto no encontrado"}`))
			}
		case "/antigravity/update_tasks":
			var p map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&p)
			mu.Lock()
			recordedPayloads = append(recordedPayloads, p)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/antigravity/tasks":
			projID := r.URL.Query().Get("project_id")
			_, _ = w.Write([]byte(`[{"id": 1, "name": "Tarea de proyecto ` + projID + `", "stage_name": "En curso"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	t.Setenv("PLANESGO_URL", server.URL)
	t.Setenv("PLANESGO_TOKEN", "test-multi-session-token")

	repoDir := t.TempDir()

	// --- OPCIÓN 1: Dos sesiones concurrentes en el mismo repo ---
	// Sesión 1 vincula PRO1 (como el repo aún no tiene .planesgo.json, crea tanto el de sesión como el del repo)
	res1 := executeToolCall("planesgo_set_project", map[string]interface{}{
		"project_name": "PRO1",
		"project_path": repoDir,
		"session_id":   "sess-1",
	})
	if res1.IsError {
		t.Fatalf("planesgo_set_project sess-1 falló: %+v", res1.Content)
	}

	// Sesión 2 vincula PRO2 en el MISMO repositorio
	res2 := executeToolCall("planesgo_set_project", map[string]interface{}{
		"project_name": "PRO2",
		"project_path": repoDir,
		"session_id":   "sess-2",
	})
	if res2.IsError {
		t.Fatalf("planesgo_set_project sess-2 falló: %+v", res2.Content)
	}

	// 1a. Verificar que .planesgo.json en la raíz del repo sigue siendo PRO1 (no fue pisado por sess-2)
	repoCfg, _, err := findConfig(repoDir)
	if err != nil {
		t.Fatalf("findConfig(repoDir) falló: %v", err)
	}
	if repoCfg.OdooProjectID != 101 {
		t.Errorf("el .planesgo.json del repo fue pisado: esperado ID 101 (PRO1), obtenido %d (%s)", repoCfg.OdooProjectID, repoCfg.OdooProjectName)
	}

	// 1b. Verificar que cada sesión resuelve su propio proyecto independientemente
	_, cfg1, err := newClientWithSession("sess-1", repoDir)
	if err != nil {
		t.Fatalf("newClientWithSession(sess-1) falló: %v", err)
	}
	if cfg1.OdooProjectID != 101 {
		t.Errorf("sess-1 debería resolver proyecto 101, obtuvo %d", cfg1.OdooProjectID)
	}

	_, cfg2, err := newClientWithSession("sess-2", repoDir)
	if err != nil {
		t.Fatalf("newClientWithSession(sess-2) falló: %v", err)
	}
	if cfg2.OdooProjectID != 202 {
		t.Errorf("sess-2 debería resolver proyecto 202, obtuvo %d", cfg2.OdooProjectID)
	}

	// 1c. Enviar beat desde sess-1 y desde sess-2 simultáneamente sin pasar project_name explícito
	if out := executeToolCall("planesgo_beat", map[string]interface{}{
		"task_name":    "Desarrollo feature A",
		"task_type":    "Desarrollo",
		"description":  "Avance en sesión 1",
		"project_path": repoDir,
		"session_id":   "sess-1",
	}); out.IsError {
		t.Fatalf("planesgo_beat sess-1 falló: %+v", out.Content)
	}

	if out := executeToolCall("planesgo_beat", map[string]interface{}{
		"task_name":    "Desarrollo feature B",
		"task_type":    "Desarrollo",
		"description":  "Avance en sesión 2",
		"project_path": repoDir,
		"session_id":   "sess-2",
	}); out.IsError {
		t.Fatalf("planesgo_beat sess-2 falló: %+v", out.Content)
	}

	mu.Lock()
	if len(recordedPayloads) != 2 {
		t.Fatalf("esperados 2 payloads registrados, obtenidos %d", len(recordedPayloads))
	}
	p1, p2 := recordedPayloads[0], recordedPayloads[1]
	mu.Unlock()

	if p1["project_id"] != float64(101) || p1["ai_session_id"] != "sess-1" {
		t.Errorf("payload sess-1 incorrecto: project_id=%v, ai_session_id=%v", p1["project_id"], p1["ai_session_id"])
	}
	if p2["project_id"] != float64(202) || p2["ai_session_id"] != "sess-2" {
		t.Errorf("payload sess-2 incorrecto: project_id=%v, ai_session_id=%v", p2["project_id"], p2["ai_session_id"])
	}

	// --- OPCIÓN 2: Override explícito por project_name o project_id en las herramientas ---
	// 2a. Override por project_name ("PRO3") desde sess-1 (cuya sesión es PRO1)
	if out := executeToolCall("planesgo_stop", map[string]interface{}{
		"task_name":    "Cierre en PRO3",
		"task_type":    "Desarrollo",
		"description":  "Cierre directo con project_name explícito",
		"project_path": repoDir,
		"session_id":   "sess-1",
		"project_name": "PRO3",
	}); out.IsError {
		t.Fatalf("planesgo_stop con project_name=PRO3 falló: %+v", out.Content)
	}

	mu.Lock()
	p3 := recordedPayloads[len(recordedPayloads)-1]
	mu.Unlock()
	if p3["project_id"] != float64(303) || p3["project_name"] != "PRO3 - OVERRIDE EXPLICITO" {
		t.Errorf("override por project_name falló: project_id=%v, project_name=%v", p3["project_id"], p3["project_name"])
	}

	// 2b. Override por project_id numérico (404) sin project_name: no debe arrastrar el nombre antiguo
	if out := executeToolCall("planesgo_add_hours", map[string]interface{}{
		"hours":        1.5,
		"description":  "Imputación directa a ID 404",
		"project_path": repoDir,
		"session_id":   "sess-1",
		"project_id":   float64(404),
	}); out.IsError {
		t.Fatalf("planesgo_add_hours con project_id=404 falló: %+v", out.Content)
	}

	mu.Lock()
	p4 := recordedPayloads[len(recordedPayloads)-1]
	mu.Unlock()
	if p4["project_id"] != float64(404) {
		t.Errorf("override por project_id falló: esperado 404, obtenido %v", p4["project_id"])
	}
	if p4["project_name"] != "" && p4["project_name"] != nil {
		t.Errorf("override por project_id no debía arrastrar project_name antiguo, obtuvo %v", p4["project_name"])
	}

	// 2c. planesgo_list_tasks respeta sesión (202) y override explícito (303)
	tasksOut := executeToolCall("planesgo_list_tasks", map[string]interface{}{
		"project_path": repoDir,
		"session_id":   "sess-2",
	})
	if tasksOut.IsError || len(tasksOut.Content) == 0 || !strings.Contains(tasksOut.Content[0].Text, "Tarea de proyecto 202") {
		t.Errorf("planesgo_list_tasks para sess-2 debía consultar project_id=202, obtuvo: %+v", tasksOut.Content)
	}

	tasksOverrideOut := executeToolCall("planesgo_list_tasks", map[string]interface{}{
		"project_path": repoDir,
		"session_id":   "sess-2",
		"project_name": "PRO3",
	})
	if tasksOverrideOut.IsError || len(tasksOverrideOut.Content) == 0 || !strings.Contains(tasksOverrideOut.Content[0].Text, "Tarea de proyecto 303") {
		t.Errorf("planesgo_list_tasks con override PRO3 debía consultar project_id=303, obtuvo: %+v", tasksOverrideOut.Content)
	}
}

