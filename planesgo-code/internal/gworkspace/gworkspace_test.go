package gworkspace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGoogle: tokeninfo, Drive y Sheets simulados. Los tokens válidos y sus ámbitos los decide la prueba.
type fakeGoogle struct {
	mu        sync.Mutex
	scopes    map[string]string // token → ámbitos
	expired   map[string]bool   // token que la API rechaza con 401
	needQuota bool              // Drive exige x-goog-user-project
	noSheets  bool              // la API de Sheets no está habilitada
	calls     []string
	quotaSeen string
}

func (g *fakeGoogle) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tokeninfo", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		g.mu.Lock()
		sc, ok := g.scopes[r.PostForm.Get("access_token")]
		g.mu.Unlock()
		if r.URL.Query().Get("access_token") != "" {
			t.Error("el token no debe ir en la URL")
		}
		if !ok {
			w.WriteHeader(400)
			w.Write([]byte(`{"error_description":"Invalid Value"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"scope": sc, "email": "luis@planesnet.com", "expires_in": "3599"})
	})
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		g.mu.Lock()
		defer g.mu.Unlock()
		g.calls = append(g.calls, r.Method+" "+r.URL.Path)
		if r.Method != http.MethodGet {
			t.Errorf("petición que no es de lectura: %s %s", r.Method, r.URL.Path)
		}
		if _, ok := g.scopes[tok]; !ok || g.expired[tok] {
			w.WriteHeader(401)
			w.Write([]byte(`{"error":{"code":401,"message":"Invalid Credentials"}}`))
			return false
		}
		g.quotaSeen = r.Header.Get("x-goog-user-project")
		if g.needQuota && g.quotaSeen == "" {
			w.WriteHeader(403)
			w.Write([]byte(`{"error":{"code":403,"message":"Drive API has not been used in project 764086051850","details":[{"reason":"SERVICE_DISABLED"}]}}`))
			return false
		}
		return true
	}
	files := map[string]File{
		"DOC1234567890":  {ID: "DOC1234567890", Name: "Informe", MimeType: mimeDoc, ModifiedTime: "2026-10-01T10:00:00Z"},
		"SHEET123456789": {ID: "SHEET123456789", Name: "Datos", MimeType: mimeSheet, ModifiedTime: "2026-10-02T10:00:00Z"},
		"PDF12345678901": {ID: "PDF12345678901", Name: "Factura.pdf", MimeType: "application/pdf", Size: "1234", WebViewLink: "https://drive.google.com/file/d/PDF12345678901/view"},
		"FOLDER12345678": {ID: "FOLDER12345678", Name: "Proyectos", MimeType: mimeFolder},
		"BIGDOC12345678": {ID: "BIGDOC12345678", Name: "Grande", MimeType: mimeDoc},
		"TXT12345678901": {ID: "TXT12345678901", Name: "notas.txt", MimeType: "text/plain"},
		"SHORTCUT123456": {ID: "SHORTCUT123456", Name: "Acceso", MimeType: mimeShortcut},
		"SHEETNOAPI1234": {ID: "SHEETNOAPI1234", Name: "Sin API", MimeType: mimeSheet},
	}
	mux.HandleFunc("GET /drive/v3/files", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		q := r.URL.Query().Get("q")
		var out []File
		switch {
		case strings.Contains(q, "'FOLDER12345678' in parents"), strings.Contains(q, "'root' in parents"):
			out = []File{files["DOC1234567890"], files["SHEET123456789"]}
		case strings.Contains(q, "fullText contains 'informe'"):
			if !strings.Contains(q, "trashed = false") || r.URL.Query().Get("orderBy") != "" {
				t.Errorf("consulta: %s orderBy=%s", q, r.URL.Query().Get("orderBy"))
			}
			out = []File{files["DOC1234567890"]}
		case strings.Contains(q, `fullText contains 'l\'eau'`):
			out = nil
		default:
			t.Errorf("consulta inesperada: %s", q)
		}
		json.NewEncoder(w).Encode(map[string]any{"files": out})
	})
	mux.HandleFunc("GET /drive/v3/files/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		id := r.PathValue("id")
		f, ok := files[id]
		if !ok {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"code":404,"message":"File not found"}}`))
			return
		}
		if id == "SHORTCUT123456" {
			f.ShortcutDetails = &struct {
				TargetID       string `json:"targetId"`
				TargetMimeType string `json:"targetMimeType"`
			}{TargetID: "DOC1234567890", TargetMimeType: mimeDoc}
		}
		if r.URL.Query().Get("alt") == "media" {
			w.Write([]byte("línea 1\nlínea 2\n"))
			return
		}
		json.NewEncoder(w).Encode(f)
	})
	mux.HandleFunc("GET /drive/v3/files/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		mt := r.URL.Query().Get("mimeType")
		switch r.PathValue("id") {
		case "DOC1234567890":
			if mt != "text/markdown" {
				t.Errorf("export %s", mt)
			}
			w.Write([]byte("# Informe\n\nTexto **importante**.\n"))
		case "BIGDOC12345678":
			w.Write([]byte(strings.Repeat("ñ", 50_000)))
		case "SHEETNOAPI1234":
			w.Write([]byte("a,b\n1,2\n"))
		default:
			w.WriteHeader(400)
		}
	})
	mux.HandleFunc("GET /sheets/v4/spreadsheets/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		if r.PathValue("id") == "SHEETNOAPI1234" || g.noSheets {
			w.WriteHeader(403)
			w.Write([]byte(`{"error":{"code":403,"message":"Sheets API has not been used","details":[{"reason":"SERVICE_DISABLED"}]}}`))
			return
		}
		w.Write([]byte(`{"sheets":[{"properties":{"sheetId":0,"title":"Ventas"}},{"properties":{"sheetId":77,"title":"Gastos de l'año"}},{"properties":{"sheetId":9,"title":"Oculta","hidden":true}}]}`))
	})
	mux.HandleFunc("GET /sheets/v4/spreadsheets/{id}/values:batchGet", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var vrs []map[string]any
		for _, rg := range r.URL.Query()["ranges"] {
			vrs = append(vrs, map[string]any{"range": rg, "values": [][]any{{"mes", "importe"}, {"enero", "1.000,5"}, {"feb, mar", 3}}})
		}
		json.NewEncoder(w).Encode(map[string]any{"valueRanges": vrs})
	})
	return mux
}

