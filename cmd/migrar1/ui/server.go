package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"pasigo/cmd/migrar1/db"
	"pasigo/cmd/migrar1/engine"
	"pasigo/cmd/migrar1/odoo"
	"strconv"
	"sync"
	"time"
)

type Server struct {
	db       *db.DB
	migrator *engine.Migrator
	mu       sync.RWMutex
	httpSrv  *http.Server
	port     int
}

func NewServer(database *db.DB, port int) *Server {
	if port <= 0 {
		port = 8990
	}
	return &Server{
		db:   database,
		port: port,
	}
}

type ConnectionPayload struct {
	Source struct {
		URL      string `json:"url"`
		DB       string `json:"db"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"source"`
	Target struct {
		URL      string `json:"url"`
		DB       string `json:"db"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"target"`
	StartDate string `json:"startDate"`
	TicketID  int    `json:"ticket_id,omitempty"`
	BatchSize int    `json:"batch_size,omitempty"`
}

func (s *Server) getOrCreateMigrator(payload ConnectionPayload) *engine.Migrator {
	s.mu.Lock()
	defer s.mu.Unlock()

	srcClient := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Source.URL,
		DB:       payload.Source.DB,
		Username: payload.Source.Username,
		Password: payload.Source.Password,
	})

	tgtClient := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Target.URL,
		DB:       payload.Target.DB,
		Username: payload.Target.Username,
		Password: payload.Target.Password,
	})

	s.migrator = engine.NewMigrator(s.db, srcClient, tgtClient, engine.MigrationConfig{
		StartDate: payload.StartDate,
		BatchSize: payload.BatchSize,
	})

	return s.migrator
}

func (s *Server) Start() (int, error) {
	mux := http.NewServeMux()

	// Frontend principal
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(IndexHTML))
	})

	// API Endpoints
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/test-connections", s.handleTestConnections)
	mux.HandleFunc("/api/dry-run", s.handleDryRun)
	mux.HandleFunc("/api/migrate-ticket", s.handleMigrateTicket)
	mux.HandleFunc("/api/migrate-batch", s.handleMigrateBatch)
	mux.HandleFunc("/api/migrate-all", s.handleMigrateAll)
	mux.HandleFunc("/api/mappings", s.handleListMappings)
	mux.HandleFunc("/api/mappings/save", s.handleSaveMapping)
	mux.HandleFunc("/api/mappings/export", s.handleExportMappings)
	mux.HandleFunc("/api/sync-records", s.handleListSyncRecords)
	mux.HandleFunc("/api/logs", s.handleListLogs)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		// Reintento con puerto aleatorio si el indicado está ocupado
		listener, err = net.Listen("tcp", ":0")
		if err != nil {
			return 0, err
		}
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	s.port = actualPort

	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		_ = s.httpSrv.Serve(listener)
	}()

	return actualPort, nil
}

func (s *Server) Stop(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleTestConnections(w http.ResponseWriter, r *http.Request) {
	var payload ConnectionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	srcClient := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Source.URL,
		DB:       payload.Source.DB,
		Username: payload.Source.Username,
		Password: payload.Source.Password,
	})
	srcUID, srcErr := srcClient.Authenticate(ctx)

	tgtClient := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Target.URL,
		DB:       payload.Target.DB,
		Username: payload.Target.Username,
		Password: payload.Target.Password,
	})
	tgtUID, tgtErr := tgtClient.Authenticate(ctx)

	resp := map[string]interface{}{
		"source_ok":    srcErr == nil && srcUID > 0,
		"source_uid":   srcUID,
		"source_error": "",
		"target_ok":    tgtErr == nil && tgtUID > 0,
		"target_uid":   tgtUID,
		"target_error": "",
	}
	if srcErr != nil {
		resp["source_error"] = srcErr.Error()
	}
	if tgtErr != nil {
		resp["target_error"] = tgtErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleDryRun(w http.ResponseWriter, r *http.Request) {
	var payload ConnectionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mig := s.getOrCreateMigrator(payload)
	report, err := mig.DryRun(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) handleMigrateTicket(w http.ResponseWriter, r *http.Request) {
	var payload ConnectionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mig := s.getOrCreateMigrator(payload)
	_ = mig.InitSchemaDiscovery(r.Context())

	rec, err := mig.MigrateTicket(r.Context(), payload.TicketID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "record": rec})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rec)
}

