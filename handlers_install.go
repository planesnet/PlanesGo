package main

import (
	"archive/tar"
	"compress/gzip"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// Instalación servida por el propio PlanesGo, para que no dependa de que el repositorio sea público.
//
//   - /install/claude-hook.sh: instalador del hook de Claude Code (público, no lleva secretos).
//   - /install/planesgo.tar.gz: hook, comandos /planesgo y /pgo y código de planesgo-mcp (público: es lo que
//     el instalador necesita en el «Setup script» de la nube, donde no hay variables de entorno).
//   - /install/psf.tar.gz: Planes Software Factory (carpeta PSF/). Solo con token de empleado.

//go:embed scripts/install-claude-hook.sh .claude/hooks/planesgo_claude_hook.py .claude/commands/planesgo.md .claude/commands/pgo.md go.mod go.sum cmd/planesgo-mcp/main.go
var planesgoBundleFS embed.FS

//go:embed all:PSF
var psfBundleFS embed.FS

func setupInstallRoutes(mux *http.ServeMux, state *AppState) {
	mux.HandleFunc("/install/claude-hook.sh", handleInstallScript)
	mux.HandleFunc("/install/planesgo.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		serveTarGz(w, planesgoBundleFS, ".", "planesgo.tar.gz")
	})
	mux.HandleFunc("/install/psf.tar.gz", state.handleInstallPSF)
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
