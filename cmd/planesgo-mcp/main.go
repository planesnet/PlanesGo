package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	Version       = "1.2.45"
	DefaultServer = "https://planesgo.autopyme.com"

	TaskTypeAnalisisDiseno = "Análisis y diseño"
	TaskTypeDesarrollo     = "Desarrollo"
	TaskTypePruebas        = "Pruebas"
)

var CanonicalTaskTypes = []string{
	TaskTypeAnalisisDiseno,
	TaskTypeDesarrollo,
	TaskTypePruebas,
}

func matchCanonicalType(t string) (string, bool) {
	norm := strings.ToLower(strings.TrimSpace(t))
	norm = strings.ReplaceAll(norm, "á", "a")
	norm = strings.ReplaceAll(norm, "é", "e")
	norm = strings.ReplaceAll(norm, "í", "i")
	norm = strings.ReplaceAll(norm, "ó", "o")
	norm = strings.ReplaceAll(norm, "ú", "u")
	norm = strings.ReplaceAll(norm, "ñ", "n")

	switch norm {
	case "analisis y diseno", "analisis y diseño", "análisis y diseño", "analisis", "análisis", "analysis", "investigacion", "auditoria", "diseno", "diseño", "design", "planificacion", "arquitectura":
		return TaskTypeAnalisisDiseno, true
	case "desarrollo", "dev", "development", "implementacion", "implementación", "impl", "ajuste", "ajustes", "fix", "fixes", "bugfix", "refactor", "servidor", "server", "infraestructura", "infra", "ops", "cliente", "client", "soporte", "support":
		return TaskTypeDesarrollo, true
	case "pruebas", "prueba", "test", "tests", "testing", "qa", "verificacion", "validacion":
		return TaskTypePruebas, true
	}
	return "", false
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func cleanAntigravityTaskName(taskName string) string {
	name := strings.TrimSpace(taskName)
	for {
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "[AGY]") {
			name = strings.TrimSpace(name[5:])
			continue
		}
		if strings.HasPrefix(upper, "[ANTIGRAVITY]") {
			name = strings.TrimSpace(name[14:])
			continue
		}
		break
	}
	return name
}

