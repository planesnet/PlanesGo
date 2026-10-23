package servidor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"planesgo-code/internal/gbrowser"
	"planesgo-code/internal/sesiones"
)

// El agente lee Google con «pgo drive …» y «pgo gmail …»: el backend pasa cada página a Electron por el canal de la
// app de escritorio (que solo usa Electron, con su secreto) y responde lo que este ha leído.
func TestGoogleViaDesktop(t *testing.T) {
	s := &Servidor{Puerto: "7801", TokenEscritorio: "electron", TokenAgente: "agente", Sesiones: sesiones.NuevoAlmacen(),
		Google: gbrowser.New(gbrowser.NewBridge())}
	h := s.Handler()
	pide := func(metodo, ruta, cuerpo, agente, cookie string) (int, string) {
		r := httptest.NewRequest(metodo, "http://127.0.0.1:7801"+ruta, strings.NewReader(cuerpo))
		r.Header.Set("X-PGO", "1")
		if agente != "" {
			r.Header.Set("X-PGO-Agente", agente)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: cookieEscritorio, Value: cookie})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	tok := s.tokenProyecto(7)
	// Sin app de escritorio (modo -navegador o Electron cerrado): lo dice.
	if c, b := pide("GET", "/api/pgo/drive-buscar?q=informe&proyecto=7", "", tok, ""); c != http.StatusServiceUnavailable || !strings.Contains(b, "aplicación de escritorio") {
		t.Fatalf("%d %s", c, b)
	}
	// El canal solo es para Electron: ni el agente ni nadie sin el secreto.
	if c, _ := pide("GET", "/api/escritorio/google/trabajo", "", tok, ""); c != http.StatusForbidden {
		t.Fatalf("agente: %d", c)
	}
	if c, _ := pide("GET", "/api/escritorio/google/trabajo", "", "", "otro"); c != http.StatusForbidden {
		t.Fatalf("otro secreto: %d", c)
	}
	if c, _ := pide("POST", "/api/escritorio/google/resultado", `{"id":"x"}`, "", "otro"); c != http.StatusForbidden {
		t.Fatalf("otro secreto: %d", c)
	}
	// Electron: recoge cada trabajo y responde (una búsqueda de Drive y, luego, Google pide iniciar sesión).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trabajos := make(chan gbrowser.Job, 4)
	go func() {
		for ctx.Err() == nil {
			c, b := pide("GET", "/api/escritorio/google/trabajo", "", "", "electron")
			if c == http.StatusNoContent {
				continue
			}
			var j gbrowser.Job
			if c != http.StatusOK || json.Unmarshal([]byte(b), &j) != nil {
				t.Errorf("trabajo: %d %s", c, b)
				return
			}
			trabajos <- j
			res := gbrowser.Result{ID: j.ID, Status: 200, Items: []gbrowser.Item{{ID: "1DocIdXyzw", Lines: []string{"Informe", "12 oct 2026"}, Href: "https://docs.google.com/document/d/1DocIdXyzw/edit"}}}
			if j.Mode == gbrowser.ModeText {
				res = gbrowser.Result{ID: j.ID, Login: true, URL: "https://accounts.google.com/ServiceLogin"}
			}
			cuerpo, _ := json.Marshal(res)
			if c, b := pide("POST", "/api/escritorio/google/resultado", string(cuerpo), "", "electron"); c != http.StatusOK || !strings.Contains(b, `"esperado":true`) {
				t.Errorf("resultado: %d %s", c, b)
			}
		}
	}()
	for !s.Google.Bridge.Available() {
		time.Sleep(time.Millisecond)
	}
	c, b := pide("GET", "/api/pgo/drive-buscar?q=informe&proyecto=7", "", tok, "")
	if c != http.StatusOK || !strings.Contains(b, `[doc] Informe · 12 oct 2026 · 1DocIdXyzw`) {
		t.Fatalf("%d %s", c, b)
	}
	if j := <-trabajos; j.Project != 7 || j.Mode != gbrowser.ModeDrive || j.URL != "https://drive.google.com/drive/search?q=informe" {
		t.Fatalf("%+v", j)
	}
	// Sin sesión de Google: lo dice y abre accounts.google.com en el panel Web del proyecto (una vez).
	ev := s.Eventos.alta()
	defer s.Eventos.baja(ev)
	for i := 0; i < 2; i++ {
		c, b = pide("GET", "/api/pgo/drive-leer?ref="+"https://docs.google.com/document/d/1DocIdXyzw/edit&proyecto=7", "", tok, "")
		if c != http.StatusServiceUnavailable || !strings.Contains(b, "inicia sesión en Google") {
			t.Fatalf("%d %s", c, b)
		}
		<-trabajos
	}
	select {
	case e := <-ev:
		if !strings.Contains(string(e), `"tipo":"abrir"`) || !strings.Contains(string(e), "accounts.google.com") || !strings.Contains(string(e), `"proyecto":7`) {
			t.Fatalf("%s", e)
		}
	default:
		t.Fatal("no se abre el inicio de sesión")
	}
	select {
	case e := <-ev:
		t.Fatalf("dos veces seguidas: %s", e)
	default:
	}
	if c, _ := pide("GET", "/api/pgo/gmail-borrar?proyecto=7", "", tok, ""); c != http.StatusNotFound {
		t.Fatalf("orden desconocida: %d", c)
	}
	if c, _ := pide("GET", "/api/pgo/drive-buscar?q=x&proyecto=7", "", s.tokenProyecto(8), ""); c != http.StatusForbidden {
		t.Fatalf("token de otro proyecto: %d", c)
	}
}

// Con -navegador (sin Electron), el canal tampoco se abre a quien no traiga el secreto.
func TestGoogleChannelBrowserMode(t *testing.T) {
	s := &Servidor{Puerto: "7801", PermitirNavegador: true, Sesiones: sesiones.NuevoAlmacen(), Google: gbrowser.New(gbrowser.NewBridge())}
	r := httptest.NewRequest("GET", "http://127.0.0.1:7801/api/escritorio/google/trabajo", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("%d", w.Code)
	}
}
