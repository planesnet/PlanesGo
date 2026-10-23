package servidor

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/procesos"
	"planesgo-code/internal/sesiones"
)

// Programas del proyecto ejecutados en local (Odoo, servicios, el programa en desarrollo): los lanza el usuario
// desde el panel o el agente con la orden «pgo» (que habla con /api/pgo/* con el TokenAgente de su entorno).
// La interfaz se entera de los cambios por /api/eventos (SSE) y abre las URL en el navegador integrado.

// Configuracion es una forma guardada de arrancar un programa: <carpeta>/.psf/ejecutar.json.
type Configuracion struct {
	Nombre string `json:"nombre"`
	Orden  string `json:"orden"`
	URL    string `json:"url,omitempty"` // dónde abrirlo si el programa no lo dice en su salida
}

func leeConfiguraciones(carpeta string) []Configuracion {
	l := []Configuracion{}
	if b, err := os.ReadFile(filepath.Join(carpeta, ".psf", "ejecutar.json")); err == nil {
		json.Unmarshal(b, &l)
	}
	return l
}

func guardaConfiguracion(carpeta string, c Configuracion) error {
	l := leeConfiguraciones(carpeta)
	puesta := false
	for i := range l {
		if l[i].Nombre == c.Nombre {
			l[i], puesta = c, true
		}
	}
	if !puesta {
		l = append(l, c)
	}
	b, _ := json.MarshalIndent(l, "", "  ")
	if err := os.MkdirAll(filepath.Join(carpeta, ".psf"), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(carpeta, ".psf", "ejecutar.json"), append(b, '\n'), 0o644)
}

// Eventos reparte avisos a las ventanas abiertas (SSE).
type Eventos struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (e *Eventos) Emite(v any) {
	b, _ := json.Marshal(v)
	e.mu.Lock()
	defer e.mu.Unlock()
	for c := range e.subs {
		select {
		case c <- b:
		default: // ventana lenta: se pierde el aviso, no se bloquea
		}
	}
}

func (e *Eventos) alta() chan []byte {
	c := make(chan []byte, 32)
	e.mu.Lock()
	if e.subs == nil {
		e.subs = map[chan []byte]struct{}{}
	}
	e.subs[c] = struct{}{}
	e.mu.Unlock()
	return c
}

func (e *Eventos) baja(c chan []byte) {
	e.mu.Lock()
	delete(e.subs, c)
	e.mu.Unlock()
}

// eventos: avisos para la interfaz ({"tipo":"procesos"} o {"tipo":"abrir","url":…}).
func (s *Servidor) eventos(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	fl, _ := w.(http.Flusher)
	c := s.Eventos.alta()
	defer s.Eventos.baja(c)
	t := time.NewTicker(25 * time.Second)
	defer t.Stop()
	fmt.Fprint(w, ": hola\n\n")
	for {
		if fl != nil {
			fl.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			fmt.Fprintf(w, "data: %s\n\n", b)
		case <-t.C:
			fmt.Fprint(w, ": sigo\n\n")
		}
	}
}

// AvisaProcesos se conecta al gestor: cada cambio llega a la interfaz; y la primera URL de un proceso se abre.
func (s *Servidor) AvisaProcesos() {
	abiertas := map[string]bool{}
	var mu sync.Mutex
	s.Procesos.Aviso = func() {
		s.Eventos.Emite(map[string]string{"tipo": "procesos"})
		for _, p := range s.Procesos.Lista(0) {
			clave := p.Nombre + "|" + p.Inicio.String()
			mu.Lock()
			nueva := p.URL != "" && p.Estado == "curso" && !abiertas[clave]
			if nueva {
				abiertas[clave] = true
			}
			mu.Unlock()
			if nueva {
				s.Eventos.Emite(map[string]any{"tipo": "abrir", "url": p.URL, "proyecto": p.Proyecto, "proceso": p.Nombre})
			}
		}
	}
}

func urlWeb(u string) bool {
	x, err := url.Parse(u)
	return err == nil && (x.Scheme == "http" || x.Scheme == "https") && x.Host != ""
}

// ---- desde la interfaz ----

func (s *Servidor) procesosLista(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	carpeta := r.URL.Query().Get("carpeta")
	cfg := []Configuracion{}
	if carpeta != "" && s.Indice.Pertenece(id, carpeta) {
		cfg = leeConfiguraciones(carpeta)
	}
	jsonOK(w, map[string]any{"procesos": s.Procesos.Lista(id), "configuraciones": cfg})
}

