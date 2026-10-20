package studiotv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	tokEmitir = "emitir-123"
	tokVer    = "ver-456"
)

var jpeg = []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3, 0xFF, 0xD9}

func nuevoServidor(t *testing.T) (*Studio, *httptest.Server) {
	t.Helper()
	s := Nuevo(tokEmitir, tokVer)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return s, srv
}

func peticion(t *testing.T, metodo, url, token, tipo string, cuerpo []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(metodo, url, bytes.NewReader(cuerpo))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Body.Close() })
	return r
}

func TestTokens(t *testing.T) {
	_, srv := nuevoServidor(t)
	casos := []struct {
		nombre, metodo, ruta, token string
		cuerpo                      []byte
		quiere                      int
	}{
		{"frame sin token", "POST", "/api/frame", "", jpeg, 401},
		{"frame con token de ver", "POST", "/api/frame", tokVer, jpeg, 401},
		{"frame con token de emitir", "POST", "/api/frame", tokEmitir, jpeg, 204},
		{"página sin token", "GET", "/", "", nil, 401},
		{"página con token de emitir", "GET", "/", tokEmitir, nil, 401},
		{"página con token de ver", "GET", "/", tokVer, nil, 200},
		{"canales con token de ver", "GET", "/api/canales", tokVer, nil, 200},
		{"salud sin token", "GET", "/healthz", "", nil, 200},
	}
	for _, c := range casos {
		if r := peticion(t, c.metodo, srv.URL+c.ruta, c.token, "", c.cuerpo); r.StatusCode != c.quiere {
			t.Errorf("%s: estado %d, quiero %d", c.nombre, r.StatusCode, c.quiere)
		}
	}
	// El token también vale en ?t=, que es como lo usa el navegador.
	if r := peticion(t, "GET", srv.URL+"/?t="+tokVer, "", "", nil); r.StatusCode != 200 {
		t.Errorf("?t= con token de ver: estado %d", r.StatusCode)
	}
}

func TestValidaciones(t *testing.T) {
	_, srv := nuevoServidor(t)
	if r := peticion(t, "POST", srv.URL+"/api/frame", tokEmitir, "", []byte("no es jpeg")); r.StatusCode != 400 {
		t.Errorf("frame no JPEG: estado %d", r.StatusCode)
	}
	if r := peticion(t, "POST", srv.URL+"/api/estado", tokEmitir, "", []byte("{roto")); r.StatusCode != 400 {
		t.Errorf("estado no JSON: estado %d", r.StatusCode)
	}
	if r := peticion(t, "POST", srv.URL+"/api/frame?canal=no%20vale", tokEmitir, "", jpeg); r.StatusCode != 400 {
		t.Errorf("canal con espacio: estado %d", r.StatusCode)
	}
	if r := peticion(t, "GET", srv.URL+"/snapshot?canal=nadie", tokVer, "", nil); r.StatusCode != 404 {
		t.Errorf("snapshot de canal inexistente: estado %d", r.StatusCode)
	}
}

func TestCanales(t *testing.T) {
	s, srv := nuevoServidor(t)
	peticion(t, "POST", srv.URL+"/api/frame", tokEmitir, "", jpeg) // sin canal: el de por defecto
	req, _ := http.NewRequest("POST", srv.URL+"/api/estado", strings.NewReader(`{"caso":"EX-1","estado":"curso"}`))
	req.Header.Set("Authorization", "Bearer "+tokEmitir)
	req.Header.Set("X-Canal", "qa-madera")
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 204 {
		t.Fatalf("estado por cabecera X-Canal: %v %v", r, err)
	}

	r := peticion(t, "GET", srv.URL+"/api/canales", tokVer, "", nil)
	var l []InfoCanal
	if err := json.NewDecoder(r.Body).Decode(&l); err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 || l[0].Canal != CanalPorDefecto || l[1].Canal != "qa-madera" {
		t.Fatalf("canales: %+v", l)
	}
	if !l[0].ConImagen || l[1].ConImagen || !l[1].EnDirecto || !strings.Contains(string(l[1].Estado), "EX-1") {
		t.Errorf("datos de los canales: %+v", l)
	}

	r = peticion(t, "GET", srv.URL+"/snapshot", tokVer, "", nil)
	if b, _ := io.ReadAll(r.Body); !bytes.Equal(b, jpeg) || r.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("snapshot: %v %q", b, r.Header.Get("Content-Type"))
	}

	// Sin señal ni espectadores durante más de la caducidad, el canal desaparece.
	ahora := time.Now().Add(2 * caducidad)
	s.mu.Lock()
	s.ahora = func() time.Time { return ahora }
	s.mu.Unlock()
	if l := s.Canales(); len(l) != 0 {
		t.Errorf("tras caducar quedan canales: %+v", l)
	}
}