func NormalizeTaskType(taskName, description, explicitType string) (string, string) {
	raw := cleanAntigravityTaskName(taskName)

	var detectedType string

	// 1. Si el nombre ya comienza por un corchete de tipo ej. [Análisis y diseño] o [Desarrollo] o [Pruebas]
	if strings.HasPrefix(raw, "[") {
		idx := strings.Index(raw, "]")
		if idx > 1 {
			bracketContent := raw[1:idx]
			if canon, ok := matchCanonicalType(bracketContent); ok {
				detectedType = canon
			}
		}
	}

	// 2. Si no se detectó en corchetes pero se pasó explicitType
	if detectedType == "" && explicitType != "" {
		if canon, ok := matchCanonicalType(explicitType); ok {
			detectedType = canon
		}
	}

	// 3. Inferencia automática por heurística semántica:
	// Priorizar análisis del título/nombre para evitar que palabras accesorias de la descripción sobreescriban el tipo.
	if detectedType == "" {
		rawNorm := strings.ToLower(raw)
		rawNorm = strings.ReplaceAll(rawNorm, "á", "a")
		rawNorm = strings.ReplaceAll(rawNorm, "é", "e")
		rawNorm = strings.ReplaceAll(rawNorm, "í", "i")
		rawNorm = strings.ReplaceAll(rawNorm, "ó", "o")
		rawNorm = strings.ReplaceAll(rawNorm, "ú", "u")
		rawNorm = strings.ReplaceAll(rawNorm, "ñ", "n")

		if containsAny(rawNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(rawNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "revis", "planificac", "explorac", "research", "benchmark", "inspecc", "arquitect") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(rawNorm, "desarroll", "implementac", "servidor", "server", "systemd", "nginx", "apache", "docker", "deploy", "despliegue", "ssh", "puerto", "backup", "cron", "proxy", "daemon", "demon", "firewall", "sysadmin", "cliente", "usuario", "soporte", "ticket", "reunion", "consulta", "duda", "demo", "capacitacion", "formacion", "funcional", "tarifa", "ajuste", "ajustes", "fix", "bug", "error", "correccion", "corregir", "refactor", "tweak", "patch", "parche", "limpieza", "lint", "linter", "estilo", "padding", "css", "tipografia") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 4. Si el título no arrojó coincidencias, inspeccionar la descripción
	if detectedType == "" && description != "" {
		descNorm := strings.ToLower(description)
		descNorm = strings.ReplaceAll(descNorm, "á", "a")
		descNorm = strings.ReplaceAll(descNorm, "é", "e")
		descNorm = strings.ReplaceAll(descNorm, "í", "i")
		descNorm = strings.ReplaceAll(descNorm, "ó", "o")
		descNorm = strings.ReplaceAll(descNorm, "ú", "u")
		descNorm = strings.ReplaceAll(descNorm, "ñ", "n")

		if containsAny(descNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(descNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "planificac") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(descNorm, "desarroll", "implementac", "servidor", "server", "systemd", "docker", "deploy", "cliente", "ajuste", "fix", "bug") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 5. Fallback por defecto: Desarrollo
	if detectedType == "" {
		detectedType = TaskTypeDesarrollo
	}

	// El nombre de la tarea es exclusivamente el nombre canónico sin prefijos [AGY]
	return detectedType, detectedType
}

// Config representa el archivo .planesgo.json encontrado en el proyecto
type Config struct {
	OdooProjectID            int    `json:"odoo_project_id"`
	OdooProjectName          string `json:"odoo_project_name"`
	PlanesGoURL              string `json:"planesgo_url"`
	AutoCreateTasks          bool   `json:"auto_create_tasks"`
	HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	IdleTimeoutMinutes       int    `json:"idle_timeout_minutes"`
}

// AuthConfig representa el archivo ~/.planesgo_auth.json
type AuthConfig struct {
	AntigravityToken string `json:"antigravity_token"`
	PlanesGoURL      string `json:"planesgo_url"`
	UserEmail        string `json:"user_email"`
}

// JSON-RPC y MCP tipos
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError"`
}

// findConfig busca .planesgo.json hacia arriba desde el directorio especificado o de trabajo
func findConfig(customDir ...string) (*Config, string, error) {
	var startDirs []string
	for _, d := range customDir {
		if strings.TrimSpace(d) != "" {
			startDirs = append(startDirs, strings.TrimSpace(d))
		}
	}
	if len(startDirs) == 0 {
		if wd, err := os.Getwd(); err == nil && wd != "" {
			startDirs = append(startDirs, wd)
		} else if pwd := os.Getenv("PWD"); pwd != "" {
			startDirs = append(startDirs, pwd)
		}
	}

	for _, dir := range startDirs {
		curr, err := filepath.Abs(dir)
		if err != nil {
			curr = dir
		}
		for {
			candidate := filepath.Join(curr, ".planesgo.json")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				data, err := os.ReadFile(candidate)
				if err != nil {
					return nil, candidate, err
				}
				var cfg Config
				if err := json.Unmarshal(data, &cfg); err != nil {
					return nil, candidate, fmt.Errorf("error al parsear .planesgo.json: %w", err)
				}
				return &cfg, candidate, nil
			}

			parent := filepath.Dir(curr)
			if parent == curr {
				break
			}
			curr = parent
		}
	}

	return nil, "", fmt.Errorf("no se encontró .planesgo.json en el directorio actual ni en sus padres")
}

// getAuth obtiene el token y la URL de PlanesGo
func getAuth(cfg *Config) (token string, apiURL string, err error) {
	apiURL = DefaultServer
	if cfg != nil && cfg.PlanesGoURL != "" {
		apiURL = cfg.PlanesGoURL
	}

	token = strings.TrimSpace(os.Getenv("ANTIGRAVITY_TOKEN"))
	if token != "" {
		return token, cleanURL(apiURL), nil
	}

	home, err := os.UserHomeDir()
	if err == nil {
		authPath := filepath.Join(home, ".planesgo_auth.json")
		if data, err := os.ReadFile(authPath); err == nil {
			var auth AuthConfig
			if err := json.Unmarshal(data, &auth); err == nil {
				if auth.AntigravityToken != "" {
					token = auth.AntigravityToken
				}
				if auth.PlanesGoURL != "" && (cfg == nil || cfg.PlanesGoURL == "") {
					apiURL = auth.PlanesGoURL
				}
			}
		}
	}

	if token == "" {
		return "", cleanURL(apiURL), fmt.Errorf("token de autenticación no encontrado en $ANTIGRAVITY_TOKEN ni en ~/.planesgo_auth.json")
	}

	return token, cleanURL(apiURL), nil
}

func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || strings.Contains(u, "localhost") || strings.Contains(u, "8089") {
		return DefaultServer
	}
	return strings.TrimRight(u, "/")
}

// PlanesGoClient gestiona peticiones HTTP a PlanesGo
type PlanesGoClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func newClient(customDir ...string) (*PlanesGoClient, *Config, error) {
	cfg, _, findErr := findConfig(customDir...)
	token, apiURL, authErr := getAuth(cfg)
	if authErr != nil {
		return nil, cfg, authErr
	}
	return &PlanesGoClient{
		BaseURL: apiURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}, cfg, findErr
}

// CheckStatus ejecuta la verificación de Fase 0
func (c *PlanesGoClient) CheckStatus(projectID int, projectName string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/antigravity/status", c.BaseURL)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	if projectID > 0 {
		q.Set("project_id", strconv.Itoa(projectID))
	}
	if projectName != "" {
		q.Set("project_name", projectName)
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Antigravity-Token", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error de conexión con PlanesGo (%s): %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("respuesta inválida de PlanesGo (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := "error desconocido"
		if msg, ok := res["error"].(string); ok {
			errMsg = msg
		}
		return res, fmt.Errorf("PlanesGo HTTP %d: %s", resp.StatusCode, errMsg)
	}

	return res, nil
}

// SendTaskAction envía latido o stop a PlanesGo
func (c *PlanesGoClient) SendTaskAction(action, taskName string, taskID, projectID int, projectName, description, taskType string) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/antigravity/update_tasks", c.BaseURL)

	payload := map[string]interface{}{
		"action":       action,
		"task_name":    taskName,
		"task_id":      taskID,
		"project_id":   projectID,
		"project_name": projectName,
		"description":  description,
		"task_type":    taskType,
		"token":        c.Token,
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Antigravity-Token", c.Token)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error de conexión con PlanesGo: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var res map[string]interface{}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("respuesta inválida de PlanesGo (HTTP %d): %s", resp.StatusCode, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		errMsg := "error desconocido"
		if msg, ok := res["error"].(string); ok {
			errMsg = msg
		}
		return res, fmt.Errorf("PlanesGo HTTP %d: %s", resp.StatusCode, errMsg)
	}

	return res, nil
}

