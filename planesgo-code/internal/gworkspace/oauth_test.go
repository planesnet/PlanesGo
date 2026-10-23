package gworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testClientID     = "123-abc.apps.googleusercontent.com"
	testClientSecret = "GOCSPX-secreto-de-prueba"
	testClientFile   = `{"installed":{"client_id":"` + testClientID + `","project_id":"planes-dev","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","client_secret":"` + testClientSecret + `","redirect_uris":["http://localhost"]}}`
)

// fakeOAuth: el servidor de autorización de Google simulado (token y revoke) delante del Drive simulado.
type fakeOAuth struct {
	g         *fakeGoogle
	mu        sync.Mutex
	scope     string            // lo que concede el usuario en la pantalla de Google
	codes     map[string]string // código → challenge PKCE
	redirects map[string]string // código → redirect_uri
	refresh   map[string]string // token de renovación válido → correo
	revoked   []string
	refreshes int
	down      bool // el endpoint de revocación no contesta bien
	n         int
}

func idToken(email string) string {
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(`{"alg":"RS256"}`)) + "." + enc.EncodeToString([]byte(`{"email":"`+email+`","email_verified":true}`)) + ".firma"
}

func (o *fakeOAuth) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", o.g.handler(t))
	mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f := r.PostForm
		if r.URL.RawQuery != "" {
			t.Error("nada en la URL del endpoint de tokens")
		}
		o.mu.Lock()
		defer o.mu.Unlock()
		bad := func(e string) {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": e, "error_description": "simulado"})
		}
		if f.Get("client_id") != testClientID || f.Get("client_secret") != testClientSecret {
			bad("invalid_client")
			return
		}
		o.n++
		acc := fmt.Sprintf("acc%d", o.n)
		switch f.Get("grant_type") {
		case "authorization_code":
			ch, ok := o.codes[f.Get("code")]
			sum := sha256.Sum256([]byte(f.Get("code_verifier")))
			if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != ch || o.redirects[f.Get("code")] != f.Get("redirect_uri") {
				bad("invalid_grant")
				return
			}
			delete(o.codes, f.Get("code"))
			ref := fmt.Sprintf("ref%d", o.n)
			o.refresh[ref] = "luis@planesnet.com"
			o.g.mu.Lock()
			o.g.scopes[acc] = o.scope
			o.g.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"access_token": acc, "expires_in": 3599, "refresh_token": ref, "scope": o.scope, "id_token": idToken("luis@planesnet.com"), "token_type": "Bearer"})
		case "refresh_token":
			o.refreshes++
			if _, ok := o.refresh[f.Get("refresh_token")]; !ok {
				bad("invalid_grant")
				return
			}
			o.g.mu.Lock()
			o.g.scopes[acc] = o.scope
			o.g.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"access_token": acc, "expires_in": 3599, "scope": o.scope, "token_type": "Bearer"})
		default:
			bad("unsupported_grant_type")
		}
	})
	mux.HandleFunc("POST /oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		o.mu.Lock()
		defer o.mu.Unlock()
		if o.down {
			w.WriteHeader(503)
			return
		}
		tok := r.PostForm.Get("token")
		o.revoked = append(o.revoked, tok)
		delete(o.refresh, tok)
		w.Write([]byte("{}"))
	})
	return mux
}

