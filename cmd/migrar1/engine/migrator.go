package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"pasigo/cmd/migrar1/db"
	"pasigo/cmd/migrar1/odoo"
	"strings"
	"sync"
	"time"
)

type MigrationConfig struct {
	StartDate     string `json:"start_date"` // "2025-01-01"
	BatchSize     int    `json:"batch_size"`
	SourceTicketModel string `json:"source_ticket_model"` // "helpdesk.ticket"
	TargetTicketModel string `json:"target_ticket_model"`
}

type DryRunReport struct {
	TotalSourceTickets     int                  `json:"total_source_tickets"`
	TotalSourceTimesheets  int                  `json:"total_source_timesheets"`
	AlreadySyncedTickets   int                  `json:"already_synced_tickets"`
	PendingTickets         int                  `json:"pending_tickets"`
	UnresolvedPartners     []odoo.Partner       `json:"unresolved_partners"`
	UnresolvedUsers        []odoo.User          `json:"unresolved_users"`
	UnresolvedProjects     []odoo.Project       `json:"unresolved_projects"`
	ReadyToMigrateTickets  int                  `json:"ready_to_migrate_tickets"`
	Timestamp              time.Time            `json:"timestamp"`
}

type Migrator struct {
	mu           sync.RWMutex
	db           *db.DB
	sourceClient *odoo.Client
	targetClient *odoo.Client
	registry     *odoo.MasterRegistry
	cfg          MigrationConfig

	// Campos soportados en destino
	targetTicketFields    map[string]map[string]interface{}
	targetTimesheetFields map[string]map[string]interface{}
}

