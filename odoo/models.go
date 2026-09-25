package odoo

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Many2One representa un campo Many2one de Odoo (que puede ser [id, "Nombre"] o false).
type Many2One struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (m *Many2One) UnmarshalJSON(data []byte) error {
	// Odoo devuelve false si el campo Many2one está vacío
	var isBool bool
	if err := json.Unmarshal(data, &isBool); err == nil {
		m.ID = 0
		m.Name = ""
		return nil
	}

	// Si tiene valor, Odoo devuelve [id, "nombre"]
	var arr []interface{}
	if err := json.Unmarshal(data, &arr); err == nil && len(arr) >= 2 {
		if idFloat, ok := arr[0].(float64); ok {
			m.ID = int(idFloat)
		}
		if nameStr, ok := arr[1].(string); ok {
			m.Name = nameStr
		}
		return nil
	}

	return nil
}

func (m Many2One) String() string {
	if m.Name != "" {
		return m.Name
	}
	if m.ID > 0 {
		return fmt.Sprintf("#%d", m.ID)
	}
	return "-"
}

// Tag representa una etiqueta de tarea en Odoo (project.tags).
type Tag struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Color int    `json:"color,omitempty"`
}

// TimesheetEntry representa un registro de horas de Odoo (account.analytic.line).
type TimesheetEntry struct {
	ID                 int         `json:"id"`
	Date               string      `json:"date"`
	Name               string      `json:"name"`
	UnitAmount         float64     `json:"unit_amount"`
	ProjectID          Many2One    `json:"project_id"`
	TaskID             Many2One    `json:"task_id"`
	EmployeeID         Many2One    `json:"employee_id"`
	UserID             Many2One    `json:"user_id"`
	PartnerID          Many2One    `json:"partner_id"`
	TimesheetInvoiceID Many2One    `json:"timesheet_invoice_id"`
	BillingRef         interface{} `json:"billing_ref"` // Indica si está facturado en Odoo 14
	IsTimerRunning     bool        `json:"is_timer_running"`
	Tags               []Tag       `json:"tags,omitempty"`
	TagIDs             []int       `json:"tag_ids,omitempty"`
	HelpdeskTicketID   Many2One    `json:"helpdesk_ticket_id,omitempty"`
}

// IsInvoiced indica si la imputación de horas ya ha sido facturada en Odoo.
func (t *TimesheetEntry) IsInvoiced() bool {
	if t.TimesheetInvoiceID.ID > 0 {
		return true
	}
	if t.BillingRef != nil {
		switch v := t.BillingRef.(type) {
		case bool:
			return v
		case string:
			s := strings.TrimSpace(v)
			return s != "" && s != "false" && s != "-" && s != "0"
		case float64:
			return v > 0
		case int:
			return v > 0
		case []interface{}:
			return len(v) > 0
		}
	}
	return false
}

// DisplayEmployee obtiene el nombre del empleado o del usuario si no hay empleado asociado.
func (t *TimesheetEntry) DisplayEmployee() string {
	if t.EmployeeID.Name != "" {
		return t.EmployeeID.Name
	}
	if t.UserID.Name != "" {
		return t.UserID.Name
	}
	return "Sin asignar"
}

// IsAntigravity indica si la imputación o su tarea proviene del sistema Antigravity.
func (t *TimesheetEntry) IsAntigravity() bool {
	for _, tag := range t.Tags {
		if strings.EqualFold(tag.Name, "Antigravity") || strings.EqualFold(tag.Name, "AGY") {
			return true
		}
	}
	taskName := strings.ToUpper(t.TaskID.Name)
	desc := strings.ToUpper(t.Name)
	return strings.HasPrefix(taskName, "[AGY]") ||
		strings.HasPrefix(taskName, "[ANTIGRAVITY]") ||
		strings.Contains(taskName, "ANTIGRAVITY") ||
		strings.HasPrefix(desc, "[ANTIGRAVITY]") ||
		strings.HasPrefix(desc, "[AGY]")
}

// IsHoraMaquina indica si la imputación tiene la etiqueta Hora Máquina o proviene de Antigravity.
func (t *TimesheetEntry) IsHoraMaquina() bool {
	for _, tag := range t.Tags {
		if strings.EqualFold(tag.Name, "Hora Máquina") || strings.EqualFold(tag.Name, "Hora Maquina") {
			return true
		}
	}
	return t.IsAntigravity()
}

// IsHoraHombre indica si la imputación es de trabajo humano (no es de máquina/IA).
func (t *TimesheetEntry) IsHoraHombre() bool {
	for _, tag := range t.Tags {
		if strings.EqualFold(tag.Name, "Hora Hombre") {
			return true
		}
	}
	return !t.IsHoraMaquina()
}

