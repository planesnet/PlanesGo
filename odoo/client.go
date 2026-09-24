package odoo

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"pasigo/config"
	"strings"
	"sync"
	"time"
)

var (
	// sharedTransport mantiene un pool de conexiones TCP/TLS persistentes con Odoo (Keep-Alive)
	// evitando renegociar TLS y TCP en cada petición HTTP.
	sharedTransport = &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 25,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		ExpectContinueTimeout: 1 * time.Second,
	}

	// sharedHTTPClient cliente HTTP compartido global con transporte optimizado
	sharedHTTPClient = &http.Client{
		Transport: sharedTransport,
		Timeout:   35 * time.Second,
	}

	clientPoolMu sync.RWMutex
	clientPool   = make(map[string]*Client)
)

type Client struct {
	config     config.OdooConfig
	httpClient *http.Client
	uid        int
	mu         sync.RWMutex

	// Caché en memoria para datos maestros poco volátiles
	projectsCache     []Project
	projectsCachedAt  time.Time
	employeesCache    []Employee
	employeesCachedAt   time.Time
	ticketsCache        []Ticket
	ticketsCachedAt     time.Time
	userUIDCache        map[string]int
	partnerAvatarFields []string
}

func poolKey(cfg config.OdooConfig) string {
	return fmt.Sprintf("%s|%s|%s|%s", cfg.URL, cfg.DB, cfg.Username, cfg.Password)
}

// GetClient obtiene un cliente Odoo con conexión persistente y autenticación en caché.
func GetClient(cfg config.OdooConfig) *Client {
	key := poolKey(cfg)

	clientPoolMu.RLock()
	c, ok := clientPool[key]
	clientPoolMu.RUnlock()
	if ok && c != nil {
		return c
	}

	clientPoolMu.Lock()
	defer clientPoolMu.Unlock()
	if c, ok = clientPool[key]; ok && c != nil {
		return c
	}

	c = &Client{
		config:       cfg,
		httpClient:   sharedHTTPClient,
		userUIDCache: make(map[string]int),
	}
	clientPool[key] = c
	return c
}

// InvalidateClient remueve del pool un cliente con credenciales desactualizadas.
func InvalidateClient(cfg config.OdooConfig) {
	key := poolKey(cfg)
	clientPoolMu.Lock()
	delete(clientPool, key)
	clientPoolMu.Unlock()
}

// NewClient devuelve un cliente reutilizado del pool con conexión persistente.
func NewClient(cfg config.OdooConfig) *Client {
	return GetClient(cfg)
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

	url := c.config.URL + "/jsonrpc"
	params := map[string]interface{}{
		"service": service,
		"method":  method,
		"args":    args,
	}
	if kwargs != nil {
		params["kwargs"] = kwargs
	}

	reqBody, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "call",
		Params:  params,
		ID:      time.Now().UnixNano(),
	})
	if err != nil {
		return nil, fmt.Errorf("error serializando petición JSON-RPC: %w", err)
	}

	startCall := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creando petición HTTP: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	elapsed := time.Since(startCall)
	if err != nil {
		log.Printf("[ODOO-RPC FAIL] %s.%s (%s) en %v: %v", service, method, url, elapsed, err)
		return nil, fmt.Errorf("error de conexión con Odoo en %s (duración %v): %w", url, elapsed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[ODOO-RPC FAIL] %s.%s error leyendo cuerpo en %v: %v", service, method, elapsed, err)
		return nil, fmt.Errorf("error leyendo respuesta de Odoo: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[ODOO-RPC HTTP %d] %s.%s en %v (cuerpo: %s)", resp.StatusCode, service, method, elapsed, string(respBody[:min(len(respBody), 256)]))
		return nil, fmt.Errorf("Odoo respondió con estado HTTP %d", resp.StatusCode)
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		log.Printf("[ODOO-RPC JSON ERR] %s.%s en %v: %v", service, method, elapsed, err)
		return nil, fmt.Errorf("error decodificando respuesta JSON-RPC: %w", err)
	}

	if rpcResp.Error != nil {
		log.Printf("[ODOO-RPC ERROR] %s.%s en %v: código=%d, mensaje=%s, datos=%v", service, method, elapsed, rpcResp.Error.Code, rpcResp.Error.Message, rpcResp.Error.Data)
		return nil, fmt.Errorf("error de Odoo: %s (código: %d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	log.Printf("[ODOO-RPC OK] %s.%s en %v (bytes: %d)", service, method, elapsed, len(respBody))
	return rpcResp.Result, nil
}

// ExecuteKW ejecuta una llamada genérica al modelo especificado en Odoo.
func (c *Client) ExecuteKW(ctx context.Context, model, method string, args []interface{}, kwargs map[string]interface{}) (json.RawMessage, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, err
	}
	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	callArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		model,
		method,
		args,
	}
	res, err := c.call(ctx, "object", "execute_kw", callArgs, kwargs)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			callArgs[1] = newUID
			res, err = c.call(ctx, "object", "execute_kw", callArgs, kwargs)
		}
	}
	return res, err
}

// Authenticate autentica contra el endpoint común de Odoo y devuelve el UID.
// Si el cliente ya tiene un UID autenticado previamente, lo devuelve de inmediato sin llamadas de red.
func (c *Client) Authenticate(ctx context.Context) (int, error) {
	c.mu.RLock()
	if c.uid > 0 {
		cachedUID := c.uid
		c.mu.RUnlock()
		return cachedUID, nil
	}
	c.mu.RUnlock()

	return c.ForceAuthenticate(ctx)
}

// ForceAuthenticate fuerza una re-autenticación contra Odoo actualizando el UID persistente.
func (c *Client) ForceAuthenticate(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.config.Username == "" || c.config.Password == "" {
		return 0, errors.New("faltan credenciales de Odoo (usuario o contraseña/token vacíos)")
	}

	args := []interface{}{
		c.config.DB,
		c.config.Username,
		c.config.Password,
		map[string]interface{}{},
	}

	resultRaw, err := c.call(ctx, "common", "authenticate", args, nil)
	if err != nil {
		return 0, fmt.Errorf("fallo de autenticación en Odoo: %w", err)
	}

	var uid int
	if err := json.Unmarshal(resultRaw, &uid); err != nil || uid == 0 {
		c.uid = 0
		return 0, errors.New("autenticación fallida: usuario o contraseña incorrectos")
	}

	c.uid = uid
	return uid, nil
}

// InvalidateProjectsCache limpia la caché en memoria de proyectos.
func (c *Client) InvalidateProjectsCache() {
	c.mu.Lock()
	c.projectsCache = nil
	c.projectsCachedAt = time.Time{}
	c.mu.Unlock()
}

