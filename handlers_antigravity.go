package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"pasigo/odoo"
)

// AntigravityTaskPayload representa los datos enviados por Antigravity para imputación y latidos.
type AntigravityTaskPayload struct {
	ProjectID       int     `json:"project_id"`
	ProjectName     string  `json:"project_name"`
	TaskID          int     `json:"task_id"`
	TaskName        string  `json:"task_name"`
	TimesheetID     int     `json:"timesheet_id"`
	Description     string  `json:"description"`
	Action          string  `json:"action"` // "heartbeat" (defecto), "start", "stop", "pause"
	ElapsedSeconds  int     `json:"elapsed_seconds"`
	UnitAmount      float64 `json:"unit_amount"`
	ClientTimestamp int64   `json:"client_timestamp"`
	Token           string  `json:"token,omitempty"`
	UserEmail       string  `json:"user_email,omitempty"`
}

// resolveAntigravitySession autentica la petición de Antigravity utilizando el token de seguridad
// por empleado (cabecera X-Antigravity-Token, Authorization: Bearer, o token en JSON/query).
func (state *AppState) resolveAntigravitySession(r *http.Request, explicitToken string, explicitEmail string) (*SessionData, error) {
	token := strings.TrimSpace(explicitToken)
	if token == "" {
		token = strings.TrimSpace(r.Header.Get("X-Antigravity-Token"))
	}
	if token == "" {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			token = strings.TrimSpace(authHeader[7:])
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	// 1. Autenticación directa por AntigravityToken
	if token != "" && state.userStore != nil {
		if userSetting, ok := state.userStore.GetUserByAntigravityToken(token); ok {
			return &SessionData{
				URL:        userSetting.OdooURL,
				DB:         userSetting.OdooDB,
				Username:   userSetting.OdooUser,
				Password:   userSetting.OdooToken,
				UserEmail:  userSetting.Email,
				AuthMethod: "antigravity_token",
			}, nil
		}
		return nil, fmt.Errorf("token de seguridad de Antigravity inválido o desconocido")
	}

	// 2. Cookie de sesión web si está disponible
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		if sess, err := decodeSession(cookie.Value); err == nil && sess != nil {
			return sess, nil
		}
	}

	// 3. Fallback con email explícito si el store tiene usuario configurado
	email := strings.TrimSpace(explicitEmail)
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
		if uSetting, ok := state.userStore.GetSettings(email); ok {
			return &SessionData{
				URL:        uSetting.OdooURL,
				DB:         uSetting.OdooDB,
				Username:   uSetting.OdooUser,
				Password:   uSetting.OdooToken,
				UserEmail:  uSetting.Email,
				AuthMethod: "cli_email_fallback",
			}, nil
		}
	}

	return nil, fmt.Errorf("autenticación requerida: proporcione X-Antigravity-Token válido")
}

// handleAntigravity es el punto de entrada unificado para el controlador /antigravity
func (state *AppState) handleAntigravity(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		state.handleAntigravityStatus(w, r)
	case http.MethodPost:
		state.handleAntigravityUpdateTasks(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
	}
}

// projectNamesMatch compara dos nombres de proyecto de forma insensible a mayúsculas/minúsculas
// y tolerante a formatos compactos sin espacios/guiones (ej. PlanesSecurityAlert vs PLANES SECURITY ALERT).
func projectNamesMatch(name1, name2 string) bool {
	n1 := strings.ToUpper(strings.TrimSpace(name1))
	n2 := strings.ToUpper(strings.TrimSpace(name2))
	if n1 == "" || n2 == "" {
		return false
	}
	if n1 == n2 {
		return true
	}
	compact := func(s string) string {
		var sb strings.Builder
		for _, r := range s {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				sb.WriteRune(r)
			}
		}
		return sb.String()
	}
	c1 := compact(n1)
	c2 := compact(n2)
	return c1 != "" && c1 == c2
}

// cleanAntigravityTaskName extrae el nombre legible de una tarea sin el prefijo técnico [AGY] o [ANTIGRAVITY]
func cleanAntigravityTaskName(taskName string) string {
	name := strings.TrimSpace(taskName)
	if strings.HasPrefix(strings.ToUpper(name), "[AGY]") {
		return strings.TrimSpace(name[5:])
	}
	if strings.HasPrefix(strings.ToUpper(name), "[ANTIGRAVITY]") {
		return strings.TrimSpace(name[14:])
	}
	return name
}

