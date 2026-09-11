package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pasigo/config"
	"pasigo/odoo"
)

// handleDashboard maneja la ruta principal ("/") renderizando los proyectos, partes de horas y empleados
func (state *AppState) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var session *SessionData
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		session, _ = decodeSession(cookie.Value)
	}

	if session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Resolver credenciales personalizadas de Odoo (persistent store > session > fallback)
	currentOdooCfg := state.resolveUserOdooConfig(session)

	state.mu.RLock()
	serverPort := state.cfg.Server.Port
	state.mu.RUnlock()

	activeCfg := &config.Config{
		Server: config.ServerConfig{Port: serverPort},
		Odoo:   currentOdooCfg,
	}

	var entries []odoo.TimesheetEntry
	var projects []odoo.Project
	var activeEmployees []odoo.Employee
	var pendingTickets []odoo.Ticket
	var fetchErr error
	var client *odoo.Client
	var uid int
	hasOdooToken := (currentOdooCfg.Password != "" && currentOdooCfg.DB != "")
	if currentOdooCfg.Password != "" && currentOdooCfg.DB == "" {
		fetchErr = fmt.Errorf("Base de datos de Odoo no configurada. Por favor, ve a Ajustes para especificarla.")
	}

	if hasOdooToken {
		client = odoo.GetClient(currentOdooCfg)
		log.Printf("[INDEX] Iniciando consulta a Odoo. URL=%s, DB=%s, Usuario=%s", currentOdooCfg.URL, currentOdooCfg.DB, currentOdooCfg.Username)

		// Asegurar sesión/autenticación una sola vez antes de lanzar peticiones paralelas
		authStart := time.Now()
		authCtx, cancelAuth := context.WithTimeout(r.Context(), 15*time.Second)
		var authErr error
		uid, authErr = client.Authenticate(authCtx)
		cancelAuth()

		if authErr != nil {
			log.Printf("[INDEX ERROR] Error de autenticación con Odoo tras %v: %v", time.Since(authStart), authErr)
			fetchErr = authErr
		} else {
			log.Printf("[INDEX OK] Autenticado en Odoo en %v con UID=%d", time.Since(authStart), uid)
			var wg sync.WaitGroup
			var pErr, tsErr, empErr, tkErr error
			var projList []odoo.Project
			var tsEntries []odoo.TimesheetEntry
			var empList []odoo.Employee
			var tkList []odoo.Ticket

			wg.Add(4)

			// 1. Obtener proyectos concurrentemente
			go func() {
				defer wg.Done()
				pStart := time.Now()
				ctxProj, cancelProj := context.WithTimeout(r.Context(), 20*time.Second)
				defer cancelProj()
				projList, pErr = client.GetProjects(ctxProj, nil)
				log.Printf("[INDEX-GOROUTINE] GetProjects completado en %v (items: %d, err: %v)", time.Since(pStart), len(projList), pErr)
			}()

			// Determinar semana solicitada (?date=YYYY-MM-DD) o semana actual por defecto
			now := time.Now()
			targetDate := now
			if qDate := strings.TrimSpace(r.URL.Query().Get("date")); qDate != "" {
				if parsed, pErr := time.Parse("2006-01-02", qDate); pErr == nil {
					targetDate = parsed
				}
			}
			weekday := int(targetDate.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			weekMonday := targetDate.AddDate(0, 0, -(weekday - 1))
			weekSunday := weekMonday.AddDate(0, 0, 6)
			startOfWeekStr := weekMonday.Format("2006-01-02")
			endOfWeekStr := weekSunday.Format("2006-01-02")

			// 2. Obtener partes de horas de la semana concurrentemente
			go func() {
				defer wg.Done()
				tsStart := time.Now()
				ctxTS, cancelTS := context.WithTimeout(r.Context(), 20*time.Second)
				defer cancelTS()
				domainWeek := []interface{}{
					[]interface{}{"date", ">=", startOfWeekStr},
					[]interface{}{"date", "<=", endOfWeekStr},
				}
				tsEntries, tsErr = client.GetTimesheets(ctxTS, domainWeek)
				log.Printf("[INDEX-GOROUTINE] GetTimesheets semana (%s al %s) completado en %v (items: %d, err: %v)", startOfWeekStr, endOfWeekStr, time.Since(tsStart), len(tsEntries), tsErr)
			}()

			// 3. Obtener únicamente los trabajadores activos de Odoo concurrentemente
			go func() {
				defer wg.Done()
				empStart := time.Now()
				ctxEmp, cancelEmp := context.WithTimeout(r.Context(), 15*time.Second)
				defer cancelEmp()
				empList, empErr = client.GetEmployees(ctxEmp, nil)
				log.Printf("[INDEX-GOROUTINE] GetEmployees completado en %v (items: %d, err: %v)", time.Since(empStart), len(empList), empErr)
			}()

			// 4. Obtener tickets pendientes asignados al usuario concurrentemente
			go func() {
				defer wg.Done()
				tkStart := time.Now()
				ctxTk, cancelTk := context.WithTimeout(r.Context(), 15*time.Second)
				defer cancelTk()
				targetUID := uid
				if session != nil && session.UserEmail != "" {
					if resUID, rErr := client.ResolveUserUIDByEmail(ctxTk, session.UserEmail); rErr == nil && resUID > 0 {
						targetUID = resUID
					}
				}
				tkList, tkErr = client.GetPendingTickets(ctxTk, targetUID)
				log.Printf("[INDEX-GOROUTINE] GetPendingTickets completado en %v (items: %d, err: %v)", time.Since(tkStart), len(tkList), tkErr)
			}()

			wg.Wait()
			log.Printf("[INDEX] Todas las consultas paralelas de Odoo han finalizado.")

			if pErr != nil {
				log.Printf("[ADVERTENCIA] Error al obtener proyectos de Odoo: %v", pErr)
				fetchErr = pErr
			} else {
				projects = projList
			}

			if tsErr != nil {
				log.Printf("[ADVERTENCIA] Error al obtener partes de horas: %v", tsErr)
				if fetchErr == nil {
					fetchErr = tsErr
				}
			} else {
				entries = tsEntries
			}

			if empErr != nil {
				log.Printf("[ADVERTENCIA] Error al obtener empleados activos de Odoo: %v", empErr)
			} else {
				activeEmployees = empList
			}

			if tkErr != nil {
				log.Printf("[INFO] Tickets pendientes no disponibles: %v", tkErr)
			} else {
				pendingTickets = tkList
			}
		}
	}

	projectHoursMap := make(map[int]float64)
	projectCountMap := make(map[int]int)
	projectLastDateMap := make(map[int]string)
	projectLastTaskMap := make(map[int]string)

	projectNameHoursMap := make(map[string]float64)
	projectNameCountMap := make(map[string]int)
	projectNameLastDateMap := make(map[string]string)
	projectNameLastTaskMap := make(map[string]string)

	var totalHours float64
	projectMap := make(map[string]bool)
	employeeMap := make(map[string]bool)

	for _, entry := range entries {
		totalHours += entry.UnitAmount
		if entry.ProjectID.ID > 0 {
			projectHoursMap[entry.ProjectID.ID] += entry.UnitAmount
			projectCountMap[entry.ProjectID.ID]++
			if _, exists := projectLastDateMap[entry.ProjectID.ID]; !exists && entry.Date != "" {
				projectLastDateMap[entry.ProjectID.ID] = entry.Date
				if entry.TaskID.Name != "" {
					projectLastTaskMap[entry.ProjectID.ID] = entry.TaskID.Name
				}
			}
		}
		if entry.ProjectID.Name != "" {
			projectMap[entry.ProjectID.Name] = true
			projectNameHoursMap[entry.ProjectID.Name] += entry.UnitAmount
			projectNameCountMap[entry.ProjectID.Name]++
			if _, exists := projectNameLastDateMap[entry.ProjectID.Name]; !exists && entry.Date != "" {
				projectNameLastDateMap[entry.ProjectID.Name] = entry.Date
				if entry.TaskID.Name != "" {
					projectNameLastTaskMap[entry.ProjectID.Name] = entry.TaskID.Name
				}
			}
		}
		emp := entry.DisplayEmployee()
		if emp != "" && emp != "Sin asignar" {
			employeeMap[emp] = true
		}
	}

	type projectTicketInfo struct {
		count       int
		latestTitle string
	}
	ticketsByProjectID := make(map[int]*projectTicketInfo)
	ticketsByProjectName := make(map[string]*projectTicketInfo)

	for _, t := range pendingTickets {
		pID := t.ProjectID.ID
		pName := strings.TrimSpace(t.ProjectID.Name)
		title := t.DisplayTitle()
		if pID > 0 {
			info, exists := ticketsByProjectID[pID]
			if !exists {
				info = &projectTicketInfo{count: 0, latestTitle: title}
				ticketsByProjectID[pID] = info
			}
			info.count++
		}
		if pName != "" && pName != "-" {
			info, exists := ticketsByProjectName[pName]
			if !exists {
				info = &projectTicketInfo{count: 0, latestTitle: title}
				ticketsByProjectName[pName] = info
			}
			info.count++
		}
	}

	for i := range projects {
		pName := projects[i].DisplayNameOrName()
		if pName != "" {
			projectMap[pName] = true
		}
		if h, ok := projectHoursMap[projects[i].ID]; ok {
			projects[i].TotalHours = h
			projects[i].TimesheetCount = projectCountMap[projects[i].ID]
		} else if h, ok := projectNameHoursMap[pName]; ok {
			projects[i].TotalHours = h
			projects[i].TimesheetCount = projectNameCountMap[pName]
		}

		if d, ok := projectLastDateMap[projects[i].ID]; ok {
			projects[i].LastDate = d
			projects[i].LastTask = projectLastTaskMap[projects[i].ID]
		} else if d, ok := projectNameLastDateMap[pName]; ok {
			projects[i].LastDate = d
			projects[i].LastTask = projectNameLastTaskMap[pName]
		}

		// Enriquecer proyectos con tickets abiertos asignados al usuario
		if info, ok := ticketsByProjectID[projects[i].ID]; ok {
			projects[i].OpenTicketCount = info.count
			projects[i].TicketTitle = info.latestTitle
		} else if info, ok := ticketsByProjectName[pName]; ok {
			projects[i].OpenTicketCount = info.count
			projects[i].TicketTitle = info.latestTitle
		}
	}

	var projectsList []string
	for p := range projectMap {
		if p != "" && p != "-" {
			projectsList = append(projectsList, p)
		}
	}
	sort.Strings(projectsList)

	var employeesList []string
	activeEmpNameMap := make(map[string]bool)

	if len(activeEmployees) > 0 {
		// Usar estrictamente los trabajadores activos de Odoo (hr.employee con active = true)
		for _, emp := range activeEmployees {
			if emp.Active && strings.TrimSpace(emp.Name) != "" {
				name := strings.TrimSpace(emp.Name)
				if !activeEmpNameMap[name] {
					activeEmpNameMap[name] = true
					employeesList = append(employeesList, name)
				}
			}
		}
		sort.Strings(employeesList)
	} else {
		// Fallback si no se pudo consultar hr.employee: obtener de partes de horas
		for e := range employeeMap {
			employeesList = append(employeesList, e)
		}
		sort.Strings(employeesList)
	}

	// Determinar trabajador actual asociado a la sesión
	currentWorker := ""
	if session != nil {
		email := session.UserEmail
		if email == "" {
			email = session.Username
		}
		// 1. Intentar coincidencia con email de trabajo o usuario del empleado activo
		if email != "" && len(activeEmployees) > 0 {
			for _, emp := range activeEmployees {
				if emp.Active && (strings.EqualFold(emp.WorkEmail, email) || (emp.UserID.Name != "" && strings.EqualFold(emp.UserID.Name, email))) {
					currentWorker = emp.Name
					break
				}
			}
		}
		// 2. Coincidencia por nombre exacto en la lista de trabajadores
		if currentWorker == "" && session.UserName != "" {
			for _, emp := range employeesList {
				if strings.EqualFold(emp, session.UserName) {
					currentWorker = emp
					break
				}
			}
			if currentWorker == "" {
				sLower := strings.ToLower(session.UserName)
				for _, emp := range employeesList {
					eLower := strings.ToLower(emp)
					if strings.Contains(sLower, eLower) || strings.Contains(eLower, sLower) {
						currentWorker = emp
						break
					}
				}
			}
		}
		// 3. Coincidencia por parte de usuario del email
		if currentWorker == "" && email != "" {
			userPart := strings.ToLower(strings.Split(email, "@")[0])
			for _, emp := range employeesList {
				if strings.Contains(strings.ToLower(emp), userPart) {
					currentWorker = emp
					break
				}
			}
		}
	}
	if currentWorker == "" && len(employeesList) == 1 {
		currentWorker = employeesList[0]
	}
	if currentWorker == "" && len(employeesList) > 0 {
		currentWorker = employeesList[0]
	}

	// Calcular los proyectos pendientes:
	// Aquellos que tienen horas imputadas en las últimas 2 semanas O tienen un ticket abierto asignado al usuario
	twoWeeksAgo := time.Now().AddDate(0, 0, -14).Format("2006-01-02")

	type projAccumulator struct {
		project WorkerRecentProject
	}
	recentProjectsMap := make(map[string]*projAccumulator)
	var recentProjectsOrder []string

	// 1. Añadir proyectos con imputaciones del trabajador en las últimas 2 semanas o con tickets abiertos
	for _, entry := range entries {
		if currentWorker != "" && entry.DisplayEmployee() != currentWorker {
			continue
		}

		pName := entry.ProjectID.String()
		if pName == "" || pName == "-" {
			continue
		}

		hasOpenTicket := false
		if entry.ProjectID.ID > 0 && ticketsByProjectID[entry.ProjectID.ID] != nil {
			hasOpenTicket = true
		} else if ticketsByProjectName[pName] != nil {
			hasOpenTicket = true
		}

		isRecent := (entry.Date >= twoWeeksAgo)
		if !isRecent && !hasOpenTicket {
			continue
		}

		if acc, exists := recentProjectsMap[pName]; exists {
			acc.project.TotalHours += entry.UnitAmount
			acc.project.EntryCount++
			if entry.Date > acc.project.LastDate {
				acc.project.LastDate = entry.Date
				if entry.TaskID.Name != "" {
					acc.project.LastTask = entry.TaskID.Name
				}
			}
		} else {
			taskName := ""
			if entry.TaskID.Name != "" {
				taskName = entry.TaskID.Name
			}
			acc := &projAccumulator{
				project: WorkerRecentProject{
					ID:         entry.ProjectID.ID,
					Name:       pName,
					LastDate:   entry.Date,
					TotalHours: entry.UnitAmount,
					EntryCount: 1,
					LastTask:   taskName,
					Employee:   entry.DisplayEmployee(),
				},
			}
			recentProjectsMap[pName] = acc
			recentProjectsOrder = append(recentProjectsOrder, pName)
		}
	}

	// 2. Si no hay proyectos pendientes para el trabajador seleccionado pero hay actividad general,
	// buscar fallback si aplica
	if len(recentProjectsOrder) == 0 && len(entries) > 0 {
		for _, entry := range entries {
			if entry.Date >= twoWeeksAgo {
				emp := entry.DisplayEmployee()
				if emp != "" && emp != "Sin asignar" {
					currentWorker = emp
					break
				}
			}
		}
		if currentWorker != "" {
			for _, entry := range entries {
				if entry.DisplayEmployee() != currentWorker {
					continue
				}
				pName := entry.ProjectID.String()
				if pName == "" || pName == "-" {
					continue
				}
				hasOpenTicket := false
				if entry.ProjectID.ID > 0 && ticketsByProjectID[entry.ProjectID.ID] != nil {
					hasOpenTicket = true
				} else if ticketsByProjectName[pName] != nil {
					hasOpenTicket = true
				}
				if entry.Date < twoWeeksAgo && !hasOpenTicket {
					continue
				}
				if acc, exists := recentProjectsMap[pName]; exists {
					acc.project.TotalHours += entry.UnitAmount
					acc.project.EntryCount++
					if entry.Date > acc.project.LastDate {
						acc.project.LastDate = entry.Date
						if entry.TaskID.Name != "" {
							acc.project.LastTask = entry.TaskID.Name
						}
					}
				} else {
					taskName := ""
					if entry.TaskID.Name != "" {
						taskName = entry.TaskID.Name
					}
					acc := &projAccumulator{
						project: WorkerRecentProject{
							ID:         entry.ProjectID.ID,
							Name:       pName,
							LastDate:   entry.Date,
							TotalHours: entry.UnitAmount,
							EntryCount: 1,
							LastTask:   taskName,
							Employee:   entry.DisplayEmployee(),
						},
					}
					recentProjectsMap[pName] = acc
					recentProjectsOrder = append(recentProjectsOrder, pName)
				}
			}
		}
	}

	// 3. Añadir proyectos que tienen tickets abiertos asignados al usuario y que aún no estén en recentProjectsMap
	for _, t := range pendingTickets {
		pID := t.ProjectID.ID
		pName := strings.TrimSpace(t.ProjectID.Name)
		if pName == "" || pName == "-" {
			if pID > 0 {
				pName = fmt.Sprintf("Proyecto #%d", pID)
			} else {
				continue
			}
		}

		if _, exists := recentProjectsMap[pName]; !exists {
			recentProjectsMap[pName] = &projAccumulator{
				project: WorkerRecentProject{
					ID:         pID,
					Name:       pName,
					LastDate:   t.CreateDate,
					TotalHours: 0.0,
					EntryCount: 0,
					LastTask:   t.DisplayTitle(),
					Employee:   currentWorker,
				},
			}
			recentProjectsOrder = append(recentProjectsOrder, pName)
		}
	}

	// 4. Enlazar datos de tickets a recentProjectsMap
	for pName, acc := range recentProjectsMap {
		if acc.project.ID > 0 && ticketsByProjectID[acc.project.ID] != nil {
			info := ticketsByProjectID[acc.project.ID]
			acc.project.OpenTicketCount = info.count
			acc.project.TicketTitle = info.latestTitle
		} else if ticketsByProjectName[pName] != nil {
			info := ticketsByProjectName[pName]
			acc.project.OpenTicketCount = info.count
			acc.project.TicketTitle = info.latestTitle
		}
	}

	var recentProjects []WorkerRecentProject
	for _, pName := range recentProjectsOrder {
		recentProjects = append(recentProjects, recentProjectsMap[pName].project)
	}

	// 5. Ordenar proyectos pendientes:
	// - Proyectos con tickets abiertos primero
	// - Fecha de última imputación (descendente)
	// - Total de horas (descendente)
	sort.SliceStable(recentProjects, func(i, j int) bool {
		tI := recentProjects[i].OpenTicketCount > 0
		tJ := recentProjects[j].OpenTicketCount > 0
		if tI != tJ {
			return tI
		}
		dI := recentProjects[i].LastDate
		dJ := recentProjects[j].LastDate
		if dI != dJ {
			return dI > dJ
		}
		return recentProjects[i].TotalHours > recentProjects[j].TotalHours
	})

	// 6. Ordenar la lista completa de proyectos (projects):
	// Los proyectos pendientes van PRIMERO y en el MISMO ORDEN EXACTO que recentProjects
	pendingProjectRank := make(map[string]int)
	pendingProjectIDRank := make(map[int]int)
	for idx, rp := range recentProjects {
		pendingProjectRank[strings.ToLower(rp.Name)] = idx + 1
		if rp.ID > 0 {
			pendingProjectIDRank[rp.ID] = idx + 1
		}
	}

	sort.SliceStable(projects, func(i, j int) bool {
		pNameI := strings.ToLower(projects[i].DisplayNameOrName())
		pNameJ := strings.ToLower(projects[j].DisplayNameOrName())

		rankI := 0
		if r, ok := pendingProjectIDRank[projects[i].ID]; ok {
			rankI = r
		} else if r, ok := pendingProjectRank[pNameI]; ok {
			rankI = r
		}

		rankJ := 0
		if r, ok := pendingProjectIDRank[projects[j].ID]; ok {
			rankJ = r
		} else if r, ok := pendingProjectRank[pNameJ]; ok {
			rankJ = r
		}

		if rankI > 0 && rankJ > 0 {
			return rankI < rankJ // Mismo orden relativo que en pendientes
		}
		if rankI > 0 && rankJ == 0 {
			return true // Proyectos pendientes van antes
		}
		if rankI == 0 && rankJ > 0 {
			return false
		}

		dI := projects[i].LastDate
		dJ := projects[j].LastDate
		if dI != "" && dJ != "" {
			if dI != dJ {
				return dI > dJ
			}
			return projects[i].TotalHours > projects[j].TotalHours
		}
		if dI != "" && dJ == "" {
			return true
		}
		if dI == "" && dJ != "" {
			return false
		}
		return pNameI < pNameJ
	})

	errMsg := ""
	if fetchErr != nil {
		errMsg = fetchErr.Error()
		log.Printf("[ERROR] Consulta Odoo: %v", fetchErr)
	}

	uniqueEmployeesCount := len(employeesList)
	if uniqueEmployeesCount == 0 {
		uniqueEmployeesCount = len(employeeMap)
	}

	var activeTimer *odoo.ActiveTimer
	for _, e := range entries {
		if e.IsTimerRunning {
			accumMs := int64(e.UnitAmount * 3600 * 1000)
			activeTimer = &odoo.ActiveTimer{
				TimesheetID:   e.ID,
				ProjectID:     e.ProjectID.ID,
				ProjectName:   e.ProjectID.Name,
				TaskID:        e.TaskID.ID,
				TaskName:      e.TaskID.Name,
				Description:   e.Name,
				IsRunning:     true,
				StartedAt:     time.Now().UnixMilli() - accumMs,
				AccumulatedMs: accumMs,
				UnitAmount:    e.UnitAmount,
				Date:          e.Date,
			}
			break
		}
	}

	if activeTimer == nil && client != nil {
		targetUID := uid
		if session != nil && session.UserEmail != "" {
			if resUID, rErr := client.ResolveUserUIDByEmail(r.Context(), session.UserEmail); rErr == nil && resUID > 0 {
				targetUID = resUID
			}
		}
		if timer, tErr := client.GetActiveTimer(r.Context(), targetUID); tErr == nil && timer != nil {
			activeTimer = timer
			found := false
			for _, e := range entries {
				if e.ID == timer.TimesheetID {
					found = true
					break
				}
			}
			if !found && timer.TimesheetID > 0 {
				entries = append([]odoo.TimesheetEntry{{
					ID:             timer.TimesheetID,
					Date:           timer.Date,
					Name:           timer.Description,
					UnitAmount:     timer.UnitAmount,
					ProjectID:      odoo.Many2One{ID: timer.ProjectID, Name: timer.ProjectName},
					TaskID:         odoo.Many2One{ID: timer.TaskID, Name: timer.TaskName},
					IsTimerRunning: true,
				}}, entries...)
			}
		}
	}

	data := PageData{
		Version:              Version,
		Config:               activeCfg,
		Session:              session,
		HasOdooToken:         hasOdooToken,
		Entries:              entries,
		Projects:             projects,
		PendingTickets:       pendingTickets,
		PendingTicketsCount:  len(pendingTickets),
		OdooURL:              strings.TrimRight(currentOdooCfg.URL, "/"),
		TotalHours:           totalHours,
		TotalProjectsCount:   len(projects),
		UniqueProjectsCount:  len(projectMap),
		UniqueEmployeesCount: uniqueEmployeesCount,
		ProjectsList:         projectsList,
		EmployeesList:        employeesList,
		CurrentWorker:        currentWorker,
		RecentProjects:       recentProjects,
		Today:                time.Now().Format("2006-01-02"),
		ActiveTimer:          activeTimer,
		Error:                errMsg,
	}

	tmpl, err := getIndexTemplate()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error al cargar plantilla: %v", err), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("[ERROR] Renderizado de plantilla: %v", err)
		http.Error(w, "Error interno al renderizar la página", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	if _, err := buf.WriteTo(w); err != nil {
		log.Printf("[WARN] Error escribiendo respuesta al cliente: %v", err)
	}
}