// InvalidateEmployeesCache limpia la caché en memoria de empleados.
func (c *Client) InvalidateEmployeesCache() {
	c.mu.Lock()
	c.employeesCache = nil
	c.employeesCachedAt = time.Time{}
	c.mu.Unlock()
}

// InvalidateUserUIDCache limpia la caché de UIDs de usuarios.
func (c *Client) InvalidateUserUIDCache() {
	c.mu.Lock()
	c.userUIDCache = make(map[string]int)
	c.mu.Unlock()
}

// InvalidateTicketsCache limpia la caché en memoria de tickets.
func (c *Client) InvalidateTicketsCache() {
	c.mu.Lock()
	c.ticketsCache = nil
	c.ticketsCachedAt = time.Time{}
	c.mu.Unlock()
}

// GetTimesheets consulta los registros de horas trabajadas (account.analytic.line).
func (c *Client) GetTimesheets(ctx context.Context, domain []interface{}) ([]TimesheetEntry, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar horas: %w", err)
	}

	if domain == nil {
		domain = []interface{}{}
	}

	// Asegurar que solo se lean líneas que pertenezcan a proyectos (partes de horas válidos)
	hasProjectFilter := false
	for _, cond := range domain {
		if condArr, ok := cond.([]interface{}); ok && len(condArr) > 0 {
			if field, ok := condArr[0].(string); ok && field == "project_id" {
				hasProjectFilter = true
				break
			}
		}
	}
	effectiveDomain := make([]interface{}, 0, len(domain)+1)
	for _, d := range domain {
		effectiveDomain = append(effectiveDomain, d)
	}
	if !hasProjectFilter {
		effectiveDomain = append(effectiveDomain, []interface{}{"project_id", "!=", false})
	}

	limit := c.config.Limit
	if limit <= 0 {
		limit = 200
	}

	// Campos base estándar + campos de facturación en Odoo 14 + estado de cronómetro + partner
	fields := []string{
		"id",
		"date",
		"name",
		"unit_amount",
		"project_id",
		"task_id",
		"employee_id",
		"user_id",
		"partner_id",
		"timesheet_invoice_id",
		"billing_ref",
		"is_timer_running",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"limit":  limit,
		"order":  "date desc, id desc",
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"search_read",
		[]interface{}{effectiveDomain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Reintento con autenticación si expiró sesión
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			// Fallback 1: intentar sin billing_ref si el modelo no tiene ese campo personalizado (manteniendo is_timer_running y partner_id)
			fallbackFields := []string{
				"id",
				"date",
				"name",
				"unit_amount",
				"project_id",
				"task_id",
				"employee_id",
				"user_id",
				"partner_id",
				"timesheet_invoice_id",
				"is_timer_running",
			}
			kwargs["fields"] = fallbackFields
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			// Fallback 2: campos mínimos estándar
			minimalFields := []string{
				"id",
				"date",
				"name",
				"unit_amount",
				"project_id",
				"task_id",
				"employee_id",
				"user_id",
			}
			kwargs["fields"] = minimalFields
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

	// Únicamente mostrar partes de horas que NO están facturados (billing_ref vacío/false y sin factura)
	unInvoiced := make([]TimesheetEntry, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsInvoiced() {
			unInvoiced = append(unInvoiced, entry)
		}
	}

	// Cargar etiquetas (tags) de las tareas asociadas a las imputaciones
	c.populateTimesheetTags(ctx, uid, unInvoiced)

	return unInvoiced, nil
}

// GetProjects consulta los proyectos definidos en Odoo (project.project).
// Utiliza una caché en memoria de 60 segundos cuando no se pasa un dominio de búsqueda específico.
func (c *Client) GetProjects(ctx context.Context, domain []interface{}) ([]Project, error) {
	isBaseQuery := (domain == nil || len(domain) == 0)
	if isBaseQuery {
		c.mu.RLock()
		if len(c.projectsCache) > 0 && time.Since(c.projectsCachedAt) < 60*time.Second {
			cached := make([]Project, len(c.projectsCache))
			copy(cached, c.projectsCache)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar proyectos: %w", err)
	}

	if domain == nil {
		domain = []interface{}{}
	}

	fields := []string{
		"id",
		"name",
		"display_name",
		"user_id",
		"partner_id",
		"active",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"order":  "name asc, id desc",
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.project",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Reintento con autenticación forzada si expiró la sesión
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			return nil, fmt.Errorf("error al obtener proyectos: %w", err)
		}
	}

	var projects []Project
	if err := json.Unmarshal(resultRaw, &projects); err != nil {
		return nil, fmt.Errorf("error al parsear proyectos: %w", err)
	}

	if isBaseQuery && len(projects) > 0 {
		c.mu.Lock()
		c.projectsCache = make([]Project, len(projects))
		copy(c.projectsCache, projects)
		c.projectsCachedAt = time.Now()
		c.mu.Unlock()
	}

	return projects, nil
}

// GetProjectPartnerID obtiene el partner_id asignado a un proyecto específico en Odoo.
func (c *Client) GetProjectPartnerID(ctx context.Context, projectID int) (int, error) {
	if projectID <= 0 {
		return 0, errors.New("ID de proyecto inválido")
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return 0, err
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.project",
		"read",
		[]interface{}{[]int{projectID}},
	}
	kwargs := map[string]interface{}{
		"fields": []string{"id", "partner_id"},
	}

	raw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if newUID, aErr := c.ForceAuthenticate(ctx); aErr == nil {
			args[1] = newUID
			raw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
	}
	if err != nil {
		return 0, err
	}

	var recs []struct {
		ID        int      `json:"id"`
		PartnerID Many2One `json:"partner_id"`
	}
	if err := json.Unmarshal(raw, &recs); err == nil && len(recs) > 0 {
		return recs[0].PartnerID.ID, nil
	}
	return 0, errors.New("proyecto no encontrado")
}

// GetEmployees consulta los trabajadores/empleados definidos en Odoo (hr.employee).
// Por defecto filtra únicamente los que están activos (active = true).
// Utiliza una caché en memoria de 60 segundos cuando no se pasa un dominio de búsqueda específico.
func (c *Client) GetEmployees(ctx context.Context, domain []interface{}) ([]Employee, error) {
	isBaseQuery := (domain == nil || len(domain) == 0)
	if isBaseQuery {
		c.mu.RLock()
		if len(c.employeesCache) > 0 && time.Since(c.employeesCachedAt) < 60*time.Second {
			cached := make([]Employee, len(c.employeesCache))
			copy(cached, c.employeesCache)
			c.mu.RUnlock()
			return cached, nil
		}
		c.mu.RUnlock()
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar empleados: %w", err)
	}

	if domain == nil {
		domain = []interface{}{}
	}

	hasActiveFilter := false
	for _, cond := range domain {
		if condArr, ok := cond.([]interface{}); ok && len(condArr) > 0 {
			if field, ok := condArr[0].(string); ok && field == "active" {
				hasActiveFilter = true
				break
			}
		}
	}

	effectiveDomain := make([]interface{}, 0, len(domain)+1)
	for _, d := range domain {
		effectiveDomain = append(effectiveDomain, d)
	}
	if !hasActiveFilter {
		effectiveDomain = append(effectiveDomain, []interface{}{"active", "=", true})
	}

	fields := []string{
		"id",
		"name",
		"work_email",
		"user_id",
		"active",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"order":  "name asc, id asc",
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"hr.employee",
		"search_read",
		[]interface{}{effectiveDomain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			fallbackFields := []string{"id", "name", "user_id", "active"}
			kwargs["fields"] = fallbackFields
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			return nil, fmt.Errorf("error al obtener empleados: %w", err)
		}
	}

	var employees []Employee
	if err := json.Unmarshal(resultRaw, &employees); err != nil {
		return nil, fmt.Errorf("error al parsear empleados: %w", err)
	}

	if isBaseQuery && len(employees) > 0 {
		c.mu.Lock()
		c.employeesCache = make([]Employee, len(employees))
		copy(c.employeesCache, employees)
		c.employeesCachedAt = time.Now()
		c.mu.Unlock()
	}

	return employees, nil
}

// GetPartnerAvatar obtiene el logotipo de un partner/cliente desde res.partner en base64 y lo decodifica.
func (c *Client) GetPartnerAvatar(ctx context.Context, partnerID int) ([]byte, string, error) {
	if partnerID <= 0 {
		return nil, "", errors.New("ID de partner inválido")
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("no se pudo autenticar para leer avatar de partner: %w", err)
	}

	c.mu.RLock()
	fieldsToQuery := c.partnerAvatarFields
	c.mu.RUnlock()

	if len(fieldsToQuery) == 0 {
		// Descubrir de forma segura qué campos de imagen existen en el modelo res.partner
		candFields := []string{"avatar_128", "image_128", "image_256", "image_512", "image_1920", "image_medium", "image_small", "image", "parent_id"}
		fieldsGetArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"res.partner",
			"fields_get",
			[]interface{}{candFields},
		}
		rawFields, fErr := c.call(ctx, "object", "execute_kw", fieldsGetArgs, nil)
		if fErr != nil {
			if newUID, aErr := c.ForceAuthenticate(ctx); aErr == nil {
				uid = newUID
				fieldsGetArgs[1] = uid
				rawFields, fErr = c.call(ctx, "object", "execute_kw", fieldsGetArgs, nil)
			}
		}

		discovered := make(map[string]interface{})
		if fErr == nil {
			_ = json.Unmarshal(rawFields, &discovered)
		}

		var valid []string
		for _, cand := range candFields {
			if _, exists := discovered[cand]; exists {
				valid = append(valid, cand)
			}
		}
		if len(valid) == 0 {
			valid = []string{"image_128", "avatar_128", "image_1920", "parent_id"}
		}
		valid = append(valid, "id")

		c.mu.Lock()
		c.partnerAvatarFields = valid
		fieldsToQuery = valid
		c.mu.Unlock()
	}

	// Función para leer un registro de res.partner y extraer la primera imagen base64 válida
	fetchPartnerImage := func(pID int) ([]byte, string, int, error) {
		readArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"res.partner",
			"read",
			[]interface{}{[]int{pID}},
		}
		readKwargs := map[string]interface{}{
			"fields": fieldsToQuery,
		}

		raw, rErr := c.call(ctx, "object", "execute_kw", readArgs, readKwargs)
		if rErr != nil {
			if newUID, aErr := c.ForceAuthenticate(ctx); aErr == nil {
				uid = newUID
				readArgs[1] = uid
				raw, rErr = c.call(ctx, "object", "execute_kw", readArgs, readKwargs)
			}
		}
		if rErr != nil {
			return nil, "", 0, rErr
		}

		var records []map[string]interface{}
		if err := json.Unmarshal(raw, &records); err != nil || len(records) == 0 {
			return nil, "", 0, errors.New("partner no encontrado")
		}

		rec := records[0]

		// Comprobar campos de imagen en orden de preferencia (thumbnails ligeros primero)
		prefOrder := []string{"avatar_128", "image_128", "image_256", "image_medium", "image_512", "image_1920", "image_small", "image"}
		for _, f := range prefOrder {
			if val, ok := rec[f]; ok && val != nil {
				if strVal, isStr := val.(string); isStr && strings.TrimSpace(strVal) != "" {
					data, decErr := base64.StdEncoding.DecodeString(strings.TrimSpace(strVal))
					if decErr == nil && len(data) > 0 {
						cType := http.DetectContentType(data)
						return data, cType, 0, nil
					}
				}
			}
		}

		// Si no tiene imagen propia, comprobar si pertenece a una empresa matriz (parent_id)
		parentID := 0
		if pVal, ok := rec["parent_id"]; ok && pVal != nil {
			if pSlice, isSlice := pVal.([]interface{}); isSlice && len(pSlice) > 0 {
				if idFloat, isFloat := pSlice[0].(float64); isFloat {
					parentID = int(idFloat)
				}
			}
		}

		return nil, "", parentID, nil
	}

	data, cType, parentID, err := fetchPartnerImage(partnerID)
	if err == nil && len(data) > 0 {
		return data, cType, nil
	}

	// Si no tiene imagen directa y tiene parent_id, intentar leer el logo de la empresa matriz
	if parentID > 0 && parentID != partnerID {
		dataParent, cTypeParent, _, errParent := fetchPartnerImage(parentID)
		if errParent == nil && len(dataParent) > 0 {
			return dataParent, cTypeParent, nil
		}
	}

	return nil, "", errors.New("partner sin imagen")
}

