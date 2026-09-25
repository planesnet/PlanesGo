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
	TaskType        string  `json:"task_type,omitempty"` // "Desarrollo", "Análisis", "Ajustes", "Servidor", "Cliente"
	TicketCode      string  `json:"ticket_code,omitempty"`
	TicketID        int     `json:"ticket_id,omitempty"`
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

// CanonicalTaskTypes define los tipos normalizados de tareas de Antigravity
const (
	TaskTypeAnalisisDiseno = "Análisis y diseño"
	TaskTypeDesarrollo     = "Desarrollo"
	TaskTypePruebas        = "Pruebas"
)

var CanonicalTaskTypes = []string{
	TaskTypeAnalisisDiseno,
	TaskTypeDesarrollo,
	TaskTypePruebas,
}

// matchCanonicalType normaliza un texto a uno de los tipos canónicos si coincide.
func matchCanonicalType(t string) (string, bool) {
	norm := strings.ToLower(strings.TrimSpace(t))
	norm = strings.ReplaceAll(norm, "á", "a")
	norm = strings.ReplaceAll(norm, "é", "e")
	norm = strings.ReplaceAll(norm, "í", "i")
	norm = strings.ReplaceAll(norm, "ó", "o")
	norm = strings.ReplaceAll(norm, "ú", "u")

	switch norm {
	case "analisis y diseno", "analisis y diseño", "análisis y diseño", "analisis", "análisis", "analysis", "investigacion", "auditoria", "diseno", "diseño", "design", "planificacion", "arquitectura":
		return TaskTypeAnalisisDiseno, true
	case "desarrollo", "dev", "development", "implementacion", "implementación", "impl", "ajuste", "ajustes", "fix", "fixes", "bugfix", "refactor", "servidor", "server", "infraestructura", "infra", "ops", "cliente", "client", "soporte", "support":
		return TaskTypeDesarrollo, true
	case "pruebas", "prueba", "test", "tests", "testing", "qa", "verificacion", "validacion":
		return TaskTypePruebas, true
	}
	return "", false
}

func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

// cleanAntigravityTaskName extrae el nombre legible de una tarea sin los prefijos técnicos [AGY] o [ANTIGRAVITY]
func cleanAntigravityTaskName(taskName string) string {
	name := strings.TrimSpace(taskName)
	for {
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "[AGY]") {
			name = strings.TrimSpace(name[5:])
			continue
		}
		if strings.HasPrefix(upper, "[ANTIGRAVITY]") {
			name = strings.TrimSpace(name[14:])
			continue
		}
		break
	}
	return name
}