// handleAntigravityStatus implementa el Handshake y verificación previa obligatoria (Fase 0 de PSF).
// Permite a Antigravity comprobar que PlanesGo está activo y accesible antes de tocar código.
func (state *AppState) handleAntigravityStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	sess, err := state.resolveAntigravitySession(r, "", "")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   false,
			"error":   err.Error(),
			"hint":    "Configure su X-Antigravity-Token en las cabeceras o en ~/.planesgo_auth.json",
			"version": Version,
		})
		return
	}

	odooCfg := state.resolveUserOdooConfig(sess)
	client := odoo.GetClient(odooCfg)

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	userEmail := sess.UserEmail
	if userEmail == "" {
		userEmail = sess.Username
	}
	userUID, _ := client.ResolveUserUIDByEmail(ctx, userEmail)
	if userUID == 0 {
		userUID = client.UID()
	}

	// Comprobación estricta (case-insensitive) del nombre de proyecto si se suministra en la petición
	reqProjectName := strings.TrimSpace(r.URL.Query().Get("project_name"))
	reqProjectIDStr := strings.TrimSpace(r.URL.Query().Get("project_id"))
	reqProjectID := 0
	if reqProjectIDStr != "" {
		reqProjectID, _ = strconv.Atoi(reqProjectIDStr)
	}

	var matchedProject *odoo.Project
	if reqProjectName != "" || reqProjectID > 0 {
		projects, pErr := client.GetProjects(ctx, nil)
		if pErr != nil {
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ready":   false,
				"error":   "Error al consultar proyectos en Odoo: " + pErr.Error(),
				"version": Version,
			})
			return
		}

		if reqProjectID > 0 {
			for i := range projects {
				if projects[i].ID == reqProjectID {
					matchedProject = &projects[i]
					break
				}
			}
			if matchedProject == nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ready":   false,
					"error":   fmt.Sprintf("El proyecto con ID %d no existe en PlanesGo / Odoo", reqProjectID),
					"version": Version,
				})
				return
			}
			if reqProjectName != "" && !projectNamesMatch(reqProjectName, matchedProject.Name) {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ready":   false,
					"error":   fmt.Sprintf("El nombre del proyecto en Antigravity ('%s') no coincide con el registrado en PlanesGo/Odoo ('%s')", reqProjectName, matchedProject.Name),
					"version": Version,
				})
				return
			}
		} else if reqProjectName != "" {
			for i := range projects {
				if projectNamesMatch(reqProjectName, projects[i].Name) {
					matchedProject = &projects[i]
					break
				}
			}
			if matchedProject == nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"ready":   false,
					"error":   fmt.Sprintf("El proyecto '%s' no existe en PlanesGo / Odoo", reqProjectName),
					"version": Version,
				})
				return
			}
		}
	}

	activeTimers := state.getActiveTimers(userUID)

	resp := map[string]interface{}{
		"ready":          true,
		"service":        "PlanesGo Antigravity Gateway",
		"version":        Version,
		"user_email":     userEmail,
		"user_uid":       userUID,
		"odoo_connected": true,
		"odoo_db":        odooCfg.DB,
		"odoo_url":       odooCfg.URL,
		"active_timers":  activeTimers,
		"server_time":    time.Now().UnixMilli(),
	}
	if matchedProject != nil {
		resp["project_matched"] = true
		resp["project_id"] = matchedProject.ID
		resp["project_name"] = matchedProject.Name
	}

	json.NewEncoder(w).Encode(resp)
}

