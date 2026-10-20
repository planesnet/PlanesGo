package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDBSyncRecordsAndMappings(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migrar1_test_*")
	if err != nil {
		t.Fatalf("error creando temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	d, err := Open(dbPath)
	if err != nil {
		t.Fatalf("error abriendo base de datos: %v", err)
	}
	defer d.Close()

	// 1. Test SyncRecord Upsert & Idempotency
	rec := &SyncRecord{
		EntityType:   "helpdesk.ticket",
		SourceID:     101,
		TargetID:     5001,
		ContentHash:  "hash12345",
		Status:       "synced",
		ErrorMessage: "",
	}

	if err := d.SaveSyncRecord(rec); err != nil {
		t.Fatalf("error guardando sync record: %v", err)
	}

	got, err := d.GetSyncRecord("helpdesk.ticket", 101)
	if err != nil {
		t.Fatalf("error obteniendo sync record: %v", err)
	}
	if got == nil || got.TargetID != 5001 || got.Status != "synced" {
		t.Fatalf("registro obtenido no coincide: %+v", got)
	}

	// Actualización idempotente (mismo source_id, actualiza target o hash)
	rec.ContentHash = "hash99999"
	rec.ErrorMessage = "ninguno"
	if err := d.SaveSyncRecord(rec); err != nil {
		t.Fatalf("error actualizando sync record: %v", err)
	}

	got2, err := d.GetSyncRecord("helpdesk.ticket", 101)
	if err != nil || got2 == nil || got2.ContentHash != "hash99999" {
		t.Fatalf("actualización falló: %+v", got2)
	}

	// 2. Test MasterMappings
	mapping := &MasterMapping{
		EntityType:     "res.partner",
		SourceID:       202,
		SourceName:     "Cliente Acme S.L.",
		SourceKey:      "B12345678",
		TargetID:       888,
		TargetName:     "Acme S.L.",
		MatchCriterion: "vat",
		Status:         "resolved",
	}

	if err := d.SaveMasterMapping(mapping); err != nil {
		t.Fatalf("error guardando master mapping: %v", err)
	}

	gotMap, err := d.GetMasterMapping("res.partner", 202)
	if err != nil || gotMap == nil || gotMap.TargetID != 888 {
		t.Fatalf("error obteniendo master mapping: %+v", gotMap)
	}

	// 3. Test Logs
	d.Log("INFO", "helpdesk.ticket", 101, 5001, "Ticket migrado con éxito", "")
	logs, err := d.GetRecentLogs(10)
	if err != nil || len(logs) == 0 {
		t.Fatalf("error obteniendo logs: %v", err)
	}
	if logs[0].Message != "Ticket migrado con éxito" {
		t.Fatalf("log inesperado: %+v", logs[0])
	}

	// 4. Test Stats
	stats, err := d.GetStats()
	if err != nil {
		t.Fatalf("error obteniendo stats: %v", err)
	}
	if stats.TotalTickets != 1 || stats.SyncedTickets != 1 || stats.TotalMappings != 1 {
		t.Fatalf("stats inesperadas: %+v", stats)
	}
}
