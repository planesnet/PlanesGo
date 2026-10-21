package servidor

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"planesgo-code/internal/sesiones"
)

// Ficheros adjuntos del chat: se guardan en la carpeta de la sesión (<carpeta>/.pgocode/adjuntos/<sesión>/),
// excluida de git, para que cualquier agente los lea con sus herramientas sin salir del proyecto.

const maxAdjunto = 25 << 20

var nombreSeguro = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func dirAdjuntos(carpeta, sesion string) string {
	return filepath.Join(carpeta, ".pgocode", "adjuntos", nombreSeguro.ReplaceAllString(sesion, "_"))
}

// excluyeDeGit añade «.pgocode/» a .git/info/exclude del repositorio de la carpeta (también en copias de trabajo).
func excluyeDeGit(carpeta string) {
	out, err := exec.Command("git", "-C", carpeta, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(carpeta, dir)
	}
	f := filepath.Join(dir, "info", "exclude")
	if b, _ := os.ReadFile(f); strings.Contains(string(b), "\n.pgocode/") || strings.HasPrefix(string(b), ".pgocode/") {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(fh, ".pgocode/")
		fh.Close()
	}
}

// adjunto recibe un fichero (multipart, campo «fichero») y lo guarda en la carpeta de la sesión.
func (s *Servidor) adjunto(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	q := r.URL.Query()
	id, _ := strconv.Atoi(q.Get("proyecto"))
	carpeta, sesion := q.Get("carpeta"), q.Get("sesion")
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	if carpeta == "" || sesion == "" || !s.Indice.Pertenece(id, carpeta) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAdjunto+1<<20)
	f, cab, err := r.FormFile("fichero")
	if err != nil {
		jsonError(w, http.StatusBadRequest, "fichero no válido o de más de 25 MB")
		return
	}
	defer f.Close()
	nombre := strings.TrimSpace(nombreSeguro.ReplaceAllString(filepath.Base(cab.Filename), "_"))
	if nombre == "" || nombre == "." || nombre == ".." {
		nombre = "adjunto"
	}
	dir := dirAdjuntos(carpeta, sesion)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	excluyeDeGit(carpeta)
	ruta := filepath.Join(dir, time.Now().Format("150405")+"-"+nombre)
	dst, err := os.OpenFile(ruta, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, err := io.Copy(dst, io.LimitReader(f, maxAdjunto+1))
	dst.Close()
	if err != nil || n > maxAdjunto {
		os.Remove(ruta)
		jsonError(w, http.StatusBadRequest, "fichero de más de 25 MB")
		return
	}
	jsonOK(w, map[string]any{"nombre": cab.Filename, "ruta": ruta, "tamano": n, "tipo": cab.Header.Get("Content-Type")})
}

// adjuntosValidos: solo rutas de adjuntos de esa carpeta (lo que llega del navegador no sale de ahí).
func adjuntosValidos(carpeta string, l []string) []string {
	base := filepath.Join(carpeta, ".pgocode", "adjuntos") + string(filepath.Separator)
	var v []string
	for _, p := range l {
		p = filepath.Clean(p)
		if strings.HasPrefix(p, base) && sinSalir(base, p) {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				v = append(v, p)
			}
		}
	}
	return v
}

// LimpiaAdjuntos borra las carpetas de adjuntos (una por sesión) sin cambios en más de 30 días, en todas las
// carpetas de los proyectos. Las evidencias no se tocan: son el resultado de las pruebas.
func (s *Servidor) LimpiaAdjuntos() (borradas int) {
	limite := time.Now().AddDate(0, 0, -30)
	for _, cs := range s.Indice.Carpetas(false) {
		for _, c := range cs {
			base := filepath.Join(c, ".pgocode", "adjuntos")
			if st, err := os.Lstat(base); err != nil || !st.IsDir() {
				continue // tampoco si es un enlace
			}
			ents, _ := os.ReadDir(base)
			for _, e := range ents {
				d := filepath.Join(base, e.Name())
				if st, err := os.Lstat(d); err == nil && st.IsDir() && st.ModTime().Before(limite) && !modificadoDesde(d, limite) {
					if os.RemoveAll(d) == nil {
						borradas++
					}
				}
			}
		}
	}
	return
}

// modificadoDesde: algún fichero de la carpeta ha cambiado después de t.
func modificadoDesde(d string, t time.Time) bool {
	ents, _ := os.ReadDir(d)
	for _, e := range ents {
		if i, err := e.Info(); err == nil && i.ModTime().After(t) {
			return true
		}
	}
	return false
}
