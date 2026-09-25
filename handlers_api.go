package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"pasigo/odoo"
)

type avatarCacheItem struct {
	data        []byte
	contentType string
	etag        string
	expiresAt   time.Time
}

var (
	partnerAvatarCache   = make(map[int]avatarCacheItem)
	partnerAvatarCacheMu sync.RWMutex
	projectPartnerCache  = make(map[int]int)
	projectPartnerMu     sync.RWMutex
)

// handleAPITimesheets gestiona GET (listar) y POST (crear) partes de horas
func (state *AppState) handleAPITimesheets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No has configurado tu clave API o sesión de Odoo. Ve a Ajustes para configurarla."})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
		if dateFrom == "" {
			dateFrom = strings.TrimSpace(r.URL.Query().Get("start_date"))
		}
		dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
		if dateTo == "" {
			dateTo = strings.TrimSpace(r.URL.Query().Get("end_date"))
		}

		var domain []interface{}
		if dateFrom != "" {
			domain = append(domain, []interface{}{"date", ">=", dateFrom})
		}
		if dateTo != "" {
			domain = append(domain, []interface{}{"date", "<=", dateTo})
		}

		if projIDStr := strings.TrimSpace(r.URL.Query().Get("project_id")); projIDStr != "" {
			if pID, pErr := strconv.Atoi(projIDStr); pErr == nil && pID > 0 {
				domain = append(domain, []interface{}{"project_id", "=", pID})
			}
		}

		// Si no se especifican fechas ni proyecto, filtrar por defecto por la semana actual
		if len(domain) == 0 {
			now := time.Now()
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			monday := now.AddDate(0, 0, -(weekday - 1))
			sunday := monday.AddDate(0, 0, 6)
			domain = append(domain,
				[]interface{}{"date", ">=", monday.Format("2006-01-02")},
				[]interface{}{"date", "<=", sunday.Format("2006-01-02")},
			)
		}

		entries, err := client.GetTimesheets(ctx, domain)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		// Inyectar o marcar la tarea con temporizador activo del usuario si existe
		targetUID := 0
		if session != nil && session.UserEmail != "" {
			if resUID, rErr := client.ResolveUserUIDByEmail(ctx, session.UserEmail); rErr == nil && resUID > 0 {
				targetUID = resUID
			}
		}
		if targetUID == 0 {
			targetUID = client.UID()
		}
		timer, tErr := client.GetActiveTimer(ctx, targetUID)
		if tErr != nil || timer == nil || !timer.IsRunning {
			timer = state.getActiveTimer(targetUID)
		}
		if timer != nil && timer.IsRunning {
			found := false
			for i := range entries {
				if (timer.TimesheetID > 0 && entries[i].ID == timer.TimesheetID) ||
					(timer.TaskID > 0 && entries[i].TaskID.ID == timer.TaskID) {
					entries[i].IsTimerRunning = true
					found = true
					break
				}
			}
			if !found {
				tDate := timer.Date
				if tDate == "" {
					tDate = time.Now().Format("2006-01-02")
				}
				empName := timer.EmployeeName
				if empName == "" && session != nil {
					empName = session.UserName
					if empName == "" {
						empName = session.Username
					}
				}
				entries = append([]odoo.TimesheetEntry{{
					ID:             timer.TimesheetID,
					Date:           tDate,
					Name:           timer.Description,
					UnitAmount:     timer.UnitAmount,
					ProjectID:      odoo.Many2One{ID: timer.ProjectID, Name: timer.ProjectName},
					TaskID:         odoo.Many2One{ID: timer.TaskID, Name: timer.TaskName},
					EmployeeID:     odoo.Many2One{Name: empName},
					UserID:         odoo.Many2One{ID: targetUID, Name: empName},
					IsTimerRunning: true,
				}}, entries...)
			}
		}

		// Enriquecer y actualizar projectPartnerCache con PartnerID
		projectPartnerMu.Lock()
		for i := range entries {
			if entries[i].PartnerID.ID > 0 && entries[i].ProjectID.ID > 0 {
				projectPartnerCache[entries[i].ProjectID.ID] = entries[i].PartnerID.ID
			} else if entries[i].PartnerID.ID == 0 && entries[i].ProjectID.ID > 0 {
				if partID, ok := projectPartnerCache[entries[i].ProjectID.ID]; ok && partID > 0 {
					entries[i].PartnerID = odoo.Many2One{ID: partID}
				}
			}
		}
		projectPartnerMu.Unlock()

		json.NewEncoder(w).Encode(entries)

	case http.MethodPost:
		var req struct {
			Date        string  `json:"date"`
			ProjectID   int     `json:"project_id"`
			TaskID      int     `json:"task_id"`
			TicketID    int     `json:"ticket_id"`
			UnitAmount  float64 `json:"unit_amount"`
			Description string  `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de solicitud inválido: " + err.Error()})
			return
		}

		if req.ProjectID <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Debes seleccionar un proyecto válido."})
			return
		}
		if req.UnitAmount <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El tiempo dedicado debe ser mayor a 0 horas."})
			return
		}
		if strings.TrimSpace(req.Description) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "La descripción del trabajo es obligatoria."})
			return
		}
		if strings.TrimSpace(req.Date) == "" {
			req.Date = time.Now().Format("2006-01-02")
		}

		newID, err := client.CreateTimesheetFull(ctx, req.Date, req.ProjectID, req.TaskID, req.TicketID, req.UnitAmount, req.Description, false)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar en Odoo: " + err.Error()})
			return
		}

		userUID := client.UID()
		if session != nil && session.UserEmail != "" {
			if resolvedUID, err := client.ResolveUserUIDByEmail(ctx, session.UserEmail); err == nil && resolvedUID > 0 {
				userUID = resolvedUID
			}
		}
		state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
			"action":     "create",
			"id":         newID,
			"project_id": req.ProjectID,
			"task_id":    req.TaskID,
			"date":       req.Date,
		})

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"id":      newID,
			"message": "Parte de horas registrado correctamente en Odoo.",
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
	}
}

// handleAPITimesheetsUpdate actualiza un parte de horas existente
func (state *AppState) handleAPITimesheetsUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No has configurado tu clave API o sesión de Odoo."})
		return
	}

	var req struct {
		ID          int     `json:"id"`
		Date        string  `json:"date"`
		TaskID      int     `json:"task_id"`
		TicketID    int     `json:"ticket_id"`
		UnitAmount  float64 `json:"unit_amount"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de solicitud inválido: " + err.Error()})
		return
	}

	if req.ID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El ID del registro de horas es obligatorio."})
		return
	}
	if req.UnitAmount <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El tiempo dedicado debe ser mayor a 0 horas."})
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "La descripción del trabajo es obligatoria."})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := client.UpdateTimesheetWithTicket(ctx, req.ID, req.Date, req.TaskID, req.TicketID, req.UnitAmount, req.Description); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar parte en Odoo: " + err.Error()})
		return
	}

	userUID := client.UID()
	if session != nil && session.UserEmail != "" {
		if resolvedUID, err := client.ResolveUserUIDByEmail(ctx, session.UserEmail); err == nil && resolvedUID > 0 {
			userUID = resolvedUID
		}
	}
	state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
		"action":      "update",
		"id":          req.ID,
		"date":        req.Date,
		"task_id":     req.TaskID,
		"unit_amount": req.UnitAmount,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Parte de horas actualizado correctamente en Odoo.",
	})
}