// browser hace lo que el navegador: abre la URL de Google, «acepta» y sigue la redirección a 127.0.0.1.
func (o *fakeOAuth) browser(t *testing.T, authURL string, mutate func(q url.Values)) (int, string) {
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	for k, v := range map[string]string{"client_id": testClientID, "response_type": "code", "scope": OAuthScopes, "code_challenge_method": "S256", "access_type": "offline", "prompt": "consent"} {
		if q.Get(k) != v {
			t.Errorf("%s = %q", k, q.Get(k))
		}
	}
	if strings.Contains(authURL, testClientSecret) {
		t.Error("el secreto no va en la URL de autorización")
	}
	redirect := q.Get("redirect_uri")
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") || len(q.Get("code_challenge")) != 43 || len(q.Get("state")) < 20 {
		t.Fatalf("redirect %q challenge %q state %q", redirect, q.Get("code_challenge"), q.Get("state"))
	}
	o.mu.Lock()
	code := fmt.Sprintf("code%d", len(o.codes)+100)
	o.codes[code] = q.Get("code_challenge")
	o.redirects[code] = redirect
	o.mu.Unlock()
	back := url.Values{"code": {code}, "state": {q.Get("state")}, "scope": {OAuthScopes}}
	if mutate != nil {
		mutate(back)
	}
	res, err := http.Get(redirect + "?" + back.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func newOAuthTest(t *testing.T) (*Manager, *fakeOAuth) {
	o := &fakeOAuth{g: &fakeGoogle{scopes: map[string]string{}, expired: map[string]bool{}}, scope: "openid https://www.googleapis.com/auth/userinfo.email " + ScopeDriveReadonly,
		codes: map[string]string{}, redirects: map[string]string{}, refresh: map[string]string{}}
	srv := httptest.NewServer(o.handler(t))
	t.Cleanup(srv.Close)
	m := &Manager{Endpoints: Endpoints{TokenInfo: srv.URL + "/tokeninfo", Drive: srv.URL + "/drive/v3", Sheets: srv.URL + "/sheets/v4",
		Auth: srv.URL + "/oauth/auth", Token: srv.URL + "/oauth/token", Revoke: srv.URL + "/oauth/revoke"},
		HTTP: srv.Client(), Dir: t.TempDir()}
	return m, o
}

// connect: conecta al usuario de principio a fin.
func connect(t *testing.T, m *Manager, o *fakeOAuth, login string) {
	t.Helper()
	u, err := m.StartAuth(context.Background(), login)
	if err != nil {
		t.Fatal(err)
	}
	if st := m.Status(context.Background(), login, false); !st.Pending {
		t.Fatalf("en curso: %+v", st)
	}
	code, body := o.browser(t, u, nil)
	if code != 200 || !strings.Contains(body, "ya puede leer tu Google Drive") {
		t.Fatalf("%d %s", code, body)
	}
}

func TestClientFile(t *testing.T) {
	m, _ := newOAuthTest(t)
	ctx := context.Background()
	if st := m.Status(ctx, "luis", false); st.Client || st.Connected || st.Problem != "" {
		t.Fatalf("%+v", st)
	}
	if _, err := m.StartAuth(ctx, "luis"); !errors.Is(err, ErrNoClient) {
		t.Fatalf("sin cliente: %v", err)
	}
	for in, want := range map[string]string{
		`no es json`: "no es el fichero JSON",
		`{"web":{"client_id":"x.apps.googleusercontent.com","client_secret":"s"}}`: "«Aplicación de escritorio»",
		`{"otro":1}`: "falta «installed»",
		`{"installed":{"client_id":"x","client_secret":"s"}}`:                           "identificador",
		`{"installed":{"client_id":"x.apps.googleusercontent.com","client_secret":""}}`: "secreto",
	} {
		if _, err := m.SaveClientFile([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v", in, err)
		}
	}
	if _, err := m.SaveClientFile([]byte(strings.Repeat(" ", 70<<10))); err == nil {
		t.Error("fichero enorme aceptado")
	}
	c, err := m.SaveClientFile([]byte(testClientFile))
	if err != nil || c.ProjectID != "planes-dev" || c.Origin != ClientFromFile {
		t.Fatalf("%+v %v", c, err)
	}
	p := filepath.Join(m.Dir, ClientFile)
	fi, err := os.Stat(p)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("permisos %v %v", fi.Mode(), err)
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "redirect_uris") || strings.Contains(string(b), "token_uri") || !strings.Contains(string(b), testClientSecret) {
		t.Errorf("guardado: %s", b)
	}
	st := m.Status(ctx, "luis", false)
	if !st.Client || st.ClientOrigin != ClientFromFile || st.ClientProject != "planes-dev" {
		t.Fatalf("%+v", st)
	}
	if j, _ := json.Marshal(st); strings.Contains(string(j), testClientSecret) || strings.Contains(string(j), testClientID) {
		t.Errorf("el estado no lleva el cliente: %s", j)
	}
	// Un fichero dañado se dice (no se toma por «falta»).
	os.WriteFile(p, []byte("{"), 0o600)
	if st := m.Status(ctx, "luis", false); st.Client || !strings.Contains(st.Problem, "no vale") {
		t.Fatalf("%+v", st)
	}
}

func TestClientSource(t *testing.T) {
	m, o := newOAuthTest(t)
	calls := 0
	m.ClientSource = func(context.Context) (*OAuthClient, error) {
		calls++
		return &OAuthClient{ID: testClientID, Secret: testClientSecret, ProjectID: "planes-dev"}, nil
	}
	st := m.Status(context.Background(), "luis", false)
	if !st.Client || st.ClientOrigin != ClientFromServer {
		t.Fatalf("%+v", st)
	}
	connect(t, m, o, "luis")
	// El fichero manda sobre el servidor.
	m.SaveClientFile([]byte(testClientFile))
	n := calls
	if st := m.Status(context.Background(), "luis", false); st.ClientOrigin != ClientFromFile || calls != n {
		t.Fatalf("%+v %d", st, calls)
	}
}

func TestOAuthConnect(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	ctx := context.Background()
	connect(t, m, o, "luis@planesnet.com")
	st := m.Status(ctx, "luis@planesnet.com", false)
	if !st.Connected || st.Source != SourceOAuth || st.Email != "luis@planesnet.com" || !st.ReadOnly || st.Pending || st.Problem != "" || st.Since == "" {
		t.Fatalf("%+v", st)
	}
	// La conexión: un fichero 0600 en una carpeta 0700, con el token de renovación y sin el de acceso.
	p := m.connPath("luis@planesnet.com")
	if filepath.Base(p) != "luis@planesnet.com.json" {
		t.Errorf("fichero %s", p)
	}
	fi, _ := os.Stat(p)
	di, _ := os.Stat(filepath.Dir(p))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("permisos %v %v", fi.Mode(), di.Mode())
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"refresh_token": "ref`) || strings.Contains(string(b), "acc") || strings.Contains(string(b), testClientSecret) {
		t.Errorf("conexión: %s", b)
	}
	j, _ := json.Marshal(st)
	if strings.Contains(string(j), "ref") || strings.Contains(string(j), "acc1") {
		t.Errorf("el estado no lleva tokens: %s", j)
	}
	// Lee con el token de acceso en memoria (sin renovar).
	fs, err := m.Search(WithUser(ctx, "luis@planesnet.com"), SearchParams{Text: "informe"})
	if err != nil || len(fs) != 1 || o.refreshes != 0 {
		t.Fatalf("%v %v %d", fs, err, o.refreshes)
	}
	// Los agentes (sin usuario en el contexto): la única conexión del equipo.
	if _, err := m.Search(ctx, SearchParams{Text: "informe"}); err != nil {
		t.Fatal(err)
	}
	// Otro usuario de pgocode no usa la conexión de Luis.
	if _, err := m.Search(WithUser(ctx, "ana"), SearchParams{Text: "informe"}); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("otro usuario: %v", err)
	}
	if st := m.Status(ctx, "ana", false); st.Connected || st.Email != "" {
		t.Fatalf("ana: %+v", st)
	}
}

