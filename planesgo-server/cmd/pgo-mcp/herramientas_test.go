package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// apiFalsa imita la API de PlanesGo que usa pgo-mcp y guarda lo que recibe.
type apiFalsa struct {
	mu        sync.Mutex
	rutas     []string
	latidos   []map[string]interface{}
	proyectos map[int]string
}

func (a *apiFalsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rutas = append(a.rutas, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/antigravity/projects/search":
		q := strings.ToUpper(r.URL.Query().Get("q"))
		var res []map[string]interface{}
		for id, nombre := range a.proyectos {
			if strings.Contains(strings.ToUpper(nombre), q) {
				res = append(res, map[string]interface{}{"id": id, "name": nombre})
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"projects": res, "count": len(res)})
	case "/antigravity/status":
		id, _ := strconv.Atoi(r.URL.Query().Get("project_id"))
		nombre, ok := a.proyectos[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"proyecto no encontrado"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "project_id": id, "project_name": nombre})
	case "/antigravity/update_tasks":
		var p map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&p)
		a.latidos = append(a.latidos, p)
		timer := map[string]interface{}{"task_name": p["task_name"], "project_name": p["project_name"]}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "timer": timer})
	default:
		http.NotFound(w, r)
	}
}

func (a *apiFalsa) llamadas(ruta string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, r := range a.rutas {
		if r == ruta {
			n++
		}
	}
	return n
}

func (a *apiFalsa) copiaLatidos() []map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]map[string]interface{}(nil), a.latidos...)
}

// entornoPrueba levanta la API falsa y aísla el entorno (HOME, sesiones, token, interruptor de pgocode).
func entornoPrueba(t *testing.T) *apiFalsa {
	t.Helper()
	api := &apiFalsa{proyectos: map[int]string{
		42:  "MADPLA - Planificación",
		101: "PRO1 - CUENTA CARGO 1",
		202: "PRO2 - CUENTA CARGO 2",
		303: "PRO3 - OTRO",
	}}
	// cleanURL descarta las URL con «8089» (servidor local antiguo): se evita ese puerto
	var srv *httptest.Server
	for srv == nil || strings.Contains(srv.URL, "8089") {
		if srv != nil {
			srv.Close()
		}
		srv = httptest.NewServer(api)
	}
	t.Cleanup(srv.Close)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("PLANESGO_URL", srv.URL)
	t.Setenv("PLANESGO_TOKEN", "token-de-prueba")
	t.Setenv("PLANESGO_SESSIONS_DIR", t.TempDir())
	t.Setenv("PLANESGO_SESSION_ID", "")
	t.Setenv("PLANESGO_GESTOR", "")
	autoactualizar = func() {}
	return api
}

func texto(res ToolCallResult) string {
	if len(res.Content) == 0 {
		return ""
	}
	return res.Content[0].Text
}

func nombresHerramientas(t *testing.T) []string {
	t.Helper()
	resp := atiendePeticion(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"})
	data, _ := json.Marshal(resp.Result)
	var r struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for _, h := range r.Tools {
		nombres = append(nombres, h.Name)
	}
	return nombres
}

func TestToolsListSoloPgoProjectYPgoTime(t *testing.T) {
	entornoPrueba(t)
	nombres := nombresHerramientas(t)
	if strings.Join(nombres, ",") != "pgo_project,pgo_time" {
		t.Fatalf("tools/list debe ser exactamente pgo_project y pgo_time, es %v", nombres)
	}
	data, _ := json.Marshal(getToolsDefinition())
	if len([]rune(string(data))) > 1200 {
		t.Errorf("las definiciones ocupan %d caracteres (máximo ~1.200)", len([]rune(string(data))))
	}
}

func TestInitialize(t *testing.T) {
	entornoPrueba(t)
	resp := atiendePeticion(JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "initialize"})
	r := resp.Result.(map[string]interface{})
	if r["serverInfo"].(map[string]interface{})["name"] != "pgo-mcp" {
		t.Errorf("serverInfo.name debe ser pgo-mcp: %v", r["serverInfo"])
	}
	if r["capabilities"].(map[string]interface{})["tools"].(map[string]interface{})["listChanged"] != true {
		t.Errorf("falta la capacidad listChanged")
	}
	if r["instructions"] != mcpInstructions {
		t.Errorf("instrucciones inesperadas: %v", r["instructions"])
	}
	if atiendePeticion(JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}) != nil {
		t.Errorf("una notificación no lleva respuesta")
	}
}