// handleAPITimesheetsDelete elimina un parte de horas existente
func (state *AppState) handleAPITimesheetsDelete(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No has configurado tu clave API o sesión de Odoo."})
		return
	}

	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Cuerpo de solicitud inválido: " + err.Error()})
		return
	}

	if req.ID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El ID del registro de horas a eliminar es obligatorio."})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := client.DeleteTimesheet(ctx, req.ID); err != nil {
		errMsg := err.Error()
		if strings.Contains(strings.ToLower(errMsg), "factura") {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
			return
		}
		if strings.Contains(strings.ToLower(errMsg), "access") ||
			strings.Contains(strings.ToLower(errMsg), "denied") ||
			strings.Contains(strings.ToLower(errMsg), "permis") {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "No tienes permisos en Odoo para eliminar este parte de horas."})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al eliminar parte de horas en Odoo: " + errMsg})
		return
	}

	userUID := client.UID()
	if session != nil && session.UserEmail != "" {
		if resolvedUID, err := client.ResolveUserUIDByEmail(ctx, session.UserEmail); err == nil && resolvedUID > 0 {
			userUID = resolvedUID
		}
	}
	state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
		"action": "delete",
		"id":     req.ID,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Parte de horas eliminado correctamente de Odoo.",
	})
}

// handleAPITasks lista o crea tareas en Odoo
func (state *AppState) handleAPITasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "No has configurado tu clave API o sesión de Odoo."})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		projectIDStr := r.URL.Query().Get("project_id")
		projectID, _ := strconv.Atoi(projectIDStr)
		if projectID <= 0 {
			json.NewEncoder(w).Encode([]odoo.Task{})
			return
		}

		tasks, err := client.GetTasks(ctx, projectID, userUID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al obtener tareas: " + err.Error()})
			return
		}
		json.NewEncoder(w).Encode(tasks)

	case http.MethodPost:
		var req struct {
			ProjectID int    `json:"project_id"`
			Name      string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Datos inválidos: " + err.Error()})
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if req.ProjectID <= 0 || req.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "El proyecto y el nombre de la tarea son obligatorios."})
			return
		}

		newID, err := client.CreateTask(ctx, req.ProjectID, req.Name, userUID)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(strings.ToLower(errMsg), "access") ||
				strings.Contains(strings.ToLower(errMsg), "denied") ||
				strings.Contains(strings.ToLower(errMsg), "permis") {
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "No tienes permisos en Odoo para crear tareas en este proyecto."})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error de Odoo al crear la tarea: " + errMsg})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"id":      newID,
			"name":    req.Name,
			"message": "Tarea creada correctamente en el proyecto.",
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
	}
}