// ListTasks lista tareas abiertas en Odoo para un proyecto
func (c *PlanesGoClient) ListTasks(projectID int, projectName string) ([]map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/antigravity/tasks", c.BaseURL)
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	if projectID > 0 {
		q.Set("project_id", strconv.Itoa(projectID))
	}
	if projectName != "" {
		q.Set("project_name", projectName)
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Antigravity-Token", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error al conectar con PlanesGo: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PlanesGo HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tasks []map[string]interface{}
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, fmt.Errorf("error al parsear tareas: %w", err)
	}

	return tasks, nil
}

// executeToolCall ejecuta la herramienta solicitada por el protocolo MCP
func executeToolCall(name string, args map[string]interface{}) ToolCallResult {
	var customPath string
	if p, ok := args["project_path"].(string); ok && p != "" {
		customPath = p
	} else if p, ok := args["path"].(string); ok && p != "" {
		customPath = p
	} else if p, ok := args["cwd"].(string); ok && p != "" {
		customPath = p
	}

	client, cfg, err := newClient(customPath)
	if err != nil && name != "planesgo_set_project" {
		return ToolCallResult{
			Content: []ToolContent{{
				Type: "text",
				Text: fmt.Sprintf("❌ [Fase 0 BLOQUEO MANDATORIO]: Este proyecto no está vinculado con PlanesGo.\n" +
					"- Detalle: %v\n" +
					"- Acción requerida: Crear archivo .planesgo.json con odoo_project_id y odoo_project_name válidos.\n" +
					"Queda TERMINANTEMENTE PROHIBIDO realizar cualquier modificación, ejecución de comandos o desarrollo en este repositorio.", err),
			}},
			IsError: true,
		}
	}

	// Resolver proyecto por defecto desde .planesgo.json si existe
	projID := 0
	projName := ""
	if cfg != nil {
		projID = cfg.OdooProjectID
		projName = cfg.OdooProjectName
	}

	if val, ok := args["project_id"]; ok {
		switch v := val.(type) {
		case float64:
			projID = int(v)
		case int:
			projID = v
		case string:
			if id, err := strconv.Atoi(v); err == nil {
				projID = id
			}
		}
	}
	if val, ok := args["project_name"].(string); ok && val != "" {
		projName = val
	}

	switch name {
	case "planesgo_check":
		if cfg == nil || projID <= 0 {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: "❌ [Fase 0 BLOQUEO MANDATORIO]: Este proyecto no está vinculado con PlanesGo.\n" +
						"- Motivo: No se encontró el archivo .planesgo.json en este proyecto (o carece de odoo_project_id válido).\n" +
						"- Acción requerida: Crear .planesgo.json con odoo_project_id y odoo_project_name antes de operar.\n" +
						"Queda TERMINANTEMENTE PROHIBIDO realizar cualquier modificación, ejecución de comandos o desarrollo en este repositorio.",
				}},
				IsError: true,
			}
		}

		taskName, _ := args["task_name"].(string)

		res, err := client.CheckStatus(projID, projName)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: fmt.Sprintf("❌ [Fase 0 BLOQUEO MANDATORIO]: Error en verificación con PlanesGo: %v\nQueda TERMINANTEMENTE PROHIBIDO realizar cualquier modificación o desarrollo en este repositorio.", err),
				}},
				IsError: true,
			}
		}

		userEmail, _ := res["user_email"].(string)
		pName, _ := res["project_name"].(string)
		if pName == "" {
			pName = projName
		}
		pID := projID
		if idVal, ok := res["project_id"].(float64); ok && int(idVal) > 0 {
			pID = int(idVal)
		}

		// Si no se proporcionó taskName, comprobar si hay una tarea activa en PlanesGo
		if taskName == "" {
			if timers, ok := res["active_timers"].([]interface{}); ok && len(timers) > 0 {
				if tMap, ok := timers[0].(map[string]interface{}); ok {
					if tName, ok := tMap["task_name"].(string); ok && tName != "" {
						taskName = tName
					}
					if pName == "" {
						if timerProj, ok := tMap["project_name"].(string); ok {
							pName = timerProj
						}
					}
					if pID == 0 {
						if timerPID, ok := tMap["project_id"].(float64); ok {
							pID = int(timerPID)
						}
					}
				}
			}
		}

		taskType, _ := args["task_type"].(string)
		desc, _ := args["description"].(string)
		if taskName != "" && taskName != "Pendiente de asignar en latido" {
			_, normName := NormalizeTaskType(taskName, desc, taskType)
			taskName = normName
		}

		projStr := "No detectado"
		if pName != "" && pID > 0 {
			projStr = fmt.Sprintf("%s (ID: %d)", pName, pID)
		} else if pName != "" {
			projStr = pName
		} else if pID > 0 {
			projStr = fmt.Sprintf("ID: %d", pID)
		}

		msg := fmt.Sprintf("✅ [PSF Prerrequisito OK] Conectado exitosamente a PlanesGo.\n- Servidor: %s\n- Empleado: %s\n- Proyecto: %s\n- Tarea de imputación: %s",
			client.BaseURL, userEmail, projStr, taskName)
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: msg}},
			IsError: false,
		}

	case "planesgo_beat":
		if cfg == nil || projID <= 0 {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: "❌ [Fase 0 BLOQUEO MANDATORIO]: No se puede registrar latido. Este proyecto no está vinculado con PlanesGo (.planesgo.json no encontrado o sin odoo_project_id válido).\nQueda TERMINANTEMENTE PROHIBIDO realizar modificaciones sin imputación horaria activa.",
				}},
				IsError: true,
			}
		}

		taskName, _ := args["task_name"].(string)
		if taskName == "" {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: "❌ El argumento 'task_name' es obligatorio para enviar un latido"}},
				IsError: true,
			}
		}

		desc, _ := args["description"].(string)
		taskType, _ := args["task_type"].(string)
		taskID := 0
		if val, ok := args["task_id"]; ok {
			if idFloat, ok := val.(float64); ok {
				taskID = int(idFloat)
			}
		}

		canonicalType, normalizedTaskName := NormalizeTaskType(taskName, desc, taskType)

		res, err := client.SendTaskAction("heartbeat", normalizedTaskName, taskID, projID, projName, desc, canonicalType)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("❌ Error al enviar latido a PlanesGo: %v", err)}},
				IsError: true,
			}
		}

		msg, _ := res["message"].(string)
		projStr := projName
		if projID > 0 && projName != "" {
			projStr = fmt.Sprintf("%s (ID: %d)", projName, projID)
		} else if projID > 0 {
			projStr = fmt.Sprintf("ID: %d", projID)
		} else if projStr == "" {
			projStr = "No detectado"
		}

		output := fmt.Sprintf("⏱️ [PlanesGo] Latido registrado con éxito.\n- Proyecto: %s\n- Tarea de imputación: %s\n- Tipo: %s", projStr, normalizedTaskName, canonicalType)
		if desc != "" {
			output += fmt.Sprintf("\n- Resumen del parte: %s", desc)
		}
		if msg != "" {
			output += fmt.Sprintf("\n- Estado: %s", msg)
		}
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: output}},
			IsError: false,
		}

	case "planesgo_stop":
		if cfg == nil || projID <= 0 {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: "❌ [Fase 0 BLOQUEO MANDATORIO]: No se puede cerrar imputación. Este proyecto no está vinculado con PlanesGo (.planesgo.json no encontrado o sin odoo_project_id válido).",
				}},
				IsError: true,
			}
		}

		taskName, _ := args["task_name"].(string)
		desc, _ := args["description"].(string)
		taskType, _ := args["task_type"].(string)
		taskID := 0
		if val, ok := args["task_id"]; ok {
			if idFloat, ok := val.(float64); ok {
				taskID = int(idFloat)
			}
		}

		canonicalType, normalizedTaskName := NormalizeTaskType(taskName, desc, taskType)
		finalDesc := strings.TrimSpace(desc)
		if finalDesc == "" || finalDesc == "Trabajo en curso" {
			finalDesc = cleanAntigravityTaskName(taskName)
		}

		res, err := client.SendTaskAction("stop", normalizedTaskName, taskID, projID, projName, finalDesc, canonicalType)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("❌ Error al detener tarea en PlanesGo: %v", err)}},
				IsError: true,
			}
		}

		msg, _ := res["message"].(string)
		projStr := projName
		if projID > 0 && projName != "" {
			projStr = fmt.Sprintf("%s (ID: %d)", projName, projID)
		} else if projID > 0 {
			projStr = fmt.Sprintf("ID: %d", projID)
		} else if projStr == "" {
			projStr = "No detectado"
		}

		output := fmt.Sprintf("⏹️ [PlanesGo] Tarea finalizada e imputada en Odoo.\n- Proyecto: %s\n- Tarea: %s\n- Tipo: %s\n- Resumen del parte: %s", projStr, normalizedTaskName, canonicalType, finalDesc)
		if msg != "" {
			output += fmt.Sprintf("\n- Estado: %s", msg)
		}
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: output}},
			IsError: false,
		}

	case "planesgo_list_tasks":
		if cfg == nil || projID <= 0 {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: "❌ [Fase 0 BLOQUEO MANDATORIO]: No se pueden listar tareas. Este proyecto no está vinculado con PlanesGo (.planesgo.json no encontrado o sin odoo_project_id válido).",
				}},
				IsError: true,
			}
		}
		tasks, err := client.ListTasks(projID, projName)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("❌ Error al listar tareas: %v", err)}},
				IsError: true,
			}
		}

		if len(tasks) == 0 {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("No se encontraron tareas asignadas en Odoo para el proyecto %s (ID %d).", projName, projID)}},
				IsError: false,
			}
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📋 Tareas abiertas en Odoo para '%s' (ID %d):\n", projName, projID))
		for _, t := range tasks {
			id, _ := t["id"].(float64)
			name, _ := t["name"].(string)
			sb.WriteString(fmt.Sprintf("- [%d] %s\n", int(id), name))
		}
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: sb.String()}},
			IsError: false,
		}

	case "planesgo_status":
		res, err := client.CheckStatus(projID, projName)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("❌ Error al consultar status: %v", err)}},
				IsError: true,
			}
		}

		jsonPretty, _ := json.MarshalIndent(res, "", "  ")
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: string(jsonPretty)}},
			IsError: false,
		}

	case "planesgo_set_project":
		targetProjName, _ := args["project_name"].(string)
		targetProjID := 0
		if val, ok := args["project_id"]; ok {
			switch v := val.(type) {
			case float64:
				targetProjID = int(v)
			case int:
				targetProjID = v
			case string:
				if id, err := strconv.Atoi(v); err == nil {
					targetProjID = id
				}
			}
		}

		if strings.TrimSpace(targetProjName) == "" && targetProjID <= 0 {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: "❌ Debe indicar 'project_name' o 'project_id' para configurar el proyecto en PlanesGo.",
				}},
				IsError: true,
			}
		}

		token, apiURL, authErr := getAuth(cfg)
		if authErr != nil {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: fmt.Sprintf("❌ Error de autenticación: %v", authErr),
				}},
				IsError: true,
			}
		}

		if client == nil {
			client = &PlanesGoClient{
				BaseURL:    apiURL,
				Token:      token,
				HTTPClient: &http.Client{Timeout: 15 * time.Second},
			}
		}

		res, err := client.CheckStatus(targetProjID, targetProjName)
		if err != nil {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: fmt.Sprintf("❌ Error al validar el proyecto '%s' en PlanesGo / Odoo: %v", targetProjName, err),
				}},
				IsError: true,
			}
		}

		resolvedName, _ := res["project_name"].(string)
		if resolvedName == "" {
			resolvedName = targetProjName
		}
		resolvedID := targetProjID
		if idVal, ok := res["project_id"].(float64); ok && int(idVal) > 0 {
			resolvedID = int(idVal)
		}

		targetDir := customPath
		if targetDir == "" {
			if wd, err := os.Getwd(); err == nil && wd != "" {
				targetDir = wd
			} else if pwd := os.Getenv("PWD"); pwd != "" {
				targetDir = pwd
			}
		}
		targetDir, _ = filepath.Abs(targetDir)

		newCfg := Config{
			OdooProjectID:   resolvedID,
			OdooProjectName: resolvedName,
			PlanesGoURL:     apiURL,
		}
		data, mErr := json.MarshalIndent(newCfg, "", "  ")
		if mErr != nil {
			return ToolCallResult{
				Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("❌ Error serializando configuración: %v", mErr)}},
				IsError: true,
			}
		}
		data = append(data, '\n')

		mainFilePath := filepath.Join(targetDir, ".planesgo.json")
		if err := os.WriteFile(mainFilePath, data, 0644); err != nil {
			return ToolCallResult{
				Content: []ToolContent{{
					Type: "text",
					Text: fmt.Sprintf("❌ Error al guardar archivo %s: %v", mainFilePath, err),
				}},
				IsError: true,
			}
		}

		syncedFiles := []string{mainFilePath}
		customSubdir := filepath.Join(targetDir, "custom")
		if info, err := os.Stat(customSubdir); err == nil && info.IsDir() {
			customPath := filepath.Join(customSubdir, ".planesgo.json")
			if err := os.WriteFile(customPath, data, 0644); err == nil {
				syncedFiles = append(syncedFiles, customPath)
			}
		} else if filepath.Base(targetDir) == "custom" {
			parentPath := filepath.Join(filepath.Dir(targetDir), ".planesgo.json")
			if err := os.WriteFile(parentPath, data, 0644); err == nil {
				syncedFiles = append(syncedFiles, parentPath)
			}
		}

		var sb strings.Builder
		sb.WriteString("✅ [PlanesGo] Configuración de proyecto vinculada con éxito:\n")
		sb.WriteString(fmt.Sprintf("- Proyecto: %s (ID Odoo: %d)\n", resolvedName, resolvedID))
		sb.WriteString(fmt.Sprintf("- Servidor: %s\n", apiURL))
		sb.WriteString(fmt.Sprintf("- Archivos configurados: %s\n", strings.Join(syncedFiles, ", ")))
		sb.WriteString("- Estado: Vinculación activa y operativa para imputación horaria.")

		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: sb.String()}},
			IsError: false,
		}

	default:
		return ToolCallResult{
			Content: []ToolContent{{Type: "text", Text: fmt.Sprintf("Herramienta '%s' no reconocida", name)}},
			IsError: true,
		}
	}
}