func TestOAuthRefresh(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	now := time.Now()
	var mu sync.Mutex
	m.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	connect(t, m, o, "luis")
	ctx := WithUser(context.Background(), "luis")
	// Pasa una hora: el token de acceso caduca y se renueva solo (una vez aunque se pida a la vez).
	mu.Lock()
	now = now.Add(time.Hour)
	mu.Unlock()
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.List(ctx, "", 0); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if o.refreshes != 1 {
		t.Fatalf("renovaciones: %d", o.refreshes)
	}
	// Google rechaza el token en memoria (401): se renueva y se repite.
	o.g.mu.Lock()
	for k := range o.g.scopes {
		o.g.expired[k] = true
	}
	o.g.mu.Unlock()
	if _, err := m.List(ctx, "", 0); err != nil || o.refreshes != 2 {
		t.Fatalf("%v %d", err, o.refreshes)
	}
	// Status con «probar»: renueva para comprobar.
	if st := m.Status(ctx, "luis", true); !st.Connected || o.refreshes != 3 {
		t.Fatalf("%+v %d", st, o.refreshes)
	}
}

func TestOAuthRevokedInAccount(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	connect(t, m, o, "luis")
	// Luis quita el permiso en su cuenta de Google: la renovación da invalid_grant y la conexión sobra.
	o.mu.Lock()
	clear(o.refresh)
	o.mu.Unlock()
	st := m.Status(context.Background(), "luis", true)
	if st.Connected || !strings.Contains(st.Problem, "retirado el permiso") || st.Email != "" {
		t.Fatalf("%+v", st)
	}
	if m.hasConnection("luis") {
		t.Error("la conexión revocada sigue en disco")
	}
	if _, err := m.List(WithUser(context.Background(), "luis"), "", 0); !errors.Is(err, ErrNoAccess) {
		t.Fatal(err)
	}
}

