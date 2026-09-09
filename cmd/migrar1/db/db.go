package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
	mu sync.RWMutex
}

type SyncRecord struct {
	ID           int64     `json:"id"`
	EntityType   string    `json:"entity_type"` // "helpdesk.ticket" o "account.analytic.line"
	SourceID     int       `json:"source_id"`
	TargetID     int       `json:"target_id"`
	ContentHash  string    `json:"content_hash"`
	Status       string    `json:"status"` // "synced", "pending", "error", "skipped"
	ErrorMessage string    `json:"error_message,omitempty"`
	SyncedAt     time.Time `json:"synced_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MasterMapping struct {
	ID             int64     `json:"id"`
	EntityType     string    `json:"entity_type"` // "res.partner", "res.users", "project.project"
	SourceID       int       `json:"source_id"`
	SourceName     string    `json:"source_name"`
	SourceKey      string    `json:"source_key"` // DNI/VAT, Email, etc.
	TargetID       int       `json:"target_id"`
	TargetName     string    `json:"target_name"`
	MatchCriterion string    `json:"match_criterion"` // "vat", "email", "name", "manual"
	Status         string    `json:"status"`          // "resolved", "pending", "ignored"
	UpdatedAt      time.Time `json:"updated_at"`
}

type MigrationLog struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"` // "INFO", "WARN", "ERROR", "DEBUG"
	Entity    string    `json:"entity"`
	SourceID  int       `json:"source_id,omitempty"`
	TargetID  int       `json:"target_id,omitempty"`
	Message   string    `json:"message"`
	Payload   string    `json:"payload,omitempty"`
}