// newTest: un gestor contra el Google simulado; fetch decide qué da gcloud para cada fuente.
func newTest(t *testing.T, g *fakeGoogle, fetch map[string]string) (*Manager, *int) {
	srv := httptest.NewServer(g.handler(t))
	t.Cleanup(srv.Close)
	n := new(int)
	var mu sync.Mutex
	m := &Manager{Endpoints: Endpoints{TokenInfo: srv.URL + "/tokeninfo", Drive: srv.URL + "/drive/v3", Sheets: srv.URL + "/sheets/v4"},
		HTTP: srv.Client(), QuotaProject: "planes-dev",
		Fetch: func(_ context.Context, src string) (string, error) {
			mu.Lock()
			*n++
			mu.Unlock()
			if tok, ok := fetch[src]; ok {
				return tok, nil
			}
			return "", errors.New("sin credenciales")
		}}
	return m, n
}

const allScopes = "openid https://www.googleapis.com/auth/userinfo.email " + ScopeDriveReadonly + " https://www.googleapis.com/auth/cloud-platform"

func TestStatusADC(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	st := m.Status(context.Background(), true)
	if !st.Connected || st.Email != "luis@planesnet.com" || st.Source != SourceADC || !st.ReadOnly || !st.Gcloud {
		t.Fatalf("%+v", st)
	}
	if b, _ := json.Marshal(st); strings.Contains(string(b), `"adc"`) && strings.Contains(string(b), "token") {
		t.Errorf("el estado no lleva el token: %s", b)
	}
}

func TestStatusNoDriveScope(t *testing.T) {
	// ADC sin Drive (solo cloud-platform) y sin sesión de gcloud con Drive: dice qué ejecutar.
	g := &fakeGoogle{scopes: map[string]string{"adc": "https://www.googleapis.com/auth/cloud-platform openid", "gc": "openid"}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc", SourceGcloud: "gc"})
	st := m.Status(context.Background(), true)
	if st.Connected || !strings.Contains(st.Problem, "permiso de lectura de Drive") || st.Command != LoginCommand || st.Email == "" {
		t.Fatalf("%+v", st)
	}
	if _, err := m.Search(context.Background(), SearchParams{Text: "x"}); !errors.Is(err, ErrNoAccess) {
		t.Errorf("buscar sin acceso: %v", err)
	}
}

func TestFallbackGcloudSession(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": "openid", "gc": "openid " + ScopeDrive}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc", SourceGcloud: "gc"})
	st := m.Status(context.Background(), true)
	if !st.Connected || st.Source != SourceGcloud || st.ReadOnly {
		t.Fatalf("%+v", st)
	}
}

func TestNoGcloud(t *testing.T) {
	g := &fakeGoogle{}
	m, _ := newTest(t, g, nil)
	m.Fetch = func(context.Context, string) (string, error) { return "", ErrNoGcloud }
	st := m.Status(context.Background(), true)
	if st.Gcloud || st.Connected || !strings.Contains(st.Problem, "gcloud") {
		t.Fatalf("%+v", st)
	}
}