// handleAntigravityUpdateTasks gestiona el inicio, latidos continuos y cierre de tareas desde Antigravity.
func (state *AppState) handleAntigravityUpdateTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var payload AntigravityTaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "JSON inválido: " + err.Error()})
		return
	}

	sess, err := state.resolveAntigravitySession(r, payload.Token, payload.UserEmail)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	odooCfg := state.resolveUserOdooConfig(sess)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Credenciales de Odoo no disponibles para este usuario"})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	userEmail := sess.UserEmail
	if userEmail == "" {
		userEmail = sess.Username
	}
	userUID, _ := client.ResolveUserUIDByEmail(ctx, userEmail)
	if userUID == 0 {
		userUID = client.UID()
	}

	// Validación estricta y case-insensitive de proyecto
	if payload.ProjectName != "" || payload.ProjectID > 0 {
		projects, pErr := client.GetProjects(ctx, nil)
		if pErr == nil {
			var matchedProject *odoo.Project
			if payload.ProjectID > 0 {
				for i := range projects {
					if projects[i].ID == payload.ProjectID {
						matchedProject = &projects[i]
						break
					}
				}
				if matchedProject == nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("El proyecto con ID %d no existe en PlanesGo/Odoo", payload.ProjectID),
					})
					return
				}
				if payload.ProjectName != "" && !projectNamesMatch(payload.ProjectName, matchedProject.Name) {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("El nombre del proyecto en Antigravity ('%s') no coincide con PlanesGo/Odoo ('%s')", payload.ProjectName, matchedProject.Name),
					})
					return
				}
				payload.ProjectName = matchedProject.Name
			} else if payload.ProjectName != "" {
				for i := range projects {
					if projectNamesMatch(payload.ProjectName, projects[i].Name) {
						matchedProject = &projects[i]
						break
					}
				}
				if matchedProject == nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("El proyecto '%s' no existe en PlanesGo/Odoo", payload.ProjectName),
					})
					return
				}
				payload.ProjectID = matchedProject.ID
				payload.ProjectName = matchedProject.Name
			}
		}
	}

	// 1. Aislamiento Estricto Antigravity vs Manual:
	// Las tareas gestionadas desde Antigravity nunca se mezclan con imputaciones manuales.
	// Se garantiza que el nombre de la tarea en Odoo contenga el prefijo [AGY].
	if payload.TaskName != "" {
		tn := strings.TrimSpace(payload.TaskName)
		if !strings.HasPrefix(strings.ToUpper(tn), "[AGY]") && !strings.HasPrefix(strings.ToUpper(tn), "[ANTIGRAVITY]") {
			payload.TaskName = fmt.Sprintf("[AGY] %s", tn)
		}
	} else if payload.TaskID <= 0 {
		payload.TaskName = "[AGY] Tarea de desarrollo"
	}

	// Creación o búsqueda dinámica de tarea en Odoo si no viene con task_id
	if payload.ProjectID > 0 && payload.TaskID <= 0 && payload.TaskName != "" {
		tasks, err := client.GetTasks(ctx, payload.ProjectID, 0)
		if err == nil {
			for _, t := range tasks {
				if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(payload.TaskName)) {
					payload.TaskID = t.ID
					break
				}
			}
		}
		if payload.TaskID <= 0 {
			newID, createErr := client.CreateTask(ctx, payload.ProjectID, payload.TaskName, userUID)
			if createErr == nil && newID > 0 {
				payload.TaskID = newID
				log.Printf("[Antigravity Gateway] Tarea creada dinámicamente en Odoo: ID %d ('%s') en proyecto %d", newID, payload.TaskName, payload.ProjectID)
			} else {
				log.Printf("[Antigravity Gateway] Advertencia al crear tarea dinámica en Odoo: %v", createErr)
			}
		}
	}

	nowMs := time.Now().UnixMilli()
	state.setLastConfirmedAt(userUID, nowMs)

	// Clave identificadora del cronómetro
	timerKey := ""
	if payload.TaskID > 0 {
		timerKey = fmt.Sprintf("task_%d", payload.TaskID)
	} else if payload.TimesheetID > 0 {
		timerKey = fmt.Sprintf("ts_%d", payload.TimesheetID)
	} else if payload.ProjectID > 0 {
		timerKey = fmt.Sprintf("proj_%d", payload.ProjectID)
	} else {
		timerKey = "default"
	}

	action := strings.ToLower(strings.TrimSpace(payload.Action))
	if action == "" {
		action = "heartbeat"
	}

	switch action {
	case "stop":
		cur := state.getActiveTimerByKey(userUID, timerKey)
		if cur != nil {
			if payload.TimesheetID <= 0 {
				payload.TimesheetID = cur.TimesheetID
			}
			if payload.TaskID <= 0 {
				payload.TaskID = cur.TaskID
			}
			if payload.UnitAmount <= 0 {
				payload.UnitAmount = cur.UnitAmount
			}
			if payload.Description == "" {
				payload.Description = cur.Description
			}
		}
		if err := client.StopTimer(ctx, payload.TimesheetID, payload.TaskID, payload.UnitAmount, payload.Description); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al detener tarea en Odoo: " + err.Error()})
			return
		}
		state.clearActiveTimerForTask(userUID, payload.TaskID, payload.TimesheetID)
		state.broadcastUserEvent(userUID, "timer_stop", map[string]interface{}{
			"timesheet_id": payload.TimesheetID,
			"task_id":      payload.TaskID,
			"timer_key":    timerKey,
			"unit_amount":  payload.UnitAmount,
			"source":       "antigravity",
		})
		state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
			"action":       "timer_stop",
			"timesheet_id": payload.TimesheetID,
			"unit_amount":  payload.UnitAmount,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"action":  "stopped",
			"message": fmt.Sprintf("Tarea '%s' (ID %d) detenida e imputada correctamente", payload.TaskName, payload.TaskID),
		})
		return

	case "pause":
		state.pauseActiveTimerByKey(userUID, timerKey, payload.UnitAmount)
		cur := state.getActiveTimerByKey(userUID, timerKey)
		state.broadcastUserEvent(userUID, "timer_pause", cur)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"action":  "paused",
			"timer":   cur,
		})
		return

	default: // "heartbeat" o "start"
		cur := state.getActiveTimerByKey(userUID, timerKey)
		if cur == nil {
			// Iniciar nuevo temporizador para esta tarea
			desc := payload.Description
			if desc == "" {
				if payload.TaskName != "" {
					desc = fmt.Sprintf("[ANTIGRAVITY] %s", payload.TaskName)
				} else {
					desc = "[ANTIGRAVITY] Tarea activa en IDE"
				}
			}

			activeTimer, startErr := client.StartTimer(ctx, payload.ProjectID, payload.ProjectName, payload.TaskID, payload.TaskName, payload.TimesheetID, desc, payload.UnitAmount, "")
			if startErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar tarea en Odoo: " + startErr.Error()})
				return
			}
			activeTimer.TimerKey = timerKey
			activeTimer.Source = "antigravity"
			activeTimer.LastHeartbeat = nowMs
			if activeTimer.EmployeeName == "" && sess.UserName != "" {
				activeTimer.EmployeeName = sess.UserName
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
			if payload.ElapsedSeconds > 0 {
				deltaMs = int64(payload.ElapsedSeconds * 1000)
			} else if cur.LastHeartbeat > 0 {
				elapsed := nowMs - cur.LastHeartbeat
				// Proteger contra saltos gigantes (>15 min se considera pausa previa)
				if elapsed > 0 && elapsed <= 15*60*1000 {
					deltaMs = elapsed
				}
			}
			cur.AccumulatedMs += deltaMs
			cur.LastHeartbeat = nowMs
			cur.IsRunning = true
			cur.UnitAmount = float64(cur.AccumulatedMs) / (3600 * 1000)
			if payload.Description != "" {
				cur.Description = payload.Description
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
			"action":      action,
			"timer":       cur,
			"server_time": nowMs,
			"message":     fmt.Sprintf("Latido procesado para tarea '%s' (%s acumulado)", cur.TaskName, formatHoursDuration(cur.UnitAmount)),
		})
	}
}