// UID devuelve el UID del usuario autenticado en Odoo.
func (c *Client) UID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.uid
}

// ResolveUserUIDByEmail busca el ID de usuario en res.users por email o login.
// Si no lo encuentra, devuelve el UID autenticado del cliente como fallback.
// Cuenta con caché en memoria para evitar consultas repetitivas de red.
func (c *Client) ResolveUserUIDByEmail(ctx context.Context, email string) (int, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return c.UID(), nil
	}

	c.mu.RLock()
	if c.userUIDCache != nil {
		if cachedUID, ok := c.userUIDCache[email]; ok && cachedUID > 0 {
			c.mu.RUnlock()
			return cachedUID, nil
		}
	}
	c.mu.RUnlock()

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return 0, err
	}

	domain := []interface{}{
		"|",
		[]interface{}{"login", "=", email},
		[]interface{}{"email", "=", email},
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"res.users",
		"search_read",
		[]interface{}{domain},
	}
	kwargs := map[string]interface{}{
		"fields": []string{"id", "login", "name"},
		"limit":  1,
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err == nil {
		var users []struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(resultRaw, &users) == nil && len(users) > 0 && users[0].ID > 0 {
			c.mu.Lock()
			if c.userUIDCache == nil {
				c.userUIDCache = make(map[string]int)
			}
			c.userUIDCache[email] = users[0].ID
			c.mu.Unlock()
			return users[0].ID, nil
		}
	}

	// Buscar en hr.employee por si el login no coincide pero el work_email sí
	empDomain := []interface{}{
		"|",
		[]interface{}{"work_email", "=", email},
		[]interface{}{"name", "=", email},
	}
	empArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"hr.employee",
		"search_read",
		[]interface{}{empDomain},
	}
	empKwargs := map[string]interface{}{
		"fields": []string{"id", "name", "user_id"},
		"limit":  1,
	}
	resultEmp, errEmp := c.call(ctx, "object", "execute_kw", empArgs, empKwargs)
	if errEmp == nil {
		var emps []struct {
			ID     int      `json:"id"`
			UserID Many2One `json:"user_id"`
		}
		if json.Unmarshal(resultEmp, &emps) == nil && len(emps) > 0 && emps[0].UserID.ID > 0 {
			c.mu.Lock()
			if c.userUIDCache == nil {
				c.userUIDCache = make(map[string]int)
			}
			c.userUIDCache[email] = emps[0].UserID.ID
			c.mu.Unlock()
			return emps[0].UserID.ID, nil
		}
	}

	return uid, nil
}

