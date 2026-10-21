package main

// Cliente HTTP de la API de PlanesGo.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PlanesGoClient gestiona peticiones HTTP a PlanesGo
type PlanesGoClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
	RetryDelay time.Duration
}

// newClientWithSession resuelve la configuración del proyecto (sesión o .planesgo.json) y las credenciales.
// Devuelve el cliente aunque no haya proyecto vinculado (en ese caso, con el error de findConfig).
func newClientWithSession(sessionID string, customDir ...string) (*PlanesGoClient, *Config, error) {
	cfg, _, findErr := findConfig(customDir...)
	var targetDir string
	for _, d := range customDir {
		if strings.TrimSpace(d) != "" {
			targetDir = strings.TrimSpace(d)
			break
		}
	}
	if targetDir == "" {
		if cwd := sessionCwd(); cwd != "" && !isBackgroundDir(cwd) {
			targetDir = cwd
		}
	}
	if sessionID != "" && findErr != errMissingProjectPath {
		if sessCfg, sessErr := loadSessionConfig(sessionID, targetDir); sessErr == nil && sessCfg != nil {
			if sessCfg.PlanesGoURL == "" && cfg != nil && cfg.PlanesGoURL != "" {
				sessCfg.PlanesGoURL = cfg.PlanesGoURL
			}
			cfg = sessCfg
			findErr = nil
		}
	}
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
		RetryDelay: 2 * time.Second,
	}, cfg, findErr
}

// doWithRetry ejecuta una petición HTTP con reintentos automáticos para tolerar micro-cortes de red o caídas temporales del servidor
func (c *PlanesGoClient) doWithRetry(method, targetURL string, bodyBytes []byte, headers map[string]string) ([]byte, int, error) {
	const maxRetries = 3
	retryDelay := c.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 2 * time.Second
	}

	var lastErr error
	var lastStatusCode int

	for attempt := 1; attempt <= maxRetries; attempt++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(context.Background(), method, targetURL, bodyReader)
		if err != nil {
			return nil, 0, err
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("error de conexión con PlanesGo (%s): %w", c.BaseURL, err)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
				continue
			}
			return nil, 0, lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("error leyendo respuesta de PlanesGo: %w", readErr)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
				continue
			}
			return nil, resp.StatusCode, lastErr
		}

		lastStatusCode = resp.StatusCode

		// Códigos temporales recuperables: 502 Bad Gateway, 503 Service Unavailable, 504 Gateway Timeout, 429 Too Many Requests
		if resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout ||
			resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("PlanesGo HTTP %d: temporalmente no disponible", resp.StatusCode)
			if attempt < maxRetries {
				time.Sleep(retryDelay)
				continue
			}
			return respBody, resp.StatusCode, lastErr
		}

		return respBody, resp.StatusCode, nil
	}

	return nil, lastStatusCode, lastErr
}

// pideJSON hace la petición y decodifica la respuesta JSON en destino; un estado distinto de 200 es error.
func (c *PlanesGoClient) pideJSON(method, targetURL string, payload interface{}, destino interface{}) error {
	headers := map[string]string{
		"X-Antigravity-Token": c.Token,
		"Accept":              "application/json",
	}
	var cuerpo []byte
	if payload != nil {
		var err error
		if cuerpo, err = json.Marshal(payload); err != nil {
			return err
		}
		headers["Content-Type"] = "application/json"
	}

	body, statusCode, err := c.doWithRetry(method, targetURL, cuerpo, headers)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, destino); err != nil {
		return fmt.Errorf("respuesta inválida de PlanesGo (HTTP %d): %s", statusCode, string(body))
	}
	if statusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = "error desconocido"
		}
		return fmt.Errorf("PlanesGo HTTP %d: %s", statusCode, e.Error)
	}
	return nil
}

// SearchProjects busca proyectos de Odoo por texto parcial (GET /antigravity/projects/search), sin vincular nada.
func (c *PlanesGoClient) SearchProjects(query string) ([]map[string]interface{}, error) {
	reqURL, err := url.Parse(c.BaseURL + "/antigravity/projects/search")
	if err != nil {
		return nil, err
	}
	q := reqURL.Query()
	if query != "" {
		q.Set("q", query)
	}
	reqURL.RawQuery = q.Encode()

	var res struct {
		Projects []map[string]interface{} `json:"projects"`
	}
	if err := c.pideJSON(http.MethodGet, reqURL.String(), nil, &res); err != nil {
		return nil, err
	}
	return res.Projects, nil
}

// CheckStatus valida el proyecto y devuelve su nombre canónico y los cronómetros activos (GET /antigravity/status).
func (c *PlanesGoClient) CheckStatus(projectID int, projectName string) (map[string]interface{}, error) {
	reqURL, err := url.Parse(c.BaseURL + "/antigravity/status")
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

	var res map[string]interface{}
	err = c.pideJSON(http.MethodGet, reqURL.String(), nil, &res)
	return res, err
}

// SendTaskActionWithTokens envía latido o stop (POST /antigravity/update_tasks) con la telemetría de tokens de IA.
func (c *PlanesGoClient) SendTaskActionWithTokens(action, taskName string, taskID, projectID int, projectName, description, taskType, ticketCode string, tokensTotal, tokensInput, tokensOutput int, aiModel string, aiCost float64, aiSessionID string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"action":       action,
		"task_name":    taskName,
		"task_id":      taskID,
		"project_id":   projectID,
		"project_name": projectName,
		"description":  description,
		"task_type":    taskType,
		"ticket_code":  ticketCode,
		"token":        c.Token,
	}
	if tokensTotal > 0 {
		payload["tokens_total"] = tokensTotal
	}
	if tokensInput > 0 {
		payload["tokens_input"] = tokensInput
	}
	if tokensOutput > 0 {
		payload["tokens_output"] = tokensOutput
	}
	if aiModel != "" {
		payload["ai_model"] = aiModel
	}
	if aiCost > 0 {
		payload["ai_cost"] = aiCost
	}
	if aiSessionID != "" {
		payload["ai_session_id"] = aiSessionID
	}

	var res map[string]interface{}
	err := c.pideJSON(http.MethodPost, c.BaseURL+"/antigravity/update_tasks", payload, &res)
	return res, err
}

// SendHeartbeatWithElapsed envía una acción a /antigravity/update_tasks con un número explícito de segundos
// (elapsed_seconds), en vez de depender del tiempo real entre latidos. La usa pgo_time para añadir tiempo
// de un solo golpe: el servidor crea el registro de hoy si no existía, o le suma esos segundos.
func (c *PlanesGoClient) SendHeartbeatWithElapsed(action, taskName string, taskID, projectID int, projectName, description, taskType string, elapsedSeconds int, aiModel string) (map[string]interface{}, error) {
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
	if elapsedSeconds > 0 {
		payload["elapsed_seconds"] = elapsedSeconds
	}
	if aiModel != "" {
		payload["ai_model"] = aiModel
	}

	var res map[string]interface{}
	err := c.pideJSON(http.MethodPost, c.BaseURL+"/antigravity/update_tasks", payload, &res)
	return res, err
}