func formatHoursDuration(hours float64) string {
	totalSec := int(hours * 3600)
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// handleAntigravityTasks lista las tareas de un proyecto para el usuario autenticado con Antigravity
func (state *AppState) handleAntigravityTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}

	sess, err := state.resolveAntigravitySession(r, "", "")
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	odooCfg := state.resolveUserOdooConfig(sess)
	if odooCfg.Password == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "Credenciales de Odoo no disponibles para este usuario"})
		return
	}

	client := odoo.GetClient(odooCfg)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userEmail := sess.UserEmail
	if userEmail == "" {
		userEmail = sess.Username
	}
	userUID, _ := client.ResolveUserUIDByEmail(ctx, userEmail)
	if userUID == 0 {
		userUID = client.UID()
	}

	projectIDStr := strings.TrimSpace(r.URL.Query().Get("project_id"))
	projectID, _ := strconv.Atoi(projectIDStr)

	if projectID <= 0 {
		projectName := strings.TrimSpace(r.URL.Query().Get("project_name"))
		if projectName != "" {
			projects, pErr := client.GetProjects(ctx, nil)
			if pErr == nil {
				for _, p := range projects {
					if projectNamesMatch(projectName, p.Name) {
						projectID = p.ID
						break
					}
				}
			}
		}
	}

	if projectID <= 0 {
		json.NewEncoder(w).Encode([]odoo.Task{})
		return
	}

	tasks, err := client.GetTasks(ctx, projectID, userUID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Error al consultar tareas en Odoo: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(tasks)
}