// GetTasks consulta las tareas de un proyecto en Odoo (project.task) en tiempo real.
// Devuelve todas las tareas activas del proyecto, ordenadas priorizando aquellas
// asignadas al usuario actual. Si projectID <= 0, devuelve una lista vacía para evitar
// mezclar tareas de otros proyectos.
func (c *Client) GetTasks(ctx context.Context, projectID int, userUID int) ([]Task, error) {
	if projectID <= 0 {
		return []Task{}, nil
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar tareas: %w", err)
	}

	domain := []interface{}{
		[]interface{}{"project_id", "=", projectID},
	}

	fields := []string{
		"id",
		"name",
		"display_name",
		"project_id",
		"user_id",
		"active",
		"tag_ids",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"order":  "name asc, id desc",
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.task",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			return nil, fmt.Errorf("error al obtener tareas del proyecto: %w", err)
		}
	}

	var allTasks []Task
	if err := json.Unmarshal(resultRaw, &allTasks); err != nil {
		return nil, fmt.Errorf("error al parsear tareas: %w", err)
	}

	// Filtrado estricto en memoria: asegurar exclusivamente tareas del proyecto solicitado
	tasks := make([]Task, 0, len(allTasks))
	for _, t := range allTasks {
		if t.ProjectID.ID == projectID {
			tasks = append(tasks, t)
		}
	}

	// Si hay userUID especificado, ordenar priorizando las tareas asignadas a dicho usuario
	if userUID > 0 && len(tasks) > 1 {
		userTasks := make([]Task, 0, len(tasks))
		otherTasks := make([]Task, 0, len(tasks))
		for _, t := range tasks {
			if t.UserID.ID == userUID {
				userTasks = append(userTasks, t)
			} else {
				otherTasks = append(otherTasks, t)
			}
		}
		tasks = append(userTasks, otherTasks...)
	}

	return tasks, nil
}

// GetPendingTickets consulta los tickets de soporte pendientes de un usuario en Odoo (helpdesk.ticket).
func (c *Client) GetPendingTickets(ctx context.Context, userUID int) ([]Ticket, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar tickets: %w", err)
	}

	targetUID := userUID
	if targetUID <= 0 {
		targetUID = uid
	}

	c.mu.RLock()
	if len(c.ticketsCache) > 0 && time.Since(c.ticketsCachedAt) < 30*time.Second {
		cached := make([]Ticket, len(c.ticketsCache))
		copy(cached, c.ticketsCache)
		c.mu.RUnlock()
		return cached, nil
	}
	c.mu.RUnlock()

	// 1. Dominio principal: tickets asignados al usuario y no cerrados
	domain := []interface{}{
		[]interface{}{"user_id", "=", targetUID},
		[]interface{}{"close_date", "=", false},
	}

	fields := []string{
		"id",
		"name",
		"ticket_ref",
		"stage_id",
		"user_id",
		"partner_id",
		"project_id",
		"priority",
		"create_date",
		"close_date",
		"kanban_state",
	}

	kwargs := map[string]interface{}{
		"fields": fields,
		"order":  "priority desc, create_date desc, id desc",
		"limit":  50,
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"helpdesk.ticket",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		// Fallback 1: Si falla close_date en el dominio, intentar solo con user_id
		if err != nil {
			fallbackDomain := []interface{}{
				[]interface{}{"user_id", "=", targetUID},
			}
			fallbackFields := []string{"id", "name", "stage_id", "user_id", "partner_id", "project_id", "priority", "create_date"}
			kwargs["fields"] = fallbackFields
			args[5] = []interface{}{fallbackDomain}
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		// Fallback 2: Si el modelo helpdesk.ticket no existe o no hay permisos, retornar vacío sin error fatal
		if err != nil {
			log.Printf("[ODOO INFO] Modelo helpdesk.ticket no disponible o sin tickets: %v", err)
			return []Ticket{}, nil
		}
	}

	var tickets []Ticket
	if err := json.Unmarshal(resultRaw, &tickets); err != nil {
		log.Printf("[ODOO WARN] Error al parsear tickets: %v", err)
		return []Ticket{}, nil
	}

	// Filtrar en memoria por seguridad cualquier ticket cerrado
	pending := make([]Ticket, 0, len(tickets))
	closedKeywords := []string{"cerrad", "solucion", "cancel", "done", "closed", "solved", "resuelto"}
	for _, t := range tickets {
		if t.CloseDate != "" && t.CloseDate != "false" {
			continue
		}
		sName := strings.ToLower(t.StageName())
		isClosed := false
		for _, kw := range closedKeywords {
			if strings.Contains(sName, kw) {
				isClosed = true
				break
			}
		}
		if isClosed {
			continue
		}
		pending = append(pending, t)
	}

	c.mu.Lock()
	c.ticketsCache = make([]Ticket, len(pending))
	copy(c.ticketsCache, pending)
	c.ticketsCachedAt = time.Now()
	c.mu.Unlock()

	return pending, nil
}