func TestOffline(t *testing.T) {
	m := &Manager{Endpoints: Endpoints{TokenInfo: "http://127.0.0.1:1/tokeninfo", Drive: "http://127.0.0.1:1"},
		HTTP: &http.Client{Timeout: time.Second}, Fetch: func(context.Context, string) (string, error) { return "tok", nil }}
	if _, err := m.Search(context.Background(), SearchParams{Text: "x"}); err == nil || !strings.Contains(err.Error(), "no se pudo hablar con Google") {
		t.Fatalf("sin conexión: %v", err)
	}
}

func TestExpiredTokenRefetch(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"viejo": allScopes, "nuevo": allScopes}, expired: map[string]bool{"viejo": true}}
	fetch := map[string]string{SourceADC: "viejo"}
	m, n := newTest(t, g, fetch)
	if _, err := m.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	fetch[SourceADC] = "nuevo" // gcloud ya da otro (el de antes caducó)
	fs, err := m.Search(context.Background(), SearchParams{Text: "informe", Type: "doc"})
	if err != nil || len(fs) != 1 {
		t.Fatalf("%v %v", fs, err)
	}
	if *n != 2 {
		t.Errorf("gcloud consultado %d veces", *n)
	}
	// Ya en memoria: no se vuelve a preguntar a gcloud.
	m.Search(context.Background(), SearchParams{Text: "informe"})
	if *n != 2 {
		t.Errorf("gcloud consultado %d veces", *n)
	}
}

func TestQuotaProjectRetry(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}, needQuota: true}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	fs, err := m.List(context.Background(), "", 0)
	if err != nil || len(fs) != 2 || g.quotaSeen != "planes-dev" {
		t.Fatalf("%v %v %q", fs, err, g.quotaSeen)
	}
	// Sin proyecto de cuota: el error dice qué habilitar.
	m2, _ := newTest(t, &fakeGoogle{scopes: map[string]string{"adc": allScopes}, needQuota: true}, map[string]string{SourceADC: "adc"})
	m2.QuotaProject = ""
	if _, err := m2.List(context.Background(), "", 0); err == nil || !strings.Contains(err.Error(), "gcloud services enable") {
		t.Fatalf("%v", err)
	}
}

func TestReadDocAndShortcut(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	for _, ref := range []string{"https://docs.google.com/document/d/DOC1234567890/edit?tab=t.0", "SHORTCUT123456"} {
		c, err := m.Read(context.Background(), ReadParams{Ref: ref})
		if err != nil || c.Format != "markdown" || !strings.Contains(c.Text, "**importante**") {
			t.Fatalf("%s: %+v %v", ref, c, err)
		}
		out := FormatContent(c)
		if !strings.HasPrefix(out, "# Informe [doc · markdown · 2026-10-01 · DOC1234567890]\n") {
			t.Errorf("%q", out)
		}
	}
}

func TestReadTruncates(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	c, err := m.Read(context.Background(), ReadParams{Ref: "BIGDOC12345678", Max: 1000})
	if err != nil || !c.Truncated || len([]rune(c.Text)) != 1000 || c.Total != 0 {
		t.Fatalf("%v trunc=%v len=%d total=%d", err, c.Truncated, len([]rune(c.Text)), c.Total)
	}
	if !strings.Contains(FormatContent(c), "[recortado: 1000 caracteres; pide menos") {
		t.Error(FormatContent(c)[len(FormatContent(c))-200:])
	}
	// Por defecto, el límite general.
	c, _ = m.Read(context.Background(), ReadParams{Ref: "BIGDOC12345678"})
	if len([]rune(c.Text)) != DefaultMaxChars {
		t.Errorf("%d", len([]rune(c.Text)))
	}
}