// normalizeProjectSearch normaliza una cadena quitando tildes, espacios y caracteres especiales
func normalizeProjectSearch(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ä':
			b.WriteRune('a')
		case 'é', 'è', 'ë':
			b.WriteRune('e')
		case 'í', 'ì', 'ï':
			b.WriteRune('i')
		case 'ó', 'ò', 'ö':
			b.WriteRune('o')
		case 'ú', 'ù', 'ü':
			b.WriteRune('u')
		case 'ñ':
			b.WriteRune('n')
		default:
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// handleAPIProjects lista proyectos de Odoo con soporte para búsqueda flexible y refresco
func (state *AppState) handleAPIProjects(w http.ResponseWriter, r *http.Request) {
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	client := odoo.NewClient(odooCfg)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	query := strings.TrimSpace(r.URL.Query().Get("search"))
	if query == "" {
		query = strings.TrimSpace(r.URL.Query().Get("q"))
	}

	if r.URL.Query().Get("refresh") == "true" {
		client.InvalidateProjectsCache()
	}

	var domain []interface{}
	if query != "" {
		wildcard := "%" + query + "%"
		domain = append(domain, "|",
			[]interface{}{"name", "ilike", wildcard},
			[]interface{}{"display_name", "ilike", wildcard},
		)
		client.InvalidateProjectsCache()
	}

	projects, err := client.GetProjects(ctx, domain)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		// Reintento con dominio simple si falló la disyunción
		if query != "" {
			projects, err = client.GetProjects(ctx, []interface{}{[]interface{}{"name", "ilike", "%" + query + "%"}})
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
	}

	// Si la búsqueda directa en Odoo no encontró coincidencias (ej: "PLANESGO" vs "Planes Go"),
	// consultar todos los proyectos y aplicar coincidencia normalizada en memoria
	if query != "" && len(projects) == 0 {
		normQ := normalizeProjectSearch(query)
		if normQ != "" {
			allProjects, allErr := client.GetProjects(ctx, nil)
			if allErr == nil && len(allProjects) > 0 {
				var matched []odoo.Project
				for _, p := range allProjects {
					normName := normalizeProjectSearch(p.Name)
					normDisp := normalizeProjectSearch(p.DisplayName)
					if strings.Contains(normName, normQ) || strings.Contains(normDisp, normQ) {
						matched = append(matched, p)
					}
				}
				projects = matched
			}
		}
	}

	// Actualizar caché de relación proyecto -> partner
	if len(projects) > 0 {
		projectPartnerMu.Lock()
		for _, p := range projects {
			if p.ID > 0 && p.PartnerID.ID > 0 {
				projectPartnerCache[p.ID] = p.PartnerID.ID
			}
		}
		projectPartnerMu.Unlock()
	}

	json.NewEncoder(w).Encode(projects)
}

// handleAPIPartners busca contactos/clientes en Odoo (res.partner)
func (state *AppState) handleAPIPartners(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	query := r.URL.Query().Get("q")
	partners, err := client.GetPartners(ctx, query)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(partners)
}

// handleAPITickets consulta los tickets de soporte pendientes asignados al usuario en Odoo
func (state *AppState) handleAPITickets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		state.handleAPITicketsCreate(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	client := odoo.NewClient(odooCfg)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if r.URL.Query().Get("refresh") == "true" {
		client.InvalidateTicketsCache()
	}

	uid, err := client.Authenticate(ctx)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Consulta específica por referencia o ID de ticket
	ticketRef := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ticketRef == "" {
		ticketRef = strings.TrimSpace(r.URL.Query().Get("id"))
	}
	if ticketRef != "" {
		t, tErr := client.GetTicketByRefOrID(ctx, ticketRef)
		if tErr != nil || t == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Ticket '%s' no encontrado", ticketRef)})
			return
		}
		json.NewEncoder(w).Encode(t)
		return
	}

	targetUID := uid
	if session != nil && session.UserEmail != "" {
		if resUID, rErr := client.ResolveUserUIDByEmail(ctx, session.UserEmail); rErr == nil && resUID > 0 {
			targetUID = resUID
		}
	}
	if uParam := r.URL.Query().Get("user_id"); uParam != "" {
		if uVal, err := strconv.Atoi(uParam); err == nil {
			targetUID = uVal
		}
	}

	if r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("refresh") == "1" {
		client.InvalidateTicketsCache()
	}

	tickets, err := client.GetPendingTickets(ctx, targetUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(tickets)
}

// handleAPITicketsCreate crea un nuevo ticket en Odoo (helpdesk.ticket) asignado al usuario logueado u otro seleccionado
func (state *AppState) handleAPITicketsCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	uid, err := client.Authenticate(ctx)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	targetUID := uid
	if session != nil && session.UserEmail != "" {
		if resUID, rErr := client.ResolveUserUIDByEmail(ctx, session.UserEmail); rErr == nil && resUID > 0 {
			targetUID = resUID
		}
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		PartnerID   int    `json:"partner_id"`
		ProjectID   int    `json:"project_id"`
		TaskID      int    `json:"task_id"`
		Priority    string `json:"priority"`
		UserID      int    `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "El asunto/título del ticket es obligatorio"})
		return
	}

	assignedUID := targetUID
	if req.UserID > 0 {
		assignedUID = req.UserID
	}

	vals := map[string]interface{}{
		"name":    strings.TrimSpace(req.Name),
		"user_id": assignedUID,
	}
	if strings.TrimSpace(req.Description) != "" {
		vals["description"] = strings.TrimSpace(req.Description)
	}
	if req.PartnerID > 0 {
		vals["partner_id"] = req.PartnerID
	}
	if req.ProjectID > 0 {
		vals["project_id"] = req.ProjectID
	}
	if req.TaskID > 0 {
		vals["task_id"] = req.TaskID
	}
	if req.Priority != "" {
		vals["priority"] = req.Priority
	}

	ticketID, err := client.CreateTicket(ctx, vals)
	if err != nil {
		if req.TaskID > 0 {
			delete(vals, "task_id")
			ticketID, err = client.CreateTicket(ctx, vals)
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al crear ticket en Odoo: " + err.Error()})
			return
		}
	}

	client.InvalidateTicketsCache()
	state.broadcastUserEvent(assignedUID, "tickets_changed", map[string]interface{}{
		"action":    "ticket_created",
		"ticket_id": ticketID,
	})

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"ticket_id": ticketID,
		"message":   "Ticket creado correctamente",
	})
}

// handleAPITicketsUpdate modifica un ticket existente en Odoo (helpdesk.ticket)
func (state *AppState) handleAPITicketsUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var req struct {
		ID          int         `json:"id"`
		Name        *string     `json:"name"`
		Description *string     `json:"description"`
		PartnerID   interface{} `json:"partner_id"`
		ProjectID   interface{} `json:"project_id"`
		TaskID      interface{} `json:"task_id"`
		Priority    *string     `json:"priority"`
		UserID      interface{} `json:"user_id"`
		StageID     interface{} `json:"stage_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	if req.ID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "ID de ticket obligatorio"})
		return
	}

	vals := make(map[string]interface{})
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		vals["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		vals["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Priority != nil {
		vals["priority"] = *req.Priority
	}
	if req.PartnerID != nil {
		vals["partner_id"] = req.PartnerID
	}
	if req.ProjectID != nil {
		vals["project_id"] = req.ProjectID
	}
	if req.TaskID != nil {
		vals["task_id"] = req.TaskID
	}
	if req.UserID != nil {
		vals["user_id"] = req.UserID
	}
	if req.StageID != nil {
		vals["stage_id"] = req.StageID
	}

	if len(vals) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "No se indicaron campos para modificar"})
		return
	}

	if err := client.UpdateTicket(ctx, req.ID, vals); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar ticket en Odoo: " + err.Error()})
		return
	}

	client.InvalidateTicketsCache()
	state.broadcastUserEvent(0, "tickets_changed", map[string]interface{}{
		"action":    "ticket_updated",
		"ticket_id": req.ID,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Ticket actualizado correctamente",
	})
}