// CreateTask crea una nueva tarea en un proyecto en Odoo (project.task) asignada al trabajador.
func (c *Client) CreateTask(ctx context.Context, projectID int, name string, userUID int) (int, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return 0, fmt.Errorf("no se pudo autenticar antes de crear tarea: %w", err)
	}

	if projectID <= 0 {
		return 0, errors.New("el ID de proyecto es obligatorio")
	}
	if name == "" {
		return 0, errors.New("el nombre de la tarea es obligatorio")
	}

	vals := map[string]interface{}{
		"name":       name,
		"project_id": projectID,
	}
	if userUID > 0 {
		vals["user_id"] = userUID
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.task",
		"create",
		[]interface{}{vals},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, nil)
		}
		if err != nil && userUID > 0 {
			// Fallback para Odoo 15+ donde el campo es user_ids
			delete(vals, "user_id")
			vals["user_ids"] = []interface{}{[]interface{}{6, 0, []int{userUID}}}
			args[5] = []interface{}{vals}
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, nil)
		}
		if err != nil {
			return 0, fmt.Errorf("error al crear tarea en Odoo: %w", err)
		}
	}

	var newID int
	if err := json.Unmarshal(resultRaw, &newID); err != nil || newID == 0 {
		return 0, fmt.Errorf("respuesta inválida al crear tarea: %s", string(resultRaw))
	}

	return newID, nil
}

// GetOrCreateTag busca o crea una etiqueta en project.tags de Odoo y devuelve su ID.
func (c *Client) GetOrCreateTag(ctx context.Context, tagName string) (int, error) {
	tagName = strings.TrimSpace(tagName)
	if tagName == "" {
		return 0, errors.New("el nombre de la etiqueta no puede estar vacío")
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return 0, fmt.Errorf("error de autenticación al resolver tag: %w", err)
	}

	domain := []interface{}{
		[]interface{}{"name", "=ilike", tagName},
	}
	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.tags",
		"search_read",
		[]interface{}{domain},
	}
	kwargs := map[string]interface{}{
		"fields": []string{"id", "name"},
		"limit":  1,
	}

	raw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err == nil {
		var tags []Tag
		if err := json.Unmarshal(raw, &tags); err == nil && len(tags) > 0 {
			return tags[0].ID, nil
		}
	}

	// Si no existe, crear el tag en project.tags
	createVals := map[string]interface{}{
		"name": tagName,
	}
	createArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.tags",
		"create",
		[]interface{}{createVals},
	}
	createRaw, createErr := c.call(ctx, "object", "execute_kw", createArgs, nil)
	if createErr != nil {
		return 0, fmt.Errorf("error al crear tag '%s' en Odoo: %w", tagName, createErr)
	}

	var newTagID int
	if err := json.Unmarshal(createRaw, &newTagID); err == nil && newTagID > 0 {
		return newTagID, nil
	}
	return 0, fmt.Errorf("no se pudo parsear el ID del tag creado: %s", string(createRaw))
}

// EnsureTaskTag asegura que una tarea de Odoo tenga asignada una etiqueta específica en tag_ids.
func (c *Client) EnsureTaskTag(ctx context.Context, taskID int, tagName string) error {
	if taskID <= 0 || tagName == "" {
		return nil
	}
	tagID, err := c.GetOrCreateTag(ctx, tagName)
	if err != nil {
		return err
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	// Añadir el tag con el comando ORM (4, id) en tag_ids
	writeVals := map[string]interface{}{
		"tag_ids": []interface{}{
			[]interface{}{4, tagID},
		},
	}
	writeArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.task",
		"write",
		[]interface{}{
			[]int{taskID},
			writeVals,
		},
	}
	_, err = c.call(ctx, "object", "execute_kw", writeArgs, nil)
	return err
}

// CreateTaskWithTag crea una nueva tarea con el nombre canónico y la etiqueta indicada en tag_ids.
func (c *Client) CreateTaskWithTag(ctx context.Context, projectID int, name string, userUID int, tagName string) (int, error) {
	taskID, err := c.CreateTask(ctx, projectID, name, userUID)
	if err != nil {
		return 0, err
	}
	if tagName != "" {
		_ = c.EnsureTaskTag(ctx, taskID, tagName)
	}
	return taskID, nil
}

// populateTimesheetTags consulta en lote las etiquetas de las tareas de los partes de horas y las asigna.
func (c *Client) populateTimesheetTags(ctx context.Context, uid int, entries []TimesheetEntry) {
	if len(entries) == 0 {
		return
	}
	taskIDMap := make(map[int]bool)
	for _, e := range entries {
		if e.TaskID.ID > 0 {
			taskIDMap[e.TaskID.ID] = true
		}
	}
	if len(taskIDMap) == 0 {
		return
	}

	taskIDs := make([]int, 0, len(taskIDMap))
	for id := range taskIDMap {
		taskIDs = append(taskIDs, id)
	}

	taskDomain := []interface{}{
		[]interface{}{"id", "in", taskIDs},
	}
	taskArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.task",
		"search_read",
		[]interface{}{taskDomain},
	}
	taskKwargs := map[string]interface{}{
		"fields": []string{"id", "tag_ids"},
	}
	resRaw, err := c.call(ctx, "object", "execute_kw", taskArgs, taskKwargs)
	if err != nil {
		return
	}

	type taskTagResp struct {
		ID     int   `json:"id"`
		TagIDs []int `json:"tag_ids"`
	}
	var taskTags []taskTagResp
	if err := json.Unmarshal(resRaw, &taskTags); err != nil {
		return
	}

	allTagIDsMap := make(map[int]bool)
	taskToTagIDs := make(map[int][]int)
	for _, tt := range taskTags {
		taskToTagIDs[tt.ID] = tt.TagIDs
		for _, tagID := range tt.TagIDs {
			allTagIDsMap[tagID] = true
		}
	}

	tagMetaMap := make(map[int]Tag)
	if len(allTagIDsMap) > 0 {
		tagIDs := make([]int, 0, len(allTagIDsMap))
		for tid := range allTagIDsMap {
			tagIDs = append(tagIDs, tid)
		}
		tagDomain := []interface{}{
			[]interface{}{"id", "in", tagIDs},
		}
		tagArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"project.tags",
			"search_read",
			[]interface{}{tagDomain},
		}
		tagKwargs := map[string]interface{}{
			"fields": []string{"id", "name", "color"},
		}
		tagResRaw, err := c.call(ctx, "object", "execute_kw", tagArgs, tagKwargs)
		if err == nil {
			var fetchedTags []Tag
			if err := json.Unmarshal(tagResRaw, &fetchedTags); err == nil {
				for _, ft := range fetchedTags {
					tagMetaMap[ft.ID] = ft
				}
			}
		}
	}

	for i := range entries {
		tID := entries[i].TaskID.ID
		if tID > 0 {
			if tIDs, ok := taskToTagIDs[tID]; ok {
				entries[i].TagIDs = tIDs
				for _, tid := range tIDs {
					if tag, ok := tagMetaMap[tid]; ok {
						entries[i].Tags = append(entries[i].Tags, tag)
					} else {
						entries[i].Tags = append(entries[i].Tags, Tag{ID: tid, Name: fmt.Sprintf("Tag #%d", tid)})
					}
				}
			}
		}
		if entries[i].IsAntigravity() {
			hasAgyTag := false
			for _, tg := range entries[i].Tags {
				if strings.EqualFold(tg.Name, "Antigravity") || strings.EqualFold(tg.Name, "AGY") {
					hasAgyTag = true
					break
				}
			}
			if !hasAgyTag {
				entries[i].Tags = append(entries[i].Tags, Tag{Name: "Antigravity"})
			}
		}
	}
}

