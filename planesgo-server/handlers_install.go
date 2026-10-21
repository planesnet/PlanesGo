package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

// Instalación servida por el propio PlanesGo, para que no dependa de que el repositorio sea público.
//
//   - /install/claude-hook.sh: instalador del hook de Claude Code (público, no lleva secretos).
//   - /install/planesgo.tar.gz: hook, comando /pgo, plugin de OpenCode y código de pgo-mcp (cmd/pgo-mcp sin
//     pruebas, más go.mod y go.sum, para compilarlo con «go build ./cmd/pgo-mcp»). Es público: es lo que el
//     instalador necesita en el «Setup script» de la nube, donde no hay variables de entorno.
//   - /install/psf.tar.gz: Planes Software Factory (carpeta PSF/). Solo con token de empleado.

// Los .go de cmd/pgo-mcp van uno a uno porque embed no permite excluir los _test.go con un patrón;
// TestInstallBundleContainsHookAndSource avisa si falta alguno.
//
//go:embed scripts/install-claude-hook.sh scripts/install-opencode-plugin.sh scripts/psf-arranque.sh .claude/hooks/planesgo_claude_hook.py .claude/commands/pgo.md .opencode/plugins/planesgo.ts go.mod go.sum
//go:embed cmd/pgo-mcp/cli.go cmd/pgo-mcp/cliente.go cmd/pgo-mcp/config.go cmd/pgo-mcp/herramientas.go cmd/pgo-mcp/latidos.go cmd/pgo-mcp/mcp.go cmd/pgo-mcp/proceso_unix.go cmd/pgo-mcp/proceso_windows.go cmd/pgo-mcp/tipos.go
var planesgoBundleFS embed.FS

//go:embed all:PSF
var psfBundleFS embed.FS

func setupInstallRoutes(mux *http.ServeMux, state *AppState) {
	mux.HandleFunc("/install/claude-hook.sh", handleInstallScript)
	mux.HandleFunc("/install/opencode-plugin.sh", handleInstallOpenCodeScript)
	mux.HandleFunc("/install/planesgo.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		serveTarGz(w, planesgoBundleFS, ".", "planesgo.tar.gz")
	})
	mux.HandleFunc("/install/psf.tar.gz", state.handleInstallPSF)
	mux.HandleFunc("/install/pgocode/", handleInstallPgocode)
	mux.HandleFunc("/admin/pgocode/publicar", state.handlePublicarPgocode)
}

func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	data, err := planesgoBundleFS.ReadFile("scripts/install-claude-hook.sh")
	if err != nil {
		http.Error(w, "instalador no disponible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func handleInstallOpenCodeScript(w http.ResponseWriter, r *http.Request) {
	data, err := planesgoBundleFS.ReadFile("scripts/install-opencode-plugin.sh")
	if err != nil {
		http.Error(w, "instalador no disponible", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func (state *AppState) handleInstallPSF(w http.ResponseWriter, r *http.Request) {
	// Solo token de empleado (cabecera o query), nunca la cookie: lo descargan scripts, no el navegador
	token := strings.TrimSpace(r.Header.Get("X-Antigravity-Token"))
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); token == "" && strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		token = strings.TrimSpace(auth[7:])
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" || state.userStore == nil {
		http.Error(w, "falta el token de empleado (PLANESGO_TOKEN)", http.StatusUnauthorized)
		return
	}
	if _, ok := state.userStore.GetUserByAntigravityToken(token); !ok {
		http.Error(w, "token de empleado no válido", http.StatusForbidden)
		return
	}
	if _, err := fs.Stat(psfBundleFS, "PSF/hooks/psf_hook.py"); err != nil {
		http.Error(w, "PSF todavía no está en este servidor", http.StatusNotFound)
		return
	}
	serveTarGz(w, psfBundleFS, "PSF", "psf.tar.gz")
}

// serveTarGz empaqueta el subárbol root del FS embebido (con sus rutas tal cual) en un .tar.gz.
func serveTarGz(w http.ResponseWriter, fsys embed.FS, root, name string) {
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Cache-Control", "no-cache")
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	now := time.Now()
	fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(p, "__pycache__") {
			return nil
		}
		data, err := fsys.ReadFile(p)
		if err != nil {
			return nil
		}
		mode := int64(0644)
		if ext := path.Ext(p); ext == ".sh" || ext == ".py" || strings.Contains(p, "/bin/") {
			mode = 0755
		}
		if tw.WriteHeader(&tar.Header{Name: p, Mode: mode, Size: int64(len(data)), ModTime: now, Typeflag: tar.TypeReg}) == nil {
			tw.Write(data)
		}
		return nil
	})
	tw.Close()
	gz.Close()
}