func getToolsDefinition() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "planesgo_check",
			"description": "Comprueba el estado de conexión con PlanesGo y Odoo para el proyecto actual (Fase 0 mandatoria de PSF). Valida el token, la vinculación del proyecto y la tarea sobre la que se imputará.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task_name": map[string]interface{}{
						"type":        "string",
						"description": "Nombre de la tarea sobre la que se van a imputar horas",
					},
					"task_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"Análisis y diseño", "Desarrollo", "Pruebas"},
						"description": "Tipo normalizado de tarea: Análisis y diseño, Desarrollo, Pruebas (opcional, se infiere automáticamente si se omite)",
					},
					"project_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID numérico del proyecto en Odoo (opcional, se autodetecta de .planesgo.json)",
					},
					"project_name": map[string]interface{}{
						"type":        "string",
						"description": "Nombre del proyecto en Odoo (opcional, se autodetecta de .planesgo.json)",
					},
					"project_path": map[string]interface{}{
						"type":        "string",
						"description": "Ruta al directorio del proyecto donde se ubica .planesgo.json (opcional)",
					},
				},
			},
		},
		{
			"name":        "planesgo_beat",
			"description": "Envía un latido periódico de telemetría e imputación en tiempo real a PlanesGo y Odoo para la tarea en curso.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task_name": map[string]interface{}{
						"type":        "string",
						"description": "Nombre descriptivo de la tarea que se está ejecutando",
					},
					"task_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"Análisis y diseño", "Desarrollo", "Pruebas"},
						"description": "Tipo normalizado de tarea: Análisis y diseño, Desarrollo, Pruebas (opcional, se infiere si se omite)",
					},
					"task_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID numérico de la tarea en Odoo si ya existe (opcional)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Resumen breve del trabajo o progreso actual para el parte de horas en Odoo",
					},
					"project_path": map[string]interface{}{
						"type":        "string",
						"description": "Ruta al directorio del proyecto donde se ubica .planesgo.json (opcional)",
					},
				},
				"required": []string{"task_name"},
			},
		},
		{
			"name":        "planesgo_stop",
			"description": "Detiene el seguimiento de la tarea y consolida las horas definitivas en Odoo.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task_name": map[string]interface{}{
						"type":        "string",
						"description": "Nombre de la tarea a detener",
					},
					"task_type": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"Análisis y diseño", "Desarrollo", "Pruebas"},
						"description": "Tipo normalizado de tarea: Análisis y diseño, Desarrollo, Pruebas (opcional, se infiere si se omite)",
					},
					"task_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID numérico de la tarea a detener (opcional)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Resumen breve, claro y sustantivo del trabajo realizado para el parte de horas en Odoo",
					},
					"project_path": map[string]interface{}{
						"type":        "string",
						"description": "Ruta al directorio del proyecto donde se ubica .planesgo.json (opcional)",
					},
				},
			},
		},
		{
			"name":        "planesgo_list_tasks",
			"description": "Lista las tareas abiertas asignadas al usuario para el proyecto vinculado en Odoo.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID numérico del proyecto (opcional)",
					},
				},
			},
		},
		{
			"name":        "planesgo_status",
			"description": "Consulta el temporizador activo actual y los datos de sincronización con PlanesGo.",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
		{
			"name":        "planesgo_set_project",
			"description": "Vincula o actualiza el proyecto activo de PlanesGo (.planesgo.json) resolviendo automáticamente el ID y nombre oficial desde Odoo en un solo paso rápido.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_name": map[string]interface{}{
						"type":        "string",
						"description": "Nombre del proyecto en Odoo/PlanesGo (ej: 'FLY-PYR - CESCONILLO', 'PLANESGO', 'AUTOPYME LOGISTICS')",
					},
					"project_id": map[string]interface{}{
						"type":        "integer",
						"description": "ID numérico de Odoo si se conoce directamente (opcional)",
					},
					"project_path": map[string]interface{}{
						"type":        "string",
						"description": "Ruta al directorio raíz del proyecto donde se ubicará .planesgo.json (opcional, por defecto el directorio actual)",
					},
				},
				"required": []string{"project_name"},
			},
		},
	}
}