// CreateTimesheet crea un nuevo parte de horas (account.analytic.line) en Odoo.
func (c *Client) CreateTimesheet(ctx context.Context, date string, projectID int, taskID int, unitAmount float64, description string) (int, error) {
	return c.CreateTimesheetExtended(ctx, date, projectID, taskID, unitAmount, description, false)
}

// CreateTimesheetExtended crea un nuevo parte de horas en Odoo indicando si proviene de Antigravity (Hora Máquina).
func (c *Client) CreateTimesheetExtended(ctx context.Context, date string, projectID int, taskID int, unitAmount float64, description string, isAntigravity bool) (int, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return 0, fmt.Errorf("no se pudo autenticar antes de crear parte de horas: %w", err)
	}

	if projectID <= 0 {
		return 0, errors.New("el ID de proyecto es obligatorio")
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	if description == "" {
		description = "Horas registradas desde PlanesGo"
	}

	vals := map[string]interface{}{
		"name":        description,
		"date":        date,
		"project_id":  projectID,
		"unit_amount": unitAmount,
	}
	if uid > 0 {
		vals["user_id"] = uid
	}
	if taskID > 0 {
		vals["task_id"] = taskID
	}
	if isAntigravity {
		vals["is_antigravity"] = true
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"create",
		[]interface{}{vals},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, nil)
		}
		if err != nil {
			return 0, fmt.Errorf("error al registrar parte de horas en Odoo: %w", err)
		}
	}

	var newID int
	if err := json.Unmarshal(resultRaw, &newID); err != nil || newID == 0 {
		var ids []int
		if err2 := json.Unmarshal(resultRaw, &ids); err2 == nil && len(ids) > 0 {
			newID = ids[0]
		} else {
			return 0, fmt.Errorf("respuesta inválida al crear parte de horas: %s", string(resultRaw))
		}
	}

	return newID, nil
}

// UpdateTimesheet actualiza un registro de horas existente en Odoo (account.analytic.line).
func (c *Client) UpdateTimesheet(ctx context.Context, timesheetID int, date string, taskID int, unitAmount float64, description string) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return fmt.Errorf("no se pudo autenticar antes de actualizar parte de horas: %w", err)
	}

	if timesheetID <= 0 {
		return errors.New("el ID del registro de horas es obligatorio")
	}

	vals := map[string]interface{}{}
	if date != "" {
		vals["date"] = date
	}
	if unitAmount > 0 {
		vals["unit_amount"] = unitAmount
	}
	if description != "" {
		vals["name"] = description
	}
	if taskID > 0 {
		vals["task_id"] = taskID
	} else if taskID == -1 {
		vals["task_id"] = false
	}

	if len(vals) == 0 {
		return nil
	}

	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"write",
		[]interface{}{
			[]int{timesheetID},
			vals,
		},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, nil)
		}
		if err != nil {
			return fmt.Errorf("error al actualizar parte de horas en Odoo: %w", err)
		}
	}

	var ok bool
	if err := json.Unmarshal(resultRaw, &ok); err != nil || !ok {
		return fmt.Errorf("no se pudo confirmar la actualización en Odoo: %s", string(resultRaw))
	}

	return nil
}

// DeleteTimesheet elimina un registro de horas (account.analytic.line) en Odoo,
// siempre y cuando dicho parte de horas no haya sido facturado.
func (c *Client) DeleteTimesheet(ctx context.Context, timesheetID int) error {
	if timesheetID <= 0 {
		return errors.New("el ID del parte de horas debe ser mayor a 0")
	}

	uid, err := c.Authenticate(ctx)
	if err != nil {
		return fmt.Errorf("no se pudo autenticar antes de eliminar horas: %w", err)
	}

	// 1. Verificar si la línea existe y si está facturada antes de intentar borrar
	readArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"read",
		[]interface{}{
			[]int{timesheetID},
			[]string{"id", "timesheet_invoice_id"},
		},
	}

	readRaw, err := c.call(ctx, "object", "execute_kw", readArgs, nil)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			readArgs[1] = newUID
			readRaw, err = c.call(ctx, "object", "execute_kw", readArgs, nil)
		}
		// Fallback Odoo 14: si falla por campo timesheet_invoice_id inexistente, leer solo id
		if err != nil {
			fallbackReadArgs := []interface{}{
				c.config.DB,
				uid,
				c.config.Password,
				"account.analytic.line",
				"read",
				[]interface{}{
					[]int{timesheetID},
					[]string{"id"},
				},
			}
			readRaw, err = c.call(ctx, "object", "execute_kw", fallbackReadArgs, nil)
		}
		if err != nil {
			return fmt.Errorf("error al verificar estado del parte de horas en Odoo: %w", err)
		}
	}

	var records []struct {
		ID                 int      `json:"id"`
		TimesheetInvoiceID Many2One `json:"timesheet_invoice_id"`
	}
	if err := json.Unmarshal(readRaw, &records); err == nil && len(records) > 0 {
		if records[0].TimesheetInvoiceID.ID > 0 {
			invName := records[0].TimesheetInvoiceID.Name
			if invName == "" {
				invName = fmt.Sprintf("#%d", records[0].TimesheetInvoiceID.ID)
			}
			return fmt.Errorf("no se puede eliminar la imputación de horas porque ya ha sido facturada (Factura %s)", invName)
		}
	}

	// 2. Ejecutar borrado (unlink) en Odoo
	unlinkArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"unlink",
		[]interface{}{
			[]int{timesheetID},
		},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", unlinkArgs, nil)
	if err != nil {
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			unlinkArgs[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", unlinkArgs, nil)
		}
		if err != nil {
			return fmt.Errorf("error de Odoo al eliminar parte de horas: %w", err)
		}
	}

	var ok bool
	if err := json.Unmarshal(resultRaw, &ok); err != nil || !ok {
		return fmt.Errorf("no se pudo confirmar la eliminación en Odoo: %s", string(resultRaw))
	}

	return nil
}