// NormalizeTaskType analiza la tarea, la descripción y el tipo explícito para determinar
// de forma normalizada uno de los tipos canónicos (Análisis y diseño, Desarrollo, Pruebas)
// y devuelve el tipo canónico y el nombre de la tarea canónico sin prefijos técnicos.
func NormalizeTaskType(taskName, description, explicitType string) (string, string) {
	raw := cleanAntigravityTaskName(taskName)

	var detectedType string

	// 1. Si el nombre ya comienza por un corchete de tipo ej. [Análisis y diseño] o [Desarrollo] o [Pruebas]
	if strings.HasPrefix(raw, "[") {
		idx := strings.Index(raw, "]")
		if idx > 1 {
			bracketContent := raw[1:idx]
			if canon, ok := matchCanonicalType(bracketContent); ok {
				detectedType = canon
			}
		}
	}

	// 2. Si no se detectó en corchetes pero se pasó explicitType
	if detectedType == "" && explicitType != "" {
		if canon, ok := matchCanonicalType(explicitType); ok {
			detectedType = canon
		}
	}

	// 3. Inferencia automática por heurística semántica:
	// REGLA CLAVE: Analizar PRIMERO el título/nombre (raw) para no ser falseado por palabras accesorias de la descripción.
	if detectedType == "" {
		rawNorm := strings.ToLower(raw)
		rawNorm = strings.ReplaceAll(rawNorm, "á", "a")
		rawNorm = strings.ReplaceAll(rawNorm, "é", "e")
		rawNorm = strings.ReplaceAll(rawNorm, "í", "i")
		rawNorm = strings.ReplaceAll(rawNorm, "ó", "o")
		rawNorm = strings.ReplaceAll(rawNorm, "ú", "u")
		rawNorm = strings.ReplaceAll(rawNorm, "ñ", "n")

		// Evaluar prioridad sobre el título de la tarea
		if containsAny(rawNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(rawNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "revis", "planificac", "explorac", "research", "benchmark", "inspecc", "arquitect") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(rawNorm, "desarroll", "implementac", "servidor", "server", "systemd", "nginx", "apache", "docker", "deploy", "despliegue", "ssh", "puerto", "backup", "cron", "proxy", "daemon", "demon", "firewall", "sysadmin", "cliente", "usuario", "soporte", "ticket", "reunion", "consulta", "duda", "demo", "capacitacion", "formacion", "funcional", "tarifa", "ajuste", "ajustes", "fix", "bug", "error", "correccion", "corregir", "refactor", "tweak", "patch", "parche", "limpieza", "lint", "linter", "estilo", "padding", "css", "tipografia") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 4. Si el título no arrojó coincidencias, inspeccionar la descripción
	if detectedType == "" && description != "" {
		descNorm := strings.ToLower(description)
		descNorm = strings.ReplaceAll(descNorm, "á", "a")
		descNorm = strings.ReplaceAll(descNorm, "é", "e")
		descNorm = strings.ReplaceAll(descNorm, "í", "i")
		descNorm = strings.ReplaceAll(descNorm, "ó", "o")
		descNorm = strings.ReplaceAll(descNorm, "ú", "u")
		descNorm = strings.ReplaceAll(descNorm, "ñ", "n")

		if containsAny(descNorm, "prueba", "pruebas", "test", "testing", "tests", "qa", "verificac", "validac") {
			detectedType = TaskTypePruebas
		} else if containsAny(descNorm, "analis", "disen", "design", "investigac", "auditor", "diagnostic", "estudio", "planificac") {
			detectedType = TaskTypeAnalisisDiseno
		} else if containsAny(descNorm, "desarroll", "implementac", "servidor", "server", "systemd", "docker", "deploy", "cliente", "ajuste", "fix", "bug") {
			detectedType = TaskTypeDesarrollo
		}
	}

	// 5. Fallback por defecto: Desarrollo
	if detectedType == "" {
		detectedType = TaskTypeDesarrollo
	}

	// El nombre canónico en Odoo es exclusivamente el tipo canónico (ej. "Análisis y diseño", "Desarrollo", "Pruebas")
	return detectedType, detectedType
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

	// Resolución y validación estricta de ticket de soporte
	if payload.TicketCode != "" || payload.TicketID > 0 {
		refOrID := payload.TicketCode
		if refOrID == "" {
			refOrID = strconv.Itoa(payload.TicketID)
		}
		t, tErr := client.GetTicketByRefOrID(ctx, refOrID)
		if tErr != nil || t == nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("No se encontró el ticket '%s' en Odoo", refOrID),
			})
			return
		}
		if t.IsClosed() {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("El ticket %s (#%d) está cerrado. Un ticket cerrado no se puede volver a abrir ni imputar tiempos.", t.TicketRef, t.ID),
			})
			return
		}
		payload.TicketID = t.ID
		payload.TicketCode = t.TicketRef
		if payload.ProjectID <= 0 && t.ProjectID.ID > 0 {
			payload.ProjectID = t.ProjectID.ID
			payload.ProjectName = t.ProjectID.Name
		}
		if payload.TaskID <= 0 && t.TaskID.ID > 0 {
			payload.TaskID = t.TaskID.ID
			payload.TaskName = t.TaskID.Name
		}
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

	// 1. Normalización y Aislamiento Estricto Antigravity:
	// Las tareas gestionadas desde Antigravity se clasifican en los 3 tipos normalizados
	// (Análisis y diseño, Desarrollo, Pruebas) y utilizan exclusivamente
	// el nombre canónico sin prefijos técnicos, registrando la procedencia de Antigravity en tag_ids.
	canonicalType, canonicalName := NormalizeTaskType(payload.TaskName, payload.Description, payload.TaskType)
	payload.TaskType = canonicalType
	payload.TaskName = canonicalName

	// Creación o búsqueda dinámica de tarea en Odoo si no viene con task_id
	if payload.ProjectID > 0 && payload.TaskID <= 0 && payload.TaskName != "" {
		tasks, err := client.GetTasks(ctx, payload.ProjectID, 0)
		if err == nil {
			// 1.1 Coincidencia exacta con el nombre canónico
			for _, t := range tasks {
				if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(payload.TaskName)) {
					payload.TaskID = t.ID
					break
				}
			}
			// 1.2 Reutilizar tareas estándar existentes según el tipo canónico
			if payload.TaskID <= 0 {
				for _, t := range tasks {
					tUpper := strings.ToUpper(strings.TrimSpace(t.Name))
					tNorm := strings.ReplaceAll(strings.ReplaceAll(tUpper, "Á", "A"), "É", "E")
					tNorm = strings.ReplaceAll(strings.ReplaceAll(tNorm, "Í", "I"), "Ó", "O")
					tNorm = strings.ReplaceAll(strings.ReplaceAll(tNorm, "Ú", "U"), "Ñ", "N")

					if payload.TaskType == TaskTypeAnalisisDiseno {
						if tNorm == "ANALISIS Y DISENO" || tNorm == "ANALISIS" || tNorm == "DISENO" || strings.HasPrefix(tNorm, "ANALISIS") {
							payload.TaskID = t.ID
							break
						}
					} else if payload.TaskType == TaskTypeDesarrollo {
						if tNorm == "DESARROLLO" || tNorm == "IMPLEMENTACION" || strings.HasPrefix(tNorm, "DESARROLLO") {
							payload.TaskID = t.ID
							break
						}
					} else if payload.TaskType == TaskTypePruebas {
						if tNorm == "PRUEBAS" || tNorm == "TESTS" || tNorm == "TEST" || tNorm == "TESTING" || tNorm == "QA" || strings.HasPrefix(tNorm, "PRUEBAS") {
							payload.TaskID = t.ID
							break
						}
					}
				}
			}
		}
		if payload.TaskID <= 0 {
			newID, createErr := client.CreateTaskWithTag(ctx, payload.ProjectID, payload.TaskName, userUID, "Antigravity")
			if createErr == nil && newID > 0 {
				payload.TaskID = newID
				log.Printf("[Antigravity Gateway] Tarea canónica creada en Odoo: ID %d ('%s') con tag 'Antigravity' en proyecto %d", newID, payload.TaskName, payload.ProjectID)
			} else {
				log.Printf("[Antigravity Gateway] Advertencia al crear tarea canónica en Odoo: %v", createErr)
			}
		} else {
			// Asegurar que la tarea asignada tenga el tag Antigravity
			go func(tID int) {
				_ = client.EnsureTaskTag(context.Background(), tID, "Antigravity")
			}(payload.TaskID)
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

		// Resumen breve y sustantivo para el parte de horas en Odoo
		desc := strings.TrimSpace(payload.Description)
		if desc == "" || desc == "Trabajo en curso" || strings.HasPrefix(desc, "[ANTIGRAVITY]") || strings.HasPrefix(desc, "[AGY]") {
			desc = fmt.Sprintf("[%s] %s", canonicalType, cleanAntigravityTaskName(payload.TaskName))
		}
		payload.Description = desc

		if err := client.StopTimer(ctx, payload.TimesheetID, payload.TaskID, payload.UnitAmount, payload.Description); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Error al detener tarea en Odoo: " + err.Error()})
			return
		}
		if payload.TicketID <= 0 && cur != nil && cur.TicketID > 0 {
			payload.TicketID = cur.TicketID
		}
		if payload.TicketID > 0 && payload.TimesheetID > 0 {
			_ = client.UpdateTimesheetWithTicket(ctx, payload.TimesheetID, "", payload.TaskID, payload.TicketID, 0, "")
		}
		state.clearActiveTimerForTask(userUID, payload.TaskID, payload.TimesheetID)
		state.broadcastUserEvent(userUID, "timer_stop", map[string]interface{}{
			"timesheet_id": payload.TimesheetID,
			"task_id":      payload.TaskID,
			"ticket_id":    payload.TicketID,
			"timer_key":    timerKey,
			"unit_amount":  payload.UnitAmount,
			"task_type":    canonicalType,
			"source":       "antigravity",
		})
		state.broadcastUserEvent(userUID, "timesheets_changed", map[string]interface{}{
			"action":       "timer_stop",
			"timesheet_id": payload.TimesheetID,
			"unit_amount":  payload.UnitAmount,
		})
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"action":      "stopped",
			"task_id":     payload.TaskID,
			"task_name":   payload.TaskName,
			"task_type":   canonicalType,
			"ticket_id":   payload.TicketID,
			"description": payload.Description,
			"message":     fmt.Sprintf("Tarea '%s' (ID %d) detenida e imputada correctamente", payload.TaskName, payload.TaskID),
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
			desc := strings.TrimSpace(payload.Description)
			if desc == "" || desc == "Trabajo en curso" || strings.HasPrefix(desc, "[ANTIGRAVITY]") || strings.HasPrefix(desc, "[AGY]") {
				desc = fmt.Sprintf("[%s] %s", canonicalType, cleanAntigravityTaskName(payload.TaskName))
			}

			activeTimer, startErr := client.StartTimerExtended(ctx, payload.ProjectID, payload.ProjectName, payload.TaskID, payload.TaskName, payload.TimesheetID, desc, payload.UnitAmount, "", true)
			if startErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": "Error al iniciar tarea en Odoo: " + startErr.Error()})
				return
			}
			activeTimer.TimerKey = timerKey
			activeTimer.Source = "antigravity"
			activeTimer.LastHeartbeat = nowMs
			if payload.TicketID > 0 {
				activeTimer.TicketID = payload.TicketID
				activeTimer.TicketRef = payload.TicketCode
				if activeTimer.TimesheetID > 0 {
					_ = client.UpdateTimesheetWithTicket(ctx, activeTimer.TimesheetID, "", payload.TaskID, payload.TicketID, 0, "")
				}
			}
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
				"ticket_id":    activeTimer.TicketID,
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
			if payload.Description != "" && payload.Description != "Trabajo en curso" {
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
			"task_type":   canonicalType,
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