// handleAPIUsers devuelve la lista de usuarios activos de Odoo para asignación
func (state *AppState) handleAPIUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	users, err := client.GetAssignableUsers(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(users)
}

// handleAPITicketsClose da por cerrado definitivamente un ticket en Odoo
func (state *AppState) handleAPITicketsClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	session, _ := state.resolveAntigravitySession(r, "", "")
	if session == nil {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			session, _ = decodeSession(cookie.Value)
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	uid, err := client.Authenticate(ctx)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	targetUID := uid
	if session != nil && session.UserEmail != "" {
		if resUID, rErr := client.ResolveUserUIDByEmail(ctx, session.UserEmail); rErr == nil && resUID > 0 {
			targetUID = resUID
		}
	}

	var req struct {
		TicketID    int    `json:"ticket_id"`
		TicketRef   string `json:"ticket_ref"`
		Subject     string `json:"subject"`
		Description string `json:"description"`
		SendReport  bool   `json:"send_report"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	targetTicketID := req.TicketID
	if targetTicketID <= 0 && req.TicketRef != "" {
		if t, tErr := client.GetTicketByRefOrID(ctx, req.TicketRef); tErr == nil && t != nil {
			targetTicketID = t.ID
		}
	}

	if targetTicketID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Debe especificar un ID o código de ticket válido"})
		return
	}

	sessionUserEmail := ""
	if session != nil && session.UserEmail != "" {
		sessionUserEmail = session.UserEmail
	}

	resultMsg, err := client.CloseTicket(ctx, targetTicketID, req.Subject, req.Description, req.SendReport, sessionUserEmail)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	client.InvalidateTicketsCache()
	state.broadcastUserEvent(targetUID, "tickets_changed", map[string]interface{}{
		"action":      "ticket_closed",
		"ticket_id":   targetTicketID,
		"send_report": req.SendReport,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"ticket_id": targetTicketID,
		"message":   resultMsg,
	})
}

// handleHealth retorna el estado del servidor
func (state *AppState) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": Version,
		"port":    state.cfg.Server.Port,
	})
}

// handlePing retorna un ping simple
func (state *AppState) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("pong\n"))
}

// handleAPITimerActive consulta si el usuario tiene una imputación o tarea activa en Odoo
func (state *AppState) handleAPITimerActive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		json.NewEncoder(w).Encode(map[string]interface{}{"active": nil})
		return
	}

	client := odoo.NewClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	cur := state.getActiveTimer(userUID)

	// Consultar el estado real en Odoo
	odooTimer, err := client.GetActiveTimer(ctx, userUID)
	if err == nil && odooTimer != nil && odooTimer.IsRunning {
		if cur == nil {
			// No teníamos temporizador en memoria local: adoptar el de Odoo
			state.setActiveTimer(userUID, odooTimer)
			cur = odooTimer
		} else if cur.TimesheetID != odooTimer.TimesheetID || cur.TaskID != odooTimer.TaskID {
			// El usuario cambió de tarea/imputación en Odoo o desde otro cliente: actualizar
			state.setActiveTimer(userUID, odooTimer)
			cur = odooTimer
		} else if !cur.IsRunning {
			// En memoria estaba pausado pero en Odoo se reanudó: reanudar
			state.resumeActiveTimer(userUID)
			cur = state.getActiveTimer(userUID)
		}
		// Si coincide con cur y cur.IsRunning, se conserva cur en memoria (StartedAt y AccumulatedMs exactos)
	}

	timersList := state.getActiveTimers(userUID)
	serverNowMs := time.Now().UnixMilli()
	if cur == nil && len(timersList) == 0 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":      nil,
			"active_list": []interface{}{},
			"server_time": serverNowMs,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"active":            cur,
		"active_list":       timersList,
		"server_time":       serverNowMs,
		"last_confirmed_at": state.getLastConfirmedAt(userUID),
	})
}

// handleAPITimerStart inicia un trabajo en Odoo (action_timer_start / is_timer_running=true)
func (state *AppState) handleAPITimerStart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión de Odoo no configurada"})
		return
	}

	var req struct {
		ProjectID   int     `json:"project_id"`
		ProjectName string  `json:"project_name"`
		TaskID      int     `json:"task_id"`
		TaskName    string  `json:"task_name"`
		TicketID    int     `json:"ticket_id"`
		TicketRef   string  `json:"ticket_ref"`
		TicketName  string  `json:"ticket_name"`
		TimesheetID int     `json:"timesheet_id"`
		Description string  `json:"description"`
		UnitAmount  float64 `json:"unit_amount"`
		Date        string  `json:"date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	// Si se especifica ticket, validar y completar automáticamente proyecto, tarea y descripción
	if req.TicketID > 0 || req.TicketRef != "" {
		refOrID := req.TicketRef
		if refOrID == "" {
			refOrID = strconv.Itoa(req.TicketID)
		}
		if t, tErr := client.GetTicketByRefOrID(ctx, refOrID); tErr == nil && t != nil {
			if t.IsClosed() {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "El ticket está cerrado. Un ticket cerrado no se puede volver a abrir ni imputar tiempos."})
				return
			}
			req.TicketID = t.ID
			if req.TicketRef == "" {
				req.TicketRef = t.TicketRef
			}
			if req.TicketName == "" {
				req.TicketName = t.Name
			}
			if req.ProjectID <= 0 && t.ProjectID.ID > 0 {
				req.ProjectID = t.ProjectID.ID
				req.ProjectName = t.ProjectID.Name
			}
			if req.TaskID <= 0 && t.TaskID.ID > 0 {
				req.TaskID = t.TaskID.ID
				req.TaskName = t.TaskID.Name
			}
			if req.Description == "" {
				req.Description = fmt.Sprintf("[%s] %s", t.TicketRef, t.Name)
			}
		}
	}

	// Creación o búsqueda dinámica de tarea en Odoo si no se especificó task_id pero sí task_name
	if req.ProjectID > 0 && req.TaskID <= 0 && req.TaskName != "" {
		if tasks, err := client.GetTasks(ctx, req.ProjectID, 0); err == nil {
			for _, t := range tasks {
				if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(req.TaskName)) {
					req.TaskID = t.ID
					break
				}
			}
		}
		if req.TaskID <= 0 {
			if newID, err := client.CreateTask(ctx, req.ProjectID, req.TaskName, userUID); err == nil && newID > 0 {
				req.TaskID = newID
				log.Printf("[PlanesGo] Tarea '%s' creada dinámicamente con ID %d para proyecto %d", req.TaskName, newID, req.ProjectID)
			}
		}
	}

	activeTimer, err := client.StartTimer(ctx, req.ProjectID, req.ProjectName, req.TaskID, req.TaskName, req.TimesheetID, req.Description, req.UnitAmount, req.Date)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar en Odoo: " + err.Error()})
		return
	}

	nowMs := time.Now().UnixMilli()
	state.setLastConfirmedAt(userUID, nowMs)

	if activeTimer != nil {
		if req.TicketID > 0 {
			activeTimer.TicketID = req.TicketID
			activeTimer.TicketRef = req.TicketRef
			activeTimer.TicketName = req.TicketName
			if activeTimer.TimesheetID > 0 {
				_ = client.UpdateTimesheetWithTicket(ctx, activeTimer.TimesheetID, "", req.TaskID, req.TicketID, 0, "")
			}
		}
		if activeTimer.EmployeeName == "" {
			if session != nil && session.UserName != "" {
				activeTimer.EmployeeName = session.UserName
			} else if odooCfg.Username != "" {
				activeTimer.EmployeeName = odooCfg.Username
			}
		}
		state.setActiveTimer(userUID, activeTimer)
		state.broadcastUserEvent(userUID, "timer_start", activeTimer)
		state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
			"action":       "timer_start",
			"timesheet_id": activeTimer.TimesheetID,
			"project_id":   activeTimer.ProjectID,
			"task_id":      activeTimer.TaskID,
			"ticket_id":    activeTimer.TicketID,
		})
	}

	json.NewEncoder(w).Encode(activeTimer)
}

