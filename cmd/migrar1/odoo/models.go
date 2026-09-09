package odoo

import "encoding/json"

// Partner representa un contacto/empresa (res.partner)
type Partner struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	VAT       string `json:"vat"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	IsCompany bool   `json:"is_company"`
	Active    bool   `json:"active"`
}

// User representa un usuario del sistema (res.users)
type User struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Login   string `json:"login"`
	Email   string `json:"email"`
	Active  bool   `json:"active"`
}

// Employee representa un trabajador (hr.employee)
type Employee struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	WorkEmail string `json:"work_email"`
	UserID int    `json:"user_id"`
	Active bool   `json:"active"`
}

// Project representa un proyecto (project.project)
type Project struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Active      bool   `json:"active"`
}

// Ticket representa un ticket de soporte / helpdesk (helpdesk.ticket)
type Ticket struct {
	ID             int             `json:"id"`
	Name           string          `json:"name"`
	TicketRef      string          `json:"ticket_ref,omitempty"`
	Description    string          `json:"description,omitempty"`
	CreateDate     string          `json:"create_date"`
	CloseDate      string          `json:"close_date,omitempty"`
	StageID        json.RawMessage `json:"stage_id,omitempty"` // [id, name] o id
	TeamID         json.RawMessage `json:"team_id,omitempty"`
	PartnerID      json.RawMessage `json:"partner_id,omitempty"`
	PartnerName    string          `json:"partner_name,omitempty"`
	PartnerEmail   string          `json:"partner_email,omitempty"`
	UserID         json.RawMessage `json:"user_id,omitempty"`
	Priority       string          `json:"priority,omitempty"`
	ProjectID      json.RawMessage `json:"project_id,omitempty"`
	RawFields      map[string]interface{} `json:"raw_fields,omitempty"`
}

// TimesheetEntry representa una línea de parte de horas (account.analytic.line)
type TimesheetEntry struct {
	ID          int             `json:"id"`
	Date        string          `json:"date"`
	Name        string          `json:"name"`
	UnitAmount  float64         `json:"unit_amount"`
	ProjectID   json.RawMessage `json:"project_id,omitempty"`
	TaskID      json.RawMessage `json:"task_id,omitempty"`
	TicketID    json.RawMessage `json:"ticket_id,omitempty"` // o helpdesk_ticket_id
	EmployeeID  json.RawMessage `json:"employee_id,omitempty"`
	UserID      json.RawMessage `json:"user_id,omitempty"`
	RawFields   map[string]interface{} `json:"raw_fields,omitempty"`
}

// Helper para extraer [ID, Nombre] de campos Many2one
func ParseMany2One(raw json.RawMessage) (int, string) {
	if len(raw) == 0 || string(raw) == "false" || string(raw) == "null" {
		return 0, ""
	}

	var arr []interface{}
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) >= 2 {
		idFloat, ok := arr[0].(float64)
		if ok {
			nameStr, _ := arr[1].(string)
			return int(idFloat), nameStr
		}
	}

	var idFloat float64
	if err := json.Unmarshal(raw, &idFloat); err == nil {
		return int(idFloat), ""
	}

	return 0, ""
}
