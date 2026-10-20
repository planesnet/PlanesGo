// Package servidor es la API HTTP del arnés PSFCode y sirve la interfaz web.
//
// Reglas:
//   - Todo pide sesión, que se abre con usuario y contraseña de Odoo (planesnet.autopyme.com).
//   - Toda acción va ligada a un proyecto de Odoo que el usuario pueda ver con su propia sesión de Odoo.
//   - Los secretos (sesión de Odoo, token de ver de Studio TV, claves de API) no salen del servidor.
package servidor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/odoo"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/sesiones"
)

const cookie = "psf_sesion"

// vigenciaProyecto: cuánto vale la comprobación de acceso a un proyecto antes de repetirla en Odoo.
const vigenciaProyecto = 5 * time.Minute

// StudioLocal describe el estudio de Studio TV que arranca el propio arnés.
type StudioLocal struct {
	URL       string // dirección del estudio (para emitir y para ver)
	TokenEmit string
	TokenVer  string
}

// Servidor reúne lo que necesita la API.
type Servidor struct {
	Odoo        *odoo.Cliente
	Sesiones    *sesiones.Almacen
	Indice      *proyectos.Indice
	Proveedores *llm.Registro
	Web         fs.FS // interfaz compilada (index.html y assets)
	Studio      *StudioLocal
	Version     string

	mu      sync.Mutex
	nombres map[int]string // nombre de cada proyecto comprobado
}

// Handler devuelve el manejador HTTP completo.
func (s *Servidor) Handler() http.Handler {
	s.nombres = map[int]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.conSesion(s.logout))
	mux.HandleFunc("GET /api/yo", s.conSesion(s.yo))
	mux.HandleFunc("GET /api/proyectos", s.conSesion(s.proyectos))
	mux.HandleFunc("GET /api/proveedores", s.conSesion(s.proveedores))
	mux.HandleFunc("POST /api/chat", s.conSesion(s.chat))
	mux.HandleFunc("POST /api/abrir", s.conSesion(s.abrir))
	mux.HandleFunc("GET /api/studio/conexion", s.conSesion(s.studioConexion))
	mux.HandleFunc("POST /api/studio/conexion", s.conSesion(s.studioConecta))
	mux.HandleFunc("GET /api/studio/{recurso}", s.conSesion(s.studioProxy))
	if s.Web != nil {
		mux.Handle("GET /", s.estaticos())
	}
	return cabeceras(mux)
}

func cabeceras(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// Defensa CSRF: además de SameSite=Strict, toda escritura debe llevar la cabecera X-PSF.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-PSF") != "1" {
			jsonError(w, http.StatusForbidden, "falta la cabecera X-PSF")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// estaticos sirve la interfaz; cualquier ruta desconocida devuelve index.html (aplicación de una página).
func (s *Servidor) estaticos() http.Handler {
	fsrv := http.FileServer(http.FS(s.Web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.Web, p); err != nil {
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fsrv.ServeHTTP(w, r)
	})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func lee(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 4<<20))
	return d.Decode(v)
}

type manejador func(http.ResponseWriter, *http.Request, *sesiones.Sesion)

func (s *Servidor) conSesion(f manejador) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookie)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "inicia sesión")
			return
		}
		ses, ok := s.Sesiones.Busca(c.Value)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "la sesión ha caducado: vuelve a entrar")
			return
		}
		f(w, r, ses)
	}
}

// errorOdoo traduce un error de Odoo; si la sesión de Odoo caducó, cierra también la del arnés.
func (s *Servidor) errorOdoo(w http.ResponseWriter, ses *sesiones.Sesion, err error) {
	if errors.Is(err, odoo.ErrSesionCaducada) {
		s.Sesiones.Borra(ses.Token)
		jsonError(w, http.StatusUnauthorized, "la sesión de Odoo ha caducado: vuelve a entrar")
		return
	}
	jsonError(w, http.StatusBadGateway, "Odoo: "+err.Error())
}

// proyecto comprueba que el usuario puede ver el proyecto en Odoo y devuelve su nombre.
// Es la llave de todo: ninguna acción se hace sin pasar por aquí.
func (s *Servidor) proyecto(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, id int) (string, bool) {
	if id <= 0 {
		jsonError(w, http.StatusBadRequest, "elige un proyecto de Odoo")
		return "", false
	}
	if ses.ProyectoComprobado(id, time.Now()) {
		s.mu.Lock()
		n := s.nombres[id]
		s.mu.Unlock()
		return n, true
	}
	ps, err := s.Odoo.Proyectos(r.Context(), ses.OdooSesion, id)
	if err != nil {
		s.errorOdoo(w, ses, err)
		return "", false
	}
	if len(ps) == 0 {
		jsonError(w, http.StatusForbidden, "ese proyecto no existe en Odoo o no tienes acceso")
		return "", false
	}
	ses.MarcaProyecto(id, time.Now().Add(vigenciaProyecto))
	s.mu.Lock()
	s.nombres[id] = ps[0].Nombre
	s.mu.Unlock()
	return ps[0].Nombre, true
}