// handleAPITimerTick sincroniza en segundo plano las horas acumuladas en Odoo sin pausar el cronómetro
func (state *AppState) handleAPITimerTick(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión no configurada"})
		return
	}

	var req struct {
		TimesheetID int     `json:"timesheet_id"`
		UnitAmount  float64 `json:"unit_amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	if req.TimesheetID > 0 && req.UnitAmount > 0 {
		client := odoo.GetClient(odooCfg)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		_ = client.UpdateTimerUnits(ctx, req.TimesheetID, req.UnitAmount)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPITimerConfirm sincroniza la confirmación de actividad del temporizador entre múltiples dispositivos/navegadores
func (state *AppState) handleAPITimerConfirm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión de Odoo no configurada"})
		return
	}

	var req struct {
		TimesheetID int     `json:"timesheet_id"`
		Description string  `json:"description"`
		UnitAmount  float64 `json:"unit_amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	nowMs := time.Now().UnixMilli()
	state.setLastConfirmedAt(userUID, nowMs)

	if req.TimesheetID > 0 {
		if req.UnitAmount > 0 {
			_ = client.UpdateTimerUnits(ctx, req.TimesheetID, req.UnitAmount)
		}
		if strings.TrimSpace(req.Description) != "" {
			_ = client.UpdateTimesheetDescription(ctx, req.TimesheetID, strings.TrimSpace(req.Description))
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":           true,
		"last_confirmed_at": nowMs,
	})
}

