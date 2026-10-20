// Package servidor es la API HTTP del arnés Planes Go Code y sirve la interfaz web.
//
// Reglas:
//   - Todo pide sesión, que se abre con usuario y contraseña de Odoo (planesnet.autopyme.com).
//   - Toda acción va ligada a un proyecto de Odoo que el usuario pueda ver con su propia sesión de Odoo.
//   - Los secretos (sesión de Odoo, token de ver de Studio TV, claves de API) no salen del servidor.
package servidor

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"planesgo-code/internal/actualizacion"
	"planesgo-code/internal/github"
	"planesgo-code/internal/llm"
	"planesgo-code/internal/odoo"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/qa"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/sesiones"
)

const cookie = "pgo_sesion"

// cookieEscritorio la pone Electron en su ventana con el TokenEscritorio.
const cookieEscritorio = "pgo_escritorio"

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
	// Puerto en el que escucha: solo se aceptan peticiones a 127.0.0.1, localhost o [::1] en este puerto
	// (defensa contra DNS rebinding: una web ajena no puede hablar con la API aunque resuelva a 127.0.0.1).
	Puerto string
	// TokenEscritorio: secreto que genera la app de escritorio (Electron) y guarda como cookie en su ventana.
	// Si está puesto, toda petición tiene que traerlo: un navegador normal no puede usar Planes Go Code.
	TokenEscritorio string
	// Actualizador: autoactualización desde PlanesGo (nil: sin actualizaciones).
	Actualizador *actualizacion.Actualizador
	// QA: pruebas lanzadas desde la app (campañas .psf/qa del proyecto).
	QA qa.Ejecutor
	// DirRamas: dónde se crean las copias de trabajo de otras ramas (git worktree) de las sesiones.
	DirRamas string
	// Procesos: programas del proyecto en marcha (Odoo, servicios…); Eventos: avisos a la interfaz (SSE).
	Procesos *procesos.Gestor
	Eventos  Eventos
	// GitHub: issues y pull requests del repositorio (con la sesión de «gh» del usuario).
	GitHub github.Lector
	// TokenAgente: secreto que reciben los agentes (PGO_TOKEN) para usar la orden «pgo»; DirBin: carpeta con «pgo».
	TokenAgente string
	DirBin      string
	// EstudioOcupado dice si el Studio TV local está en uso (directo o grabación).
	EstudioOcupado func() bool
	activos        atomic.Int64 // chats y agentes en curso
	// PermitirNavegador abre Planes Go Code también al navegador (parámetro -navegador): sin él, y sin TokenEscritorio,
	// no se atiende ninguna petición.
	PermitirNavegador bool

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
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, r *http.Request) { jsonOK(w, map[string]string{"version": s.Version}) })
	mux.HandleFunc("GET /api/proyecto/repo", s.conSesion(s.repoProyecto))
	// Campañas de pruebas del proyecto: casos, vídeos, pruebas que se pueden lanzar y comentarios.
	mux.HandleFunc("GET /api/qa/campanas", s.conSesion(s.qaCampanas))
	mux.HandleFunc("GET /api/qa/casos", s.conSesion(s.qaCasos))
	mux.HandleFunc("GET /api/qa/media", s.conSesion(s.qaMedia))
	mux.HandleFunc("GET /api/qa/pruebas", s.conSesion(s.qaPruebas))
	mux.HandleFunc("POST /api/qa/lanzar", s.conSesion(s.qaLanzar))
	mux.HandleFunc("POST /api/qa/parar", s.conSesion(s.qaParar))
	mux.HandleFunc("GET /api/qa/comentarios", s.conSesion(s.qaComentarios))
	mux.HandleFunc("POST /api/qa/comentario", s.conSesion(s.qaComenta))
	mux.HandleFunc("GET /api/avatar", s.conSesion(s.avatar))
	mux.HandleFunc("GET /api/actualizacion", s.conSesion(s.actualizacion))
	mux.HandleFunc("POST /api/actualizacion/buscar", s.conSesion(s.buscaActualizacion))
	mux.HandleFunc("GET /api/proyectos", s.conSesion(s.proyectos))
	mux.HandleFunc("GET /api/proveedores", s.conSesion(s.proveedores))
	mux.HandleFunc("POST /api/chat", s.conSesion(s.chat))
	mux.HandleFunc("POST /api/abrir", s.conSesion(s.abrir))
	mux.HandleFunc("GET /api/eventos", s.conSesion(s.eventos))
	mux.HandleFunc("GET /api/github", s.conSesion(s.githubEstado))
	mux.HandleFunc("GET /api/procesos", s.conSesion(s.procesosLista))
	mux.HandleFunc("POST /api/procesos/lanzar", s.conSesion(s.procesosLanza))
	mux.HandleFunc("POST /api/procesos/parar", s.conSesion(s.procesosPara))
	mux.HandleFunc("GET /api/procesos/salida", s.conSesion(s.procesosSalida))
	mux.HandleFunc("GET /api/pgo/{orden}", s.pgo)
	mux.HandleFunc("POST /api/pgo/{orden}", s.pgo)
	mux.HandleFunc("GET /api/studio/conexion", s.conSesion(s.studioConexion))
	mux.HandleFunc("POST /api/studio/conexion", s.conSesion(s.studioConecta))
	mux.HandleFunc("GET /api/studio/{recurso}", s.conSesion(s.studioProxy))
	mux.HandleFunc("POST /api/studio/grabar", s.conSesion(s.studioProxy))
	if s.Web != nil {
		mux.Handle("GET /", s.estaticos())
	}
	return s.cabeceras(mux)
}

