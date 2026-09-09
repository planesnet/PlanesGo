package odoo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ConnectionConfig struct {
	URL      string `json:"url"`
	DB       string `json:"db"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type Client struct {
	cfg        ConnectionConfig
	httpClient *http.Client
	uid        int
	mu         sync.RWMutex
}

func NewClient(cfg ConnectionConfig) *Client {
	// Limpieza de URL
	cfg.URL = strings.TrimRight(cfg.URL, "/")

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
			},
		},
	}
}

func (c *Client) Config() ConnectionConfig {
	return c.cfg
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

func (c *Client) Call(ctx context.Context, service, method string, args []interface{}, kwargs map[string]interface{}) (json.RawMessage, error) {
	if c.cfg.URL == "" {
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
		return nil, fmt.Errorf("error serializando petición JSON-RPC: %w", err)
	}

	endpoint := fmt.Sprintf("%s/jsonrpc", c.cfg.URL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creando petición HTTP: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error de conexión con Odoo en %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("error deserializando respuesta de Odoo: %w", err)
	}

	if rpcResp.Error != nil {
		var detail string
		if rpcResp.Error.Data != nil {
			if dataBytes, err := json.Marshal(rpcResp.Error.Data); err == nil {
				detail = string(dataBytes)
			}
		}
		return nil, fmt.Errorf("error de Odoo [%d] %s: %s", rpcResp.Error.Code, rpcResp.Error.Message, detail)
	}

	return rpcResp.Result, nil
}

// Authenticate valida las credenciales y almacena el UID
func (c *Client) Authenticate(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cfg.DB == "" || c.cfg.Username == "" || c.cfg.Password == "" {
		return 0, errors.New("la base de datos, usuario y contraseña de Odoo son obligatorios")
	}

	args := []interface{}{
		c.cfg.DB,
		c.cfg.Username,
		c.cfg.Password,
		map[string]interface{}{},
	}

	resultRaw, err := c.Call(ctx, "common", "authenticate", args, nil)
	if err != nil {
		return 0, fmt.Errorf("error al autenticar con %s: %w", c.cfg.URL, err)
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

func (c *Client) ensureAuth(ctx context.Context) error {
	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	if uid == 0 {
		_, err := c.Authenticate(ctx)
		return err
	}
	return nil
}

// SearchRead consulta registros de un modelo
func (c *Client) SearchRead(ctx context.Context, model string, domain []interface{}, fields []string, limit, offset int, order string) (json.RawMessage, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	if domain == nil {
		domain = []interface{}{}
	}

	kwargs := map[string]interface{}{}
	if len(fields) > 0 {
		kwargs["fields"] = fields
	}
	if limit > 0 {
		kwargs["limit"] = limit
	}
	if offset > 0 {
		kwargs["offset"] = offset
	}
	if order != "" {
		kwargs["order"] = order
	}

	args := []interface{}{
		c.cfg.DB,
		uid,
		c.cfg.Password,
		model,
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.Call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		// Reintento con autenticación si expiró sesión
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			c.mu.RLock()
			args[1] = c.uid
			c.mu.RUnlock()
			resultRaw, err = c.Call(ctx, "object", "execute_kw", args, kwargs)
		}
	}

	return resultRaw, err
}

// SearchCount cuenta los registros que coinciden con un dominio
func (c *Client) SearchCount(ctx context.Context, model string, domain []interface{}) (int, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return 0, err
	}

	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	if domain == nil {
		domain = []interface{}{}
	}

	args := []interface{}{
		c.cfg.DB,
		uid,
		c.cfg.Password,
		model,
		"search_count",
		[]interface{}{domain},
	}

	resultRaw, err := c.Call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		return 0, err
	}

	var count int
	if err := json.Unmarshal(resultRaw, &count); err != nil {
		return 0, fmt.Errorf("error al parsear conteo: %w", err)
	}
	return count, nil
}

// Create crea un nuevo registro en el modelo indicado
func (c *Client) Create(ctx context.Context, model string, values map[string]interface{}) (int, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return 0, err
	}

	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	args := []interface{}{
		c.cfg.DB,
		uid,
		c.cfg.Password,
		model,
		"create",
		[]interface{}{values},
	}

	resultRaw, err := c.Call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		return 0, fmt.Errorf("error creando registro en %s: %w", model, err)
	}

	var newID int
	if err := json.Unmarshal(resultRaw, &newID); err != nil || newID == 0 {
		return 0, fmt.Errorf("respuesta inválida al crear en %s: %s", model, string(resultRaw))
	}

	return newID, nil
}

// Write actualiza un registro existente
func (c *Client) Write(ctx context.Context, model string, id int, values map[string]interface{}) error {
	if err := c.ensureAuth(ctx); err != nil {
		return err
	}

	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	args := []interface{}{
		c.cfg.DB,
		uid,
		c.cfg.Password,
		model,
		"write",
		[]interface{}{[]int{id}, values},
	}

	resultRaw, err := c.Call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		return fmt.Errorf("error actualizando registro %d en %s: %w", id, model, err)
	}

	var success bool
	if err := json.Unmarshal(resultRaw, &success); err != nil || !success {
		return fmt.Errorf("la actualización del registro %d en %s devolvió: %s", id, model, string(resultRaw))
	}

	return nil
}

// FieldsGet obtiene la definición de campos del modelo para detectar campos disponibles
func (c *Client) FieldsGet(ctx context.Context, model string, attributes []string) (map[string]map[string]interface{}, error) {
	if err := c.ensureAuth(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	uid := c.uid
	c.mu.RUnlock()

	kwargs := map[string]interface{}{}
	if len(attributes) > 0 {
		kwargs["attributes"] = attributes
	}

	args := []interface{}{
		c.cfg.DB,
		uid,
		c.cfg.Password,
		model,
		"fields_get",
		[]interface{}{},
	}

	resultRaw, err := c.Call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		return nil, err
	}

	var fields map[string]map[string]interface{}
	if err := json.Unmarshal(resultRaw, &fields); err != nil {
		return nil, fmt.Errorf("error parseando campos de %s: %w", model, err)
	}

	return fields, nil
}