// Planes Go Code (planesgo-code): paquete para la instalación y la autoactualización.
// El paquete y su version.json se publican sin reconstruir PlanesGo (POST /admin/pgocode/publicar, solo
// administradores) y se guardan en el volumen persistente data/pgocode/. El instalador y el perfil de AppArmor
// van en la imagen (pgocode/). Todo es público para descargar: el paquete no lleva secretos y la app pide
// usuario de Odoo para todo.
const (
	dirPgocodeImagen    = "pgocode"
	dirPgocodePublicado = "data/pgocode"
	paquetePgocode      = "pgocode-linux-amd64.tar.gz"
	maxPaquetePgocode   = 200 << 20
)

var archivosPgocode = map[string]string{
	"version.json":     "application/json",
	paquetePgocode:     "application/gzip",
	"instalar.sh":      "text/x-shellscript; charset=utf-8",
	"apparmor-pgocode": "text/plain; charset=utf-8",
}

func handleInstallPgocode(w http.ResponseWriter, r *http.Request) {
	nombre := strings.TrimPrefix(r.URL.Path, "/install/pgocode/")
	tipo, ok := archivosPgocode[nombre]
	if !ok || r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	dir := dirPgocodeImagen
	if nombre == "version.json" || nombre == paquetePgocode {
		dir = dirPgocodePublicado
	}
	w.Header().Set("Content-Type", tipo)
	if nombre == "version.json" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, path.Join(dir, nombre))
}

// handlePublicarPgocode recibe una versión nueva de Planes Go Code (multipart: version, electron, firma y paquete),
// calcula su huella y la publica. La firma (ed25519 en base64, de make publicar) se guarda tal cual en
// version.json: la comprueban las apps con su clave pública; sin ella no instalan la versión. Solo administradores (token de PlanesGo o sesión de administrador).
func (state *AppState) handlePublicarPgocode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Método no permitido"})
		return
	}
	session, err := state.resolveAntigravitySession(r, "")
	if err != nil || !session.IsAdmin() {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "solo un administrador puede publicar Planes Go Code"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPaquetePgocode+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "se espera multipart con version, electron y paquete: " + err.Error()})
		return
	}
	version, electron := strings.TrimSpace(r.FormValue("version")), strings.TrimSpace(r.FormValue("electron"))
	valido := regexp.MustCompile(`^[0-9][0-9A-Za-z.+-]{0,40}$`)
	if !valido.MatchString(version) || !valido.MatchString(electron) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "version o electron no válidos"})
		return
	}
	firma := strings.TrimSpace(r.FormValue("firma"))
	if b, err := base64.StdEncoding.DecodeString(firma); err != nil || len(b) != 64 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "falta la firma de la versión o no es válida (make publicar la añade)"})
		return
	}
	f, _, err := r.FormFile("paquete")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "falta el paquete"})
		return
	}
	defer f.Close()
	if err := os.MkdirAll(dirPgocodePublicado, 0o755); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	tmp, err := os.CreateTemp(dirPgocodePublicado, "paquete-*")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(f, maxPaquetePgocode+1))
	tmp.Close()
	if err != nil || n > maxPaquetePgocode || n < 18 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "paquete vacío, demasiado grande o incompleto"})
		return
	}
	if cab, err := os.ReadFile(tmp.Name()); err != nil || cab[0] != 0x1f || cab[1] != 0x8b {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "el paquete no es un .tar.gz"})
		return
	}
	pub := map[string]any{"version": version, "sha256": hex.EncodeToString(h.Sum(nil)), "bytes": n,
		"archivo": paquetePgocode, "electron": electron, "firma": firma}
	b, _ := json.Marshal(pub)
	// Primero el paquete y después version.json: una app nunca ve una huella sin su paquete.
	if err := os.Rename(tmp.Name(), path.Join(dirPgocodePublicado, paquetePgocode)); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	vtmp := path.Join(dirPgocodePublicado, "version.json.tmp")
	if err := os.WriteFile(vtmp, append(b, '\n'), 0o644); err != nil || os.Rename(vtmp, path.Join(dirPgocodePublicado, "version.json")) != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "no se pudo publicar version.json"})
		return
	}
	log.Printf("[Planes Go Code] %s publicó la versión %s (%d bytes)", session.UserEmail, version, n)
	json.NewEncoder(w).Encode(pub)
}