// MarshalJSON serializa TimesheetEntry agregando campos calculados para la API y vistas cliente.
func (t TimesheetEntry) MarshalJSON() ([]byte, error) {
	type Alias TimesheetEntry
	return json.Marshal(&struct {
		Alias
		IsAntigravity bool   `json:"is_antigravity"`
		IsHoraMaquina bool   `json:"is_hora_maquina"`
		IsHoraHombre  bool   `json:"is_hora_hombre"`
		CleanTaskName string `json:"clean_task_name"`
	}{
		Alias:         Alias(t),
		IsAntigravity: t.IsAntigravity(),
		IsHoraMaquina: t.IsHoraMaquina(),
		IsHoraHombre:  t.IsHoraHombre(),
		CleanTaskName: t.CleanTaskName(),
	})
}

// CleanTaskName devuelve el nombre de la tarea sin el prefijo técnico [AGY] o [ANTIGRAVITY].
func (t *TimesheetEntry) CleanTaskName() string {
	name := strings.TrimSpace(t.TaskID.Name)
	if strings.HasPrefix(strings.ToUpper(name), "[AGY]") {
		return strings.TrimSpace(name[5:])
	}
	if strings.HasPrefix(strings.ToUpper(name), "[ANTIGRAVITY]") {
		return strings.TrimSpace(name[14:])
	}
	return name
}

// FormattedHours devuelve las horas con 2 decimales y formato amigable (ej: "4.50h" o "4h 30m").
func (t *TimesheetEntry) FormattedHours() string {
	hours := int(t.UnitAmount)
	minutes := int((t.UnitAmount - float64(hours)) * 60)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %02dm (%.2fh)", hours, minutes, t.UnitAmount)
}

// Project representa un proyecto definido en Odoo (project.project).
type Project struct {
	ID                int      `json:"id"`
	Name              string   `json:"name"`
	DisplayName       string   `json:"display_name"`
	UserID            Many2One `json:"user_id"`            // Responsable del proyecto
	PartnerID         Many2One `json:"partner_id"`         // Cliente o contacto asociado
	TaskCount         int      `json:"task_count"`         // Cantidad de tareas
	Active            bool     `json:"active"`             // Estado activo / archivado
	PrivacyVisibility string   `json:"privacy_visibility"` // Visibilidad
	TotalHours        float64  `json:"total_hours"`        // Horas totales registradas
	TimesheetCount    int      `json:"timesheet_count,omitempty"` // Número de partes de horas registrados
	LastDate          string   `json:"last_date,omitempty"` // Fecha de última imputación (YYYY-MM-DD)
	LastTask          string   `json:"last_task,omitempty"` // Última tarea imputada
	OpenTicketCount   int      `json:"open_ticket_count,omitempty"`
	TicketTitle       string   `json:"ticket_title,omitempty"`
}

func (p *Project) FormattedLastDate() string {
	if p.LastDate == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", p.LastDate)
	if err != nil {
		return p.LastDate
	}
	return t.Format("02/01/2006")
}

func (p *Project) DisplayNameOrName() string {
	if p.Name != "" {
		return p.Name
	}
	if p.DisplayName != "" {
		return p.DisplayName
	}
	if p.ID > 0 {
		return fmt.Sprintf("Proyecto #%d", p.ID)
	}
	return "Sin nombre"
}

func (p *Project) DisplayManager() string {
	if p.UserID.Name != "" {
		return p.UserID.Name
	}
	return "Sin asignar"
}

func (p *Project) DisplayPartner() string {
	if p.PartnerID.Name != "" {
		return p.PartnerID.Name
	}
	return ""
}

func (p *Project) FormattedTotalHours() string {
	hours := int(p.TotalHours)
	minutes := int((p.TotalHours - float64(hours)) * 60)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %02dm (%.2fh)", hours, minutes, p.TotalHours)
}

// Task representa una tarea de un proyecto en Odoo (project.task).
type Task struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	ProjectID   Many2One `json:"project_id"`
	UserID      Many2One `json:"user_id"`
	Active      bool     `json:"active"`
	TagIDs      []int    `json:"tag_ids,omitempty"`
	Tags        []Tag    `json:"tags,omitempty"`
}

func (t *Task) DisplayNameOrName() string {
	if t.Name != "" {
		return t.Name
	}
	if t.DisplayName != "" {
		return t.DisplayName
	}
	if t.ID > 0 {
		return fmt.Sprintf("Tarea #%d", t.ID)
	}
	return "Sin nombre"
}