func TestModoDormido(t *testing.T) {
	entornoPrueba(t)

	// Por variable de entorno
	t.Setenv("PLANESGO_GESTOR", "pgocode")
	if n := nombresHerramientas(t); len(n) != 0 {
		t.Errorf("con PLANESGO_GESTOR=pgocode tools/list debe estar vacío: %v", n)
	}
	resp := atiendePeticion(JSONRPCRequest{JSONRPC: "2.0", ID: 2, Method: "tools/call",
		Params: json.RawMessage(`{"name":"pgo_time","arguments":{"minutes":5}}`)})
	if texto(resp.Result.(ToolCallResult)) != textoGestorPgocode {
		t.Errorf("en modo dormido tools/call debe responder el texto de pgocode: %+v", resp.Result)
	}

	// Por fichero ~/.planesgo/gestor
	t.Setenv("PLANESGO_GESTOR", "")
	if n := nombresHerramientas(t); len(n) != 2 {
		t.Fatalf("sin interruptor deben verse las dos herramientas: %v", n)
	}
	if err := os.MkdirAll(filepath.Dir(ficheroGestor()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ficheroGestor(), []byte("pgocode\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n := nombresHerramientas(t); len(n) != 0 {
		t.Errorf("con ~/.planesgo/gestor=pgocode tools/list debe estar vacío: %v", n)
	}
}

func TestHerramientasAntiguasRedirigen(t *testing.T) {
	api := entornoPrueba(t)
	for nombre := range herramientasAntiguas {
		res := executeToolCall(nombre, map[string]interface{}{"hours": 1.0, "project_name": "PRO1"})
		if res.IsError {
			t.Errorf("%s no debe dar error: %s", nombre, texto(res))
		}
		if !strings.Contains(texto(res), "ya no existe") || !strings.Contains(texto(res), "pgo_project") || !strings.Contains(texto(res), "pgo_time") {
			t.Errorf("%s: texto de redirección inesperado: %s", nombre, texto(res))
		}
	}
	if len(api.rutas) != 0 {
		t.Errorf("los nombres antiguos no deben llamar a la API: %v", api.rutas)
	}
	if res := executeToolCall("otra_cosa", nil); !res.IsError {
		t.Errorf("una herramienta desconocida sí es un error")
	}
}

func TestPgoProjectBuscaSinVincular(t *testing.T) {
	api := entornoPrueba(t)
	dir := t.TempDir()

	// Una sola coincidencia, con session_id y project_path: aun así no vincula
	res := executeToolCall("pgo_project", map[string]interface{}{"query": "madpla", "project_path": dir, "session_id": "s1"})
	if res.IsError {
		t.Fatalf("la búsqueda falló: %s", texto(res))
	}
	if !strings.Contains(texto(res), "1. [42] MADPLA - Planificación") {
		t.Errorf("falta la candidata numerada: %s", texto(res))
	}
	if !strings.Contains(texto(res), "Pregunta al usuario") || !strings.Contains(texto(res), "project_id") {
		t.Errorf("falta la indicación de preguntar al usuario: %s", texto(res))
	}
	if _, err := os.Stat(filepath.Join(dir, ".planesgo.json")); !os.IsNotExist(err) {
		t.Errorf("buscar no debe escribir .planesgo.json")
	}
	if p := sessionConfigPath("s1"); fileExists(p) {
		t.Errorf("buscar no debe escribir la configuración de sesión")
	}
	if api.llamadas("/antigravity/status") != 0 || api.llamadas("/antigravity/update_tasks") != 0 {
		t.Errorf("buscar solo debe consultar la búsqueda: %v", api.rutas)
	}

	// Sin coincidencias
	if res := executeToolCall("pgo_project", map[string]interface{}{"query": "nada-parecido"}); res.IsError || strings.Contains(texto(res), "Pregunta") {
		t.Errorf("sin coincidencias: %s", texto(res))
	}
	// Sin query ni project_id
	if res := executeToolCall("pgo_project", map[string]interface{}{}); !res.IsError {
		t.Errorf("sin query ni project_id debe dar error")
	}
}

func TestPgoProjectVincula(t *testing.T) {
	entornoPrueba(t)
	dir := t.TempDir()

	res := executeToolCall("pgo_project", map[string]interface{}{"project_id": float64(42), "project_path": dir})
	if res.IsError || !strings.Contains(texto(res), "MADPLA - Planificación") {
		t.Fatalf("vincular falló: %s", texto(res))
	}
	cfg, _, err := findConfig(dir)
	if err != nil {
		t.Fatalf("no se escribió .planesgo.json: %v", err)
	}
	if cfg.OdooProjectID != 42 || cfg.OdooProjectName != "MADPLA - Planificación" {
		t.Errorf("configuración inesperada: %+v", cfg)
	}

	if res := executeToolCall("pgo_project", map[string]interface{}{"project_id": float64(999), "project_path": dir}); !res.IsError {
		t.Errorf("un proyecto inexistente debe dar error: %s", texto(res))
	}
}

func TestPgoTimeConvierteMinutosYPasaLaTarea(t *testing.T) {
	api := entornoPrueba(t)
	dir := t.TempDir()
	if res := executeToolCall("pgo_project", map[string]interface{}{"project_id": float64(42), "project_path": dir}); res.IsError {
		t.Fatal(texto(res))
	}

	res := executeToolCall("pgo_time", map[string]interface{}{"minutes": float64(30), "task": "Pruebas de integración", "description": "Batería de pruebas", "project_path": dir})
	if res.IsError {
		t.Fatalf("pgo_time falló: %s", texto(res))
	}
	if texto(res) != "Añadidos 30 min a MADPLA - Planificación · Pruebas" {
		t.Errorf("texto inesperado: %q", texto(res))
	}
	l := api.copiaLatidos()
	if len(l) != 2 || l[0]["action"] != "heartbeat" || l[1]["action"] != "stop" {
		t.Fatalf("se esperaba un latido y un stop: %v", l)
	}
	if l[0]["elapsed_seconds"] != float64(1800) {
		t.Errorf("30 min deben ser 1800 s, son %v", l[0]["elapsed_seconds"])
	}
	if l[0]["task_name"] != "Pruebas" || l[0]["project_id"] != float64(42) || l[0]["description"] != "Batería de pruebas" {
		t.Errorf("latido inesperado: %v", l[0])
	}
	if _, ok := l[1]["elapsed_seconds"]; ok {
		t.Errorf("el stop no debe sumar segundos: %v", l[1])
	}

	// Sin tarea: la tarea por defecto (la de los latidos automáticos), que el servidor deja en «Desarrollo»
	res = executeToolCall("pgo_time", map[string]interface{}{"minutes": 1.5, "project_path": dir})
	l = api.copiaLatidos()
	if res.IsError || l[2]["elapsed_seconds"] != float64(90) || l[2]["task_name"] != "Desarrollo" || l[2]["task_type"] != "Desarrollo" {
		t.Errorf("sin tarea: %s / %v", texto(res), l[2])
	}

	for _, m := range []interface{}{float64(0), float64(-5), float64(2000), nil} {
		if res := executeToolCall("pgo_time", map[string]interface{}{"minutes": m, "project_path": dir}); !res.IsError {
			t.Errorf("minutes=%v debe dar error", m)
		}
	}
	if res := executeToolCall("pgo_time", map[string]interface{}{"minutes": float64(5), "project_path": t.TempDir()}); !res.IsError {
		t.Errorf("sin proyecto vinculado debe dar error: %s", texto(res))
	}
}

func cli(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	codigo, servidor := ejecutaCLI(args, &out, &errOut)
	if servidor {
		t.Fatalf("%v no debía arrancar el servidor MCP", args)
	}
	return codigo, out.String() + errOut.String()
}

func TestCLIBeatYStopSeMantienen(t *testing.T) {
	api := entornoPrueba(t)
	dir := t.TempDir()

	codigo, salida := cli(t, "--beat", "--path", dir, "--project-id", "101", "--task", "Claude Code - Sesión IA",
		"--type", "Desarrollo", "--desc", "Cambios en el login", "--model", "Claude Code", "--session-id", "sess-9",
		"--tokens", "15", "--tokens-in", "10", "--tokens-out", "5")
	if codigo != 0 {
		t.Fatalf("--beat falló (%d): %s", codigo, salida)
	}
	codigo, salida = cli(t, "--stop", "--path", dir, "--project-id", "101", "--task", "Claude Code - Sesión IA",
		"--desc", "Cambios en el login", "--model", "Claude Code", "--session-id", "sess-9", "--tokens", "20")
	if codigo != 0 || !strings.Contains(salida, "Imputado") {
		t.Fatalf("--stop falló (%d): %s", codigo, salida)
	}

	l := api.copiaLatidos()
	if len(l) != 2 {
		t.Fatalf("se esperaban 2 envíos: %v", l)
	}
	beat, stop := l[0], l[1]
	if beat["action"] != "heartbeat" || beat["project_id"] != float64(101) || beat["project_name"] != "PRO1 - CUENTA CARGO 1" ||
		beat["task_name"] != "Desarrollo" || beat["description"] != "Cambios en el login" || beat["ai_model"] != "Claude Code" ||
		beat["ai_session_id"] != "sess-9" || beat["tokens_total"] != float64(15) || beat["tokens_input"] != float64(10) || beat["tokens_output"] != float64(5) {
		t.Errorf("latido inesperado: %v", beat)
	}
	if stop["action"] != "stop" || stop["project_id"] != float64(101) || stop["tokens_total"] != float64(20) {
		t.Errorf("stop inesperado: %v", stop)
	}

	// --check con --project-id sigue funcionando
	if codigo, salida := cli(t, "--check", "--path", dir, "--project-id", "202"); codigo != 0 || !strings.Contains(salida, "PRO2") {
		t.Errorf("--check falló (%d): %s", codigo, salida)
	}
	// --beat sin --task es un error, como antes
	if codigo, _ := cli(t, "--beat", "--path", dir, "--project-id", "101"); codigo == 0 {
		t.Errorf("--beat sin --task debe fallar")
	}
}

func TestCLIProyectoYTiempo(t *testing.T) {
	api := entornoPrueba(t)
	dir := t.TempDir()

	// --project busca y no vincula
	codigo, salida := cli(t, "--project", "madpla", "--path", dir)
	if codigo != 0 || !strings.Contains(salida, "[42] MADPLA") || !strings.Contains(salida, "--link") {
		t.Fatalf("--project (%d): %s", codigo, salida)
	}
	if _, err := os.Stat(filepath.Join(dir, ".planesgo.json")); !os.IsNotExist(err) {
		t.Fatalf("--project no debe vincular")
	}
	// --project-id sin --link no vincula
	if codigo, _ := cli(t, "--project-id", "42", "--path", dir); codigo == 0 {
		t.Errorf("--project-id sin --link debe fallar")
	}
	// --link sin --project-id
	if codigo, _ := cli(t, "--link", "--path", dir); codigo == 0 {
		t.Errorf("--link sin --project-id debe fallar")
	}
	// --project-id N --link vincula
	if codigo, salida := cli(t, "--project-id", "42", "--link", "--path", dir); codigo != 0 || !strings.Contains(salida, "MADPLA") {
		t.Fatalf("--link (%d): %s", codigo, salida)
	}
	// --time con --task
	codigo, salida = cli(t, "--time", "45", "--task", "Revisión de diseño", "--path", dir)
	if codigo != 0 || !strings.Contains(salida, "Añadidos 45 min a MADPLA - Planificación · Análisis y diseño") {
		t.Fatalf("--time (%d): %s", codigo, salida)
	}
	if l := api.copiaLatidos(); l[0]["elapsed_seconds"] != float64(2700) {
		t.Errorf("45 min deben ser 2700 s: %v", l[0])
	}
	if codigo, _ := cli(t, "--time", "0", "--path", dir); codigo == 0 {
		t.Errorf("--time 0 debe fallar")
	}

	// Opciones retiradas
	for _, quitada := range []string{"--list", "--close-ticket=T1", "--ticket=T1", "--subject=x", "--stream=on", "--add-hours=1", "--search-project=x", "--set-project=x", "--init=x"} {
		if codigo, _ := cli(t, quitada); codigo != 2 {
			t.Errorf("%s debía estar retirada (código %d)", quitada, codigo)
		}
	}
	if codigo, salida := cli(t, "--version"); codigo != 0 || !strings.HasPrefix(salida, "pgo-mcp v") {
		t.Errorf("--version: %s", salida)
	}
	if _, servidor := ejecutaCLI(nil, &bytes.Buffer{}, &bytes.Buffer{}); !servidor {
		t.Errorf("sin opciones debe arrancar el servidor MCP")
	}
}

// Dos sesiones en el mismo repositorio imputan cada una a su proyecto, y --project-id manda sobre la sesión.
func TestMultisesion(t *testing.T) {
	api := entornoPrueba(t)
	repo := t.TempDir()

	for _, v := range []struct {
		sesion string
		id     float64
	}{{"sess-1", 101}, {"sess-2", 202}} {
		if res := executeToolCall("pgo_project", map[string]interface{}{"project_id": v.id, "project_path": repo, "session_id": v.sesion}); res.IsError {
			t.Fatalf("vincular %s: %s", v.sesion, texto(res))
		}
	}
	// El .planesgo.json del repositorio se queda con el primero
	if cfg, _, err := findConfig(repo); err != nil || cfg.OdooProjectID != 101 {
		t.Fatalf("el .planesgo.json del repositorio no debía pisarse: %+v %v", cfg, err)
	}

	for _, s := range []string{"sess-1", "sess-2"} {
		if res := ordenBeat(map[string]interface{}{"task_name": "Desarrollo", "project_path": repo, "session_id": s, "ai_model": "Claude Code"}); res.IsError {
			t.Fatalf("beat %s: %s", s, texto(res))
		}
	}
	if res := ordenStop(map[string]interface{}{"task_name": "Desarrollo", "project_path": repo, "session_id": "sess-1", "project_id": float64(303), "ai_model": "Claude Code"}); res.IsError {
		t.Fatalf("stop: %s", texto(res))
	}
	l := api.copiaLatidos()
	if l[0]["project_id"] != float64(101) || l[0]["ai_session_id"] != "sess-1" {
		t.Errorf("sess-1: %v", l[0])
	}
	if l[1]["project_id"] != float64(202) || l[1]["ai_session_id"] != "sess-2" {
		t.Errorf("sess-2: %v", l[1])
	}
	if l[2]["project_id"] != float64(303) || l[2]["project_name"] != "PRO3 - OTRO" {
		t.Errorf("project_id explícito: %v", l[2])
	}
}
