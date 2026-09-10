package odoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"pasigo/config"
	"strings"
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
			Timeout: 30 * time.Second,
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

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creando petición HTTP: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error de conexión con Odoo en %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Odoo respondió con estado HTTP %d", resp.StatusCode)
	}

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("error decodificando respuesta JSON-RPC: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("error de Odoo: %s (código: %d)", rpcResp.Error.Message, rpcResp.Error.Code)
	}

	return rpcResp.Result, nil
}

// Authenticate autentica contra el endpoint común de Odoo y devuelve el UID.
func (c *Client) Authenticate(ctx context.Context) (int, error) {
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
		return 0, errors.New("autenticación fallida: usuario o contraseña incorrectos")
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

	// Campos base estándar 100% compatibles con Odoo 14 hr_timesheet
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
		"limit":  limit,
		"order":  "date desc, id desc",
	}

	args := []interface{}{
		c.config.DB,
		c.uid,
		c.config.Password,
		"account.analytic.line",
		"search_read",
		[]interface{}{effectiveDomain},
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

// GetProjects consulta los proyectos definidos en Odoo (project.project).
func (c *Client) GetProjects(ctx context.Context, domain []interface{}) ([]Project, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return nil, fmt.Errorf("no se pudo autenticar antes de consultar proyectos: %w", err)
		}
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
		c.uid,
		c.config.Password,
		"project.project",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		// Reintento con autenticación si expiró la sesión
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
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

	return projects, nil
}

// GetEmployees consulta los trabajadores/empleados definidos en Odoo (hr.employee).
// Por defecto filtra únicamente los que están activos (active = true).
func (c *Client) GetEmployees(ctx context.Context, domain []interface{}) ([]Employee, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return nil, fmt.Errorf("no se pudo autenticar antes de consultar empleados: %w", err)
		}
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
		c.uid,
		c.config.Password,
		"hr.employee",
		"search_read",
		[]interface{}{effectiveDomain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
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

	return employees, nil
}

// UID devuelve el UID del usuario autenticado en Odoo.
func (c *Client) UID() int {
	return c.uid
}

// ResolveUserUIDByEmail busca el ID de usuario en res.users por email o login.
// Si no lo encuentra, devuelve el UID autenticado del cliente como fallback.
func (c *Client) ResolveUserUIDByEmail(ctx context.Context, email string) (int, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return 0, err
		}
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return c.uid, nil
	}

	domain := []interface{}{
		"|",
		[]interface{}{"login", "=", email},
		[]interface{}{"email", "=", email},
	}

	args := []interface{}{
		c.config.DB,
		c.uid,
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
		c.uid,
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
			return emps[0].UserID.ID, nil
		}
	}

	return c.uid, nil
}

// GetTasks consulta las tareas de un proyecto en Odoo (project.task).
// Si userUID > 0, filtra únicamente las tareas asignadas a ese trabajador/usuario.
func (c *Client) GetTasks(ctx context.Context, projectID int, userUID int) ([]Task, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return nil, fmt.Errorf("no se pudo autenticar antes de consultar tareas: %w", err)
		}
	}

	domain := []interface{}{}
	if projectID > 0 {
		domain = append(domain, []interface{}{"project_id", "=", projectID})
	}
	if userUID > 0 {
		domain = append(domain, []interface{}{"user_id", "=", userUID})
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
		c.uid,
		c.config.Password,
		"project.task",
		"search_read",
		[]interface{}{domain},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, kwargs)
	if err != nil {
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
			resultRaw, err = c.call(ctx, "object", "execute_kw", args, kwargs)
		}
		if err != nil && userUID > 0 {
			// Fallback para Odoo 15+ donde el campo es user_ids
			domain15 := []interface{}{}
			if projectID > 0 {
				domain15 = append(domain15, []interface{}{"project_id", "=", projectID})
			}
			domain15 = append(domain15, []interface{}{"user_ids", "in", []int{userUID}})
			fallbackFields := []string{"id", "name", "display_name", "project_id", "active"}
			kwargs["fields"] = fallbackFields
			args[5] = []interface{}{domain15}
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

	// Filtrado de seguridad en memoria si userUID > 0 y la tarea tiene UserID poblado
	if userUID > 0 {
		filtered := make([]Task, 0, len(tasks))
		for _, t := range tasks {
			if t.UserID.ID == 0 || t.UserID.ID == userUID {
				filtered = append(filtered, t)
			}
		}
		return filtered, nil
	}

	return tasks, nil
}

// CreateTask crea una nueva tarea en un proyecto en Odoo (project.task) asignada al trabajador.
func (c *Client) CreateTask(ctx context.Context, projectID int, name string, userUID int) (int, error) {
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return 0, fmt.Errorf("no se pudo autenticar antes de crear tarea: %w", err)
		}
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
		c.uid,
		c.config.Password,
		"project.task",
		"create",
		[]interface{}{vals},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
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
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return 0, fmt.Errorf("no se pudo autenticar antes de crear parte de horas: %w", err)
		}
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
		c.uid,
		c.config.Password,
		"account.analytic.line",
		"create",
		[]interface{}{vals},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", args, nil)
	if err != nil {
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
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
	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return fmt.Errorf("no se pudo autenticar antes de actualizar parte de horas: %w", err)
		}
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
		c.uid,
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
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			args[1] = c.uid
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

	if c.uid == 0 {
		if _, err := c.Authenticate(ctx); err != nil {
			return fmt.Errorf("no se pudo autenticar antes de eliminar horas: %w", err)
		}
	}

	// 1. Verificar si la línea existe y si está facturada antes de intentar borrar
	readArgs := []interface{}{
		c.config.DB,
		c.uid,
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
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			readArgs[1] = c.uid
			readRaw, err = c.call(ctx, "object", "execute_kw", readArgs, nil)
		}
		// Fallback Odoo 14: si falla por campo timesheet_invoice_id inexistente, leer solo id
		if err != nil {
			fallbackReadArgs := []interface{}{
				c.config.DB,
				c.uid,
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
		c.uid,
		c.config.Password,
		"account.analytic.line",
		"unlink",
		[]interface{}{
			[]int{timesheetID},
		},
	}

	resultRaw, err := c.call(ctx, "object", "execute_kw", unlinkArgs, nil)
	if err != nil {
		if _, authErr := c.Authenticate(ctx); authErr == nil {
			unlinkArgs[1] = c.uid
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