// Employee representa un trabajador de Odoo (hr.employee).
type Employee struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	WorkEmail string   `json:"work_email"`
	UserID    Many2One `json:"user_id"`
	Active    bool     `json:"active"`
}

// ResUser representa un usuario del sistema Odoo (res.users) para asignaciones de tickets y tareas.
type ResUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
	Email string `json:"email"`
}

// Ticket representa un ticket de soporte/helpdesk en Odoo (helpdesk.ticket).
type Ticket struct {
	ID              int      `json:"id"`
	Name            string   `json:"name"`
	Number          string   `json:"number,omitempty"`
	TicketRef       string   `json:"ticket_ref,omitempty"`
	Description     string   `json:"description,omitempty"`
	StageID         Many2One `json:"stage_id"`
	UserID          Many2One `json:"user_id"`
	PartnerID       Many2One `json:"partner_id"`
	ProjectID       Many2One `json:"project_id"`
	TaskID          Many2One `json:"task_id,omitempty"`
	Priority        string   `json:"priority,omitempty"`
	CreateDate      string   `json:"create_date,omitempty"`
	Closed          bool     `json:"closed,omitempty"`
	ClosedDate      string   `json:"closed_date,omitempty"`
	CloseDate       string   `json:"close_date,omitempty"`
	KanbanState     string   `json:"kanban_state,omitempty"`
	TotalHoursSpent float64  `json:"total_hours_spent,omitempty"`
}

