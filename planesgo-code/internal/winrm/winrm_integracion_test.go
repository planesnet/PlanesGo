//go:build integracion

package winrm

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// Against a real Windows with WinRM: PGO_WINRMTEST_HOST (host[:port]), PGO_WINRMTEST_USER (user or DOMAIN\user),
// PGO_WINRMTEST_PASSWORD and, for HTTPS, PGO_WINRMTEST_HTTPS=1 (PGO_WINRMTEST_TRUST=1 for a self-signed certificate).
// Use a test computer, never a client's. go test -tags integracion ./internal/winrm
func TestRealServer(t *testing.T) {
	host, user, pw := os.Getenv("PGO_WINRMTEST_HOST"), os.Getenv("PGO_WINRMTEST_USER"), os.Getenv("PGO_WINRMTEST_PASSWORD")
	if host == "" || user == "" || pw == "" {
		t.Skip("sin PGO_WINRMTEST_HOST, PGO_WINRMTEST_USER ni PGO_WINRMTEST_PASSWORD")
	}
	ctx := context.Background()
	cfg := Config{Host: host, User: user, Password: pw, HTTPS: os.Getenv("PGO_WINRMTEST_HTTPS") == "1", TrustCertificate: os.Getenv("PGO_WINRMTEST_TRUST") == "1"}

	bad := cfg
	bad.Password = "mala-" + pw
	if _, err := Open(ctx, bad); err == nil || !strings.Contains(err.Error(), "contraseña") {
		t.Fatalf("wrong password: %v", err)
	}
	ro, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if ro.Server == "" || strings.Contains(ro.Config().Password, pw) {
		t.Errorf("server %q", ro.Server)
	}

	// Output in UTF-8, errors as text, exit codes.
	r, err := ro.Run(ctx, "'año €'; Get-Date -Format yyyy", false)
	if err != nil || !strings.Contains(r.Stdout, "año €") || r.ExitCode != 0 || !r.ReadOnly {
		t.Fatalf("run: %+v %v", r, err)
	}
	r, err = ro.Run(ctx, `Get-Item C:\no-existe-pgo`, false)
	if err != nil || r.ExitCode == 0 || !strings.Contains(r.Stderr, "no-existe-pgo") || strings.Contains(r.Stderr, "CLIXML") {
		t.Errorf("error stream: %+v %v", r, err)
	}
	if _, err := ro.Run(ctx, `New-Item $env:TEMP\pgo.txt`, true); err == nil {
		t.Error("read only: New-Item must be refused")
	}
	// ConstrainedLanguage: .NET beyond the core types is out even if the check let it through.
	r, _ = ro.Run(ctx, "$ExecutionContext.SessionState.LanguageMode", false)
	if !strings.Contains(r.Stdout, "ConstrainedLanguage") {
		t.Errorf("language mode: %q", r.Stdout)
	}

	// With changes allowed: write, upload and download a file (non-ASCII bytes and more than one chunk).
	cfg.AllowChanges = true
	rw, err := Open(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer rw.Close()
	tmp, err := rw.Run(ctx, "$env:TEMP", true)
	if err != nil || tmp.Stdout == "" {
		t.Fatalf("temp: %+v %v", tmp, err)
	}
	remote := strings.TrimSpace(tmp.Stdout) + `\pgo-winrm-prueba.bin`
	defer rw.Run(ctx, "Remove-Item -LiteralPath '"+remote+"' -ErrorAction SilentlyContinue", true)
	data := bytes.Repeat([]byte("año €\x00\xff\r\n"), 20000)
	if n, err := rw.Upload(ctx, bytes.NewReader(data), remote); err != nil || n != int64(len(data)) {
		t.Fatalf("upload: %d %v", n, err)
	}
	var back bytes.Buffer
	if n, err := ro.Download(ctx, remote, &back); err != nil || n != int64(len(data)) || !bytes.Equal(back.Bytes(), data) {
		t.Fatalf("download: %d %v", n, err)
	}
	if _, err := ro.Upload(ctx, bytes.NewReader(data), remote); err == nil {
		t.Error("upload on a read-only connection")
	}
	// A long script goes by the standard input, not the command line.
	long := strings.Repeat("$x = 1\n", 3000) + "'fin'"
	if r, err := rw.Run(ctx, long, false); err != nil || strings.TrimSpace(r.Stdout) != "fin" {
		t.Errorf("long script: %+v %v", r, err)
	}
	if r, _ := rw.Run(ctx, "exit 7", true); r.ExitCode != 7 {
		t.Errorf("exit code: %d", r.ExitCode)
	}
}