func TestLimiteDeCanales(t *testing.T) {
	s := Nuevo(tokEmitir, tokVer)
	for i := 0; i < maxCanales; i++ {
		if err := s.publica(strings.Repeat("c", i+1), jpeg, nil); err != nil {
			t.Fatalf("canal %d: %v", i, err)
		}
	}
	if err := s.publica("uno-mas", jpeg, nil); err != errDemasiados {
		t.Errorf("canal %d: %v, quiero errDemasiados", maxCanales+1, err)
	}
}

// directo abre un GET de larga duración y devuelve un lector de su cuerpo.
func directo(t *testing.T, url string) *bufio.Reader {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Body.Close() })
	if r.StatusCode != 200 {
		t.Fatalf("%s: estado %d", url, r.StatusCode)
	}
	return bufio.NewReader(r.Body)
}

func TestStream(t *testing.T) {
	_, srv := nuevoServidor(t)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{}`)) // el emisor crea el canal
	rd := directo(t, srv.URL+"/stream?canal=a&t="+tokVer)
	peticion(t, "POST", srv.URL+"/api/frame?canal=b", tokEmitir, "", []byte{0xFF, 0xD8, 'b'}) // otro canal: no debe llegar
	peticion(t, "POST", srv.URL+"/api/frame?canal=a", tokEmitir, "", jpeg)
	var cab []string
	for {
		l, err := rd.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if l == "\r\n" {
			break
		}
		cab = append(cab, strings.TrimSpace(l))
	}
	if cab[0] != "--pco" || cab[1] != "Content-Type: image/jpeg" {
		t.Fatalf("cabecera de la parte: %q", cab)
	}
	b := make([]byte, len(jpeg))
	if _, err := io.ReadFull(rd, b); err != nil || !bytes.Equal(b, jpeg) {
		t.Errorf("fotograma: %v %v", b, err)
	}
}

func TestEvents(t *testing.T) {
	_, srv := nuevoServidor(t)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{"paso":"uno"}`))
	rd := directo(t, srv.URL+"/events?canal=a&t="+tokVer)
	// Al conectar llega el último estado; luego, los nuevos.
	for _, quiero := range []string{`data: {"paso":"uno"}`, `data: {"paso":"dos"}`} {
		if quiero == `data: {"paso":"dos"}` {
			peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{"paso":"dos"}`))
		}
		l, err := rd.ReadString('\n')
		if err != nil || strings.TrimSpace(l) != quiero {
			t.Fatalf("evento %q, quiero %q (%v)", l, quiero, err)
		}
		rd.ReadString('\n')
	}
}

// Detrás de un servidor con WriteTimeout (como PlanesGo, 60 s) el directo debe seguir vivo.
func TestStreamSobreviveWriteTimeout(t *testing.T) {
	s := Nuevo(tokEmitir, tokVer)
	srv := httptest.NewUnstartedServer(s.Handler())
	srv.Config.WriteTimeout = 300 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{}`))
	rd := directo(t, srv.URL+"/events?canal=a&t="+tokVer)
	rd.ReadString('\n') // estado inicial
	rd.ReadString('\n')
	time.Sleep(600 * time.Millisecond)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte(`{"paso":"tarde"}`))
	if l, err := rd.ReadString('\n'); err != nil || strings.TrimSpace(l) != `data: {"paso":"tarde"}` {
		t.Fatalf("tras el WriteTimeout: %q %v", l, err)
	}
}

// Quien solo tiene el token de ver no puede crear canales (no bloquea a los emisores con el límite).
func TestEspectadorNoCreaCanales(t *testing.T) {
	s, srv := nuevoServidor(t)
	for _, ruta := range []string{"/stream", "/events"} {
		if r := peticion(t, "GET", srv.URL+ruta+"?canal=inventado", tokVer, "", nil); r.StatusCode != 404 {
			t.Errorf("%s de un canal inexistente: estado %d, quiero 404", ruta, r.StatusCode)
		}
	}
	if l := s.Canales(); len(l) != 0 {
		t.Errorf("un espectador ha creado canales: %+v", l)
	}
}

// Un estado con saltos de línea se compacta: si no, partiría el evento SSE.
func TestEstadoCompactado(t *testing.T) {
	_, srv := nuevoServidor(t)
	peticion(t, "POST", srv.URL+"/api/estado?canal=a", tokEmitir, "", []byte("{\n  \"paso\": \"uno\"\n}"))
	rd := directo(t, srv.URL+"/events?canal=a&t="+tokVer)
	if l, _ := rd.ReadString('\n'); strings.TrimSpace(l) != `data: {"paso":"uno"}` {
		t.Fatalf("evento %q", l)
	}
}

// Un fotograma por encima del límite se rechaza con 413 en vez de publicarse cortado.
func TestFrameDemasiadoGrande(t *testing.T) {
	_, srv := nuevoServidor(t)
	grande := append([]byte{0xFF, 0xD8}, make([]byte, maxFrame)...)
	if r := peticion(t, "POST", srv.URL+"/api/frame?canal=a", tokEmitir, "", grande); r.StatusCode != 413 {
		t.Errorf("estado %d, quiero 413", r.StatusCode)
	}
}