func TestOAuthDisconnect(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	connect(t, m, o, "luis")
	b, _ := os.ReadFile(m.connPath("luis"))
	var c connection
	json.Unmarshal(b, &c)
	if err := m.Disconnect(context.Background(), "luis"); err != nil {
		t.Fatal(err)
	}
	if len(o.revoked) != 1 || o.revoked[0] != c.RefreshToken || m.hasConnection("luis") {
		t.Fatalf("revocados %v", o.revoked)
	}
	if st := m.Status(context.Background(), "luis", false); st.Connected || st.Source != "" {
		t.Fatalf("%+v", st)
	}
	// Sin conexión: no pasa nada.
	if err := m.Disconnect(context.Background(), "luis"); err != nil {
		t.Fatal(err)
	}
	// Google no confirma la revocación: se borra igual y se avisa.
	connect(t, m, o, "luis")
	o.mu.Lock()
	o.down = true
	o.mu.Unlock()
	if err := m.Disconnect(context.Background(), "luis"); err == nil || !strings.Contains(err.Error(), "myaccount.google.com/permissions") || m.hasConnection("luis") {
		t.Fatalf("%v %v", err, m.hasConnection("luis"))
	}
}

func TestOAuthCallbackErrors(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	ctx := context.Background()

	// Otro state: se rechaza y se sigue esperando.
	u, _ := m.StartAuth(ctx, "luis")
	if code, _ := o.browser(t, u, func(q url.Values) { q.Set("state", "otro") }); code != 400 {
		t.Fatalf("state: %d", code)
	}
	if st := m.Status(ctx, "luis", false); !st.Pending {
		t.Fatalf("%+v", st)
	}
	// El usuario cancela en Google.
	code, body := o.browser(t, u, func(q url.Values) { q.Del("code"); q.Set("error", "access_denied") })
	if code != 200 || !strings.Contains(body, "Has cancelado") {
		t.Fatalf("%d %s", code, body)
	}
	if st := m.Status(ctx, "luis", false); st.Pending || st.Connected || !strings.Contains(st.Problem, "Has cancelado") {
		t.Fatalf("%+v", st)
	}

	// Desmarca el permiso de Drive: no sirve, se revoca lo concedido y no se guarda nada.
	o.mu.Lock()
	o.scope = "openid https://www.googleapis.com/auth/userinfo.email"
	o.mu.Unlock()
	u, _ = m.StartAuth(ctx, "luis")
	if _, body := o.browser(t, u, nil); !strings.Contains(body, "permiso de ver tus ficheros") {
		t.Fatal(body)
	}
	if st := m.Status(ctx, "luis", false); st.Connected || !strings.Contains(st.Problem, "Drive") || m.hasConnection("luis") || len(o.revoked) != 1 {
		t.Fatalf("%+v %v", st, o.revoked)
	}

	// Un nuevo «Conectar» sustituye al anterior (el puerto viejo se cierra) y borra el error.
	o.mu.Lock()
	o.scope = OAuthScopes
	o.mu.Unlock()
	u1, _ := m.StartAuth(ctx, "luis")
	u2, _ := m.StartAuth(ctx, "luis")
	if st := m.Status(ctx, "luis", false); !st.Pending || st.Problem != "" {
		t.Fatalf("%+v", st)
	}
	time.Sleep(50 * time.Millisecond)
	r1, _ := url.Parse(u1)
	if _, err := http.Get(r1.Query().Get("redirect_uri")); err == nil {
		t.Error("el puerto del primer intento sigue abierto")
	}
	if _, body := o.browser(t, u2, nil); !strings.Contains(body, "ya puede leer") {
		t.Fatal(body)
	}
	if !m.Status(ctx, "luis", false).Connected {
		t.Fatal("no conectado")
	}
}

