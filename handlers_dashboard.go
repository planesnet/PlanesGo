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
	var fetchErr error
	hasOdooToken := (currentOdooCfg.Password != "" && currentOdooCfg.DB != "")
	if currentOdooCfg.Password != "" && currentOdooCfg.DB == "" {
		fetchErr = fmt.Errorf("Base de datos de Odoo no configurada. Por favor, ve a Ajustes para especificarla.")
	}

	if hasOdooToken {
		client := odoo.GetClient(currentOdooCfg)
		log.Printf("[INDEX] Iniciando consulta a Odoo. URL=%s, DB=%s, Usuario=%s", currentOdooCfg.URL, currentOdooCfg.DB, currentOdooCfg.Username)

		// Asegurar sesión/autenticación una sola vez antes de lanzar peticiones paralelas
		authStart := time.Now()
		authCtx, cancelAuth := context.WithTimeout(r.Context(), 15*time.Second)
		uid, authErr := client.Authenticate(authCtx)
		cancelAuth()

		if authErr != nil {
			log.Printf("[INDEX ERROR] Error de autenticación con Odoo tras %v: %v", time.Since(authStart), authErr)
			fetchErr = authErr
		} else {
			log.Printf("[INDEX OK] Autenticado en Odoo en %v con UID=%d", time.Since(authStart), uid)
			var wg sync.WaitGroup
			var pErr, tsErr, empErr error
			var projList []odoo.Project
			var tsEntries []odoo.TimesheetEntry
			var empList []odoo.Employee

			wg.Add(3)

			// 1. Obtener proyectos concurrentemente
			go func() {
				defer wg.Done()
				pStart := time.Now()
				ctxProj, cancelProj := context.WithTimeout(r.Context(), 20*time.Second)
				defer cancelProj()
				projList, pErr = client.GetProjects(ctxProj, nil)
				log.Printf("[INDEX-GOROUTINE] GetProjects completado en %v (items: %d, err: %v)", time.Since(pStart), len(projList), pErr)
			}()

			// 2. Obtener partes de horas concurrentemente
			go func() {
				defer wg.Done()
				tsStart := time.Now()
				ctxTS, cancelTS := context.WithTimeout(r.Context(), 25*time.Second)
				defer cancelTS()
				tsEntries, tsErr = client.GetTimesheets(ctxTS, nil)
				log.Printf("[INDEX-GOROUTINE] GetTimesheets completado en %v (items: %d, err: %v)", time.Since(tsStart), len(tsEntries), tsErr)
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
		}
	}

	projectHoursMap := make(map[int]float64)
	projectCountMap := make(map[int]int)
	projectNameHoursMap := make(map[string]float64)
	projectNameCountMap := make(map[string]int)

	var totalHours float64
	projectMap := make(map[string]bool)
	employeeMap := make(map[string]bool)

	for _, entry := range entries {
		totalHours += entry.UnitAmount
		if entry.ProjectID.ID > 0 {
			projectHoursMap[entry.ProjectID.ID] += entry.UnitAmount
			projectCountMap[entry.ProjectID.ID]++
		}
		if entry.ProjectID.Name != "" {
			projectMap[entry.ProjectID.Name] = true
			projectNameHoursMap[entry.ProjectID.Name] += entry.UnitAmount
			projectNameCountMap[entry.ProjectID.Name]++
		}
		emp := entry.DisplayEmployee()
		if emp != "" && emp != "Sin asignar" {
			employeeMap[emp] = true
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

	// Calcular los últimos proyectos utilizados por el trabajador
	type projAccumulator struct {
		project WorkerRecentProject
	}
	recentProjectsMap := make(map[string]*projAccumulator)
	var recentProjectsOrder []string

	for _, entry := range entries {
		if currentWorker != "" && entry.DisplayEmployee() != currentWorker {
			continue
		}

		pName := entry.ProjectID.String()
		if pName == "" || pName == "-" {
			continue
		}

		if acc, exists := recentProjectsMap[pName]; exists {
			acc.project.TotalHours += entry.UnitAmount
			acc.project.EntryCount++
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

	// Si no hay proyectos para el trabajador seleccionado pero hay partes en total, seleccionar primer trabajador con actividad
	if len(recentProjectsOrder) == 0 && len(entries) > 0 {
		for _, entry := range entries {
			emp := entry.DisplayEmployee()
			if emp != "" && emp != "Sin asignar" {
				currentWorker = emp
				break
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
				if acc, exists := recentProjectsMap[pName]; exists {
					acc.project.TotalHours += entry.UnitAmount
					acc.project.EntryCount++
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

	var recentProjects []WorkerRecentProject
	for _, pName := range recentProjectsOrder {
		recentProjects = append(recentProjects, recentProjectsMap[pName].project)
	}

	errMsg := ""
	if fetchErr != nil {
		errMsg = fetchErr.Error()
		log.Printf("[ERROR] Consulta Odoo: %v", fetchErr)
	}

	uniqueEmployeesCount := len(employeesList)
	if uniqueEmployeesCount == 0 {
		uniqueEmployeesCount = len(employeeMap)
	}

	data := PageData{
		Version:              Version,
		Config:               activeCfg,
		Session:              session,
		HasOdooToken:         hasOdooToken,
		Entries:              entries,
		Projects:             projects,
		TotalHours:           totalHours,
		TotalProjectsCount:   len(projects),
		UniqueProjectsCount:  len(projectMap),
		UniqueEmployeesCount: uniqueEmployeesCount,
		ProjectsList:         projectsList,
		EmployeesList:        employeesList,
		CurrentWorker:        currentWorker,
		RecentProjects:       recentProjects,
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