func (s *Servidor) procesosLanza(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Nombre   string `json:"nombre"`
		Orden    string `json:"orden"`
		URL      string `json:"url"`
		Guardar  bool   `json:"guardar"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "esa carpeta no pertenece al proyecto")
		return
	}
	if p.URL != "" && !urlWeb(p.URL) {
		jsonError(w, http.StatusBadRequest, "URL no válida")
		return
	}
	if p.Guardar {
		if err := guardaConfiguracion(p.Carpeta, Configuracion{Nombre: p.Nombre, Orden: p.Orden, URL: p.URL}); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	x, err := s.Procesos.Lanza(p.Nombre, p.Proyecto, p.Carpeta, p.Orden, "usuario", s.entornoPrograma(p.Proyecto))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("programa lanzado por %s: %s (%s)", ses.Login, p.Nombre, p.Carpeta)
	if p.URL != "" {
		s.Eventos.Emite(map[string]any{"tipo": "abrir", "url": p.URL, "proyecto": p.Proyecto, "proceso": p.Nombre})
	}
	jsonOK(w, x)
}

// quitaConfiguracion: borra un programa guardado (si no estaba, nada).
func quitaConfiguracion(carpeta, nombre string) error {
	l := leeConfiguraciones(carpeta)
	n := l[:0]
	for _, c := range l {
		if c.Nombre != nombre {
			n = append(n, c)
		}
	}
	if len(n) == len(leeConfiguraciones(carpeta)) {
		return nil
	}
	b, _ := json.MarshalIndent(n, "", "  ")
	return os.WriteFile(filepath.Join(carpeta, ".psf", "ejecutar.json"), append(b, '\n'), 0o644)
}

// procesosBorraConfig: quita un programa de los guardados del proyecto (.psf/ejecutar.json); si está en marcha, sigue.
func (s *Servidor) procesosBorraConfig(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Nombre   string `json:"nombre"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "esa carpeta no pertenece al proyecto")
		return
	}
	if err := quitaConfiguracion(p.Carpeta, p.Nombre); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.AvisaProcesos()
	jsonOK(w, map[string]bool{"ok": true})
}