// desdeEscritorio dice si la petición viene de la app de escritorio (o si se permite el navegador).
func (s *Servidor) desdeEscritorio(r *http.Request) bool {
	if s.PermitirNavegador {
		return true
	}
	if s.TokenEscritorio == "" {
		return false
	}
	c, err := r.Cookie(cookieEscritorio)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.TokenEscritorio)) == 1
}

const paginaSoloEscritorio = `<!doctype html><html lang="es"><head><meta charset="utf-8"><title>Planes Go Code</title>
<style>body{font:15px/1.5 system-ui,sans-serif;max-width:560px;margin:15vh auto;padding:0 16px;color:#1f1e1b;background:#faf9f5}
code{background:#f0eee6;border-radius:4px;padding:1px 5px}</style></head><body>
<h1>Planes Go Code</h1><p>Planes Go Code se abre desde su <b>aplicación de escritorio</b>, no desde el navegador.</p>
<p>Para usarlo en el navegador (por ejemplo, para desarrollar), arranca el backend con <code>pgocode -navegador</code>.</p>
</body></html>`

// hostLocal dice si host (con puerto) es 127.0.0.1, localhost o [::1] en el puerto del arnés.
func (s *Servidor) hostLocal(host string) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil || p != s.Puerto {
		return false
	}
	return h == "127.0.0.1" || h == "::1" || strings.EqualFold(h, "localhost")
}

func (s *Servidor) cabeceras(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !s.hostLocal(r.Host) {
			jsonError(w, http.StatusMisdirectedRequest, "host no permitido")
			return
		}
		agente := esRutaAgente(r.URL.Path) && s.esAgente(r)
		if r.URL.Path != "/healthz" && !agente && !s.desdeEscritorio(r) {
			if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusForbidden)
				io.WriteString(w, paginaSoloEscritorio)
				return
			}
			jsonError(w, http.StatusForbidden, "Planes Go Code solo se abre desde la aplicación de escritorio")
			return
		}
		// Si el navegador dice de dónde viene la petición, tiene que ser del propio arnés.
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || u.Scheme != "http" || !s.hostLocal(u.Host) {
				jsonError(w, http.StatusForbidden, "origen no permitido")
				return
			}
		}
		// Defensa CSRF: además de SameSite=Strict, toda escritura debe llevar la cabecera X-PGO.
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("X-PGO") != "1" {
			jsonError(w, http.StatusForbidden, "falta la cabecera X-PGO")
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

// maxPeticion es el tamaño máximo de una petición JSON (una conversación larga con una API cabe de sobra).
const maxPeticion = 8 << 20

func lee(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxPeticion))
	return d.Decode(v)
}

// errorLectura responde al fallo de lee: 413 si la petición es demasiado grande, 400 si no es válida.
func errorLectura(w http.ResponseWriter, err error) {
	var grande *http.MaxBytesError
	if errors.As(err, &grande) {
		jsonError(w, http.StatusRequestEntityTooLarge, "la conversación es demasiado larga: empieza una nueva")
		return
	}
	jsonError(w, http.StatusBadRequest, "petición no válida")
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

// repoProyecto da el tipo de hosting, el repositorio y las ramas de una carpeta del proyecto.
func (s *Servidor) repoProyecto(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id := idProyecto(r)
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	carpeta := r.URL.Query().Get("carpeta")
	if carpeta == "" || !s.Indice.Pertenece(id, carpeta) {
		jsonError(w, http.StatusBadRequest, "esa carpeta no pertenece al proyecto")
		return
	}
	jsonOK(w, repo.Lee(r.Context(), carpeta))
}

// Ocupado dice si hay algo en curso que una actualización interrumpiría.
func (s *Servidor) Ocupado() bool {
	return s.activos.Load() > 0 || s.QA.EnCurso() > 0 || (s.Procesos != nil && s.Procesos.EnCurso() > 0) || (s.EstudioOcupado != nil && s.EstudioOcupado())
}

func (s *Servidor) actualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Actualizador == nil {
		jsonOK(w, actualizacion.Estado{Version: s.Version, Estado: "desactivada"})
		return
	}
	e := s.Actualizador.Estado()
	jsonOK(w, map[string]any{"version": e.Version, "estado": e.Estado, "nueva": e.Nueva, "error": e.Error,
		"revisado": e.Revisado, "ocupado": s.Ocupado()})
}