func (s *Server) handleMigrateBatch(w http.ResponseWriter, r *http.Request) {
	var payload ConnectionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mig := s.getOrCreateMigrator(payload)
	_ = mig.InitSchemaDiscovery(r.Context())

	size := payload.BatchSize
	if size <= 0 {
		size = 10
	}

	// Consultar los siguientes tickets de origen desde 2025
	domain := []interface{}{
		[]interface{}{"create_date", ">=", payload.StartDate + " 00:00:00"},
	}

	ticketsRaw, err := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Source.URL,
		DB:       payload.Source.DB,
		Username: payload.Source.Username,
		Password: payload.Source.Password,
	}).SearchRead(r.Context(), "helpdesk.ticket", domain, []string{"id"}, size, 0, "create_date asc")

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var tickets []odoo.Ticket
	_ = json.Unmarshal(ticketsRaw, &tickets)

	processed := 0
	errorsCount := 0

	for _, t := range tickets {
		_, err := mig.MigrateTicket(r.Context(), t.ID)
		if err != nil {
			errorsCount++
		} else {
			processed++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"processed": processed,
		"errors":    errorsCount,
		"total":     len(tickets),
	})
}

func (s *Server) handleMigrateAll(w http.ResponseWriter, r *http.Request) {
	var payload ConnectionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mig := s.getOrCreateMigrator(payload)
	_ = mig.InitSchemaDiscovery(r.Context())

	domain := []interface{}{
		[]interface{}{"create_date", ">=", payload.StartDate + " 00:00:00"},
	}

	srcClient := odoo.NewClient(odoo.ConnectionConfig{
		URL:      payload.Source.URL,
		DB:       payload.Source.DB,
		Username: payload.Source.Username,
		Password: payload.Source.Password,
	})

	ticketsRaw, err := srcClient.SearchRead(r.Context(), "helpdesk.ticket", domain, []string{"id"}, 0, 0, "create_date asc")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var tickets []odoo.Ticket
	_ = json.Unmarshal(ticketsRaw, &tickets)

	synced := 0
	for _, t := range tickets {
		if _, err := mig.MigrateTicket(r.Context(), t.ID); err == nil {
			synced++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{
		"total":  len(tickets),
		"synced": synced,
	})
}

func (s *Server) handleListMappings(w http.ResponseWriter, r *http.Request) {
	entity := r.URL.Query().Get("entity")
	mappings, err := s.db.ListMasterMappings(entity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(mappings)
}

func (s *Server) handleSaveMapping(w http.ResponseWriter, r *http.Request) {
	var m db.MasterMapping
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.db.SaveMasterMapping(&m); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleExportMappings(w http.ResponseWriter, r *http.Request) {
	mappings, err := s.db.ListMasterMappings("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	export := odoo.ExternalMappingsFile{
		Partners: make(map[string]int),
		Users:    make(map[string]int),
		Projects: make(map[string]int),
	}

	for _, m := range mappings {
		if m.TargetID > 0 {
			key := strconv.Itoa(m.SourceID)
			switch m.EntityType {
			case "res.partner":
				export.Partners[key] = m.TargetID
			case "res.users":
				export.Users[key] = m.TargetID
			case "project.project":
				export.Projects[key] = m.TargetID
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=\"mappings.json\"")
	json.NewEncoder(w).Encode(export)
}

func (s *Server) handleListSyncRecords(w http.ResponseWriter, r *http.Request) {
	records, err := s.db.ListSyncRecords(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

func (s *Server) handleListLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.db.GetRecentLogs(100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}
