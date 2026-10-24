package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPsArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "x.ps1")
	os.WriteFile(script, []byte("Get-Service\nGet-Date"), 0o644)
	local := filepath.Join(dir, "datos.txt")
	os.WriteFile(local, []byte("hola"), 0o644)

	r, err := psArgs([]string{"Get-ChildItem 'C:\\Program Files'"})
	if err != nil || r.endpoint != "ps" || r.body["script"] != "Get-ChildItem 'C:\\Program Files'" || r.body["escribir"] != false {
		t.Errorf("script: %+v %v", r, err)
	}
	r, err = psArgs([]string{"-f", script, "--escribir"})
	if err != nil || r.body["script"] != "Get-Service\nGet-Date" || r.body["escribir"] != true {
		t.Errorf("-f: %+v %v", r, err)
	}
	r, err = psArgs([]string{"baja", `C:\logs\a.log`, "a.log"})
	if err != nil || r.endpoint != "ps-download" || r.body["path"] != `C:\logs\a.log` || r.local != "a.log" {
		t.Errorf("baja: %+v %v", r, err)
	}
	r, err = psArgs([]string{"sube", local, `C:\tmp\datos.txt`, "--escribir"})
	if err != nil || r.endpoint != "ps-upload" || r.body["path"] != `C:\tmp\datos.txt` || r.body["data"] != base64.StdEncoding.EncodeToString([]byte("hola")) || r.body["escribir"] != true {
		t.Errorf("sube: %+v %v", r, err)
	}
	big := filepath.Join(dir, "big")
	os.WriteFile(big, make([]byte, maxPsTransfer+1), 0o644)
	for _, bad := range [][]string{nil, {""}, {"a", "b"}, {"-f"}, {"-f", filepath.Join(dir, "no")}, {"--filas", "3"},
		{"baja", "x"}, {"sube", local}, {"sube", big, "C:\\x", "--escribir"}} {
		if _, err := psArgs(bad); err == nil {
			t.Errorf("%v should fail", bad)
		}
	}
}

func TestPsText(t *testing.T) {
	out, errText := psText(psResult{Stdout: "a\r\nb\r\n\r\n\r\n", ExitCode: 0})
	if out != "a\nb\n" || errText != "" {
		t.Errorf("ok: %q %q", out, errText)
	}
	out, errText = psText(psResult{Stderr: "Get-Item : no existe\r\n", ExitCode: 1, Truncated: true})
	if out != "" || !strings.HasPrefix(errText, "Get-Item : no existe\n(código de salida 1; salida recortada") {
		t.Errorf("error: %q %q", out, errText)
	}
}