// GetServerVersion consulta la versión del servidor Odoo (compatible con Odoo 14.0+).
func (c *Client) GetServerVersion(ctx context.Context) (string, error) {
	resultRaw, err := c.call(ctx, "common", "version", []interface{}{}, nil)
	if err != nil {
		return "", err
	}

	var info struct {
		ServerVersion string `json:"server_version"`
		ServerSerie   string `json:"server_serie"`
	}
	if err := json.Unmarshal(resultRaw, &info); err != nil {
		return "", err
	}

	if info.ServerVersion != "" {
		return info.ServerVersion, nil
	}
	return info.ServerSerie, nil
}

// StartTimer inicia un temporizador de trabajo en Odoo llamando a action_timer_start en account.analytic.line o project.task,
// o marcando is_timer_running = true. Es acumulativo sobre las horas ya imputadas en la tarea o proyecto en la fecha especificada.
func (c *Client) StartTimer(ctx context.Context, projectID int, projectName string, taskID int, taskName string, timesheetID int, description string, initialHours float64, workDate string) (*ActiveTimer, error) {
	return c.StartTimerExtended(ctx, projectID, projectName, taskID, taskName, timesheetID, description, initialHours, workDate, false)
}

// StartTimerExtended inicia un temporizador de trabajo indicando si proviene de Antigravity (Hora Máquina).
func (c *Client) StartTimerExtended(ctx context.Context, projectID int, projectName string, taskID int, taskName string, timesheetID int, description string, initialHours float64, workDate string, isAntigravity bool) (*ActiveTimer, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de iniciar cronómetro: %w", err)
	}

	now := time.Now()
	targetDate := strings.TrimSpace(workDate)
	if targetDate == "" {
		targetDate = now.Format("2006-01-02")
	}
	if description == "" {
		description = "Trabajo en curso"
	}

	actualTimesheetID := timesheetID
	currentHours := initialHours

	// 1. Si no se proporcionó un timesheetID (iniciando un nuevo trabajo), crear la imputación directamente en account.analytic.line
	if actualTimesheetID <= 0 {
		newID, createErr := c.CreateTimesheetExtended(ctx, targetDate, projectID, taskID, currentHours, description, isAntigravity)
		if createErr != nil {
			log.Printf("[PlanesGo Odoo] Error al crear parte de horas en Odoo al iniciar temporizador: %v", createErr)
			return nil, fmt.Errorf("error al crear parte de horas en Odoo: %w", createErr)
		}
		actualTimesheetID = newID
	} else if currentHours <= 0 {
		// Si teníamos timesheetID pero currentHours era 0, consultar su unit_amount actual en Odoo
		readArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"read",
			[]interface{}{[]int{actualTimesheetID}},
			map[string]interface{}{
				"fields": []string{"unit_amount"},
			},
		}
		if rRaw, rErr := c.call(ctx, "object", "execute_kw", readArgs, nil); rErr == nil {
			var recs []struct {
				UnitAmount float64 `json:"unit_amount"`
			}
			if json.Unmarshal(rRaw, &recs) == nil && len(recs) > 0 {
				currentHours = recs[0].UnitAmount
			}
		}
	}

	// 2. Invocar acción nativa de Odoo action_timer_start en account.analytic.line si está disponible
	if actualTimesheetID > 0 {
		startArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"action_timer_start",
			[]interface{}{[]int{actualTimesheetID}},
		}
		_, _ = c.call(ctx, "object", "execute_kw", startArgs, nil)
	}

	// 3. Si hay tarea asignada, invocar action_timer_start en project.task de forma asíncrona para máxima rapidez
	if taskID > 0 {
		go func(tID, uID int) {
			taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			taskStartArgs := []interface{}{
				c.config.DB,
				uID,
				c.config.Password,
				"project.task",
				"action_timer_start",
				[]interface{}{[]int{tID}},
			}
			_, _ = c.call(taskCtx, "object", "execute_kw", taskStartArgs, nil)
		}(taskID, uid)
	}

	accumMs := int64(currentHours * 3600 * 1000)
	return &ActiveTimer{
		ProjectID:     projectID,
		ProjectName:   projectName,
		TaskID:        taskID,
		TaskName:      taskName,
		TimesheetID:   actualTimesheetID,
		Description:   description,
		UnitAmount:    currentHours,
		Date:          targetDate,
		AccumulatedMs: accumMs,
		IsRunning:     true,
		StartedAt:     now.UnixMilli(),
	}, nil
}

// UpdateTimerUnits actualiza el unit_amount acumulado de un parte de horas en Odoo periódicamente sin pausarlo
func (c *Client) UpdateTimerUnits(ctx context.Context, timesheetID int, unitAmount float64) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}
	if timesheetID <= 0 || unitAmount <= 0 {
		return nil
	}
	writeArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"write",
		[]interface{}{
			[]int{timesheetID},
			map[string]interface{}{
				"unit_amount": unitAmount,
			},
		},
	}
	_, err = c.call(ctx, "object", "execute_kw", writeArgs, nil)
	return err
}

// UpdateTimesheetDescription actualiza la descripción de un parte de horas en Odoo
func (c *Client) UpdateTimesheetDescription(ctx context.Context, timesheetID int, description string) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}
	if timesheetID <= 0 || strings.TrimSpace(description) == "" {
		return nil
	}
	writeArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"write",
		[]interface{}{
			[]int{timesheetID},
			map[string]interface{}{
				"name": description,
			},
		},
	}
	_, err = c.call(ctx, "object", "execute_kw", writeArgs, nil)
	return err
}

// PauseTimer pausa el cronómetro activo en Odoo ejecutando action_timer_pause / action_timer_stop y actualizando unit_amount
func (c *Client) PauseTimer(ctx context.Context, timesheetID int, taskID int, unitAmount float64) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	if timesheetID > 0 {
		pauseArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"action_timer_pause",
			[]interface{}{[]int{timesheetID}},
		}
		_, _ = c.call(ctx, "object", "execute_kw", pauseArgs, nil)

		if unitAmount > 0 {
			writeArgs := []interface{}{
				c.config.DB,
				uid,
				c.config.Password,
				"account.analytic.line",
				"write",
				[]interface{}{
					[]int{timesheetID},
					map[string]interface{}{
						"unit_amount": unitAmount,
					},
				},
			}
			_, _ = c.call(ctx, "object", "execute_kw", writeArgs, nil)
		}
	}

	if taskID > 0 {
		go func(tID, uID int) {
			taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			taskPauseArgs := []interface{}{
				c.config.DB,
				uID,
				c.config.Password,
				"project.task",
				"action_timer_pause",
				[]interface{}{[]int{tID}},
			}
			_, _ = c.call(taskCtx, "object", "execute_kw", taskPauseArgs, nil)
		}(taskID, uid)
	}

	return nil
}

