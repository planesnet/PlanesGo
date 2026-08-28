package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"pasigo/config"
	"time"
)

type Client struct {
	config     config.OdooConfig
	httpClient *http.Client
	uid        int
}

func NewClient(cfg config.OdooConfig) *Client {
	return &Client{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int64       `json:"id"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

func (c *Client) call(ctx context.Context, service, method string, args []interface{}, kwargs map[string]interface{}) (json.RawMessage, error) {
	if c.config.URL == "" {
		return nil, errors.New("la URL de Odoo no está configurada")
	}

	params := map[string]interface{}{
		"service": service,
		"method":  method,
		"args":    args,
	}
	if kwargs != nil {
		params["kwargs"] = kwargs
	}

	reqPayload := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "call",
		Params:  params,
		ID:      time.Now().UnixNano(),
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("error al serializar petición JSON-RPC: %w", err)
	}

	endpoint := fmt.Sprintf("%s/jsonrpc", c.config.URL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error al crear petición HTTP: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error de conexión con Odoo en %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("error al deserializar respuesta de Odoo: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("error de Odoo: %s (código: %d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	return rpcResp.Result, nil
}

// Authenticate autentica las credenciales con Odoo y guarda el UID obtenido.
func (c *Client) Authenticate(ctx context.Context) (int, error) {
	if c.config.DB == "" || c.config.Username == "" {
		return 0, errors.New("la base de datos y el usuario de Odoo son obligatorios")
	}

	args := []interface{}{
		c.config.DB,
		c.config.Username,
		c.config.Password,
		map[string]interface{}{},
	}

	resultRaw, err := c.call(ctx, "common", "authenticate", args, nil)
	if err != nil {
		return 0, err
	}

	var uid int
	if err := json.Unmarshal(resultRaw, &uid); err != nil || uid == 0 {
		var isFalse bool
		if json.Unmarshal(resultRaw, &isFalse) == nil && !isFalse {
			return 0, errors.New("autenticación fallida: usuario o contraseña incorrectos")
		}
		return 0, fmt.Errorf("respuesta de autenticación no válida: %s", string(resultRaw))
	}

	c.uid = uid
	return uid, nil
}

// GetTimesheets consulta los registros de horas trabajadas (account.analytic.line).
func (c *Client) GetTimesheets(ctx context.Context, domain []interface{}) ([]TimesheetEntry, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return nil, fmt.Errorf("no se pudo autenticar antes de consultar horas: %w", err)
		}
	}

	if domain == nil {
		domain = []interface{}{}
	}

	fields := []string{
		"id",
		"date",
		"name",
		"unit_amount",
		"project_id",
		"task_id",
		"employee_id",
		"user_id",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"limit":  c.config.Limit,
		"order":  "date desc, id desc",
	}

	args := []interface{}{
		c.config.DB,
		c.uid,
		c.config.Password,
		"account.analytic.line",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		// Reintento con autenticación si expiró sesión
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			return nil, fmt.Errorf("error al obtener partes de horas: %w", err)
		}
	}

	var entries []TimesheetEntry
	if err := json.Unmarshal(resultRaw, &entries); err != nil {
		return nil, fmt.Errorf("error al parsear partes de horas: %w", err)
	}

	return entries, nil
}
