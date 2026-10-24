package freerdp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExplain(t *testing.T) {
	for log, want := range map[string]string{
		"x ERRCONNECT_LOGON_FAILURE [0x00020014]": "Usuario o contraseña incorrectos.",
		"ERRCONNECT_CONNECT_FAILED":               "No se puede llegar",
		"ERRCONNECT_CONNECT_CANCELLED":            "",
		"fusermount3: mount failed: Operation not permitted\n[ERROR][com.freerdp.core] - [rdp_set_error_info]: ERRINFO_RPC_INITIATED_DISCONNECT [0x00010001]": "",
		"ERRINFO_DISCONNECTED_BY_OTHER_CONNECTION":                                            "otra conexión",
		"krb5 ... default realm\nfallo raro de verdad":                                        "fallo raro de verdad",
		"[ERROR][com.winpr.sspi.Kerberos] - Cannot find KDC for realm \"MAZANA\"\notro fallo": "otro fallo",
	} {
		got := Explain(log, 131)
		if want == "" && got != "" || !strings.Contains(got, want) || strings.Contains(got, "default realm") || strings.Contains(got, "KDC") {
			t.Errorf("%q -> %q", log, got)
		}
	}
	if Explain("", 0) != "" || Explain("", 12) != "" {
		t.Error("un cierre normal no es un error")
	}
}

func TestValidateTarget(t *testing.T) {
	for _, c := range []struct {
		host, user string
		ok         bool
	}{
		{"192.168.125.229", `MAZANA\planes`, true},
		{"sage.local:3390", "planes", true},
		{"127.0.0.1:3392", "prueba", true},
		{"", "u", false},
		{"-h", "u", false},
		{"/v:x", "u", false},
		{"h x", "u", false},
		{"h", "", false},
		{"h", "-u", false},
		{"h", `D\-u`, false},
		{"h", "u /p:x", false},
		{"h", `a\b\c`, false},
		{"h", "u,x", false},
	} {
		if err := ValidateTarget(c.host, c.user); (err == nil) != c.ok {
			t.Errorf("%q %q: %v", c.host, c.user, err)
		}
	}
}

func TestSecureArgs(t *testing.T) {
	a := strings.Join(SecureArgs("h:3390", `DOM\u`), " ")
	for _, want := range []string{"/v:h:3390", "/u:u", "/d:DOM", "/from-stdin:force", "/sec:nla", "/tls:seclevel:2", "/cert:tofu", "/gfx:AVC444"} {
		if !strings.Contains(a, want) {
			t.Errorf("falta %s en %s", want, a)
		}
	}
	if a := strings.Join(SecureArgs("h", "u"), " "); strings.Contains(a, "/d:") {
		t.Errorf("sin dominio: %s", a)
	}
}

// A certificate warning that did not stop the connection does not hide a wrong password.
func TestExplainLoginAfterCertificateWarning(t *testing.T) {
	log := "[tls_print_new_certificate_warn]: The host key for 127.0.0.1:3392 has changed\nHost key verification failed.\n[nla_recv_pdu]: ERRCONNECT_LOGON_FAILURE [0x00020014]"
	if got := Explain(log, 131); got != "Usuario o contraseña incorrectos." {
		t.Errorf("%q", got)
	}
	if got := Explain("certificate has changed\nERRCONNECT_TLS_CONNECT_FAILED", 131); !strings.Contains(got, "certificado") {
		t.Errorf("%q", got)
	}
}

// Each connection has its own shared folder, private (700), and nothing outside the connectors' folder is named.
func TestExchangePath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a, _ := ExchangePath("agente", "589", "s1")
	b, _ := ExchangePath("agente", "590", "s1")
	if a == b || !strings.HasSuffix(a, filepath.Join("intercambio", "agente", "589", "s1")) {
		t.Errorf("%s %s", a, b)
	}
	for _, bad := range [][]string{{".."}, {"a/b"}, {""}, {"agente", "../../x"}} {
		if _, err := ExchangePath(bad...); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if r, _ := RecordingsPath("agente", "589", "s1"); strings.HasPrefix(r, ExchangeDir()) {
		t.Error("the recordings must not be in a shared folder")
	}
	if args := DriveArgs(a); len(args) != 1 || args[0] != "/drive:Intercambio,"+a {
		t.Fatalf("drive: %v", args)
	}
	for d := a; d != filepath.Dir(dataDir()); d = filepath.Dir(d) {
		if st, err := os.Stat(d); err != nil || st.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v", d, st.Mode().Perm(), err)
		}
	}
}