// handleAPITimerPause pausa el trabajo activo en Odoo
func (state *AppState) handleAPITimerPause(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión de Odoo no configurada"})
		return
	}

	var req struct {
		TimesheetID int     `json:"timesheet_id"`
		TaskID      int     `json:"task_id"`
		UnitAmount  float64 `json:"unit_amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := client.PauseTimer(ctx, req.TimesheetID, req.TaskID, req.UnitAmount); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}
	state.pauseActiveTimerForTask(userUID, req.TaskID, req.TimesheetID, req.UnitAmount)
	state.broadcastUserEvent(userUID, "timer_pause", map[string]interface{}{
		"timesheet_id": req.TimesheetID,
		"task_id":      req.TaskID,
		"unit_amount":  req.UnitAmount,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPITimerResume reanuda el trabajo en Odoo
func (state *AppState) handleAPITimerResume(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión de Odoo no configurada"})
		return
	}

	var req struct {
		TimesheetID int `json:"timesheet_id"`
		TaskID      int `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := client.ResumeTimer(ctx, req.TimesheetID, req.TaskID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}
	nowMs := time.Now().UnixMilli()
	state.setLastConfirmedAt(userUID, nowMs)
	state.resumeActiveTimerWithTimesheet(userUID, req.TimesheetID, req.TaskID)
	state.broadcastUserEvent(userUID, "timer_resume", map[string]interface{}{
		"timesheet_id": req.TimesheetID,
		"task_id":      req.TaskID,
	})
	state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
		"action":       "timer_resume",
		"timesheet_id": req.TimesheetID,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPITimerStop detiene y finaliza el trabajo en Odoo
func (state *AppState) handleAPITimerStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión de Odoo no configurada"})
		return
	}

	var req struct {
		TimesheetID int     `json:"timesheet_id"`
		TaskID      int     `json:"task_id"`
		TicketID    int     `json:"ticket_id"`
		UnitAmount  float64 `json:"unit_amount"`
		Description string  `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	if req.TicketID <= 0 {
		if curTimer := state.getActiveTimer(userUID); curTimer != nil && curTimer.TicketID > 0 {
			req.TicketID = curTimer.TicketID
		}
	}

	if err := client.StopTimer(ctx, req.TimesheetID, req.TaskID, req.UnitAmount, req.Description); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if req.TicketID > 0 && req.TimesheetID > 0 {
		_ = client.UpdateTimesheetWithTicket(ctx, req.TimesheetID, "", req.TaskID, req.TicketID, 0, "")
	}
	if req.TimesheetID > 0 || req.TaskID > 0 {
		state.clearActiveTimerForTask(userUID, req.TaskID, req.TimesheetID)
	} else {
		state.clearActiveTimer(userUID)
	}

	state.broadcastUserEvent(userUID, "timer_stop", map[string]interface{}{
		"timesheet_id": req.TimesheetID,
		"task_id":      req.TaskID,
		"unit_amount":  req.UnitAmount,
	})
	state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
		"action":       "timer_stop",
		"timesheet_id": req.TimesheetID,
		"unit_amount":  req.UnitAmount,
	})

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPITimerHeartbeat procesa los latidos continuos de actividad (heartbeats) provenientes
// de Antigravity o procesos locales, sumando tiempo en vivo, resolviendo o creando tareas
// dinámicamente en Odoo y sincronizando las unidades acumuladas.
func (state *AppState) handleAPITimerHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ProjectID       int     `json:"project_id"`
		ProjectName     string  `json:"project_name"`
		TaskID          int     `json:"task_id"`
		TaskName        string  `json:"task_name"`
		TimesheetID     int     `json:"timesheet_id"`
		Description     string  `json:"description"`
		Source          string  `json:"source"`
		ElapsedSeconds  int     `json:"elapsed_seconds"`
		ClientTimestamp int64   `json:"client_timestamp"`
		UserEmail       string  `json:"user_email,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	// Permitir llamadas sin cookie (p.ej. scripts CLI o Antigravity local) usando req.UserEmail o cabecera
	if session == nil {
		email := strings.TrimSpace(req.UserEmail)
		if email == "" {
			email = strings.TrimSpace(r.Header.Get("X-User-Email"))
		}
		if email == "" && state.userStore != nil {
			for _, u := range state.userStore.GetAllSettings() {
				if u.Email != "" && u.OdooToken != "" {
					email = u.Email
					break
				}
			}
		}
		if email != "" && state.userStore != nil {
			if uSettings, ok := state.userStore.GetSettings(email); ok {
				session = &SessionData{
					URL:        uSettings.OdooURL,
					DB:         uSettings.OdooDB,
					Username:   uSettings.OdooUser,
					Password:   uSettings.OdooToken,
					UserEmail:  uSettings.Email,
					AuthMethod: "cli",
				}
			}
		}
	}

	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Sesión o credenciales de Odoo no disponibles"})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userEmail := ""
	if session != nil {
		userEmail = session.UserEmail
		if userEmail == "" {
			userEmail = session.Username
		}
	}
	userUID := 0
	if userEmail != "" {
		userUID, _ = client.ResolveUserUIDByEmail(ctx, userEmail)
	}
	if userUID == 0 {
		userUID = client.UID()
	}

	// 1. Creación o búsqueda dinámica de tarea en Odoo
	if req.ProjectID > 0 && req.TaskID <= 0 && req.TaskName != "" {
		tasks, err := client.GetTasks(ctx, req.ProjectID, 0)
		if err == nil {
			for _, t := range tasks {
				if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(req.TaskName)) {
					req.TaskID = t.ID
					break
				}
			}
		}
		if req.TaskID <= 0 {
			newID, createErr := client.CreateTask(ctx, req.ProjectID, req.TaskName, userUID)
			if createErr == nil && newID > 0 {
				req.TaskID = newID
				log.Printf("[PlanesGo Heartbeat] Tarea creada dinámicamente en Odoo: ID %d ('%s') en proyecto %d", newID, req.TaskName, req.ProjectID)
			} else {
				log.Printf("[PlanesGo Heartbeat] Advertencia al crear tarea dinámica en Odoo: %v", createErr)
			}
		}
	}

	nowMs := time.Now().UnixMilli()
	state.setLastConfirmedAt(userUID, nowMs)

	// Clave identificadora del temporizador
	timerKey := ""
	if req.TaskID > 0 {
		timerKey = fmt.Sprintf("task_%d", req.TaskID)
	} else if req.TimesheetID > 0 {
		timerKey = fmt.Sprintf("ts_%d", req.TimesheetID)
	} else if req.ProjectID > 0 {
		timerKey = fmt.Sprintf("proj_%d", req.ProjectID)
	} else {
		timerKey = "default"
	}

	cur := state.getActiveTimerByKey(userUID, timerKey)
	if cur == nil {
		// Iniciar nuevo temporizador
		source := req.Source
		if source == "" {
			source = "antigravity"
		}
		desc := req.Description
		if desc == "" {
			if req.TaskName != "" {
				desc = fmt.Sprintf("[%s] %s", strings.ToUpper(source), req.TaskName)
			} else {
				desc = fmt.Sprintf("[%s] Trabajo en curso", strings.ToUpper(source))
			}
		}

		activeTimer, startErr := client.StartTimer(ctx, req.ProjectID, req.ProjectName, req.TaskID, req.TaskName, req.TimesheetID, desc, 0, "")
		if startErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar en Odoo: " + startErr.Error()})
			return
		}
		activeTimer.TimerKey = timerKey
		activeTimer.Source = source
		activeTimer.LastHeartbeat = nowMs
		if activeTimer.EmployeeName == "" && session != nil && session.UserName != "" {
			activeTimer.EmployeeName = session.UserName
		}
		state.setActiveTimer(userUID, activeTimer)
		state.broadcastUserEvent(userUID, "timer_start", activeTimer)
		state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
			"action":       "timer_start",
			"timesheet_id": activeTimer.TimesheetID,
			"project_id":   activeTimer.ProjectID,
			"task_id":      activeTimer.TaskID,
		})
		cur = activeTimer
	} else {
		// Temporizador existente: registrar latido y acumular tiempo
		deltaMs := int64(0)
		if req.ElapsedSeconds > 0 {
			deltaMs = int64(req.ElapsedSeconds * 1000)
		} else if cur.LastHeartbeat > 0 {
			elapsed := nowMs - cur.LastHeartbeat
			if elapsed > 0 && elapsed <= 15*60*1000 {
				deltaMs = elapsed
			}
		}
		cur.AccumulatedMs += deltaMs
		cur.LastHeartbeat = nowMs
		cur.IsRunning = true
		cur.UnitAmount = float64(cur.AccumulatedMs) / (3600 * 1000)
		if cur.UnitAmount > 24.0 {
			log.Printf("[API Beat] ADVERTENCIA: UnitAmount excesivo (%f h), limitando a 24h", cur.UnitAmount)
			cur.UnitAmount = 24.0
			cur.AccumulatedMs = 24 * 3600 * 1000
		}
		if req.Description != "" {
			cur.Description = req.Description
		}
		state.setActiveTimer(userUID, cur)

		// Actualizar unidades acumuladas en Odoo de forma asíncrona
		if cur.TimesheetID > 0 {
			go func(tID int, units float64) {
				updateCtx, cancelUpdate := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancelUpdate()
				_ = client.UpdateTimerUnits(updateCtx, tID, units)
			}(cur.TimesheetID, cur.UnitAmount)
		}

		state.broadcastUserEvent(userUID, "timer_tick", cur)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"timer":       cur,
		"server_time": nowMs,
		"message":     "Latido registrado correctamente",
	})
}

