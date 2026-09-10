package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pasigo/odoo"
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
		entries, err := client.GetTimesheets(ctx, nil)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
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

	client := odoo.NewClient(odooCfg)
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
		projectIDStr := r.URL.Query().Get("project_id")
		projectID, _ := strconv.Atoi(projectIDStr)

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
