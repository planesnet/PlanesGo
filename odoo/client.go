package odoo

import (
	"bytes"
	"context"
	"crypto/tls"
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
	employeesCachedAt time.Time
	ticketsCache      []Ticket
	ticketsCachedAt   time.Time
	userUIDCache      map[string]int
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

	// Campos base estándar + campos de facturación en Odoo 14 + estado de cronómetro
	fields := []string{
		"id",
		"date",
		"name",
		"unit_amount",
		"project_id",
		"task_id",
		"employee_id",
		"user_id",
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
		// Reintento con autenticación si expiró sesión
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil {
			// Fallback 1: intentar sin billing_ref si el modelo no tiene ese campo personalizado (manteniendo is_timer_running)
			fallbackFields := []string{
				"id",
				"date",
				"name",
				"unit_amount",
				"project_id",
				"task_id",
				"employee_id",
				"user_id",
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
		"task_count",
		"active",
		"privacy_visibility",
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
		// Reintento con autenticación forzada si expiró la sesión
		if newUID, authErr := c.ForceAuthenticate(ctx); authErr == nil {
			args[1] = newUID
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		// Fallback con menos campos si algún campo opcional falló
		if err != nil {
			fallbackFields := []string{"id", "name", "display_name", "user_id", "partner_id", "active"}
			kwargs["fields"] = fallbackFields
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
// asignadas al usuario actual.
func (c *Client) GetTasks(ctx context.Context, projectID int, userUID int) ([]Task, error) {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("no se pudo autenticar antes de consultar tareas: %w", err)
	}

	domain := []interface{}{}
	if projectID > 0 {
		domain = append(domain, []interface{}{"project_id", "=", projectID})
	}

	fields := []string{
		"id",
		"name",
		"display_name",
		"project_id",
		"user_id",
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

	var tasks []Task
	if err := json.Unmarshal(resultRaw, &tasks); err != nil {
		return nil, fmt.Errorf("error al parsear tareas: %w", err)
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

// CreateTimesheet crea un nuevo parte de horas (account.analytic.line) en Odoo.
func (c *Client) CreateTimesheet(ctx context.Context, date string, projectID int, taskID int, unitAmount float64, description string) (int, error) {
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
	if taskID > 0 {
		vals["task_id"] = taskID
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
		return 0, fmt.Errorf("respuesta inválida al crear parte de horas: %s", string(resultRaw))
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

	// 1. Si no hay timesheetID proporcionado, intentar buscar una imputación existente de la fecha objetivo para este usuario y proyecto
	if actualTimesheetID <= 0 && projectID > 0 {
		domain := []interface{}{
			[]interface{}{"date", "=", targetDate},
			[]interface{}{"project_id", "=", projectID},
			[]interface{}{"user_id", "=", uid},
		}
		if taskID > 0 {
			domain = append(domain, []interface{}{"task_id", "=", taskID})
		}
		searchArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"search_read",
			[]interface{}{domain},
			map[string]interface{}{
				"fields": []string{"id", "unit_amount", "name", "project_id", "task_id"},
				"limit":  1,
				"order":  "id desc",
			},
		}
		if sRaw, sErr := c.call(ctx, "object", "execute_kw", searchArgs, nil); sErr == nil {
			var found []struct {
				ID         int     `json:"id"`
				UnitAmount float64 `json:"unit_amount"`
				Name       string  `json:"name"`
			}
			if json.Unmarshal(sRaw, &found) == nil && len(found) > 0 {
				actualTimesheetID = found[0].ID
				if currentHours <= 0 {
					currentHours = found[0].UnitAmount
				}
				if description == "Trabajo en curso" && found[0].Name != "" {
					description = found[0].Name
				}
			}
		}
	}

	// 2. Si todavía no hay timesheetID, crear la imputación de inicio en account.analytic.line
	if actualTimesheetID <= 0 {
		vals := map[string]interface{}{
			"name":             description,
			"date":             targetDate,
			"project_id":       projectID,
			"unit_amount":      currentHours,
			"is_timer_running": true,
		}
		if taskID > 0 {
			vals["task_id"] = taskID
		}

		createArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"create",
			[]interface{}{vals},
		}

		resultRaw, createErr := c.call(ctx, "object", "execute_kw", createArgs, nil)
		if createErr == nil {
			var newID int
			if json.Unmarshal(resultRaw, &newID) == nil && newID > 0 {
				actualTimesheetID = newID
			}
		}
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

	// 3. Invocar acción nativa de Odoo action_timer_start en account.analytic.line
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

		// Asegurar que is_timer_running quede activado en Odoo si el campo existe
		writeArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"write",
			[]interface{}{
				[]int{actualTimesheetID},
				map[string]interface{}{
					"is_timer_running": true,
				},
			},
		}
		_, _ = c.call(ctx, "object", "execute_kw", writeArgs, nil)
	}

	// 4. Si hay tarea asignada, invocar action_timer_start en project.task de forma asíncrona para máxima rapidez
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
		TimesheetID:   actualTimesheetID,
		TaskID:        taskID,
		TaskName:      taskName,
		ProjectID:     projectID,
		ProjectName:   projectName,
		Description:   description,
		IsRunning:     true,
		StartedAt:     now.UnixMilli() - accumMs,
		AccumulatedMs: accumMs,
		UnitAmount:    currentHours,
		Date:          targetDate,
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

// PauseTimer pausa el cronómetro activo en Odoo ejecutando action_timer_pause / action_timer_stop o escribiendo is_timer_running=false
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

		vals := map[string]interface{}{
			"is_timer_running": false,
		}
		if unitAmount > 0 {
			vals["unit_amount"] = unitAmount
		}
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

// ResumeTimer reanuda el cronómetro activo en Odoo ejecutando action_timer_resume / action_timer_start o escribiendo is_timer_running=true
func (c *Client) ResumeTimer(ctx context.Context, timesheetID int, taskID int) error {
	uid, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	if timesheetID > 0 {
		resumeArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"action_timer_resume",
			[]interface{}{[]int{timesheetID}},
		}
		_, _ = c.call(ctx, "object", "execute_kw", resumeArgs, nil)

		writeArgs := []interface{}{
			c.config.DB,
			uid,
			c.config.Password,
			"account.analytic.line",
			"write",
			[]interface{}{
				[]int{timesheetID},
				map[string]interface{}{
					"is_timer_running": true,
				},
			},
		}
		_, _ = c.call(ctx, "object", "execute_kw", writeArgs, nil)
	}

	if taskID > 0 {
		go func(tID, uID int) {
			taskCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			taskResumeArgs := []interface{}{
				c.config.DB,
				uID,
				c.config.Password,
				"project.task",
				"action_timer_resume",
				[]interface{}{[]int{tID}},
			}
			_, _ = c.call(taskCtx, "object", "execute_kw", taskResumeArgs, nil)
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

		vals := map[string]interface{}{
			"is_timer_running": false,
		}
		if unitAmount > 0 {
			vals["unit_amount"] = unitAmount
		}
		if description != "" {
			vals["name"] = description
		}
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
				StartedAt:     time.Now().UnixMilli() - accumulatedMs,
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