func runMCPServer() {
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	fmt.Fprintf(os.Stderr, "[planesgo-mcp] Servidor MCP iniciado v%s (stdio)\n", Version)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintf(os.Stderr, "[planesgo-mcp] Error parseando request JSON-RPC: %v\n", err)
			continue
		}

		// Manejo de peticiones según el método MCP
		switch req.Method {
		case "initialize":
			resp := JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]interface{}{
						"tools": map[string]interface{}{},
					},
					"serverInfo": map[string]interface{}{
						"name":    "planesgo-mcp",
						"version": Version,
					},
				},
			}
			sendResponse(resp)

		case "notifications/initialized":
			// Notificación del cliente: no requiere respuesta
			fmt.Fprintf(os.Stderr, "[planesgo-mcp] Handshake MCP completado con cliente\n")

		case "ping":
			sendResponse(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]interface{}{},
			})

		case "tools/list":
			sendResponse(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]interface{}{
					"tools": getToolsDefinition(),
				},
			})

		case "tools/call":
			var params ToolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				sendResponse(JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &RPCError{
						Code:    -32602,
						Message: fmt.Sprintf("Parámetros inválidos para tools/call: %v", err),
					},
				})
				continue
			}

			result := executeToolCall(params.Name, params.Arguments)
			sendResponse(JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			})

		default:
			if req.ID != nil {
				sendResponse(JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &RPCError{
						Code:    -32601,
						Message: fmt.Sprintf("Método '%s' no implementado", req.Method),
					},
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[planesgo-mcp] Error leyendo stdin: %v\n", err)
	}
}

func sendResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[planesgo-mcp] Error serializando respuesta: %v\n", err)
		return
	}
	os.Stdout.Write(data)
	os.Stdout.Write([]byte("\n"))
}

func main() {
	checkFlag := flag.Bool("check", false, "Ejecuta verificación de Fase 0 (como planesgo-track check)")
	beatFlag := flag.Bool("beat", false, "Envía un latido para la tarea indicada")
	stopFlag := flag.Bool("stop", false, "Detiene la tarea indicada")
	listFlag := flag.Bool("list", false, "Lista tareas de Odoo para el proyecto actual")
	setProjFlag := flag.String("set-project", "", "Vincula y configura el proyecto indicado en .planesgo.json")
	initFlag := flag.String("init", "", "Alias de --set-project para inicializar/vincular proyecto")
	projectFlag := flag.String("project", "", "Alias de --set-project")
	taskFlag := flag.String("task", "", "Nombre de la tarea")
	typeFlag := flag.String("type", "", "Tipo de tarea: Análisis y diseño, Desarrollo, Pruebas (opcional)")
	descFlag := flag.String("desc", "", "Descripción del trabajo")
	pathFlag := flag.String("path", "", "Ruta personalizada al proyecto (opcional)")
	versionFlag := flag.Bool("version", false, "Muestra versión y sale")
	vFlag := flag.Bool("v", false, "Muestra versión y sale")

	flag.Parse()

	if *versionFlag || *vFlag {
		fmt.Printf("planesgo-mcp v%s\n", Version)
		return
	}

	// Modo CLI directo
	if *checkFlag {
		args := map[string]interface{}{}
		if *taskFlag != "" {
			args["task_name"] = *taskFlag
		}
		if *typeFlag != "" {
			args["task_type"] = *typeFlag
		}
		if *descFlag != "" {
			args["description"] = *descFlag
		}
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		res := executeToolCall("planesgo_check", args)
		if len(res.Content) > 0 {
			fmt.Println(res.Content[0].Text)
		}
		if res.IsError {
			os.Exit(1)
		}
		return
	}

	if *beatFlag {
		if *taskFlag == "" {
			fmt.Fprintln(os.Stderr, "Error: debe indicar --task '<nombre>'")
			os.Exit(1)
		}
		args := map[string]interface{}{
			"task_name":   *taskFlag,
			"description": *descFlag,
		}
		if *typeFlag != "" {
			args["task_type"] = *typeFlag
		}
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		res := executeToolCall("planesgo_beat", args)
		if len(res.Content) > 0 {
			fmt.Println(res.Content[0].Text)
		}
		if res.IsError {
			os.Exit(1)
		}
		return
	}

	if *stopFlag {
		args := map[string]interface{}{
			"task_name":   *taskFlag,
			"description": *descFlag,
		}
		if *typeFlag != "" {
			args["task_type"] = *typeFlag
		}
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		res := executeToolCall("planesgo_stop", args)
		if len(res.Content) > 0 {
			fmt.Println(res.Content[0].Text)
		}
		if res.IsError {
			os.Exit(1)
		}
		return
	}

	if *listFlag {
		args := map[string]interface{}{}
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		res := executeToolCall("planesgo_list_tasks", args)
		if len(res.Content) > 0 {
			fmt.Println(res.Content[0].Text)
		}
		if res.IsError {
			os.Exit(1)
		}
		return
	}

	targetProj := *setProjFlag
	if targetProj == "" && *initFlag != "" {
		targetProj = *initFlag
	}
	if targetProj == "" && *projectFlag != "" && !*checkFlag && !*beatFlag && !*stopFlag && !*listFlag {
		targetProj = *projectFlag
	}

	if targetProj != "" {
		args := map[string]interface{}{
			"project_name": targetProj,
		}
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		res := executeToolCall("planesgo_set_project", args)
		if len(res.Content) > 0 {
			fmt.Println(res.Content[0].Text)
		}
		if res.IsError {
			os.Exit(1)
		}
		return
	}

	// Por defecto sin flags: servidor MCP sobre stdio
	runMCPServer()
}