func (t *Ticket) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID              int         `json:"id"`
		Name            interface{} `json:"name"`
		Number          interface{} `json:"number"`
		TicketRef       interface{} `json:"ticket_ref"`
		Description     interface{} `json:"description"`
		StageID         Many2One    `json:"stage_id"`
		UserID          Many2One    `json:"user_id"`
		PartnerID       Many2One    `json:"partner_id"`
		ProjectID       Many2One    `json:"project_id"`
		TaskID          Many2One    `json:"task_id"`
		Priority        interface{} `json:"priority"`
		CreateDate      interface{} `json:"create_date"`
		Closed          interface{} `json:"closed"`
		ClosedDate      interface{} `json:"closed_date"`
		CloseDate       interface{} `json:"close_date"`
		KanbanState     interface{} `json:"kanban_state"`
		TotalHoursSpent float64     `json:"total_hours_spent"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	t.ID = raw.ID
	if s, ok := raw.Name.(string); ok {
		t.Name = s
	}
	if s, ok := raw.Number.(string); ok {
		t.Number = s
	}
	if s, ok := raw.TicketRef.(string); ok {
		t.TicketRef = s
	}
	if s, ok := raw.Description.(string); ok {
		t.Description = s
	}
	t.StageID = raw.StageID
	t.UserID = raw.UserID
	t.PartnerID = raw.PartnerID
	t.ProjectID = raw.ProjectID
	t.TaskID = raw.TaskID
	switch v := raw.Priority.(type) {
	case string:
		t.Priority = v
	case float64:
		t.Priority = strconv.Itoa(int(v))
	case int:
		t.Priority = strconv.Itoa(v)
	}
	if s, ok := raw.CreateDate.(string); ok {
		t.CreateDate = s
	}
	if b, ok := raw.Closed.(bool); ok {
		t.Closed = b
	}
	if s, ok := raw.ClosedDate.(string); ok {
		t.ClosedDate = s
	}
	if s, ok := raw.CloseDate.(string); ok {
		t.CloseDate = s
	}
	if s, ok := raw.KanbanState.(string); ok {
		t.KanbanState = s
	}
	t.TotalHoursSpent = raw.TotalHoursSpent
	return nil
}

func (t *Ticket) DisplayTitle() string {
	ref := t.Number
	if ref == "" {
		ref = t.TicketRef
	}
	if ref != "" && !strings.Contains(t.Name, ref) {
		return fmt.Sprintf("[%s] %s", ref, t.Name)
	}
	if t.Name != "" {
		return t.Name
	}
	return fmt.Sprintf("Ticket %d", t.ID)
}

func (t *Ticket) StageName() string {
	if t.StageID.Name != "" {
		return t.StageID.Name
	}
	return "Pendiente"
}

func (t *Ticket) PartnerName() string {
	if t.PartnerID.Name != "" {
		return t.PartnerID.Name
	}
	return "-"
}

func (t *Ticket) ProjectName() string {
	if t.ProjectID.Name != "" {
		return t.ProjectID.Name
	}
	return ""
}

func (t *Ticket) ProjectIDValue() int {
	return t.ProjectID.ID
}

func (t *Ticket) TaskName() string {
	if t.TaskID.Name != "" {
		return t.TaskID.Name
	}
	return ""
}

func (t *Ticket) TaskIDValue() int {
	return t.TaskID.ID
}

func (t *Ticket) PriorityStars() int {
	switch strings.TrimSpace(t.Priority) {
	case "3":
		return 3
	case "2":
		return 2
	case "1":
		return 1
	default:
		return 0
	}
}

func (t *Ticket) IsClosed() bool {
	if t.CloseDate != "" && t.CloseDate != "false" {
		return true
	}
	sName := strings.ToLower(t.StageName())
	closedKeywords := []string{"cerrad", "solucion", "cancel", "done", "closed", "solved", "resuelto"}
	for _, kw := range closedKeywords {
		if strings.Contains(sName, kw) {
			return true
		}
	}
	return false
}

func (t *Ticket) FormattedDate() string {
	if len(t.CreateDate) >= 10 {
		return t.CreateDate[:10]
	}
	return t.CreateDate
}

// ActiveTimer representa el estado del cronómetro de trabajo en vivo sincronizado con Odoo.
type ActiveTimer struct {
	TimerKey      string   `json:"timer_key,omitempty"`      // Clave identificadora única del temporizador (ej. task_id o hash)
	TimesheetID   int      `json:"timesheet_id"`
	TaskID        int      `json:"task_id"`
	ProjectID     int      `json:"project_id"`
	ProjectName   string   `json:"project_name"`
	TaskName      string   `json:"task_name"`
	TicketID      int      `json:"ticket_id,omitempty"`
	TicketRef     string   `json:"ticket_ref,omitempty"`
	TicketName    string   `json:"ticket_name,omitempty"`
	Description   string   `json:"description"`
	IsRunning     bool     `json:"is_running"`
	StartedAt     int64    `json:"started_at"`      // Timestamp unix en milisegundos
	LastHeartbeat int64    `json:"last_heartbeat,omitempty"` // Timestamp unix del último latido de actividad
	AccumulatedMs int64    `json:"accumulated_ms"`  // Milisegundos acumulados
	UnitAmount    float64  `json:"unit_amount"`     // Horas calculadas en decimal
	Date          string   `json:"date,omitempty"`
	Source        string   `json:"source,omitempty"`        // Origen del temporizador: "antigravity", "web", "extension"
	EmployeeName  string   `json:"employee_name,omitempty"`
	Tags          []Tag    `json:"tags,omitempty"`
}

// IsAntigravity indica si el temporizador activo proviene de Antigravity.
func (t *ActiveTimer) IsAntigravity() bool {
	if strings.EqualFold(t.Source, "antigravity") {
		return true
	}
	for _, tag := range t.Tags {
		if strings.EqualFold(tag.Name, "Antigravity") || strings.EqualFold(tag.Name, "AGY") {
			return true
		}
	}
	taskName := strings.ToUpper(t.TaskName)
	desc := strings.ToUpper(t.Description)
	return strings.HasPrefix(taskName, "[AGY]") ||
		strings.HasPrefix(taskName, "[ANTIGRAVITY]") ||
		strings.Contains(taskName, "ANTIGRAVITY") ||
		strings.HasPrefix(desc, "[ANTIGRAVITY]") ||
		strings.HasPrefix(desc, "[AGY]")
}

// CleanTaskName devuelve el nombre de la tarea sin el prefijo técnico [AGY] o [ANTIGRAVITY].
func (t *ActiveTimer) CleanTaskName() string {
	name := strings.TrimSpace(t.TaskName)
	if strings.HasPrefix(strings.ToUpper(name), "[AGY]") {
		return strings.TrimSpace(name[5:])
	}
	if strings.HasPrefix(strings.ToUpper(name), "[ANTIGRAVITY]") {
		return strings.TrimSpace(name[14:])
	}
	return name
}

// Partner representa un contacto o cliente de Odoo (res.partner)
type Partner struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (p *Partner) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID          int         `json:"id"`
		Name        interface{} `json:"name"`
		DisplayName interface{} `json:"display_name"`
		Email       interface{} `json:"email"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.ID = raw.ID
	if s, ok := raw.Name.(string); ok {
		p.Name = s
	}
	if s, ok := raw.DisplayName.(string); ok {
		p.DisplayName = s
	} else if p.Name != "" {
		p.DisplayName = p.Name
	}
	if s, ok := raw.Email.(string); ok {
		p.Email = s
	}
	return nil
}