func (s *Servidor) buscaActualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Actualizador != nil {
		s.Actualizador.BuscarYa()
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// avatar sirve la foto del usuario de Odoo (con su sesión); sin foto, 404 y la interfaz pone las iniciales.
func (s *Servidor) avatar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	b, tipo, err := s.Odoo.Avatar(r.Context(), ses.OdooSesion, ses.UID)
	if err != nil {
		jsonError(w, http.StatusNotFound, "sin avatar")
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(b)
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
	s.activos.Add(1) // mientras haya un chat o un agente en curso no se actualiza
	defer s.activos.Add(-1)
	var p struct {
		Proyecto     int           `json:"proyecto"`
		Proveedor    string        `json:"proveedor"`
		Modelo       string        `json:"modelo"`
		Carpeta      string        `json:"carpeta"`
		SesionAgente string        `json:"sesion_agente"`
		Rama         string        `json:"rama"`
		Permisos     string        `json:"permisos"`
		Esfuerzo     string        `json:"esfuerzo"`
		Mensajes     []llm.Mensaje `json:"mensajes"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if len(p.Mensajes) == 0 {
		jsonError(w, http.StatusBadRequest, "no hay mensaje")
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
	if info.Tipo == "api" {
		// El modelo tiene que ser uno de los que ofrece el proveedor (va en la URL o en el cuerpo de la API).
		if p.Modelo == "" {
			p.Modelo = info.PorDefecto
		}
		if p.Modelo == "" || (len(info.Modelos) > 0 && !slices.Contains(info.Modelos, p.Modelo)) {
			jsonError(w, http.StatusBadRequest, "modelo no disponible para "+info.Nombre)
			return
		}
	}
	if info.Tipo == "agente" {
		// Modelo y esfuerzo van como argumentos del agente: solo los de su lista.
		// Un modelo que ya no está en su lista (p. ej. «por defecto» guardado en la sesión): el de por defecto.
		if p.Modelo != "" && !slices.Contains(info.Modelos, p.Modelo) {
			p.Modelo = info.PorDefecto
		}
		if p.Esfuerzo != "" && !slices.Contains(info.Esfuerzos, p.Esfuerzo) {
			p.Esfuerzo = ""
		}
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
	// La sesión trabaja en su rama: si no es la de la carpeta, en una copia de trabajo propia (git worktree).
	if info.NecesitaDir && p.Carpeta != "" && p.Rama != "" {
		dir, err := repo.CarpetaDeRama(r.Context(), p.Carpeta, p.Rama, s.DirRamas)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Carpeta = dir
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
	if p.Permisos != "" && p.Permisos != "lectura" && p.Permisos != "editar" && p.Permisos != "auto" {
		p.Permisos = "lectura"
	}
	err := prov.Chat(r.Context(), llm.Peticion{Modelo: p.Modelo, Sistema: sistema, Mensajes: p.Mensajes,
		Carpeta: p.Carpeta, SesionAgente: p.SesionAgente, Permisos: p.Permisos, Esfuerzo: p.Esfuerzo, Entorno: s.entornoAgente(p.Proyecto)}, func(e llm.Evento) { envia(e) })
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
// transporteStudio limita la espera de las cabeceras del estudio: uno que acepta la conexión y no responde
// no puede dejar peticiones colgadas (el navegador solo abre unas pocas por servidor).
var transporteStudio = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 10 * time.Second
	return t
}()

// recursosStudio: ruta en el estudio y tipo de contenido que se acepta de vuelta.
var recursosStudio = map[string]struct{ ruta, tipo string }{
	"canales":  {"/api/canales", "application/json"},
	"snapshot": {"/snapshot", "image/jpeg"},
	"stream":   {"/stream", "multipart/x-mixed-replace"},
	"events":   {"/events", "text/event-stream"},
	// grabación: la controla quien mira (por defecto, solo directo)
	"grabar":      {"/api/grabar", "application/json"},
	"grabaciones": {"/api/grabaciones", "application/json"},
	"grabacion":   {"/grabacion", "video/mp4"},
}

func (s *Servidor) studioProxy(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	rec, ok := recursosStudio[r.PathValue("recurso")]
	ruta := rec.ruta
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
	for _, k := range []string{"canal", "id"} {
		if v := r.URL.Query().Get(k); v != "" {
			q.Set(k, v)
		}
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
		Transport:     transporteStudio,
		FlushInterval: -1, // vídeo y eventos en directo
		// Lo que devuelve el estudio llega al navegador en el origen del arnés: solo el tipo esperado,
		// sin cookies y sin poder ejecutarse como página.
		ModifyResponse: func(res *http.Response) error {
			res.Header.Del("Set-Cookie")
			res.Header.Set("Content-Security-Policy", "sandbox")
			res.Header.Set("X-Content-Type-Options", "nosniff")
			if res.StatusCode == http.StatusOK && !strings.HasPrefix(res.Header.Get("Content-Type"), rec.tipo) {
				return fmt.Errorf("el estudio devolvió %q en lugar de %s", res.Header.Get("Content-Type"), rec.tipo)
			}
			if res.StatusCode != http.StatusOK {
				res.Header.Set("Content-Type", "text/plain; charset=utf-8")
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if r.Context().Err() == nil {
				jsonError(w, http.StatusBadGateway, "estudio: "+err.Error())
			}
		},
	}
	px.ServeHTTP(w, r)
}

// --- Pruebas (campañas .psf/qa del proyecto) ---

// campana comprueba el proyecto y devuelve la carpeta de la campaña pedida (?campana= o en el cuerpo).
func (s *Servidor) campana(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, proyecto int, id string) (string, bool) {
	if _, ok := s.proyecto(w, r, ses, proyecto); !ok {
		return "", false
	}
	dir, err := qa.Dir(s.Indice.Carpetas(false)[proyecto], id)
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return "", false
	}
	return dir, true
}

func (s *Servidor) qaCampanas(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id := idProyecto(r)
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	jsonOK(w, qa.Busca(s.Indice.Carpetas(false)[id]))
}

func (s *Servidor) qaCasos(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	dir, ok := s.campana(w, r, ses, idProyecto(r), r.URL.Query().Get("campana"))
	if !ok {
		return
	}
	b, err := qa.Casos(dir)
	if err != nil || !json.Valid(b) {
		jsonError(w, http.StatusInternalServerError, "casos.json no válido")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(b)
}

func (s *Servidor) qaMedia(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	dir, ok := s.campana(w, r, ses, idProyecto(r), r.URL.Query().Get("campana"))
	if !ok {
		return
	}
	ruta, err := qa.Media(dir, r.URL.Query().Get("archivo"))
	if err != nil {
		jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	http.ServeFile(w, r, ruta) // admite Range: saltar a un marcador
}

func (s *Servidor) qaPruebas(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id := r.URL.Query().Get("campana")
	dir, ok := s.campana(w, r, ses, idProyecto(r), id)
	if !ok {
		return
	}
	jsonOK(w, map[string]any{"pruebas": qa.Pruebas(dir), "ejecuciones": s.QA.Lista(id)})
}

func (s *Servidor) qaLanzar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Campana  string `json:"campana"`
		Script   string `json:"script"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	dir, ok := s.campana(w, r, ses, p.Proyecto, p.Campana)
	if !ok {
		return
	}
	x, err := s.QA.Lanza(p.Campana, dir, p.Script)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("prueba lanzada por %s: %s %s", ses.Login, p.Campana, p.Script)
	jsonOK(w, x)
}

func (s *Servidor) qaParar(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Campana  string `json:"campana"`
		ID       string `json:"id"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if _, ok := s.campana(w, r, ses, p.Proyecto, p.Campana); !ok {
		return
	}
	jsonOK(w, map[string]bool{"parada": s.QA.Para(p.ID)})
}

func (s *Servidor) qaComentarios(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	dir, ok := s.campana(w, r, ses, idProyecto(r), r.URL.Query().Get("campana"))
	if !ok {
		return
	}
	jsonOK(w, qa.Comentarios(dir))
}

func (s *Servidor) qaComenta(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int     `json:"proyecto"`
		Campana  string  `json:"campana"`
		Caso     string  `json:"caso"`
		T        float64 `json:"t"`
		Texto    string  `json:"texto"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	dir, ok := s.campana(w, r, ses, p.Proyecto, p.Campana)
	if !ok {
		return
	}
	c := qa.Comentario{Caso: p.Caso, T: p.T, Texto: strings.TrimSpace(p.Texto), Autor: ses.Nombre, Fecha: time.Now()}
	if err := qa.Comenta(dir, c); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, c)
}