func TestOAuthTimeout(t *testing.T) {
	m, _ := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	m.AuthTimeout = 50 * time.Millisecond
	u, err := m.StartAuth(context.Background(), "luis")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	st := m.Status(context.Background(), "luis", false)
	if st.Pending || !strings.Contains(st.Problem, "Se acabó el tiempo") {
		t.Fatalf("%+v", st)
	}
	r, _ := url.Parse(u)
	if _, err := http.Get(r.Query().Get("redirect_uri")); err == nil {
		t.Error("el puerto sigue abierto")
	}
}

func TestOAuthAgentUser(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	connect(t, m, o, "luis")
	connect(t, m, o, "ana")
	ctx := context.Background()
	// Dos conexiones y nadie en la interfaz: los agentes no saben de quién usar.
	if _, err := m.List(ctx, "", 0); !errors.Is(err, ErrNoAccess) {
		t.Fatalf("%v", err)
	}
	m.SetActive("ana")
	if m.userFor(ctx) != "ana" {
		t.Fatal(m.userFor(ctx))
	}
	if _, err := m.List(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	// La interfaz manda: cada usuario, la suya.
	if m.userFor(WithUser(ctx, "luis")) != "luis" || m.userFor(WithUser(ctx, "pepe")) != "" {
		t.Fatal("usuario de la interfaz")
	}
}

func TestOAuthBeatsGcloud(t *testing.T) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	o.g.scopes["gc"] = "openid " + ScopeDrive
	m.Fetch = func(context.Context, string) (string, error) { return "gc", nil }
	// Sin conexión propia: el respaldo de gcloud, en silencio.
	if st := m.Status(context.Background(), "luis", false); !st.Connected || st.Source != SourceADC || st.ReadOnly {
		t.Fatalf("%+v", st)
	}
	connect(t, m, o, "luis")
	st := m.Status(context.Background(), "luis", false)
	if st.Source != SourceOAuth || !st.ReadOnly {
		t.Fatalf("%+v", st)
	}
}

func TestConnName(t *testing.T) {
	for in, want := range map[string]string{"luis@planesnet.com": "luis@planesnet.com.json", "Luis": "luis.json"} {
		if got := connName(in); got != want {
			t.Errorf("%s: %s", in, got)
		}
	}
	for _, in := range []string{"../x", "a/b", ".oculto", "", "ñandú"} {
		got := connName(in)
		if !strings.HasPrefix(got, "u-") || strings.ContainsAny(got, "/\\") {
			t.Errorf("%q: %s", in, got)
		}
	}
	if connName("a/b") == connName("a_b") {
		t.Error("colisión")
	}
}

func TestEmailFromIDToken(t *testing.T) {
	if e := emailFromIDToken(idToken("luis@planesnet.com")); e != "luis@planesnet.com" {
		t.Fatal(e)
	}
	if emailFromIDToken("basura") != "" || emailFromIDToken("a.!!!.c") != "" {
		t.Fatal("token no válido")
	}
}
