package winrm

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/text/encoding/unicode"

	wsman "github.com/masterzen/winrm"
)

func TestValidate(t *testing.T) {
	for _, c := range []Config{
		{Host: "192.168.125.229", User: `MAZANA\luis`, Password: "x"},
		{Host: "sage.mazana.local:5986", User: "Administrador", Password: "x", HTTPS: true},
		{Host: "[fe80::1]:5985", User: "luis", Password: "x"},
		{Host: "SAGE", User: `MAZANA\José Pérez`, Password: "x"},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
	for _, c := range []Config{
		{Host: "-oProxyCommand=x", User: "u", Password: "x"},
		{Host: "host;rm", User: "u", Password: "x"},
		{Host: "host:99999", User: "u", Password: "x"},
		{Host: "host:0", User: "u", Password: "x"},
		{Host: "http://host", User: "u", Password: "x"},
		{Host: "h", User: "", Password: "x"},
		{Host: "h", User: "-u", Password: "x"},
		{Host: "h", User: `a\b\c`, Password: "x"},
		{Host: "h", User: "u\nx", Password: "x"},
		{Host: "h", User: "u"},
	} {
		if c.Validate() == nil {
			t.Errorf("%+v should be refused", c)
		}
	}
}

func TestEndpointAndTransport(t *testing.T) {
	c := Config{Host: "sage", User: `MAZANA\luis`, Password: "secreto"}
	if h, p := c.hostPort(); h != "sage" || p != HTTPPort {
		t.Errorf("http default: %s %d", h, p)
	}
	ep := c.endpoint()
	if ep.HTTPS || ep.Insecure || ep.Port != 5985 || ep.Timeout != responseTimeout {
		t.Errorf("http endpoint: %+v", ep)
	}
	params, err := c.params()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := params.TransportDecorator().(*wsman.Encryption); !ok || params.Timeout != operationTimeout {
		t.Errorf("HTTP must use NTLM message encryption: %T", params.TransportDecorator())
	}

	c.HTTPS = true
	if ep := c.endpoint(); !ep.HTTPS || ep.Insecure || ep.Port != HTTPSPort {
		t.Errorf("https endpoint validates the certificate: %+v", ep)
	}
	params, _ = c.params()
	if _, ok := params.TransportDecorator().(*wsman.ClientNTLM); !ok {
		t.Errorf("HTTPS: NTLM inside TLS: %T", params.TransportDecorator())
	}
	c.TrustCertificate, c.Host = true, "[fe80::1]:6000"
	if ep := c.endpoint(); !ep.Insecure || ep.Port != 6000 || ep.Host != "fe80::1" {
		t.Errorf("trusted certificate and port: %+v", ep)
	}
	// TrustCertificate means nothing over HTTP.
	if ep := (Config{Host: "h", TrustCertificate: true}).endpoint(); ep.Insecure {
		t.Error("insecure over http")
	}
	if s := c.String(); strings.Contains(s, "secreto") || s != `MAZANA\luis@[fe80::1]:6000 (https)` {
		t.Errorf("String: %s", s)
	}
}

// decodeCommand: the PowerShell script of an encoded command line.
func decodeCommand(t *testing.T, cmd string) string {
	t.Helper()
	const prefix = "powershell.exe -NoLogo -NoProfile -NonInteractive -EncodedCommand "
	b64, ok := strings.CutPrefix(cmd, prefix)
	if !ok {
		t.Fatalf("command: %s", cmd)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	s, err := unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM).NewDecoder().String(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCommand(t *testing.T) {
	ro, err := command(true, true)
	if err != nil {
		t.Fatal(err)
	}
	s := decodeCommand(t, ro)
	for _, want := range []string{"[Console]::OutputEncoding = [Text.Encoding]::UTF8", "$pgoInput = $input",
		"LanguageMode = 'ConstrainedLanguage'", "Invoke-Expression $pgoScript | Out-String -Stream", "exit 1"} {
		if !strings.Contains(s, want) {
			t.Errorf("read-only bootstrap without %q:\n%s", want, s)
		}
	}
	if strings.Index(s, "ConstrainedLanguage") > strings.Index(s, "Invoke-Expression") {
		t.Error("the language mode has to be set before running the script")
	}
	rw, _ := command(false, false)
	if s := decodeCommand(t, rw); strings.Contains(s, "ConstrainedLanguage") || strings.Contains(s, "Out-String") {
		t.Errorf("full bootstrap:\n%s", s)
	}
	if len(ro) > 2000 {
		t.Errorf("the command line has to stay short (the script goes by stdin): %d", len(ro))
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		`C:\Program Files\x.log`: `'C:\Program Files\x.log'`,
		`C:\it's`:                `'C:\it''s'`,
		"C:\\a\u2019b":           "'C:\\a\u2019\u2019b'",
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "  ", "a\nb", "a\x00b", strings.Repeat("a", 1025)} {
		if validRemotePath(bad) == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

func TestCapped(t *testing.T) {
	c := &capped{max: 5}
	c.Write([]byte("abc"))
	if n, err := c.Write([]byte("defgh")); n != 5 || err != nil {
		t.Fatalf("write: %d %v", n, err)
	}
	c.Write([]byte("ijk"))
	if c.String() != "abcde" || !c.truncated {
		t.Errorf("capped: %q %v", c.String(), c.truncated)
	}
}

func TestBase64Lines(t *testing.T) {
	data := bytes.Repeat([]byte("año €\x00\xff"), 5000)
	var stream bytes.Buffer
	for off := 0; off < len(data); off += 999 {
		stream.WriteString(base64.StdEncoding.EncodeToString(data[off:min(off+999, len(data))]) + "\r\n")
	}
	var out bytes.Buffer
	d := &base64Lines{w: &out, max: MaxTransfer}
	for b := stream.Bytes(); len(b) > 0; { // in uneven pieces, as the output arrives
		n := min(777, len(b))
		d.Write(b[:n])
		b = b[n:]
	}
	if err := d.flush(); err != nil || !bytes.Equal(out.Bytes(), data) || d.n != int64(len(data)) {
		t.Fatalf("decoded %d bytes, err %v", out.Len(), err)
	}
	small := &base64Lines{w: &bytes.Buffer{}, max: 10}
	small.Write([]byte(base64.StdEncoding.EncodeToString(make([]byte, 11)) + "\n"))
	if small.flush() == nil {
		t.Error("over the limit")
	}
	bad := &base64Lines{w: &bytes.Buffer{}, max: 10}
	bad.Write([]byte("no es base64!\n"))
	if bad.flush() == nil {
		t.Error("invalid base64")
	}
}

func TestCleanErrors(t *testing.T) {
	in := "#< CLIXML\r\n<Objs Version=\"1.1.0.1\" xmlns=\"http://schemas.microsoft.com/powershell/2004/04\">" +
		"<Obj S=\"progress\" RefId=\"0\"><TN RefId=\"0\"><T>System.Management.Automation.PSCustomObject</T></TN><MS><I64 N=\"SourceId\">1</I64></MS></Obj>" +
		"<S S=\"Error\">Get-Item : No se encuentra la ruta de acceso 'C:\\no' porque no existe._x000D__x000A_</S>" +
		"<S S=\"Error\">+ CategoryInfo : ObjectNotFound_x000D__x000A_</S>" +
		"<S S=\"warning\">cuidado_x000D__x000A_</S></Objs>"
	want := "Get-Item : No se encuentra la ruta de acceso 'C:\\no' porque no existe.\n+ CategoryInfo : ObjectNotFound\nADVERTENCIA: cuidado\n"
	if got := cleanErrors(in); got != want {
		t.Errorf("cleanErrors:\n%q\nwant\n%q", got, want)
	}
	if got := cleanErrors("plain error\n"); got != "plain error\n" {
		t.Errorf("plain text stays: %q", got)
	}
}

func TestExplain(t *testing.T) {
	for in, want := range map[string]string{
		"http error 401: ":                                                         "contraseña",
		"NTLM negotiation returned HTTP 401":                                       "contraseña",
		"x509: certificate signed by unknown authority":                            "certificado",
		"dial tcp 10.0.0.1:5985: connect: connection refused":                      "no responde",
		"ntlmssp: negotiated 40-bit key is below the required minimum of 128 bits": "HTTPS",
	} {
		if got := explain(errorString(in)).Error(); !strings.Contains(got, want) {
			t.Errorf("explain(%q) = %q", in, got)
		}
	}
	if explain(nil) != nil {
		t.Error("nil")
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