func idProyecto(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	return n
}

func (s *Servidor) login(w http.ResponseWriter, r *http.Request) {
	var p struct{ Login, Password string }
	if err := lee(r, &p); err != nil || strings.TrimSpace(p.Login) == "" || p.Password == "" {
		jsonError(w, http.StatusBadRequest, "usuario y contraseña obligatorios")
		return
	}
	u, err := s.Odoo.Autentica(r.Context(), strings.TrimSpace(p.Login), p.Password)
	if err != nil {
		if errors.Is(err, odoo.ErrCredenciales) {
			jsonError(w, http.StatusUnauthorized, err.Error())
		} else {
			jsonError(w, http.StatusBadGateway, err.Error())
		}
		return
	}
	ses, err := s.Sesiones.Crea(u.UID, u.Nombre, u.Login, u.Sesion)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: ses.Token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: int(sesiones.Duracion.Seconds())})
	log.Printf("sesión abierta: %s (uid %d)", u.Login, u.UID)
	jsonOK(w, map[string]any{"nombre": u.Nombre, "login": u.Login})
}

func (s *Servidor) logout(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Odoo.Cierra(ctx, ses.OdooSesion)
	s.Sesiones.Borra(ses.Token)
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Servidor) yo(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	jsonOK(w, map[string]any{"nombre": ses.Nombre, "login": ses.Login, "odoo": s.Odoo.URL, "version": s.Version})
}

type proyectoAPI struct {
	odoo.Proyecto
	Carpetas []string `json:"carpetas"`
}

func (s *Servidor) proyectos(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	ps, err := s.Odoo.Proyectos(r.Context(), ses.OdooSesion, 0)
	if err != nil {
		s.errorOdoo(w, ses, err)
		return
	}
	carpetas := s.Indice.Carpetas(r.URL.Query().Get("recargar") == "1")
	l := make([]proyectoAPI, len(ps))
	hasta := time.Now().Add(vigenciaProyecto)
	s.mu.Lock()
	for i, p := range ps {
		l[i] = proyectoAPI{Proyecto: p, Carpetas: carpetas[p.ID]}
		if l[i].Carpetas == nil {
			l[i].Carpetas = []string{}
		}
		s.nombres[p.ID] = p.Nombre
		ses.MarcaProyecto(p.ID, hasta)
	}
	s.mu.Unlock()
	jsonOK(w, l)
}

func (s *Servidor) proveedores(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	jsonOK(w, s.Proveedores.Lista(r.Context()))
}

// chat responde en streaming (SSE): eventos {texto}, {sesion_agente}, {error} y {fin}.
func (s *Servidor) chat(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto     int           `json:"proyecto"`
		Proveedor    string        `json:"proveedor"`
		Modelo       string        `json:"modelo"`
		Carpeta      string        `json:"carpeta"`
		SesionAgente string        `json:"sesion_agente"`
		Mensajes     []llm.Mensaje `json:"mensajes"`
	}
	if err := lee(r, &p); err != nil || len(p.Mensajes) == 0 {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	nombre, ok := s.proyecto(w, r, ses, p.Proyecto)
	if !ok {
		return
	}
	prov, ok := s.Proveedores.Busca(p.Proveedor)
	if !ok {
		jsonError(w, http.StatusBadRequest, "proveedor desconocido")
		return
	}
	info := prov.Info(r.Context())
	if !info.Disponible {
		jsonError(w, http.StatusBadRequest, info.Nombre+": "+info.Motivo)
		return
	}
	if p.Carpeta != "" && !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "esa carpeta no pertenece al proyecto")
		return
	}
	if info.NecesitaDir && p.Carpeta == "" {
		if cs := s.Indice.Carpetas(false)[p.Proyecto]; len(cs) > 0 {
			p.Carpeta = cs[0]
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	fl, _ := w.(http.Flusher)
	var mu sync.Mutex
	envia := func(v any) {
		b, _ := json.Marshal(v)
		mu.Lock()
		fmt.Fprintf(w, "data: %s\n\n", b)
		if fl != nil {
			fl.Flush()
		}
		mu.Unlock()
	}
	sistema := fmt.Sprintf("Trabajas para %s en el proyecto de Odoo «%s» (id %d) de Planes Software Factory. Responde en español.",
		ses.Nombre, nombre, p.Proyecto)
	if p.Carpeta != "" {
		sistema += " Carpeta local del proyecto: " + p.Carpeta + "."
	}
	err := prov.Chat(r.Context(), llm.Peticion{Modelo: p.Modelo, Sistema: sistema, Mensajes: p.Mensajes,
		Carpeta: p.Carpeta, SesionAgente: p.SesionAgente}, func(e llm.Evento) { envia(e) })
	if err != nil && r.Context().Err() == nil {
		envia(map[string]string{"error": err.Error()})
	}
	envia(map[string]bool{"fin": true})
}

// abrir abre una URL en el navegador del sistema (el arnés corre en el equipo del usuario).
func (s *Servidor) abrir(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		URL      string `json:"url"`
	}
	if err := lee(r, &p); err != nil {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	u, err := url.Parse(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		jsonError(w, http.StatusBadRequest, "solo se abren direcciones http o https")
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u.String())
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u.String())
	default:
		cmd = exec.Command("xdg-open", u.String())
	}
	if err := cmd.Start(); err != nil {
		jsonError(w, http.StatusInternalServerError, "no se pudo abrir el navegador: "+err.Error())
		return
	}
	go cmd.Wait()
	jsonOK(w, map[string]bool{"ok": true})
}