func NewMigrator(database *db.DB, sourceClient, targetClient *odoo.Client, cfg MigrationConfig) *Migrator {
	if cfg.StartDate == "" {
		cfg.StartDate = "2025-01-01"
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.SourceTicketModel == "" {
		cfg.SourceTicketModel = "helpdesk.ticket"
	}
	if cfg.TargetTicketModel == "" {
		cfg.TargetTicketModel = "helpdesk.ticket"
	}

	return &Migrator{
		db:           database,
		sourceClient: sourceClient,
		targetClient: targetClient,
		registry:     odoo.NewMasterRegistry(database),
		cfg:          cfg,
	}
}

func (m *Migrator) Registry() *odoo.MasterRegistry {
	return m.registry
}

// InitSchemaDiscovery inicializa y analiza los campos disponibles en el Odoo destino
func (m *Migrator) InitSchemaDiscovery(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 1. Cargar maestros en memoria para resolución rápida
	if err := m.registry.LoadTargetMasters(ctx, m.targetClient); err != nil {
		m.db.Log("WARN", "system", 0, 0, "No se pudieron precargar todos los maestros de destino", err.Error())
	}

	// 2. Descubrir campos de helpdesk.ticket en destino
	ticketFields, err := m.targetClient.FieldsGet(ctx, m.cfg.TargetTicketModel, nil)
	if err != nil {
		m.db.Log("WARN", m.cfg.TargetTicketModel, 0, 0, "No se pudieron obtener campos del modelo de tickets en destino", err.Error())
	} else {
		m.targetTicketFields = ticketFields
	}

	// 3. Descubrir campos de account.analytic.line en destino
	timesheetFields, err := m.targetClient.FieldsGet(ctx, "account.analytic.line", nil)
	if err != nil {
		m.db.Log("WARN", "account.analytic.line", 0, 0, "No se pudieron obtener campos de partes de horas en destino", err.Error())
	} else {
		m.targetTimesheetFields = timesheetFields
	}

	return nil
}

// DryRun realiza un análisis completo sin escribir ningún dato en destino
func (m *Migrator) DryRun(ctx context.Context) (*DryRunReport, error) {
	if err := m.InitSchemaDiscovery(ctx); err != nil {
		return nil, err
	}

	report := &DryRunReport{
		Timestamp: time.Now(),
	}

	// 1. Consultar tickets de origen desde 2025-01-01
	domain := []interface{}{
		[]interface{}{"create_date", ">=", m.cfg.StartDate + " 00:00:00"},
	}
	ticketsRaw, err := m.sourceClient.SearchRead(ctx, m.cfg.SourceTicketModel, domain, []string{
		"id", "name", "create_date", "partner_id", "user_id", "project_id", "stage_id",
	}, 0, 0, "create_date asc")
	if err != nil {
		return nil, fmt.Errorf("error al consultar tickets de origen: %w", err)
	}

	var tickets []odoo.Ticket
	if err := json.Unmarshal(ticketsRaw, &tickets); err != nil {
		return nil, fmt.Errorf("error parseando tickets: %w", err)
	}
	report.TotalSourceTickets = len(tickets)

	// 2. Consultar partes de horas de origen desde 2025-01-01
	tsDomain := []interface{}{
		[]interface{}{"date", ">=", m.cfg.StartDate},
	}
	timesheetsRaw, err := m.sourceClient.SearchRead(ctx, "account.analytic.line", tsDomain, []string{"id", "date", "name", "unit_amount"}, 0, 0, "date asc")
	if err == nil {
		var timesheets []odoo.TimesheetEntry
		_ = json.Unmarshal(timesheetsRaw, &timesheets)
		report.TotalSourceTimesheets = len(timesheets)
	}

	unresolvedPartnersMap := make(map[int]odoo.Partner)
	unresolvedUsersMap := make(map[int]odoo.User)
	unresolvedProjectsMap := make(map[int]odoo.Project)

	readyCount := 0

	for _, t := range tickets {
		// Comprobar si ya está sincronizado
		rec, _ := m.db.GetSyncRecord(m.cfg.SourceTicketModel, t.ID)
		if rec != nil && rec.Status == "synced" {
			report.AlreadySyncedTickets++
			continue
		}
		report.PendingTickets++

		isReady := true

		// Verificar Partner
		partnerID, partnerName := odoo.ParseMany2One(t.PartnerID)
		if partnerID > 0 {
			_, _, _, pErr := m.registry.ResolvePartner(odoo.Partner{ID: partnerID, Name: partnerName})
			if pErr != nil {
				isReady = false
				if _, exists := unresolvedPartnersMap[partnerID]; !exists {
					unresolvedPartnersMap[partnerID] = odoo.Partner{ID: partnerID, Name: partnerName}
				}
			}
		}

		// Verificar User
		userID, userName := odoo.ParseMany2One(t.UserID)
		if userID > 0 {
			_, _, _, uErr := m.registry.ResolveUser(odoo.User{ID: userID, Name: userName})
			if uErr != nil {
				isReady = false
				if _, exists := unresolvedUsersMap[userID]; !exists {
					unresolvedUsersMap[userID] = odoo.User{ID: userID, Name: userName}
				}
			}
		}

		// Verificar Project
		projID, projName := odoo.ParseMany2One(t.ProjectID)
		if projID > 0 {
			_, _, _, prErr := m.registry.ResolveProject(odoo.Project{ID: projID, Name: projName})
			if prErr != nil {
				isReady = false
				if _, exists := unresolvedProjectsMap[projID]; !exists {
					unresolvedProjectsMap[projID] = odoo.Project{ID: projID, Name: projName}
				}
			}
		}

		if isReady {
			readyCount++
		}
	}

	for _, p := range unresolvedPartnersMap {
		report.UnresolvedPartners = append(report.UnresolvedPartners, p)
	}
	for _, u := range unresolvedUsersMap {
		report.UnresolvedUsers = append(report.UnresolvedUsers, u)
	}
	for _, pr := range unresolvedProjectsMap {
		report.UnresolvedProjects = append(report.UnresolvedProjects, pr)
	}

	report.ReadyToMigrateTickets = readyCount
	return report, nil
}

// MigrateTicket realiza la migración idempotente de un único ticket y sus partes de horas
func (m *Migrator) MigrateTicket(ctx context.Context, sourceTicketID int) (*db.SyncRecord, error) {
	// 1. Obtener ticket completo del origen
	ticketRaw, err := m.sourceClient.SearchRead(ctx, m.cfg.SourceTicketModel, []interface{}{
		[]interface{}{"id", "=", sourceTicketID},
	}, nil, 1, 0, "")
	if err != nil {
		return nil, fmt.Errorf("error leyendo ticket %d de origen: %w", sourceTicketID, err)
	}

	var tickets []map[string]interface{}
	if err := json.Unmarshal(ticketRaw, &tickets); err != nil || len(tickets) == 0 {
		return nil, fmt.Errorf("ticket %d no encontrado en origen", sourceTicketID)
	}
	srcTicketData := tickets[0]

	// 2. Calcular hash de contenido
	contentHash, err := db.CalculateHash(srcTicketData)
	if err != nil {
		contentHash = fmt.Sprintf("h_%d_%d", sourceTicketID, time.Now().Unix())
	}

	// 3. Comprobar registro en SQLite
	rec, err := m.db.GetSyncRecord(m.cfg.SourceTicketModel, sourceTicketID)
	if err != nil {
		return nil, fmt.Errorf("error en base de datos local: %w", err)
	}

	if rec != nil && rec.Status == "synced" && rec.ContentHash == contentHash && rec.TargetID > 0 {
		// Ya está sincronizado e idéntico -> Idempotencia pura
		m.db.Log("INFO", m.cfg.SourceTicketModel, sourceTicketID, rec.TargetID, "Ticket ya sincronizado sin cambios (omitido)", "")
		return rec, nil
	}

	if rec == nil {
		rec = &db.SyncRecord{
			EntityType:  m.cfg.SourceTicketModel,
			SourceID:    sourceTicketID,
			ContentHash: contentHash,
			Status:      "pending",
		}
	}

	// 4. Preparar valores para el Odoo destino mapeando relaciones
	targetValues := make(map[string]interface{})

	// Nombre / Asunto
	if name, ok := srcTicketData["name"].(string); ok {
		targetValues["name"] = name
	}
	if desc, ok := srcTicketData["description"].(string); ok && m.hasTargetField(m.cfg.TargetTicketModel, "description") {
		targetValues["description"] = desc
	}
	if prio, ok := srcTicketData["priority"].(string); ok && m.hasTargetField(m.cfg.TargetTicketModel, "priority") {
		targetValues["priority"] = prio
	}

	// Partner (CLIENTE) -> Estricto: NO CREAR
	if pRaw, ok := srcTicketData["partner_id"]; ok && pRaw != nil && pRaw != false {
		pBytes, _ := json.Marshal(pRaw)
		pID, pName := odoo.ParseMany2One(pBytes)
		if pID > 0 {
			targetPID, _, _, pErr := m.registry.ResolvePartner(odoo.Partner{ID: pID, Name: pName})
			if pErr != nil {
				errMsg := fmt.Sprintf("No se puede migrar ticket %d: %s", sourceTicketID, pErr.Error())
				rec.Status = "error"
				rec.ErrorMessage = errMsg
				_ = m.db.SaveSyncRecord(rec)
				m.db.Log("ERROR", m.cfg.SourceTicketModel, sourceTicketID, 0, errMsg, "")
				return rec, fmt.Errorf("%s", errMsg)
			}
			targetValues["partner_id"] = targetPID
		}
	}

	// User / Trabajador -> Estricto: NO CREAR
	if uRaw, ok := srcTicketData["user_id"]; ok && uRaw != nil && uRaw != false {
		uBytes, _ := json.Marshal(uRaw)
		uID, uName := odoo.ParseMany2One(uBytes)
		if uID > 0 {
			targetUID, _, _, uErr := m.registry.ResolveUser(odoo.User{ID: uID, Name: uName})
			if uErr == nil && targetUID > 0 {
				targetValues["user_id"] = targetUID
			}
		}
	}

	// Project -> Estricto: NO CREAR
	if prRaw, ok := srcTicketData["project_id"]; ok && prRaw != nil && prRaw != false && m.hasTargetField(m.cfg.TargetTicketModel, "project_id") {
		prBytes, _ := json.Marshal(prRaw)
		prID, prName := odoo.ParseMany2One(prBytes)
		if prID > 0 {
			targetPrID, _, _, prErr := m.registry.ResolveProject(odoo.Project{ID: prID, Name: prName})
			if prErr == nil && targetPrID > 0 {
				targetValues["project_id"] = targetPrID
			}
		}
	}

	// 5. Ejecutar create o write en destino
	var targetID int
	if rec.TargetID > 0 {
		// Actualización
		if err := m.targetClient.Write(ctx, m.cfg.TargetTicketModel, rec.TargetID, targetValues); err != nil {
			rec.Status = "error"
			rec.ErrorMessage = err.Error()
			_ = m.db.SaveSyncRecord(rec)
			m.db.Log("ERROR", m.cfg.TargetTicketModel, sourceTicketID, rec.TargetID, "Error al actualizar ticket en destino", err.Error())
			return rec, err
		}
		targetID = rec.TargetID
		m.db.Log("INFO", m.cfg.TargetTicketModel, sourceTicketID, targetID, "Ticket actualizado con éxito en destino", "")
	} else {
		// Creación
		newID, err := m.targetClient.Create(ctx, m.cfg.TargetTicketModel, targetValues)
		if err != nil {
			rec.Status = "error"
			rec.ErrorMessage = err.Error()
			_ = m.db.SaveSyncRecord(rec)
			m.db.Log("ERROR", m.cfg.TargetTicketModel, sourceTicketID, 0, "Error al crear ticket en destino", err.Error())
			return rec, err
		}
		targetID = newID
		m.db.Log("INFO", m.cfg.TargetTicketModel, sourceTicketID, targetID, fmt.Sprintf("Ticket creado con éxito en destino (ID: %d)", targetID), "")
	}

	// 6. Actualizar registro en SQLite
	rec.TargetID = targetID
	rec.ContentHash = contentHash
	rec.Status = "synced"
	rec.ErrorMessage = ""
	if err := m.db.SaveSyncRecord(rec); err != nil {
		m.db.Log("WARN", "db", sourceTicketID, targetID, "Error guardando sync record", err.Error())
	}

	// 7. Migrar partes de horas vinculados a este ticket
	_ = m.migrateTimesheetsForTicket(ctx, sourceTicketID, targetID)

	return rec, nil
}

func (m *Migrator) migrateTimesheetsForTicket(ctx context.Context, srcTicketID, targetTicketID int) error {
	// Buscar partes de horas del origen asociados a este ticket
	domain := []interface{}{
		[]interface{}{"helpdesk_ticket_id", "=", srcTicketID},
	}
	tsRaw, err := m.sourceClient.SearchRead(ctx, "account.analytic.line", domain, nil, 0, 0, "id asc")
	if err != nil {
		// Reintento con 'ticket_id' alternativo
		domain2 := []interface{}{
			[]interface{}{"ticket_id", "=", srcTicketID},
		}
		tsRaw, err = m.sourceClient.SearchRead(ctx, "account.analytic.line", domain2, nil, 0, 0, "id asc")
	}

	if err != nil || len(tsRaw) == 0 {
		return nil
	}

	var timesheets []map[string]interface{}
	if err := json.Unmarshal(tsRaw, &timesheets); err != nil {
		return err
	}

	for _, ts := range timesheets {
		srcTSID, ok := ts["id"].(float64)
		if !ok {
			continue
		}
		srcID := int(srcTSID)

		tsHash, _ := db.CalculateHash(ts)
		tsRec, _ := m.db.GetSyncRecord("account.analytic.line", srcID)
		if tsRec != nil && tsRec.Status == "synced" && tsRec.ContentHash == tsHash && tsRec.TargetID > 0 {
			continue
		}

		if tsRec == nil {
			tsRec = &db.SyncRecord{
				EntityType:  "account.analytic.line",
				SourceID:    srcID,
				ContentHash: tsHash,
				Status:      "pending",
			}
		}

		tsValues := make(map[string]interface{})
		if name, ok := ts["name"].(string); ok {
			tsValues["name"] = name
		}
		if date, ok := ts["date"].(string); ok {
			tsValues["date"] = date
		}
		if unitAmount, ok := ts["unit_amount"].(float64); ok {
			tsValues["unit_amount"] = unitAmount
		}

		// Asignar al ticket destino
		if m.hasTargetField("account.analytic.line", "helpdesk_ticket_id") {
			tsValues["helpdesk_ticket_id"] = targetTicketID
		} else if m.hasTargetField("account.analytic.line", "ticket_id") {
			tsValues["ticket_id"] = targetTicketID
		}

		// Mapear Proyecto
		if prRaw, ok := ts["project_id"]; ok && prRaw != nil && prRaw != false {
			prBytes, _ := json.Marshal(prRaw)
			prID, prName := odoo.ParseMany2One(prBytes)
			if prID > 0 {
				tgtPrID, _, _, err := m.registry.ResolveProject(odoo.Project{ID: prID, Name: prName})
				if err == nil && tgtPrID > 0 {
					tsValues["project_id"] = tgtPrID
				}
			}
		}

		// Mapear Usuario / Empleado
		if uRaw, ok := ts["user_id"]; ok && uRaw != nil && uRaw != false {
			uBytes, _ := json.Marshal(uRaw)
			uID, uName := odoo.ParseMany2One(uBytes)
			if uID > 0 {
				tgtUID, _, _, err := m.registry.ResolveUser(odoo.User{ID: uID, Name: uName})
				if err == nil && tgtUID > 0 {
					tsValues["user_id"] = tgtUID
				}
			}
		}

		if tsRec.TargetID > 0 {
			_ = m.targetClient.Write(ctx, "account.analytic.line", tsRec.TargetID, tsValues)
			tsRec.Status = "synced"
			_ = m.db.SaveSyncRecord(tsRec)
		} else {
			newTSID, err := m.targetClient.Create(ctx, "account.analytic.line", tsValues)
			if err == nil {
				tsRec.TargetID = newTSID
				tsRec.Status = "synced"
				_ = m.db.SaveSyncRecord(tsRec)
				m.db.Log("INFO", "account.analytic.line", srcID, newTSID, fmt.Sprintf("Parte de horas migrado para ticket %d", targetTicketID), "")
			} else {
				tsRec.Status = "error"
				tsRec.ErrorMessage = err.Error()
				_ = m.db.SaveSyncRecord(tsRec)
				m.db.Log("ERROR", "account.analytic.line", srcID, 0, "Error migrando parte de horas", err.Error())
			}
		}
	}

	return nil
}

func (m *Migrator) hasTargetField(model, field string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if strings.HasPrefix(model, "helpdesk") && m.targetTicketFields != nil {
		_, ok := m.targetTicketFields[field]
		return ok
	}
	if model == "account.analytic.line" && m.targetTimesheetFields != nil {
		_, ok := m.targetTimesheetFields[field]
		return ok
	}
	return true
}
