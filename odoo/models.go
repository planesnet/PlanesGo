package odoo

import (
	"encoding/json"
	"fmt"
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

// TimesheetEntry representa un registro de horas de Odoo (account.analytic.line).
type TimesheetEntry struct {
	ID          int      `json:"id"`
	Date        string   `json:"date"`
	Name        string   `json:"name"`
	UnitAmount  float64  `json:"unit_amount"`
	ProjectID   Many2One `json:"project_id"`
	TaskID      Many2One `json:"task_id"`
	EmployeeID  Many2One `json:"employee_id"`
	UserID      Many2One `json:"user_id"`
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
	TimesheetCount    int      `json:"timesheet_count"`    // Cantidad de partes de horas
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
	return "-"
}

func (p *Project) FormattedTotalHours() string {
	hours := int(p.TotalHours)
	minutes := int((p.TotalHours - float64(hours)) * 60)
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %02dm (%.2fh)", hours, minutes, p.TotalHours)
}