func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = "migrar1.db"
	}
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("error al abrir base de datos SQLite %s: %w", dbPath, err)
	}

	d := &DB{db: sqlDB}
	if err := d.initSchema(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("error al inicializar esquema SQLite: %w", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) initSchema() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS sync_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			source_id INTEGER NOT NULL,
			target_id INTEGER NOT NULL DEFAULT 0,
			content_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			error_message TEXT NOT NULL DEFAULT '',
			synced_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(entity_type, source_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sync_records_entity_src ON sync_records(entity_type, source_id);`,
		`CREATE INDEX IF NOT EXISTS idx_sync_records_status ON sync_records(status);`,

		`CREATE TABLE IF NOT EXISTS master_mappings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_type TEXT NOT NULL,
			source_id INTEGER NOT NULL,
			source_name TEXT NOT NULL DEFAULT '',
			source_key TEXT NOT NULL DEFAULT '',
			target_id INTEGER NOT NULL DEFAULT 0,
			target_name TEXT NOT NULL DEFAULT '',
			match_criterion TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			updated_at DATETIME NOT NULL,
			UNIQUE(entity_type, source_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_master_mappings_lookup ON master_mappings(entity_type, source_id);`,

		`CREATE TABLE IF NOT EXISTS migration_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			level TEXT NOT NULL,
			entity TEXT NOT NULL DEFAULT '',
			source_id INTEGER NOT NULL DEFAULT 0,
			target_id INTEGER NOT NULL DEFAULT 0,
			message TEXT NOT NULL,
			payload TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE INDEX IF NOT EXISTS idx_migration_logs_time ON migration_logs(timestamp DESC);`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return fmt.Errorf("error ejecutando consulta '%s': %w", q, err)
		}
	}

	return nil
}

func CalculateHash(data interface{}) (string, error) {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

// GetSyncRecord obtiene el registro de sincronización por entidad y source_id
func (d *DB) GetSyncRecord(entityType string, sourceID int) (*SyncRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var r SyncRecord
	var syncedAt sql.NullTime
	err := d.db.QueryRow(
		`SELECT id, entity_type, source_id, target_id, content_hash, status, error_message, synced_at, updated_at
		 FROM sync_records WHERE entity_type = ? AND source_id = ?`,
		entityType, sourceID,
	).Scan(&r.ID, &r.EntityType, &r.SourceID, &r.TargetID, &r.ContentHash, &r.Status, &r.ErrorMessage, &syncedAt, &r.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if syncedAt.Valid {
		r.SyncedAt = syncedAt.Time
	}
	return &r, nil
}

// SaveSyncRecord guarda o actualiza un registro de sincronización de manera idempotente
func (d *DB) SaveSyncRecord(rec *SyncRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	rec.UpdatedAt = now
	if rec.Status == "synced" && rec.SyncedAt.IsZero() {
		rec.SyncedAt = now
	}

	query := `INSERT INTO sync_records (entity_type, source_id, target_id, content_hash, status, error_message, synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, source_id) DO UPDATE SET
			target_id = excluded.target_id,
			content_hash = excluded.content_hash,
			status = excluded.status,
			error_message = excluded.error_message,
			synced_at = COALESCE(excluded.synced_at, sync_records.synced_at),
			updated_at = excluded.updated_at;`

	var syncedAt interface{}
	if !rec.SyncedAt.IsZero() {
		syncedAt = rec.SyncedAt
	}

	_, err := d.db.Exec(query, rec.EntityType, rec.SourceID, rec.TargetID, rec.ContentHash, rec.Status, rec.ErrorMessage, syncedAt, rec.UpdatedAt)
	return err
}

// GetMasterMapping obtiene un mapeo maestro existente
func (d *DB) GetMasterMapping(entityType string, sourceID int) (*MasterMapping, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var m MasterMapping
	err := d.db.QueryRow(
		`SELECT id, entity_type, source_id, source_name, source_key, target_id, target_name, match_criterion, status, updated_at
		 FROM master_mappings WHERE entity_type = ? AND source_id = ?`,
		entityType, sourceID,
	).Scan(&m.ID, &m.EntityType, &m.SourceID, &m.SourceName, &m.SourceKey, &m.TargetID, &m.TargetName, &m.MatchCriterion, &m.Status, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SaveMasterMapping registra o actualiza un mapeo maestro
func (d *DB) SaveMasterMapping(m *MasterMapping) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	m.UpdatedAt = time.Now()
	query := `INSERT INTO master_mappings (entity_type, source_id, source_name, source_key, target_id, target_name, match_criterion, status, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, source_id) DO UPDATE SET
			source_name = excluded.source_name,
			source_key = excluded.source_key,
			target_id = excluded.target_id,
			target_name = excluded.target_name,
			match_criterion = excluded.match_criterion,
			status = excluded.status,
			updated_at = excluded.updated_at;`

	_, err := d.db.Exec(query, m.EntityType, m.SourceID, m.SourceName, m.SourceKey, m.TargetID, m.TargetName, m.MatchCriterion, m.Status, m.UpdatedAt)
	return err
}

// ListMasterMappings devuelve todos los mapeos de una entidad o todos
func (d *DB) ListMasterMappings(entityType string) ([]MasterMapping, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var rows *sql.Rows
	var err error
	if entityType != "" {
		rows, err = d.db.Query(`SELECT id, entity_type, source_id, source_name, source_key, target_id, target_name, match_criterion, status, updated_at
			FROM master_mappings WHERE entity_type = ? ORDER BY status ASC, source_name ASC`, entityType)
	} else {
		rows, err = d.db.Query(`SELECT id, entity_type, source_id, source_name, source_key, target_id, target_name, match_criterion, status, updated_at
			FROM master_mappings ORDER BY entity_type ASC, status ASC, source_name ASC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MasterMapping
	for rows.Next() {
		var m MasterMapping
		if err := rows.Scan(&m.ID, &m.EntityType, &m.SourceID, &m.SourceName, &m.SourceKey, &m.TargetID, &m.TargetName, &m.MatchCriterion, &m.Status, &m.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, nil
}

// Log registra un evento en la auditoría
func (d *DB) Log(level, entity string, sourceID, targetID int, message, payload string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	_, _ = d.db.Exec(
		`INSERT INTO migration_logs (timestamp, level, entity, source_id, target_id, message, payload)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		now, level, entity, sourceID, targetID, message, payload,
	)
}

// GetRecentLogs obtiene los últimos N logs
func (d *DB) GetRecentLogs(limit int) ([]MigrationLog, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := d.db.Query(
		`SELECT id, timestamp, level, entity, source_id, target_id, message, payload
		 FROM migration_logs ORDER BY id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []MigrationLog
	for rows.Next() {
		var l MigrationLog
		if err := rows.Scan(&l.ID, &l.Timestamp, &l.Level, &l.Entity, &l.SourceID, &l.TargetID, &l.Message, &l.Payload); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// SyncStats devuelve estadísticas de sincronización
type SyncStats struct {
	TotalTickets     int `json:"total_tickets"`
	SyncedTickets    int `json:"synced_tickets"`
	PendingTickets   int `json:"pending_tickets"`
	ErrorTickets     int `json:"error_tickets"`
	TotalTimesheets  int `json:"total_timesheets"`
	SyncedTimesheets int `json:"synced_timesheets"`
	PendingMappings  int `json:"pending_mappings"`
	TotalMappings    int `json:"total_mappings"`
}

func (d *DB) GetStats() (SyncStats, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var stats SyncStats

	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'helpdesk.ticket'`).Scan(&stats.TotalTickets)
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'helpdesk.ticket' AND status = 'synced'`).Scan(&stats.SyncedTickets)
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'helpdesk.ticket' AND status = 'pending'`).Scan(&stats.PendingTickets)
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'helpdesk.ticket' AND status = 'error'`).Scan(&stats.ErrorTickets)

	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'account.analytic.line'`).Scan(&stats.TotalTimesheets)
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM sync_records WHERE entity_type = 'account.analytic.line' AND status = 'synced'`).Scan(&stats.SyncedTimesheets)

	_ = d.db.QueryRow(`SELECT COUNT(*) FROM master_mappings`).Scan(&stats.TotalMappings)
	_ = d.db.QueryRow(`SELECT COUNT(*) FROM master_mappings WHERE status = 'pending'`).Scan(&stats.PendingMappings)

	return stats, nil
}

// ListSyncRecords devuelve los registros de sincronización
func (d *DB) ListSyncRecords(limit int) ([]SyncRecord, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	rows, err := d.db.Query(
		`SELECT id, entity_type, source_id, target_id, content_hash, status, error_message, synced_at, updated_at
		 FROM sync_records ORDER BY updated_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []SyncRecord
	for rows.Next() {
		var r SyncRecord
		var syncedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.EntityType, &r.SourceID, &r.TargetID, &r.ContentHash, &r.Status, &r.ErrorMessage, &syncedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		if syncedAt.Valid {
			r.SyncedAt = syncedAt.Time
		}
		records = append(records, r)
	}
	return records, nil
}