// handleAPIVersion devuelve la versión actual de PlanesGo para auto-recarga PWA
func (state *AppState) handleAPIVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	json.NewEncoder(w).Encode(map[string]string{
		"version": Version,
	})
}

// handleManifest sirve el manifiesto PWA con el tipo MIME correcto
func (state *AppState) handleManifest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, "static/manifest.json")
}

// handleServiceWorker sirve sw.js permitiendo scope raíz "/"
func (state *AppState) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	http.ServeFile(w, r, "static/sw.js")
}

// handleAPIPartnerAvatar sirve y cachea el logotipo auténtico del partner desde Odoo
func (state *AppState) handleAPIPartnerAvatar(w http.ResponseWriter, r *http.Request) {
	partnerID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	projectID, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("project_id")))

	// Si no vino partnerID directo pero vino projectID, resolver desde caché en memoria
	if partnerID <= 0 && projectID > 0 {
		projectPartnerMu.RLock()
		partnerID = projectPartnerCache[projectID]
		projectPartnerMu.RUnlock()
	}

	if partnerID <= 0 && projectID <= 0 {
		http.Error(w, "ID no válido", http.StatusBadRequest)
		return
	}

	now := time.Now()

	// 1. Comprobar caché en memoria si ya tenemos partnerID
	if partnerID > 0 {
		partnerAvatarCacheMu.RLock()
		cached, found := partnerAvatarCache[partnerID]
		partnerAvatarCacheMu.RUnlock()

		if found && now.Before(cached.expiresAt) {
			if len(cached.data) == 0 {
				// Negativo en caché: el partner no tiene logotipo. Devolver 404 para no mostrar nada.
				w.Header().Set("Cache-Control", "public, max-age=30")
				http.Error(w, "Sin imagen", http.StatusNotFound)
				return
			}
			// Validar ETag del cliente
			if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && ifNoneMatch == cached.etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", cached.contentType)
			w.Header().Set("ETag", cached.etag)
			w.Header().Set("Cache-Control", "public, max-age=600")
			w.Write(cached.data)
			return
		}
	}

	// 2. Resolver configuración de Odoo autenticada
	var session *SessionData
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}
	odooCfg := state.resolveUserOdooConfig(session)
	if odooCfg.Password == "" || odooCfg.DB == "" {
		http.Error(w, "No autenticado", http.StatusUnauthorized)
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Si aún no tenemos partnerID pero tenemos projectID, consultar Odoo para el partner_id de este proyecto
	if partnerID <= 0 && projectID > 0 {
		resolvedPartID, pErr := client.GetProjectPartnerID(ctx, projectID)
		if pErr == nil && resolvedPartID > 0 {
			partnerID = resolvedPartID
			projectPartnerMu.Lock()
			projectPartnerCache[projectID] = partnerID
			projectPartnerMu.Unlock()
		}
	}

	if partnerID <= 0 {
		w.Header().Set("Cache-Control", "public, max-age=30")
		http.Error(w, "Sin imagen", http.StatusNotFound)
		return
	}

	// Volver a chequear caché de partnerID por si se resolvió justo ahora
	partnerAvatarCacheMu.RLock()
	cached, found := partnerAvatarCache[partnerID]
	partnerAvatarCacheMu.RUnlock()
	if found && now.Before(cached.expiresAt) {
		if len(cached.data) == 0 {
			w.Header().Set("Cache-Control", "public, max-age=30")
			http.Error(w, "Sin imagen", http.StatusNotFound)
			return
		}
		if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && ifNoneMatch == cached.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", cached.contentType)
		w.Header().Set("ETag", cached.etag)
		w.Header().Set("Cache-Control", "public, max-age=600")
		w.Write(cached.data)
		return
	}

	imgData, cType, imgErr := client.GetPartnerAvatar(ctx, partnerID)
	if imgErr != nil || len(imgData) == 0 {
		// Guardar negativo breve (30 segundos) en caché para no saturar Odoo y permitir ver cambios rápidos
		partnerAvatarCacheMu.Lock()
		partnerAvatarCache[partnerID] = avatarCacheItem{
			expiresAt: now.Add(30 * time.Second),
		}
		partnerAvatarCacheMu.Unlock()

		w.Header().Set("Cache-Control", "public, max-age=30")
		http.Error(w, "Sin imagen", http.StatusNotFound)
		return
	}

	etag := fmt.Sprintf(`"%x"`, md5.Sum(imgData))

	// Almacenar en caché en memoria por 10 minutos (permite actualizar logotipos sin demora excesiva)
	partnerAvatarCacheMu.Lock()
	partnerAvatarCache[partnerID] = avatarCacheItem{
		data:        imgData,
		contentType: cType,
		etag:        etag,
		expiresAt:   now.Add(10 * time.Minute),
	}
	partnerAvatarCacheMu.Unlock()

	w.Header().Set("Content-Type", cType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.Write(imgData)
}