// procesosGuarda: guarda un programa en el proyecto (.psf/ejecutar.json) sin lanzarlo, p. ej. uno que el agente
// dejó en marcha y pgocode adoptó: la próxima vez lo arranca pgocode, sin el agente ni tokens.
func (s *Servidor) procesosGuarda(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Nombre   string `json:"nombre"`
		Orden    string `json:"orden"`
		URL      string `json:"url"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "esa carpeta no pertenece al proyecto")
		return
	}
	if p.URL != "" && !urlWeb(p.URL) {
		jsonError(w, http.StatusBadRequest, "URL no válida")
		return
	}
	if strings.TrimSpace(p.Nombre) == "" || strings.TrimSpace(p.Orden) == "" {
		jsonError(w, http.StatusBadRequest, "faltan el nombre o la orden")
		return
	}
	if err := guardaConfiguracion(p.Carpeta, Configuracion{Nombre: p.Nombre, Orden: p.Orden, URL: p.URL}); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.AvisaProcesos()
	jsonOK(w, map[string]bool{"ok": true})
}

// del proyecto: el proceso tiene que ser de un proyecto al que el usuario tenga acceso.
func (s *Servidor) procesoDelUsuario(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, nombre string) bool {
	p, ok := s.Procesos.Busca(nombre)
	if !ok {
		jsonError(w, http.StatusNotFound, procesos.ErrNoHay.Error())
		return false
	}
	_, ok = s.proyecto(w, r, ses, p.Proyecto)
	return ok
}

func (s *Servidor) procesosPara(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Nombre string `json:"nombre"`
		Quitar bool   `json:"quitar"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if !s.procesoDelUsuario(w, r, ses, p.Nombre) {
		return
	}
	s.Procesos.Para(p.Nombre)
	if p.Quitar {
		s.Procesos.Quita(p.Nombre)
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Servidor) procesosSalida(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	nombre := r.URL.Query().Get("nombre")
	if !s.procesoDelUsuario(w, r, ses, nombre) {
		return
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("n"))
	l, total, _ := s.Procesos.Salida(nombre, n)
	jsonOK(w, map[string]any{"lineas": l, "total": total})
}

// ---- desde el agente (orden pgo) ----

// entornoAgente: lo que reciben los agentes para usar «pgo»: su token vale solo para su proyecto.
func (s *Servidor) entornoAgente(proyecto int) []string {
	return append(s.entornoPrograma(proyecto), "PGO_TOKEN="+s.tokenProyecto(proyecto))
}

// entornoPrograma: lo que reciben los programas que se lanzan (sin token: un programa o sus dependencias no
// pueden dar órdenes a pgocode). PLANESGO_GESTOR: PlanesGo (hooks, plugins y planesgo-mcp) se queda dormido,
// porque imputa pgocode directamente en Odoo.
func (s *Servidor) entornoPrograma(proyecto int) []string {
	e := []string{"PGO_API=http://127.0.0.1:" + s.Puerto, "PGO_PROYECTO=" + strconv.Itoa(proyecto), "PLANESGO_GESTOR=pgocode"}
	if s.DirBin != "" {
		e = append(e, "PATH="+s.DirBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	return e
}

// tokenProyecto: el token de los agentes de un proyecto (derivado del secreto de esta ejecución).
func (s *Servidor) tokenProyecto(proyecto int) string {
	m := hmac.New(sha256.New, []byte(s.TokenAgente))
	fmt.Fprintf(m, "pgo-proyecto:%d", proyecto)
	return hex.EncodeToString(m.Sum(nil))
}

// esAgente: el token es el del proyecto que dice la orden.
func (s *Servidor) esAgente(r *http.Request, proyecto int) bool {
	t := r.Header.Get("X-PGO-Agente")
	return s.TokenAgente != "" && t != "" && proyecto > 0 && subtle.ConstantTimeCompare([]byte(t), []byte(s.tokenProyecto(proyecto))) == 1
}

// dentroDelProyecto: la carpeta está en una de las del proyecto (o en una copia de rama de pgocode), también
// tras resolver los enlaces simbólicos.
func (s *Servidor) dentroDelProyecto(proyecto int, carpeta string) bool {
	real, err := filepath.EvalSymlinks(carpeta)
	if err != nil {
		return false
	}
	bases := append(s.Indice.Carpetas(false)[proyecto], s.Indice.Registradas(proyecto)...)
	if s.DirRamas != "" {
		bases = append(bases, s.DirRamas)
	}
	for _, b := range bases {
		if rb, err := filepath.EvalSymlinks(b); err == nil && dentroDe(rb, real) {
			return true
		}
	}
	return false
}

// dentroDe: ruta está en base (o es base).
func dentroDe(base, ruta string) bool {
	rel, err := filepath.Rel(base, ruta)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// delProyecto: el programa existe y es del proyecto.
func (s *Servidor) delProyecto(nombre string, proyecto int) bool {
	p, ok := s.Procesos.Busca(nombre)
	return ok && p.Proyecto == proyecto
}

// pgo atiende las órdenes del agente: ejecutar, parar, procesos, salida y abrir.
func (s *Servidor) pgo(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Nombre   string `json:"nombre"`
		Orden    string `json:"orden"`
		URL      string `json:"url"`
		N        int    `json:"n"`
	}
	if r.Method == http.MethodPost {
		if err := lee(r, &p); err != nil {
			errorLectura(w, err)
			return
		}
	} else {
		q := r.URL.Query()
		p.Nombre = q.Get("nombre")
		p.Proyecto, _ = strconv.Atoi(q.Get("proyecto"))
		p.N, _ = strconv.Atoi(q.Get("n"))
	}
	// El token es el del proyecto: el agente solo actúa en su proyecto, sus carpetas y sus programas.
	if !s.esAgente(r, p.Proyecto) {
		jsonError(w, http.StatusForbidden, "token de agente no válido para este proyecto")
		return
	}
	switch r.PathValue("orden") {
	case "parar", "salida":
		if !s.delProyecto(p.Nombre, p.Proyecto) {
			jsonError(w, http.StatusNotFound, procesos.ErrNoHay.Error())
			return
		}
	}
	switch r.PathValue("orden") {
	case "ejecutar":
		if !filepath.IsAbs(p.Carpeta) || !s.dentroDelProyecto(p.Proyecto, p.Carpeta) {
			jsonError(w, http.StatusForbidden, "la carpeta tiene que ser una del proyecto")
			return
		}
		x, err := s.Procesos.Lanza(p.Nombre, p.Proyecto, p.Carpeta, p.Orden, "agente", s.entornoPrograma(p.Proyecto))
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("programa lanzado por el agente: %s (%s)", p.Nombre, p.Carpeta)
		jsonOK(w, x)
	case "parar":
		jsonOK(w, map[string]bool{"parado": s.Procesos.Para(p.Nombre)})
	case "procesos":
		jsonOK(w, s.Procesos.Lista(p.Proyecto))
	case "salida":
		l, total, err := s.Procesos.Salida(p.Nombre, p.N)
		if err != nil {
			jsonError(w, http.StatusNotFound, err.Error())
			return
		}
		jsonOK(w, map[string]any{"lineas": l, "total": total})
	case "abrir":
		if !urlWeb(p.URL) {
			jsonError(w, http.StatusBadRequest, "URL no válida (http o https)")
			return
		}
		s.Eventos.Emite(map[string]any{"tipo": "abrir", "url": p.URL, "proyecto": p.Proyecto})
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, http.StatusNotFound, "orden desconocida")
	}
}

// esRutaAgente: las rutas que el agente puede usar sin la cookie de la app de escritorio.
func esRutaAgente(p string) bool { return strings.HasPrefix(p, "/api/pgo/") }
