//go:build integracion

// As in cmd/pgocode: SQL Server's own certificate has a negative serial number.
//
//go:debug x509negativeserial=1
package sqlserver

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
)

// Against a real SQL Server: PGO_SQLTEST_SERVER (host:port) and PGO_SQLTEST_PASSWORD (of «sa»), e.g. a container
// of mcr.microsoft.com/mssql/server. go test -tags integracion ./internal/sqlserver
func TestRealServer(t *testing.T) {
	server, pw := os.Getenv("PGO_SQLTEST_SERVER"), os.Getenv("PGO_SQLTEST_PASSWORD")
	if server == "" || pw == "" {
		t.Skip("sin PGO_SQLTEST_SERVER ni PGO_SQLTEST_PASSWORD")
	}
	ctx := context.Background()
	cfg := Config{Server: server, User: "sa", Password: pw, TrustCertificate: true}

	if _, err := Open(ctx, Config{Server: server, User: "sa", Password: "mala", TrustCertificate: true}); err == nil || !strings.Contains(err.Error(), "contraseña") {
		t.Fatalf("wrong password: %v", err)
	}
	ro, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if !strings.Contains(ro.Server, ".") || strings.Contains(ro.Config().Password, pw) {
		t.Errorf("server %q", ro.Server)
	}

	r, err := ro.Run(ctx, "SELECT 1 AS uno, N'año €' AS t, CAST(12.50 AS decimal(5,2)) AS d, CAST('6F9619FF-8B86-D011-B42D-00C04FC964FF' AS uniqueidentifier) AS g, CAST('2026-10-24T13:05:00' AS datetime2) AS f, CAST(NULL AS int) AS n", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	row := r.Sets[0].Rows[0]
	if row[0] != int64(1) || row[1] != "año €" || row[2] != "12.50" || row[3] != "6F9619FF-8B86-D011-B42D-00C04FC964FF" || row[4] != "2026-10-24T13:05:00" || row[5] != nil {
		t.Errorf("values: %#v (types %v)", row, r.Sets[0].Types)
	}

	// Read only: what the batch does is undone.
	r, err = ro.Run(ctx, "CREATE TABLE dbo.pgo_prueba(a int); INSERT dbo.pgo_prueba VALUES (1),(2); SELECT a FROM dbo.pgo_prueba", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sets) != 1 || len(r.Sets[0].Rows) != 2 || !r.RolledBack {
		t.Fatalf("read only: %+v", r)
	}
	r, _ = ro.Run(ctx, "SELECT OBJECT_ID('dbo.pgo_prueba') AS id", 0, false)
	if r.Sets[0].Rows[0][0] != nil {
		t.Fatal("read only: the table must not stay")
	}
	if _, err := ro.Run(ctx, "INSERT dbo.x VALUES (1); COMMIT", 0, false); err == nil || !strings.Contains(err.Error(), "COMMIT") {
		t.Errorf("COMMIT: %v", err)
	}
	if _, err := ro.Run(ctx, "EXEC('COMMIT'); SELECT 1", 0, false); err == nil || !strings.Contains(err.Error(), "dinámico") {
		t.Errorf("dynamic SQL: %v", err)
	}
	// A procedure with its own COMMIT gets past the check: the batch must not end as if it had been undone.
	w, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	w.SetAllowChanges(true)
	if _, err := w.Run(ctx, "CREATE OR ALTER PROCEDURE dbo.pgo_commit AS COMMIT", 0, true); err != nil {
		t.Fatal(err)
	}
	defer w.Run(ctx, "DROP PROCEDURE IF EXISTS dbo.pgo_commit", 0, true)
	if r, err := ro.Run(ctx, "EXEC dbo.pgo_commit", 0, false); err == nil {
		t.Errorf("a procedure that commits passes as read only: %+v", r)
	} else {
		t.Logf("procedure with COMMIT: %v", err)
	}
	// One that commits and opens another transaction leaves the count as it was: the transaction id gives it away.
	if _, err := w.Run(ctx, "CREATE OR ALTER PROCEDURE dbo.pgo_commit2 AS BEGIN COMMIT; BEGIN TRAN END", 0, true); err != nil {
		t.Fatal(err)
	}
	defer w.Run(ctx, "DROP PROCEDURE IF EXISTS dbo.pgo_commit2", 0, true)
	if r, err := ro.Run(ctx, "EXEC dbo.pgo_commit2", 0, false); err == nil || !strings.Contains(err.Error(), "terminado") {
		t.Errorf("a procedure that commits and reopens passes as read only: %+v %v", r, err)
	}

	// Several result sets and the row limit.
	r, err = ro.Run(ctx, "SELECT TOP 10 name FROM sys.objects; SELECT 2 AS dos", 3, false)
	if err != nil || len(r.Sets) != 2 || len(r.Sets[0].Rows) != 3 || !r.Sets[0].Truncated || r.Sets[1].Rows[0][0] != int64(2) {
		t.Fatalf("sets: %+v %v", r, err)
	}
	if _, err := ro.Run(ctx, "SELECT * FROM no_existe", 0, false); err == nil || !strings.Contains(err.Error(), "SQL Server 208") {
		t.Errorf("error: %v", err)
	}

	// Changes allowed: they stay.
	cfg.AllowChanges = true
	rw, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rw.Close()
	if _, err := rw.Run(ctx, "CREATE TABLE dbo.pgo_prueba(a int); INSERT dbo.pgo_prueba VALUES (1),(2),(3)", 0, true); err != nil {
		t.Fatal(err)
	}
	defer rw.Run(ctx, "DROP TABLE dbo.pgo_prueba", 0, true)
	r, _ = rw.Run(ctx, "SELECT COUNT(*) AS n FROM dbo.pgo_prueba", 0, false)
	if r.Sets[0].Rows[0][0] != int64(3) {
		t.Errorf("with changes allowed the rows stay: %+v", r)
	}
	// Databases: listed, chosen while connected, and the batches run there.
	rw.Run(ctx, "IF DB_ID('pgo_prueba_db') IS NULL CREATE DATABASE pgo_prueba_db", 0, true)
	defer rw.Run(ctx, "USE master; DROP DATABASE pgo_prueba_db", 0, true)
	if l, err := rw.Databases(ctx); err != nil || !slices.Contains(l, "pgo_prueba_db") || !slices.Contains(l, "master") {
		t.Fatalf("databases: %v %v", l, err)
	}
	if err := rw.SetDatabase(ctx, "pgo_prueba_db"); err != nil {
		t.Fatal(err)
	}
	r, _ = rw.Run(ctx, "SELECT DB_NAME() AS db", 0, false)
	if r.Sets[0].Rows[0][0] != "pgo_prueba_db" || rw.Config().Database != "pgo_prueba_db" {
		t.Errorf("chosen database: %+v", r)
	}
	if err := rw.SetDatabase(ctx, "no_existe"); err == nil {
		t.Error("a database the login cannot use must be refused")
	}
	rw.Run(ctx, "USE pgo_prueba_db", 0, true) // a write batch that moves its connection
	rw.SetDatabase(ctx, "")
	r, _ = rw.Run(ctx, "SELECT DB_NAME() AS db", 0, true)
	if r.Sets[0].Rows[0][0] != "master" {
		t.Errorf("back to the login's default: %+v", r)
	}
	// Switched to read only while open: a write is undone.
	rw.SetAllowChanges(false)
	rw.Run(ctx, "DELETE FROM dbo.pgo_prueba", 0, true)
	r, _ = rw.Run(ctx, "SELECT COUNT(*) AS n FROM dbo.pgo_prueba", 0, false)
	if r.Sets[0].Rows[0][0] != int64(3) || rw.Config().AllowChanges {
		t.Errorf("switched to read only: %+v", r)
	}
	rw.SetAllowChanges(true)
	// write=false on a connection that allows changes: still undone.
	rw.Run(ctx, "DELETE FROM dbo.pgo_prueba", 0, false)
	r, _ = rw.Run(ctx, "SELECT COUNT(*) AS n FROM dbo.pgo_prueba", 0, false)
	if r.Sets[0].Rows[0][0] != int64(3) {
		t.Errorf("a batch without write is undone: %+v", r)
	}
}
