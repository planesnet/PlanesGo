package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	studiotv "planesgo-code/studio-tv"
)

// Studio TV (PSFCode): estudio para ver en directo las pruebas automáticas, servido en /studio-tv/.
// Los tokens salen de STUDIO_TV_EMIT_TOKEN / STUDIO_TV_VIEW_TOKEN o, si no están, se generan una vez y se
// guardan en data/studio_tv.json (volumen persistente). Solo un administrador los ve, en /admin/studio-tv.

const rutaStudioTV = "/studio-tv/"

type tokensStudioTV struct {
	Emitir string `json:"emitir"`
	Ver    string `json:"ver"`
}

func cargaTokensStudioTV(fichero string) (tokensStudioTV, error) {
	t := tokensStudioTV{Emitir: os.Getenv("STUDIO_TV_EMIT_TOKEN"), Ver: os.Getenv("STUDIO_TV_VIEW_TOKEN")}
	if t.Emitir != "" && t.Ver != "" {
		return t, nil
	}
	if b, err := os.ReadFile(fichero); err == nil {
		if json.Unmarshal(b, &t) == nil && t.Emitir != "" && t.Ver != "" {
			return t, nil
		}
	}
	var err1, err2 error
	t.Emitir, err1 = studiotv.TokenAleatorio()
	t.Ver, err2 = studiotv.TokenAleatorio()
	if err := errors.Join(err1, err2); err != nil {
		return t, err
	}
	b, _ := json.Marshal(t)
	if err := os.MkdirAll(filepath.Dir(fichero), 0o700); err != nil {
		return t, err
	}
	return t, os.WriteFile(fichero, b, 0o600)
}

// setupStudioTV monta el estudio. Si no se pueden obtener los tokens, el estudio queda desactivado.
func setupStudioTV(mux *http.ServeMux, state *AppState) {
	t, err := cargaTokensStudioTV("data/studio_tv.json")
	if err != nil {
		log.Printf("[ADVERTENCIA] Studio TV desactivado: %v", err)
		return
	}
	mux.Handle(rutaStudioTV, http.StripPrefix(strings.TrimSuffix(rutaStudioTV, "/"), studiotv.Nuevo(t.Emitir, t.Ver).Handler()))
	mux.HandleFunc("/admin/studio-tv", state.handleAdminStudioTV(t))
	mux.HandleFunc("/api/studio-tv/emisor", state.handleStudioTVEmisor(t))
	mux.HandleFunc("/install/pco_emisor.py", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-python; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="pco_emisor.py"`)
		w.Write(studiotv.EmisorPython)
	})
	log.Printf(" Studio TV: ACTIVO en %s (tokens en /admin/studio-tv)", rutaStudioTV)
}

var plantillaStudioTV = template.Must(template.New("studio").Parse(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer"><title>Studio TV · PlanesGo</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:760px;margin:32px auto;padding:0 16px;color:#1f1e1b;background:#faf9f5}
code{display:block;background:#f0eee6;border-radius:6px;padding:8px 10px;overflow-wrap:anywhere;font:13px ui-monospace,monospace}
h2{margin-top:28px;font-size:17px}.aviso{color:#8a4b00}</style></head><body>
<h1>Studio TV</h1>
<p>Estudio para ver en directo las pruebas automáticas (PSFCode).</p>
<h2>Ver</h2>
<p>Abre este enlace o pon la URL y el token de ver en el artefacto Studio TV de PSFCode.</p>
<code>{{.Base}}?t={{.Ver}}</code>
<p>URL del estudio:</p><code>{{.Base}}</code>
<p>Token de ver:</p><code>{{.Ver}}</code>
<h2>Emitir (para la sesión que ejecuta las pruebas)</h2>
<p class="aviso">Pasa solo esto al emisor. El token de ver es solo para quien mira.</p>
<code>export PCO_URL={{.Base}} PCO_TOKEN={{.Emitir}} PCO_CANAL=&lt;nombre&gt;</code>
<p>La sesión necesita poder salir a este dominio (en Claude en la nube: <i>Network access</i> del entorno).</p>
</body></html>`))

func (state *AppState) handleAdminStudioTV(t tokensStudioTV) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := state.getSession(r)
		if session == nil {
			http.Redirect(w, r, "/login?next=/admin/studio-tv", http.StatusFound)
			return
		}
		// Igual que /admin: solo administradores autenticados con Google.
		if !session.IsAdmin() || session.AuthMethod != "google" {
			http.Error(w, "Acceso no autorizado", http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		plantillaStudioTV.Execute(w, map[string]string{"Base": baseStudioTV(r), "Ver": t.Ver, "Emitir": t.Emitir})
	}
}

// baseStudioTV es la URL pública del estudio, respetando el proxy HTTPS de delante.
func baseStudioTV(r *http.Request) string {
	esquema := "http"
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		esquema = p
	} else if r.TLS != nil {
		esquema = "https"
	}
	return esquema + "://" + r.Host + strings.TrimSuffix(rutaStudioTV, "/")
}

const instruccionesEmisor = `Para emitir en directo a Studio TV mientras ejecutas pruebas con navegador:
1. Descarga el emisor: curl -fsSL <origen>/install/pco_emisor.py -o pco_emisor.py (o usa el que ya tenga el grabador).
2. Exporta PCO_URL y PCO_TOKEN (los de esta respuesta) y PCO_CANAL con un nombre corto de la prueba (letras, números, . _ -).
3. Emisor(): llama a .frame(jpeg) con cada fotograma (p. ej. CDP Page.startScreencast) y a .estado({caso, titulo, paso, estado: curso|fin, seg, marcadores:[{t, txt}]}) en cada paso.
El emisor va en un hilo propio, a 8 imágenes por segundo como mucho, y descarta imágenes si el estudio va lento: nunca frena la prueba.
No muestres PCO_TOKEN en los resultados ni lo guardes en el repositorio.`

// handleStudioTVEmisor da a una sesión de un empleado (token de PlanesGo) los datos para emitir.
// Nunca devuelve el token de ver.
func (state *AppState) handleStudioTVEmisor(t tokensStudioTV) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
			return
		}
		session, err := state.resolveAntigravitySession(r, "")
		if err != nil || session == nil || session.UserEmail == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "autenticación requerida: token de PlanesGo del empleado"})
			return
		}
		base := baseStudioTV(r)
		origen := strings.TrimSuffix(base, strings.TrimSuffix(rutaStudioTV, "/"))
		log.Printf("[Studio TV] datos de emisión entregados a %s", session.UserEmail)
		json.NewEncoder(w).Encode(map[string]string{
			"pco_url": base, "pco_token": t.Emitir, "emisor": origen + "/install/pco_emisor.py",
			"instrucciones": strings.ReplaceAll(instruccionesEmisor, "<origen>", origen),
		})
	}
}
