package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
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

		// Enriquecer imputaciones con PartnerID si viene vacío
		projectPartnerMu.RLock()
		for i := range entries {
			if entries[i].PartnerID.ID == 0 && entries[i].ProjectID.ID > 0 {
				if partID, ok := projectPartnerCache[entries[i].ProjectID.ID]; ok && partID > 0 {
					entries[i].PartnerID = odoo.Many2One{ID: partID}
				}
			}
		}
		projectPartnerMu.RUnlock()

		json.NewEncoder(w).Encode(entries)

	case http.MethodPost:
		var req struct {
			Date        string  `json:"date"`
			ProjectID   int     `json:"project_id"`
			TaskID      int     `json:"task_id"`
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

		newID, err := client.CreateTimesheet(ctx, req.Date, req.ProjectID, req.TaskID, req.UnitAmount, req.Description)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al registrar en Odoo: " + err.Error()})
			return
		}

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

	if err := client.UpdateTimesheet(ctx, req.ID, req.Date, req.TaskID, req.UnitAmount, req.Description); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al actualizar parte en Odoo: " + err.Error()})
		return
	}

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

// handleAPITickets consulta los tickets de soporte pendientes asignados al usuario en Odoo
func (state *AppState) handleAPITickets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	var session *SessionData
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
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

	targetUID := uid
	if session != nil && session.UserEmail != "" {
		if resUID, rErr := client.ResolveUserUIDByEmail(ctx, session.UserEmail); rErr == nil && resUID > 0 {
			targetUID = resUID
		}
	}

	tickets, err := client.GetPendingTickets(ctx, targetUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(tickets)
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

	timer, err := client.GetActiveTimer(ctx, userUID)
	if err == nil && timer != nil && timer.IsRunning {
		state.setActiveTimer(userUID, timer)
	} else {
		timer = state.getActiveTimer(userUID)
	}

	if timer == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"active": nil})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"active":            timer,
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

	activeTimer, err := client.StartTimer(ctx, req.ProjectID, req.ProjectName, req.TaskID, req.TaskName, req.TimesheetID, req.Description, req.UnitAmount, req.Date)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar en Odoo: " + err.Error()})
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

	if activeTimer != nil {
		if activeTimer.EmployeeName == "" {
			if session != nil && session.UserName != "" {
				activeTimer.EmployeeName = session.UserName
			} else if odooCfg.Username != "" {
				activeTimer.EmployeeName = odooCfg.Username
			}
		}
		state.setActiveTimer(userUID, activeTimer)
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
	state.pauseActiveTimer(userUID)

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

	if err := client.StopTimer(ctx, req.TimesheetID, req.TaskID, req.UnitAmount, req.Description); err != nil {
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
	state.clearActiveTimer(userUID)

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
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

// handleAPIPartnerAvatar sirve y cachea agresivamente el logotipo del partner de Odoo
func (state *AppState) handleAPIPartnerAvatar(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimSpace(r.URL.Query().Get("id"))
	partnerID, err := strconv.Atoi(idStr)
	if err != nil || partnerID <= 0 {
		http.Error(w, "ID de partner no válido", http.StatusBadRequest)
		return
	}

	// 1. Comprobar caché en memoria
	partnerAvatarCacheMu.RLock()
	cached, found := partnerAvatarCache[partnerID]
	partnerAvatarCacheMu.RUnlock()

	now := time.Now()
	if found && now.Before(cached.expiresAt) {
		// Validar ETag del cliente
		if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != "" && ifNoneMatch == cached.etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", cached.contentType)
		w.Header().Set("ETag", cached.etag)
		w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
		w.Write(cached.data)
		return
	}

	// 2. Resolver URL base de Odoo
	var session *SessionData
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}
	odooCfg := state.resolveUserOdooConfig(session)
	baseURL := strings.TrimRight(odooCfg.URL, "/")
	if baseURL == "" {
		baseURL = DefaultOdooURL
	}

	targetURL := fmt.Sprintf("%s/web/image?model=res.partner&id=%d&field=image_128", baseURL, partnerID)
	req, reqErr := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if reqErr != nil {
		http.Error(w, "Error creando petición", http.StatusInternalServerError)
		return
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, respErr := client.Do(req)
	if respErr != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		// Servir fallback SVG neutral y guardar negativo breve
		svgPlaceholder := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#94a3b8" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M3.75 21h16.5M4.5 3h15M5.25 3v18m13.5-18v18M9 6.75h1.5m-1.5 3h1.5m-1.5 3h1.5m3-6H15m-1.5 3H15m-1.5 3H15M9 21v-3.375c0-.621.504-1.125 1.125-1.125h3.75c.621 0 1.125.504 1.125 1.125V21" /></svg>`)
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(svgPlaceholder)
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil || len(bodyBytes) == 0 {
		http.Error(w, "Error leyendo imagen", http.StatusInternalServerError)
		return
	}

	cType := resp.Header.Get("Content-Type")
	if cType == "" || strings.Contains(cType, "text/html") {
		cType = "image/png"
	}
	etag := fmt.Sprintf(`"%x"`, md5.Sum(bodyBytes))

	// Almacenar en caché en memoria por 24 horas
	item := avatarCacheItem{
		data:        bodyBytes,
		contentType: cType,
		etag:        etag,
		expiresAt:   now.Add(24 * time.Hour),
	}
	partnerAvatarCacheMu.Lock()
	partnerAvatarCache[partnerID] = item
	partnerAvatarCacheMu.Unlock()

	w.Header().Set("Content-Type", cType)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400, stale-while-revalidate=604800")
	w.Write(bodyBytes)
}