func TestReadSheet(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	ctx := context.Background()
	// Todas las visibles.
	c, err := m.Read(ctx, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/SHEET123456789/edit"})
	if err != nil || c.Format != "csv" || !strings.Contains(c.Text, "## 'Ventas' (3 filas)") || !strings.Contains(c.Text, "## 'Gastos de l''año'") ||
		strings.Contains(c.Text, "Oculta") || !strings.Contains(c.Text, `"feb, mar",3`) || !strings.Contains(c.Note, "--hoja") {
		t.Fatalf("%+v %v", c, err)
	}
	// Una hoja con rango.
	c, err = m.Read(ctx, ReadParams{Ref: "SHEET123456789", Sheet: "gastos de l'año", Range: "A1:B2"})
	if err != nil || strings.Contains(c.Text, "##") || !strings.HasPrefix(c.Text, "mes,importe\n") {
		t.Fatalf("%+v %v", c, err)
	}
	// La hoja de la URL (gid).
	c, err = m.Read(ctx, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/SHEET123456789/edit#gid=77"})
	if err != nil || strings.Contains(c.Text, "##") {
		t.Fatalf("%+v %v", c, err)
	}
	// Hoja que no existe.
	if _, err := m.Read(ctx, ReadParams{Ref: "SHEET123456789", Sheet: "Nada"}); err == nil || !strings.Contains(err.Error(), "Ventas") {
		t.Fatal(err)
	}
	// Sin la API de Sheets: la primera hoja exportada por Drive.
	c, err = m.Read(ctx, ReadParams{Ref: "SHEETNOAPI1234"})
	if err != nil || c.Text != "a,b\n1,2\n" || !strings.Contains(c.Note, "primera hoja") {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestReadOthers(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	ctx := context.Background()
	c, err := m.Read(ctx, ReadParams{Ref: "https://drive.google.com/file/d/PDF12345678901/view?usp=sharing"})
	if err != nil || c.Format != "metadatos" || !strings.Contains(FormatContent(c), "enlace: https://drive.google.com/file/d/PDF12345678901/view") {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = m.Read(ctx, ReadParams{Ref: "TXT12345678901"})
	if err != nil || c.Text != "línea 1\nlínea 2\n" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = m.Read(ctx, ReadParams{Ref: "https://drive.google.com/drive/u/0/folders/FOLDER12345678"})
	if err != nil || c.Format != "listado" || !strings.Contains(c.Text, "[hoja] Datos · 2026-10-02 · SHEET123456789") {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := m.Read(ctx, ReadParams{Ref: "NOEXISTE12345"}); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestSearchQuoting(t *testing.T) {
	g := &fakeGoogle{scopes: map[string]string{"adc": allScopes}}
	m, _ := newTest(t, g, map[string]string{SourceADC: "adc"})
	fs, err := m.Search(context.Background(), SearchParams{Text: "l'eau"})
	if err != nil || len(fs) != 0 || FormatFiles(fs) != "(nada)\n" {
		t.Fatalf("%v %v", fs, err)
	}
	if _, err := m.Search(context.Background(), SearchParams{Text: "x", Type: "vídeo"}); err == nil {
		t.Error("tipo desconocido")
	}
	if _, err := m.Search(context.Background(), SearchParams{}); err == nil {
		t.Error("búsqueda vacía")
	}
}

func TestParseRef(t *testing.T) {
	for _, c := range []struct{ in, id, gid string }{
		{"1AbCdEfGhIjK", "1AbCdEfGhIjK", ""},
		{"https://docs.google.com/document/d/1AbC-dEf_GhIj/edit", "1AbC-dEf_GhIj", ""},
		{"https://docs.google.com/spreadsheets/d/1AbCdEfGhIjK/edit#gid=123", "1AbCdEfGhIjK", "123"},
		{"https://docs.google.com/presentation/d/1AbCdEfGhIjK/edit", "1AbCdEfGhIjK", ""},
		{"https://drive.google.com/file/d/1AbCdEfGhIjK/view?usp=sharing", "1AbCdEfGhIjK", ""},
		{"https://drive.google.com/open?id=1AbCdEfGhIjK", "1AbCdEfGhIjK", ""},
		{"https://drive.google.com/drive/folders/1AbCdEfGhIjK", "1AbCdEfGhIjK", ""},
	} {
		id, gid, err := ParseRef(c.in)
		if err != nil || id != c.id || gid != c.gid {
			t.Errorf("%s: %q %q %v", c.in, id, gid, err)
		}
	}
	for _, bad := range []string{"", "corto", "https://example.com/d/1AbCdEfGhIjK", "../../etc"} {
		if _, _, err := ParseRef(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

// Prueba real (opcional): PGO_PRUEBA_GOOGLE=1 go test ./internal/gworkspace -run Real -v
// Usa la gcloud del equipo; PGO_PRUEBA_GOOGLE_REF: un documento que leer.
func TestReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_GOOGLE") != "1" {
		t.Skip("prueba contra Google de verdad: PGO_PRUEBA_GOOGLE=1")
	}
	m := New("planes-dev")
	ctx := context.Background()
	st := m.Status(ctx, true)
	t.Logf("estado: %+v", st)
	if !st.Connected {
		t.Fatal(st.Problem)
	}
	fs, err := m.List(ctx, "", 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + FormatFiles(fs))
	if ref := os.Getenv("PGO_PRUEBA_GOOGLE_REF"); ref != "" {
		c, err := m.Read(ctx, ReadParams{Ref: ref, Max: 2000})
		if err != nil {
			t.Fatal(err)
		}
		t.Log("\n" + FormatContent(c))
	}
}