// studioConexion devuelve la conexión de Studio TV del proyecto y los datos del estudio local.
func (s *Servidor) studioConexion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id := idProyecto(r)
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	res := map[string]any{}
	if c, ok := ses.Studio(id); ok {
		res["conexion"] = c
	}
	if s.Studio != nil {
		// El token de emitir del estudio local se enseña para configurar los emisores (PCO_URL / PCO_TOKEN).
		res["local"] = map[string]string{"url": s.Studio.URL, "pco_url": s.Studio.URL, "pco_token": s.Studio.TokenEmit}
	}
	jsonOK(w, res)
}

// studioConecta guarda (y prueba) la conexión con un estudio: el local del arnés o uno externo (URL + token de ver).
func (s *Servidor) studioConecta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Local    bool   `json:"local"`
		URL      string `json:"url"`
		Token    string `json:"token"`
		Quitar   bool   `json:"quitar"`
	}
	if err := lee(r, &p); err != nil {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if p.Quitar {
		ses.PonStudio(p.Proyecto, sesiones.ConexionStudio{})
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	c := sesiones.ConexionStudio{URL: strings.TrimRight(strings.TrimSpace(p.URL), "/"), Token: strings.TrimSpace(p.Token)}
	if p.Local {
		if s.Studio == nil {
			jsonError(w, http.StatusBadRequest, "el estudio local no está activo")
			return
		}
		c = sesiones.ConexionStudio{URL: s.Studio.URL, Token: s.Studio.TokenVer, Local: true}
	}
	if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || c.Token == "" {
		jsonError(w, http.StatusBadRequest, "indica la URL del estudio (http o https) y el token de ver")
		return
	}
	// Prueba la conexión antes de guardarla.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/api/canales", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		jsonError(w, http.StatusBadGateway, "no se pudo conectar con el estudio: "+err.Error())
		return
	}
	resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		jsonError(w, http.StatusBadRequest, "el estudio rechaza el token de ver")
		return
	case resp.StatusCode == http.StatusNotFound:
		jsonError(w, http.StatusBadRequest, "ese estudio es una versión antigua sin canales: arranca studio-tv nuevo")
		return
	case resp.StatusCode != http.StatusOK:
		jsonError(w, http.StatusBadGateway, "el estudio respondió "+resp.Status)
		return
	}
	ses.PonStudio(p.Proyecto, c)
	jsonOK(w, map[string]any{"conexion": c})
}

// studioProxy reenvía canales, snapshot, stream y events al estudio conectado, añadiendo el token de ver.
func (s *Servidor) studioProxy(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	rutas := map[string]string{"canales": "/api/canales", "snapshot": "/snapshot", "stream": "/stream", "events": "/events"}
	ruta, ok := rutas[r.PathValue("recurso")]
	if !ok {
		jsonError(w, http.StatusNotFound, "recurso desconocido")
		return
	}
	id := idProyecto(r)
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	c, ok := ses.Studio(id)
	if !ok {
		jsonError(w, http.StatusConflict, "conecta primero el estudio de este proyecto")
		return
	}
	destino, err := url.Parse(c.URL)
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	q := url.Values{}
	if canal := r.URL.Query().Get("canal"); canal != "" {
		q.Set("canal", canal)
	}
	px := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = destino.Scheme, destino.Host
			pr.Out.URL.Path = strings.TrimRight(destino.Path, "/") + ruta
			pr.Out.URL.RawQuery = q.Encode()
			pr.Out.Host = destino.Host
			pr.Out.Header.Del("Cookie")
			pr.Out.Header.Set("Authorization", "Bearer "+c.Token)
		},
		FlushInterval: -1, // vídeo y eventos en directo
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil {
				jsonError(w, http.StatusBadGateway, "estudio: "+err.Error())
			}
		},
	}
	px.ServeHTTP(w, r)
}
