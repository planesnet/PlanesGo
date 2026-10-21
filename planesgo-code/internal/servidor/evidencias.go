package servidor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/sesiones"
)

// Evidencias: las imágenes que devuelven las herramientas de los agentes (capturas del navegador en las pruebas,
// imágenes leídas). Se guardan en la carpeta de la sesión (<carpeta>/.pgocode/evidencias/<sesión>/, fuera de git)
// con un índice (indice.jsonl); el chat las muestra en miniatura y la TV, en su galería «Capturas».

const maxCaptura = 15 << 20

// Captura: una evidencia guardada (una línea de indice.jsonl).
type Captura struct {
	Ruta        string `json:"ruta"`
	T           string `json:"t"`
	Sesion      string `json:"sesion"`
	Titulo      string `json:"titulo,omitempty"`
	Herramienta string `json:"herramienta,omitempty"`
	Resumen     string `json:"resumen,omitempty"`
	Agente      string `json:"agente,omitempty"`
	Rama        string `json:"rama,omitempty"`
}

var muEvidencias sync.Mutex

func dirEvidencias(carpeta, sesion string) string {
	return filepath.Join(carpeta, ".pgocode", "evidencias", nombreSeguro.ReplaceAllString(sesion, "_"))
}

var extImagen = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp", "image/gif": ".gif"}

// guardaCapturas guarda las imágenes de una herramienta y las cambia por sus rutas (h.Capturas).
func guardaCapturas(carpeta string, c Captura, h *llm.Herramienta) {
	if len(h.Imagenes) == 0 || carpeta == "" || c.Sesion == "" {
		return
	}
	dir := dirEvidencias(carpeta, c.Sesion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	excluyeDeGit(carpeta)
	muEvidencias.Lock()
	defer muEvidencias.Unlock()
	ahora := time.Now()
	idx, err := os.OpenFile(filepath.Join(dir, "indice.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer idx.Close()
	for i, im := range h.Imagenes {
		ext := extImagen[im.Tipo]
		if ext == "" || len(im.Datos) > maxCaptura {
			continue
		}
		nombre := strings.Trim(nombreSeguro.ReplaceAllString(h.Nombre, "_"), "_ ")
		if len(nombre) > 40 {
			nombre = nombre[:40]
		}
		f := filepath.Join(dir, fmt.Sprintf("%s-%03d-%d-%s%s", ahora.Format("20060102-150405"), ahora.Nanosecond()/1e6, i, nombre, ext))
		if os.WriteFile(f, im.Datos, 0o644) != nil {
			continue
		}
		x := c
		x.Ruta, x.T, x.Herramienta, x.Resumen = f, ahora.Format(time.RFC3339), h.Nombre, recortaTexto(h.Resumen, 160)
		b, _ := json.Marshal(x)
		idx.Write(append(b, '\n'))
		h.Capturas = append(h.Capturas, f)
	}
	h.Imagenes = nil
}

// rutaEvidencia: una ruta vale si es una imagen dentro de .pgocode/evidencias de una carpeta del proyecto.
func (s *Servidor) rutaEvidencia(proyecto int, ruta string) (string, bool) {
	r := filepath.Clean(ruta)
	marca := string(filepath.Separator) + filepath.Join(".pgocode", "evidencias") + string(filepath.Separator)
	i := strings.Index(r, marca)
	if !filepath.IsAbs(r) || i < 0 || extImagen[mimeDe(r)] == "" || strings.Contains(r[i+len(marca):], "..") {
		return "", false
	}
	// Ni la evidencia ni la carpeta .pgocode pueden ser enlaces que lleven fuera del proyecto.
	return r, s.Indice.Pertenece(proyecto, r[:i]) && sinSalir(r[:i], r)
}

// sinSalir: la ruta, con los enlaces simbólicos resueltos, sigue dentro de base (también resuelta).
func sinSalir(base, ruta string) bool {
	rb, e1 := filepath.EvalSymlinks(base)
	rr, e2 := filepath.EvalSymlinks(ruta)
	return e1 == nil && e2 == nil && dentroDe(rb, rr)
}

func mimeDe(f string) string {
	for m, e := range extImagen {
		if strings.EqualFold(filepath.Ext(f), e) {
			return m
		}
	}
	return ""
}

// evidencia: sirve una captura.
func (s *Servidor) evidencia(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	q := r.URL.Query()
	id, _ := strconv.Atoi(q.Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	ruta, ok := s.rutaEvidencia(id, q.Get("ruta"))
	if !ok {
		jsonError(w, http.StatusBadRequest, "esa captura no es del proyecto")
		return
	}
	w.Header().Set("Content-Type", mimeDe(ruta))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, ruta)
}

// evidencias: las capturas del proyecto (de todas sus carpetas), la más reciente primero.
func (s *Servidor) evidencias(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	vistas := map[string]bool{}
	l := []Captura{}
	for _, c := range append(append([]string(nil), s.Indice.Carpetas(false)[id]...), s.Indice.Registradas(id)...) {
		if vistas[c] {
			continue
		}
		vistas[c] = true
		dirs, _ := filepath.Glob(filepath.Join(c, ".pgocode", "evidencias", "*", "indice.jsonl"))
		for _, f := range dirs {
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(fh)
			for sc.Scan() {
				var x Captura
				if json.Unmarshal(sc.Bytes(), &x) == nil {
					if _, err := os.Stat(x.Ruta); err == nil {
						l = append(l, x)
					}
				}
			}
			fh.Close()
		}
	}
	sort.Slice(l, func(i, j int) bool { return l[i].T > l[j].T })
	if len(l) > 1000 {
		l = l[:1000]
	}
	jsonOK(w, l)
}