// ResumeTimer reanuda el cronómetro activo en Odoo ejecutando action_timer_start / action_timer_resume
func (c *Client) ResumeTimer(ctx context.Context, timesheetID int, taskID int) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	if timesheetID > 0 {
		// Método nativo y estándar de Odoo timer.mixin: action_timer_start
		startArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"action_timer_start",
			[]interface{}{[]int{timesheetID}},
		}
		if _, callErr := c.call(ctx, "object", "execute_kw", startArgs, nil); callErr != nil {
			// Fallback: probar action_timer_resume si está definido por un módulo custom
			resumeArgs := []interface{}{
				c.config.DB,
				uid,
				c.config.Password,
				"account.analytic.line",
				"action_timer_resume",
				[]interface{}{[]int{timesheetID}},
			}
			_, _ = c.call(ctx, "object", "execute_kw", resumeArgs, nil)
		}
	}

	if taskID > 0 {
		go func(tID, uID int) {
			taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			taskStartArgs := []interface{}{
				c.config.DB,
				uID,
				c.config.Password,
				"project.task",
				"action_timer_start",
				[]interface{}{[]int{tID}},
			}
			if _, tErr := c.call(taskCtx, "object", "execute_kw", taskStartArgs, nil); tErr != nil {
				taskResumeArgs := []interface{}{
					c.config.DB,
					uID,
					c.config.Password,
					"project.task",
					"action_timer_resume",
					[]interface{}{[]int{tID}},
				}
				_, _ = c.call(taskCtx, "object", "execute_kw", taskResumeArgs, nil)
			}
		}(taskID, uid)
	}

	return nil
}

// StopTimer detiene y finaliza el cronómetro en Odoo actualizando la imputación con la duración total y descripción
func (c *Client) StopTimer(ctx context.Context, timesheetID int, taskID int, unitAmount float64, description string) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	if timesheetID > 0 {
		stopArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"action_timer_stop",
			[]interface{}{[]int{timesheetID}},
		}
		_, _ = c.call(ctx, "object", "execute_kw", stopArgs, nil)

		vals := map[string]interface{}{}
		if unitAmount > 0 {
			vals["unit_amount"] = unitAmount
		}
		if description != "" {
			vals["name"] = description
		}
		if len(vals) > 0 {
			writeArgs := []interface{}{
				c.config.DB,
				uid,
				c.config.Password,
				"account.analytic.line",
				"write",
				[]interface{}{
					[]int{timesheetID},
					vals,
				},
			}
			_, _ = c.call(ctx, "object", "execute_kw", writeArgs, nil)
		}
	}

	if taskID > 0 {
		go func(tID, uID int) {
			taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			taskStopArgs := []interface{}{
				c.config.DB,
				uID,
				c.config.Password,
				"project.task",
				"action_timer_stop",
				[]interface{}{[]int{tID}},
			}
			_, _ = c.call(taskCtx, "object", "execute_kw", taskStopArgs, nil)
		}(taskID, uid)
	}

	return nil
}

// GetActiveTimer busca en Odoo si el usuario actual tiene una imputación o tarea con el cronómetro activo (is_timer_running=true)
func (c *Client) GetActiveTimer(ctx context.Context, userUID int) (*ActiveTimer, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	effectiveUID := uid
	if userUID > 0 {
		effectiveUID = userUID
	}

	// 1. Buscar en account.analytic.line
	domain := []interface{}{
		[]interface{}{"user_id", "=", effectiveUID},
		[]interface{}{"is_timer_running", "=", true},
	}
	kwargs := map[string]interface{}{
		"fields": []string{"id", "name", "project_id", "task_id", "unit_amount", "date", "write_date"},
		"limit":  1,
		"order":  "write_date desc, id desc",
	}
	args := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"account.analytic.line",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, searchErr := c.call(ctx, "object", "execute_kw", args, kwargs)
	if searchErr == nil {
		var lines []struct {
			ID         int      `json:"id"`
			Name       string   `json:"name"`
			ProjectID  Many2One `json:"project_id"`
			TaskID     Many2One `json:"task_id"`
			UnitAmount float64  `json:"unit_amount"`
			WriteDate  string   `json:"write_date"`
		}
		if json.Unmarshal(resultRaw, &lines) == nil && len(lines) > 0 {
			l := lines[0]
			accumulatedMs := int64(l.UnitAmount * 3600 * 1000)
			return &ActiveTimer{
				TimesheetID:   l.ID,
				ProjectID:     l.ProjectID.ID,
				ProjectName:   l.ProjectID.Name,
				TaskID:        l.TaskID.ID,
				TaskName:      l.TaskID.Name,
				Description:   l.Name,
				IsRunning:     true,
				StartedAt:     time.Now().UnixMilli(),
				AccumulatedMs: accumulatedMs,
				UnitAmount:    l.UnitAmount,
			}, nil
		}
	}

	// 2. Si no se encontró en account.analytic.line, verificar en project.task
	taskDomain := []interface{}{
		[]interface{}{"user_id", "=", effectiveUID},
		[]interface{}{"is_timer_running", "=", true},
	}
	taskKwargs := map[string]interface{}{
		"fields": []string{"id", "name", "project_id"},
		"limit":  1,
	}
	taskArgs := []interface{}{
		c.config.DB,
		uid,
		c.config.Password,
		"project.task",
		"search_read",
		[]interface{}{taskDomain},
	}
	taskRaw, taskErr := c.call(ctx, "object", "execute_kw", taskArgs, taskKwargs)
	if taskErr == nil {
		var tasks []struct {
			ID        int      `json:"id"`
			Name      string   `json:"name"`
			ProjectID Many2One `json:"project_id"`
		}
		if json.Unmarshal(taskRaw, &tasks) == nil && len(tasks) > 0 {
			t := tasks[0]
			return &ActiveTimer{
				TimesheetID: 0,
				TaskID:      t.ID,
				TaskName:    t.Name,
				ProjectID:   t.ProjectID.ID,
				ProjectName: t.ProjectID.Name,
				Description: "Trabajo en " + t.Name,
				IsRunning:   true,
				StartedAt:   time.Now().UnixMilli(),
			}, nil
		}
	}

	return nil, nil
}
