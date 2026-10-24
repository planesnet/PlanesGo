// Package servidor es la API HTTP del arnés Planes Go Code y sirve la interfaz web.
//
// Reglas:
//   - Todo pide sesión, que se abre con usuario y contraseña de Odoo (planesnet.autopyme.com).
//   - Toda acción va ligada a un proyecto de Odoo que el usuario pueda ver con su propia sesión de Odoo.
//   - Los secretos (sesión de Odoo, token de ver de Studio TV, claves de API) no salen del servidor.
package servidor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"planesgo-code/internal/actualizacion"
	"planesgo-code/internal/agentdesktop"
	"planesgo-code/internal/bugreport"
	"planesgo-code/internal/clients"
	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/gbrowser"
	"planesgo-code/internal/github"
	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/llm"
	"planesgo-code/internal/memory"
	"planesgo-code/internal/odoo"
	"planesgo-code/internal/proceso"
	"planesgo-code/internal/procesos"
	"planesgo-code/internal/proyectos"
	"planesgo-code/internal/qa"
	"planesgo-code/internal/repo"
	"planesgo-code/internal/selector"
	"planesgo-code/internal/sesiones"
)

// cookie: nombre antiguo de la cookie de sesión, el mismo en todos los puertos (dos pgocode en el mismo host se
// pisaban la sesión). Ahora cada servidor usa la suya (cookieSesion); la antigua se acepta para no echar a nadie.
const cookie = "pgo_sesion"

// cookieSesion: el nombre de la cookie de sesión de este servidor, con su puerto (las cookies no distinguen puertos).
func (s *Servidor) cookieSesion() string {
	if s.Puerto == "" {
		return cookie
	}
	return cookie + "_" + s.Puerto
}

// poneCookieSesion guarda el token de sesión en la cookie de este servidor (vacío: la borra).
func (s *Servidor) poneCookieSesion(w http.ResponseWriter, r *http.Request, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: s.cookieSesion(), Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: maxAge})
}

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
	// Clients: las apps cliente emparejadas con este servidor (clients.go); su token va en cookieEscritorio.
	Clients *clients.Registry
	// Actualizador: autoactualización desde PlanesGo (nil: sin actualizaciones).
	Actualizador *actualizacion.Actualizador
	// QA: pruebas lanzadas desde la app (campañas .psf/qa del proyecto).
	QA qa.Ejecutor
	// DirRamas: dónde se crean las copias de trabajo de otras ramas (git worktree) de las sesiones.
	DirRamas string
	// RegistroCopias: las copias de trabajo que crea pgocode (para poder quitarlas luego sin perder nada).
	RegistroCopias *repo.Registro
	// Procesos: programas del proyecto en marcha (Odoo, servicios…); Eventos: avisos a la interfaz (SSE).
	Procesos *procesos.Gestor
	Eventos  Eventos
	// Selector: elige la skill de cada petición con embeddings (vectores guardados en disco).
	Selector *selector.Selector
	// Imputacion: el tiempo de las sesiones en PlanesGo (latidos y cierre del parte), sin pasar por el agente.
	Imputacion *imputacion.Gestor
	// GitHub: issues y pull requests del repositorio (con la sesión de «gh» del usuario).
	GitHub github.Lector
	// Errores: los fallos del propio pgocode, como issues de PlanesGo (errores.go); nil: no se informa.
	Errores *bugreport.Reporter
	// Google: Drive, Docs, Sheets, Slides y Gmail en solo lectura a través del navegador integrado (nil: no disponible).
	Google *gbrowser.Reader
	// TokenAgente: no vacío, los agentes reciben la orden «pgo» con un token propio de su sesión y turno (PGO_TOKEN,
	// agenttoken.go); DirBin: carpeta con «pgo».
	TokenAgente string
	agentes     agentTokens
	// esProcesoAgente sustituye en las pruebas la comprobación del proceso que se conecta (peer.go).
	esProcesoAgente func(*http.Request) bool
	DirBin          string
	// EstudioOcupado dice si el Studio TV local está en uso (directo o grabación).
	EstudioOcupado func() bool
	activos        atomic.Int64 // chats y agentes en curso
	// trabajando: las carpetas con un agente en curso (reparto.go: no se une en una copia en la que se trabaja).
	trabajando   ocupadas
	bloqueosRepo sync.Map // candado por repositorio (reparto.go)
	// Partes: decide si una petición se reparte (por defecto, llm.ParallelParts); sustituible en las pruebas.
	Partes func(context.Context, string) *llm.Request
	// actividad: última vez que el usuario usó la app o tenía algo a medio escribir (UnixNano).
	actividad atomic.Int64
	// PermitirNavegador abre Planes Go Code también al navegador (parámetro -navegador): sin él, y sin TokenEscritorio,
	// no se atiende ninguna petición.
	PermitirNavegador bool
	// escritorios: los escritorios remotos abiertos (rdp.go).
	escritorios rdpSessions
	// sqls: las conexiones con SQL Server de las sesiones de trabajo (sqlconn.go); SQLAudit: su registro de auditoría.
	sqls     sqlConns
	SQLAudit string
	// winrms: the WinRM (remote PowerShell) connections of the work sessions (winrmconn.go); WinRMAudit: their audit log.
	winrms     winrmConns
	WinRMAudit string
	// agentDesktops: the agents' desktops of the work sessions (agentdesktop.go); AgentDesktopAudit: the log of their
	// orders; agentDesktopOptions replaces the programs and folders in the tests.
	agentDesktops       sessionConns[*agentdesktop.Desktop]
	AgentDesktopAudit   string
	agentDesktopOptions func() (agentdesktop.Options, error)
	// anydesk: the AnyDesk sessions seen in its traces (anydesk.go); AnyDeskFile: its settings (default project and
	// the project of each remote computer).
	anydesk     anydeskSessions
	AnyDeskFile string
	// memPending: las sesiones del agente que reciben otra vez la memoria del proyecto (memory.go).
	memPending memoryPending

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
	mux.HandleFunc("GET /api/proyecto/ramas", s.conSesion(s.proyectoRamas))
	mux.HandleFunc("GET /api/proyecto/tareas", s.conSesion(s.proyectoTareas))
	mux.HandleFunc("GET /api/imputacion", s.conSesion(s.imputacionEstado))
	mux.HandleFunc("POST /api/imputacion/unificar", s.conSesion(s.imputacionUnificar))
	mux.HandleFunc("GET /api/imputacion/resumen", s.conSesion(s.imputacionResumen))
	mux.HandleFunc("GET /api/imputacion/validar", s.conSesion(s.imputacionValidar))
	mux.HandleFunc("POST /api/proyecto/carpeta", s.conSesion(s.proyectoCarpeta))
	mux.HandleFunc("GET /api/proyecto/info-carpeta", s.conSesion(s.proyectoInfoCarpeta))
	mux.HandleFunc("POST /api/proyecto/preparar", s.conSesion(s.proyectoPreparar))
	mux.HandleFunc("GET /api/proyecto/copias", s.conSesion(s.proyectoCopias))
	mux.HandleFunc("POST /api/proyecto/quitar-copia", s.conSesion(s.proyectoQuitarCopia))
	mux.HandleFunc("POST /api/proyecto/subcarpeta", s.conSesion(s.proyectoSubcarpeta))
	mux.HandleFunc("POST /api/reparto/propone", s.conSesion(s.repartoPropone))
	mux.HandleFunc("POST /api/reparto/prepara", s.conSesion(s.repartoPrepara))
	mux.HandleFunc("POST /api/reparto/une", s.conSesion(s.repartoUne))
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
	mux.HandleFunc("GET /api/ia-local", s.conSesion(s.iaLocal))
	mux.HandleFunc("POST /api/ia-local", s.conSesion(s.iaLocalGuarda))
	mux.HandleFunc("GET /api/actualizacion", s.conSesion(s.actualizacion))
	mux.HandleFunc("POST /api/actualizacion/buscar", s.conSesion(s.buscaActualizacion))
	mux.HandleFunc("POST /api/actualizacion/aplicar", s.conSesion(s.aplicaActualizacion))
	mux.HandleFunc("POST /api/actualizacion/pausa", s.conSesion(s.pausaActualizacion))
	mux.HandleFunc("POST /api/actividad", s.conSesion(s.marcaActividad))
	mux.HandleFunc("POST /api/diagnostico", s.conSesion(s.diagnostico))
	mux.HandleFunc("GET /api/errores", s.conSesion(s.errorReports))
	mux.HandleFunc("POST /api/errores", s.conSesion(s.errorReportsSave))
	mux.HandleFunc("GET /api/proyectos", s.conSesion(s.proyectos))
	mux.HandleFunc("GET /api/proveedores", s.conSesion(s.proveedores))
	mux.HandleFunc("GET /api/agentes/mcp", s.conSesion(s.servidoresMCP))
	mux.HandleFunc("POST /api/chat", s.conSesion(s.chat))
	mux.HandleFunc("POST /api/chat/clasifica", s.conSesion(s.clasifica))
	mux.HandleFunc("POST /api/chat/inyecta", s.conSesion(s.inyecta))
	mux.HandleFunc("POST /api/directo", s.conSesion(s.directo))
	mux.HandleFunc("POST /api/ruta", s.conSesion(s.rutaAnota))
	mux.HandleFunc("GET /api/chat/vigencia", s.conSesion(s.vigencia))
	mux.HandleFunc("POST /api/chat/vigencia", s.conSesion(s.vigenciaChat))
	mux.HandleFunc("POST /api/adjunto", s.conSesion(s.adjunto))
	mux.HandleFunc("GET /api/memoria", s.conSesion(s.memoriaLee))
	mux.HandleFunc("POST /api/memoria", s.conSesion(s.memoriaGuarda))
	mux.HandleFunc("GET /api/evidencia", s.conSesion(s.evidencia))
	mux.HandleFunc("GET /api/evidencias", s.conSesion(s.evidencias))
	mux.HandleFunc("GET /api/planes", s.conSesion(s.planesLista))
	mux.HandleFunc("GET /api/planes/leer", s.conSesion(s.planesLee))
	mux.HandleFunc("POST /api/planes", s.conSesion(s.planesGuarda))
	mux.HandleFunc("POST /api/planes/borrar", s.conSesion(s.planesBorra))
	mux.HandleFunc("POST /api/abrir", s.conSesion(s.abrir))
	mux.HandleFunc("GET /api/eventos", s.conSesion(s.eventos))
	mux.HandleFunc("GET /api/skills", s.conSesion(s.skillsInventario))
	mux.HandleFunc("GET /api/skills/texto", s.conSesion(s.skillsTexto))
	mux.HandleFunc("POST /api/skills/elegir", s.conSesion(s.skillsElegir))
	mux.HandleFunc("GET /api/github", s.conSesion(s.githubEstado))
	mux.HandleFunc("POST /api/github/issue", s.conSesion(s.githubCrea))
	mux.HandleFunc("POST /api/github/accion", s.conSesion(s.githubAccion))
	mux.HandleFunc("GET /api/github/issue", s.conSesion(s.githubIssue))
	mux.HandleFunc("POST /api/psf/start", s.conSesion(s.psfStart))
	mux.HandleFunc("POST /api/psf/spec", s.conSesion(s.psfSpec))
	mux.HandleFunc("POST /api/psf/sync", s.conSesion(s.psfSync))
	mux.HandleFunc("POST /api/psf/ready", s.conSesion(s.psfReady))
	mux.HandleFunc("POST /api/psf/comment", s.conSesion(s.psfComment))
	mux.HandleFunc("POST /api/psf/checks", s.conSesion(s.psfChecks))
	mux.HandleFunc("POST /api/psf/merge", s.conSesion(s.psfMerge))
	mux.HandleFunc("POST /api/psf/event", s.conSesion(s.psfEvent))
	mux.HandleFunc("POST /api/psf/fetch", s.conSesion(s.psfFetch))
	mux.HandleFunc("POST /api/psf/document", s.conSesion(s.psfDocument))
	mux.HandleFunc("POST /api/psf/discard", s.conSesion(s.psfDiscard))
	mux.HandleFunc("GET /api/escritorio/google/trabajo", s.googleTrabajo)
	mux.HandleFunc("POST /api/escritorio/google/resultado", s.googleResultado)
	mux.HandleFunc("POST "+pairPath, s.clientsPair)
	mux.HandleFunc("GET /api/clientes/yo", s.clientsMe)
	mux.HandleFunc("GET /api/clientes", s.conSesion(s.clientsList))
	mux.HandleFunc("POST /api/clientes/revocar", s.conSesion(s.clientsRevoke))
	mux.HandleFunc("GET /api/rdp", s.conSesion(s.rdpState))
	mux.HandleFunc("GET /api/sql", s.conSesion(s.sqlState))
	mux.HandleFunc("POST /api/sql/connect", s.conSesion(s.sqlConnect))
	mux.HandleFunc("POST /api/sql/disconnect", s.conSesion(s.sqlDisconnect))
	mux.HandleFunc("POST /api/sql/run", s.conSesion(s.sqlRunUI))
	mux.HandleFunc("POST /api/sql/mode", s.conSesion(s.sqlMode))
	mux.HandleFunc("POST /api/sql/database", s.conSesion(s.sqlDatabase))
	mux.HandleFunc("GET /api/winrm", s.conSesion(s.winrmState))
	mux.HandleFunc("POST /api/winrm/connect", s.conSesion(s.winrmConnect))
	mux.HandleFunc("POST /api/winrm/disconnect", s.conSesion(s.winrmDisconnect))
	mux.HandleFunc("POST /api/winrm/mode", s.conSesion(s.winrmMode))
	mux.HandleFunc("POST /api/winrm/run", s.conSesion(s.winrmRunUI))
	mux.HandleFunc("GET /api/agent-desktop", s.conSesion(s.agentDesktopState))
	mux.HandleFunc("POST /api/agent-desktop/start", s.conSesion(s.agentDesktopStart))
	mux.HandleFunc("POST /api/agent-desktop/stop", s.conSesion(s.agentDesktopStop))
	mux.HandleFunc("POST /api/rdp/conectar", s.conSesion(s.rdpConnect))
	mux.HandleFunc("POST /api/rdp/desconectar", s.conSesion(s.rdpDisconnect))
	mux.HandleFunc("GET /api/anydesk", s.conSesion(s.anydeskState))
	mux.HandleFunc("POST /api/anydesk/connect", s.conSesion(s.anydeskConnect))
	mux.HandleFunc("POST /api/anydesk/assign", s.conSesion(s.anydeskAssign))
	mux.HandleFunc("POST /api/anydesk/default", s.conSesion(s.anydeskDefault))
	mux.HandleFunc("GET /api/procesos", s.conSesion(s.procesosLista))
	mux.HandleFunc("POST /api/procesos/lanzar", s.conSesion(s.procesosLanza))
	mux.HandleFunc("POST /api/procesos/parar", s.conSesion(s.procesosPara))
	mux.HandleFunc("POST /api/procesos/guardar", s.conSesion(s.procesosGuarda))
	mux.HandleFunc("POST /api/procesos/borrar-config", s.conSesion(s.procesosBorraConfig))
	mux.HandleFunc("GET /api/procesos/externos", s.conSesion(s.procesosExternos))
	mux.HandleFunc("POST /api/procesos/externos/parar", s.conSesion(s.procesosExternosPara))
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
	return s.cabeceras(s.recovers(mux))
}

// desdeEscritorio dice si la petición viene de una app cliente emparejada (o si se permite el navegador).
func (s *Servidor) desdeEscritorio(r *http.Request) bool {
	if s.PermitirNavegador {
		return true
	}
	_, ok := s.clientOf(r)
	return ok
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
		// Solo los scripts del propio arnés (un texto del agente o de un programa no puede colar uno) y nunca dentro
		// de otra página.
		w.Header().Set("Content-Security-Policy", "script-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		if !s.hostLocal(r.Host) {
			jsonError(w, http.StatusMisdirectedRequest, "host no permitido")
			return
		}
		// Las órdenes del agente llevan su token (que /api/pgo comprueba contra el proyecto de la orden). Con el token
		// de agente o desde un proceso de agente, nada fuera de /api/pgo: ni la interfaz ni las rutas que hablan con
		// Odoo (docs/odoo-blindaje.md).
		agente := esRutaAgente(r.URL.Path) && r.Header.Get("X-PGO-Agente") != ""
		if !esRutaAgente(r.URL.Path) && r.URL.Path != "/healthz" && (r.Header.Get("X-PGO-Agente") != "" || s.fromAgentProcess(r)) {
			jsonError(w, http.StatusForbidden, "los agentes solo pueden usar la orden «pgo»")
			return
		}
		pairing := r.URL.Path == pairPath && r.Method == http.MethodPost // a client that is not paired yet
		if r.URL.Path != "/healthz" && !agente && !pairing && !s.desdeEscritorio(r) {
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
		c, err := r.Cookie(s.cookieSesion())
		antigua := false
		if err != nil && s.cookieSesion() != cookie {
			// Compatibilidad: la cookie de antes (sin puerto). Si es de este servidor, pasa a la nueva y se borra.
			c, err = r.Cookie(cookie)
			antigua = err == nil
		}
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "inicia sesión")
			return
		}
		ses, ok := s.Sesiones.Busca(c.Value)
		if !ok {
			jsonError(w, http.StatusUnauthorized, "la sesión ha caducado: vuelve a entrar")
			return
		}
		if antigua {
			s.poneCookieSesion(w, r, ses.Token, int(sesiones.Duracion.Seconds()))
			http.SetCookie(w, &http.Cookie{Name: cookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
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
	if err == nil && s.Imputacion != nil {
		s.Imputacion.ActualizaSesionOdoo(u.UID, u.Sesion) // partes y cierres pendientes con la sesión nueva
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.poneCookieSesion(w, r, ses.Token, int(sesiones.Duracion.Seconds()))
	log.Printf("sesión abierta: %s (uid %d)", u.Login, u.UID)
	jsonOK(w, map[string]any{"nombre": u.Nombre, "login": u.Login})
}

func (s *Servidor) logout(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.Odoo.Cierra(ctx, ses.OdooSesion)
	s.Sesiones.Borra(ses.Token)
	s.poneCookieSesion(w, r, "", -1)
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

// Ocupado dice si hay algo en curso que una actualización interrumpiría, o si el usuario está usando la app (la
// automática espera a que lleve un rato sin usarla).
func (s *Servidor) Ocupado() bool { return s.TrabajoEnCurso() != "" || s.usandose() }

// TrabajoEnCurso: qué hay en marcha que una actualización cortaría («» si nada). Con algo en marcha no se actualiza
// bajo ningún concepto, ni aunque lo pida el usuario.
func (s *Servidor) TrabajoEnCurso() string {
	switch {
	case s.activos.Load() > 0:
		return "un agente está trabajando"
	case proceso.GruposVivos() > 0:
		return "un agente tiene tareas en segundo plano"
	case s.QA.EnCurso() > 0:
		return "hay una prueba en marcha"
	case s.Procesos != nil && s.Procesos.EnCurso() > 0:
		return "hay un programa en marcha"
	case s.EstudioOcupado != nil && s.EstudioOcupado():
		return "el Studio TV está emitiendo o grabando"
	case s.escritorios.open() > 0:
		return "hay un escritorio remoto abierto"
	case s.anydesk.open() > 0:
		return "hay una sesión de AnyDesk abierta"
	case s.agentDesktopsRunning() > 0:
		return "hay un escritorio del agente abierto"
	}
	return ""
}

func (s *Servidor) actualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if s.Actualizador == nil {
		jsonOK(w, actualizacion.Estado{Version: s.Version, Estado: "desactivada"})
		return
	}
	e := s.Actualizador.Estado()
	res := map[string]any{"version": e.Version, "estado": e.Estado, "nueva": e.Nueva, "error": e.Error,
		"ocupado": s.Ocupado(), "trabajando": s.TrabajoEnCurso(), "pausada": e.Pausada}
	if !e.Revisado.IsZero() { // sin revisar nunca, no va (la interfaz dice «nunca»; antes salía «31/12/01»)
		res["revisado"] = e.Revisado
	}
	jsonOK(w, res)
}

// sinUsoAntes: tiempo sin que el usuario toque la app (ni tenga nada a medio escribir) antes de actualizar sola.
const sinUsoAntes = 15 * time.Minute

// usandose dice si el usuario está trabajando con la app (aunque ningún agente esté respondiendo ahora).
func (s *Servidor) usandose() bool {
	t := s.actividad.Load()
	return t != 0 && time.Since(time.Unix(0, t)) < sinUsoAntes
}

// marcaActividad: la interfaz avisa de que el usuario está usando la app o tiene un borrador. Si además dice en
// qué sesión trabaja (con la ventana en primer plano), es trabajo humano de esa sesión y se imputa.
func (s *Servidor) marcaActividad(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	s.actividad.Store(time.Now().UnixNano())
	var p struct {
		Sesion   string `json:"sesion"`
		Titulo   string `json:"titulo"`
		Proyecto int    `json:"proyecto"`
		TareaID  int    `json:"tarea_id"`
		Tarea    string `json:"tarea"`
		Humana   bool   `json:"humana"` // hubo teclado, ratón o desplazamiento en la sesión
	}
	if lee(r, &p) == nil && p.Humana && p.Sesion != "" && p.Proyecto > 0 && s.Imputacion != nil {
		if nombre, ok := s.proyecto(w, r, ses, p.Proyecto); ok {
			s.Imputacion.Actividad(imputacion.Trabajo{Sesion: p.Sesion, Titulo: p.Titulo, Proyecto: p.Proyecto, NombreProy: nombre,
				TareaID: p.TareaID, Tarea: p.Tarea, UID: ses.UID, OdooSesion: ses.OdooSesion,
				Desc: imputacion.Descripcion(p.Titulo, "trabajo en la sesión")})
		} else {
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// pausaActualizacion: el usuario pausa o reanuda las actualizaciones automáticas.
func (s *Servidor) pausaActualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Pausada bool `json:"pausada"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if s.Actualizador == nil {
		jsonError(w, http.StatusConflict, "sin autoactualización")
		return
	}
	s.Actualizador.Pausa(p.Pausada)
	log.Printf("%s: actualizaciones automáticas %s", ses.Login, map[bool]string{true: "en pausa", false: "reanudadas"}[p.Pausada])
	jsonOK(w, map[string]bool{"pausada": p.Pausada})
}

// aplicaActualizacion: el usuario pulsa el aviso; se instala ya la versión preparada, salvo que haya algo en
// marcha (entonces se instalará sola cuando acabe y lleve un rato sin usarse).
func (s *Servidor) aplicaActualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if t := s.TrabajoEnCurso(); t != "" {
		jsonError(w, http.StatusConflict, "no se puede actualizar ahora: "+t)
		return
	}
	if s.Actualizador == nil || !s.Actualizador.AplicarYa() {
		jsonError(w, http.StatusConflict, "no hay ninguna actualización preparada")
		return
	}
	log.Printf("%s pide aplicar la actualización ya", ses.Login)
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Servidor) buscaActualizacion(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	// Sin actualizador o desactivado (desarrollo, sin servidor…): se dice por qué, para que el botón responda algo.
	if s.Actualizador == nil {
		jsonOK(w, map[string]any{"ok": false, "error": "sin servidor de actualizaciones"})
		return
	}
	if e := s.Actualizador.Estado(); e.Estado == "desactivada" {
		jsonOK(w, map[string]any{"ok": false, "error": e.Error})
		return
	}
	s.Actualizador.BuscarYa()
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

// carpetaAjena: por qué no vale la carpeta de una sesión. Si ya no existe es casi siempre una copia de trabajo que
// se quitó (Actividad → Copias de trabajo): decirlo así, no «no es del proyecto».
func carpetaAjena(carpeta, proyecto string) string {
	if _, err := os.Stat(carpeta); errors.Is(err, os.ErrNotExist) {
		return fmt.Sprintf("la carpeta %s ya no existe (la subcarpeta se quitó): vuelve a crearla eligiendo la rama en las propiedades de la sesión, o elige otra carpeta", carpeta)
	}
	return fmt.Sprintf("la carpeta %s no es del proyecto «%s»: elígela en las propiedades de la sesión (la carpeta de la sesión es la que decide el repositorio)", carpeta, proyecto)
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

// maxTraspaso: el tamaño máximo del traspaso de «Continuar desde aquí» (unos 3k tokens).
const maxTraspaso = 16 << 10

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
		Autorizadas  []string      `json:"autorizadas"` // carpetas de fuera del proyecto autorizadas en la sesión
		MCP          []string      `json:"mcp"`         // servidores MCP de Claude que pide la sesión (nunca el de imputación «pgo»)
		Permisos     string        `json:"permisos"`
		Esfuerzo     string        `json:"esfuerzo"`
		Sesion       string        `json:"sesion"` // id de la sesión de pgocode (para imputar su tiempo)
		Titulo       string        `json:"titulo"`
		TareaID      int           `json:"tarea_id"` // tarea de Odoo a la que se imputa (0: por nombre)
		Tarea        string        `json:"tarea"`
		Adjuntos     []string      `json:"adjuntos"` // rutas de ficheros adjuntos (en la carpeta de la sesión)
		Mensajes     []llm.Mensaje `json:"mensajes"`
		// Pregunta aparte (no entra en la conversación): «suelta» (sin contexto, con un modelo barato) o «contexto»
		// (sobre una copia de la sesión del agente; si el agente no las permite, con el resumen de Contexto).
		Aparte   string `json:"aparte"`
		Contexto string `json:"contexto"`
		// AparteMotor «pago»: la pregunta suelta con el modelo de pago aunque haya IA local (aparte.go).
		AparteMotor string `json:"aparte_motor"`
		Idioma      string `json:"idioma"` // del navegador (es-ES, en-GB…): el agente responde en él
		// «Continuar desde aquí»: el mensaje del agente desde el que sigue (lo posterior lo olvida) y el traspaso
		// de pgocode por si el agente no puede cortar su sesión (ver llm.Peticion).
		Corte    string `json:"corte"`
		Traspaso string `json:"traspaso"`
		// Compactación del historial del agente: a partir de cuántos tokens (0: el valor por defecto) y el contexto
		// de la última llamada del turno anterior (ver llm.Peticion).
		Compactar      int `json:"compactar"`
		ContextoPrevio int `json:"contexto_previo"`
		// Podar: poda automática del historial del agente al empezar el turno (ver llm.Peticion.Prune). Sin el
		// campo, activada.
		Podar *bool `json:"podar"`
		// TextoUsuario: lo que ha escrito el usuario, sin lo que añade pgocode (memoria de trabajo, notas): de ahí
		// salen las frases de la memoria del proyecto. Sin él, el último mensaje.
		TextoUsuario string `json:"texto_usuario"`
		// NoFacts: pgocode wrote the message (e.g. the PSF prompts): no project-memory facts are taken from it
		// (the commands the agent runs still are).
		NoFacts bool `json:"no_facts"`
		// Summary: what the conversation shows for a message pgocode wrote (e.g. «PSF · Builder: …»); it describes
		// the time entry instead of the whole prompt.
		Summary string `json:"summary"`
		// CacheTTL: lo que dura la caché del prompt del agente («5m» o «1h», ver llm.Peticion.CacheTTL).
		CacheTTL string `json:"cache_ttl"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if p.Aparte != "" && p.Aparte != "suelta" && p.Aparte != "contexto" {
		jsonError(w, http.StatusBadRequest, "pregunta aparte no válida")
		return
	}
	if len(p.Mensajes) == 0 {
		jsonError(w, http.StatusBadRequest, "no hay mensaje")
		return
	}
	// El traspaso, acotado (unos 3k tokens; la interfaz ya lo recorta a eso).
	if len(p.Traspaso) > maxTraspaso {
		p.Traspaso = strings.ToValidUTF8(p.Traspaso[:maxTraspaso], "")
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
		jsonError(w, http.StatusForbidden, carpetaAjena(p.Carpeta, nombre))
		return
	}
	if info.NecesitaDir && p.Carpeta == "" {
		if cs := s.Indice.Carpetas(false)[p.Proyecto]; len(cs) > 0 {
			p.Carpeta = cs[0]
		}
	}
	// Nunca con la carpeta personal (ni la raíz) como carpeta de trabajo: el agente recorrería todo el equipo y
	// leería lo que encontrase (pasó: un turno de OpenCode leyó el registro de pgocode y gastó 2,48 $).
	if casa, _ := os.UserHomeDir(); p.Carpeta != "" && (filepath.Clean(p.Carpeta) == "/" || filepath.Clean(p.Carpeta) == filepath.Clean(casa) || dentroDe(filepath.Clean(p.Carpeta), casa)) {
		jsonError(w, http.StatusBadRequest, "la carpeta de la sesión es tu carpeta personal (o una superior): elige la del proyecto en las propiedades de la sesión")
		return
	}
	adjuntos := adjuntosValidos(p.Carpeta, p.Adjuntos)
	autorizadas := carpetasAutorizadas(p.Autorizadas)
	// Evidence goes to the session folder, not to the branch worktree (which may be removed after a merge).
	evidenceFolder := p.Carpeta
	// La memoria del proyecto, la del repositorio de la carpeta (antes de pasar a la copia de la rama: la comparten).
	memRoot := ""
	if info.Tipo == "agente" && p.Carpeta != "" && p.Aparte == "" {
		memRoot = memory.Root(p.Carpeta)
	}
	// La sesión trabaja en su rama: si no es la de la carpeta, en una copia de trabajo propia (git worktree).
	if info.NecesitaDir && p.Carpeta != "" && p.Rama != "" {
		c, err := repo.CopiaDeRama(r.Context(), p.Carpeta, p.Rama, s.DirRamas)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		c.Sesion, c.Proyecto = p.Sesion, p.Proyecto
		s.apuntaCopia(c)
		p.Carpeta = c.Carpeta
	}

	// Tiempo: pgocode lo imputa directamente en Odoo (proyecto y tarea de la sesión), sin PlanesGo ni el agente.
	sesionImp := p.Sesion
	if s.Imputacion != nil && info.NecesitaDir && p.Proyecto > 0 {
		if sesionImp == "" {
			sesionImp = fmt.Sprintf("%d-%s", p.Proyecto, filepath.Base(p.Carpeta))
		}
		pedido := ""
		if len(p.Mensajes) > 0 && p.Aparte == "" { // una pregunta aparte no cambia la descripción de la línea
			pedido = p.Mensajes[len(p.Mensajes)-1].Texto
		}
		if p.Summary != "" {
			pedido = p.Summary
		}
		fin := s.Imputacion.Empieza(imputacion.Trabajo{Sesion: sesionImp, Titulo: p.Titulo, Proyecto: p.Proyecto, NombreProy: nombre,
			TareaID: p.TareaID, Tarea: p.Tarea, UID: ses.UID, OdooSesion: ses.OdooSesion,
			Modelo: strings.TrimSpace(info.Nombre + " " + strings.TrimPrefix(p.Modelo, "por defecto")), Desc: imputacion.Descripcion(p.Titulo, pedido)})
		defer fin()
	}

	// El agente trabaja en esta carpeta mientras dure el turno (reparto.go).
	if p.Aparte == "" && p.Carpeta != "" && info.NecesitaDir {
		defer s.trabajando.entra(p.Carpeta)()
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
	idioma := llm.NombreIdioma(p.Idioma)
	sistema := fmt.Sprintf("Trabajas para %s en el proyecto de Odoo «%s» (id %d) de Planes Software Factory. %s",
		ses.Nombre, nombre, p.Proyecto, llm.InstruccionIdioma(idioma))
	if p.Carpeta != "" {
		sistema += " Carpeta local del proyecto: " + p.Carpeta + "."
	}
	if p.Permisos != "" && p.Permisos != "lectura" && p.Permisos != "editar" && p.Permisos != "auto" {
		p.Permisos = "lectura"
	}
	// Id del turno: enlaza en el registro de tiempos la petición, sus herramientas, sus tareas y su fin.
	turno := idTurno()
	// El token de la orden «pgo» vale para esta sesión y este turno: se revoca al acabar (agenttoken.go).
	tokenAgente, revocaToken := s.issueAgentToken(p.Proyecto, sesionImp)
	defer revocaToken()
	if p.Aparte != "" {
		s.aparte(turno, r, p.Aparte, p.Contexto, p.AparteMotor, prov, info, llm.Peticion{Modelo: p.Modelo, Sistema: sistema, Mensajes: p.Mensajes[len(p.Mensajes)-1:],
			Carpeta: p.Carpeta, SesionAgente: p.SesionAgente, Permisos: "lectura", Esfuerzo: p.Esfuerzo, Entorno: s.entornoAgente(p.Proyecto, tokenAgente),
			Autorizadas: autorizadas, MCP: p.MCP, Aparte: true, Idioma: idioma, CompactAt: p.Compactar, CacheTTL: p.CacheTTL, Traza: "sesión " + sesionImp + " · aparte " + turno}, sesionImp, p.Proveedor, envia)
		return
	}
	// Registro de tiempos: la petición, lo que hace el agente (herramientas, subagentes, segundo plano) y su fin.
	inicioTurno := time.Now()
	vivosTurno := 0 // procesos de este agente que siguen vivos al acabar (en segundo plano)
	if s.Imputacion != nil {
		pide := map[string]any{"turno": turno, "proveedor": p.Proveedor, "modelo": p.Modelo, "permisos": p.Permisos}
		if p.Summary != "" {
			pide["resumen"] = p.Summary // e.g. «PSF · Reviewer-Security: …»: which role of an automatic session it is
		}
		s.Imputacion.AnotaAgente(sesionImp, "agente-pide", pide)
	}
	pet := llm.Peticion{Modelo: p.Modelo, Sistema: sistema, Mensajes: p.Mensajes,
		Carpeta: p.Carpeta, SesionAgente: p.SesionAgente, Permisos: p.Permisos, Esfuerzo: p.Esfuerzo, Adjuntos: adjuntos, Entorno: s.entornoAgente(p.Proyecto, tokenAgente), Autorizadas: autorizadas, MCP: p.MCP, Idioma: idioma,
		Paralelo:  os.Getenv("PGOCODE_PARALELO") != "0" && !p.NoFacts, // a prompt written by pgocode (PSF) gets no subagent hint
		CompactAt: p.Compactar, LastContext: p.ContextoPrevio, Prune: modoPoda(p.Podar), CacheTTL: p.CacheTTL,
		AlInyectar: s.alInyectar(ses, p.Sesion, sesionImp, turno), Traza: "sesión " + sesionImp + " · turno " + turno,
		Corte: p.Corte, Traspaso: p.Traspaso}
	// Memoria del proyecto: solo cuando el agente empieza de cero (memoryReason), al final de su mensaje.
	userText := p.TextoUsuario
	if userText == "" {
		userText = withoutRelayMemory(p.Mensajes[len(p.Mensajes)-1].Texto)
	}
	if p.NoFacts {
		userText = ""
	}
	if memRoot != "" {
		last := len(pet.Mensajes) - 1
		if why := s.memoryReason(p.Proveedor, pet.Mensajes[last].Texto, pet); why != "" {
			if block := memory.ForAgent(memRoot); block != "" {
				pet.Memory = block
				envia(map[string]any{"memoria": map[string]any{"tokens": memory.Tokens(block), "motivo": why}})
				if s.Imputacion != nil {
					s.Imputacion.AnotaAgente(sesionImp, "ruta-modificador", map[string]any{"turno": turno, "modificador": "memory", "motivo": why, "tokens": memory.Tokens(block)})
				}
			}
		}
	}
	turnMem := &memoryTurn{}
	agentSession, compacted := p.SesionAgente, false
	err := prov.Chat(r.Context(), pet, func(e llm.Evento) {
		if e.SesionAgente != "" {
			agentSession = e.SesionAgente
		}
		if e.Compacted {
			compacted = true
		}
		if e.Herramienta != nil && memRoot != "" {
			turnMem.tool(e.Herramienta)
		}
		if e.Uso != nil && s.Imputacion != nil {
			s.Imputacion.SumaUso(sesionImp, e.Uso.Entrada, e.Uso.Salida, e.Uso.Coste)
			// Supervision: the context of the call and its cache (the tokens alone do not say how full it is).
			if u := e.Uso; u.Contexto > 0 || u.CacheLeida > 0 || u.Llamadas > 0 {
				s.Imputacion.AnotaAgente(sesionImp, "contexto", map[string]any{"turno": turno, "modelo": p.Modelo, "contexto": u.Contexto,
					"cache_leida": u.CacheLeida, "cache_escrita": u.CacheEscrita, "llamadas": u.Llamadas, "segundos": u.Segundos, "resumen": p.Summary})
			}
		}
		// Capturas de la herramienta: a disco, como evidencias; por el chat, solo sus rutas.
		if e.Vivos != nil {
			vivosTurno = *e.Vivos
			return
		}
		// Modificador del enrutador de intenciones que aplica el servidor: la sugerencia de subagentes.
		if e.Subagentes != "" && s.Imputacion != nil {
			s.Imputacion.AnotaAgente(sesionImp, "ruta-modificador", map[string]any{"turno": turno, "modificador": "subagents", "motivo": e.Subagentes})
		}
		if h := e.Herramienta; h != nil && len(h.Imagenes) > 0 {
			guardaCapturas(evidenceFolder, Captura{Sesion: sesionImp, Titulo: p.Titulo, Agente: p.Proveedor, Rama: p.Rama}, h)
		}
		if h := e.Herramienta; h != nil && s.Imputacion != nil {
			c := map[string]any{"turno": turno, "id": h.ID, "nombre": h.Nombre, "estado": h.Estado}
			if h.Entrada != "" {
				c["tipo"] = imputacion.TipoHerramienta(h.Nombre, h.Entrada)
				c["resumen"] = recortaTexto(h.Resumen, 120)
			}
			s.Imputacion.AnotaAgente(sesionImp, "herramienta", c)
		}
		// Programas que el agente dejó en marcha: pgocode se los queda (siguen en Programas y en la barra).
		for _, ad := range e.Adoptados {
			if s.Procesos == nil {
				break
			}
			if _, err := s.Procesos.Adopta(ad.Nombre, p.Proyecto, p.Carpeta, ad.Orden, ad.Grupo, ad.Fichero, ad.Marca); err != nil {
				log.Printf("adoptar %s: %v", ad.Nombre, err)
			} else if s.Imputacion != nil {
				s.Imputacion.AnotaAgente(sesionImp, "adoptado", map[string]any{"turno": turno, "nombre": ad.Nombre, "orden": recortaTexto(ad.Orden, 160)})
			}
		}
		if t := e.Tarea; t != nil && s.Imputacion != nil {
			s.Imputacion.AnotaAgente(sesionImp, "tarea", map[string]any{"turno": turno, "id": t.ID, "tipo": t.Tipo, "estado": t.Estado,
				"titulo": recortaTexto(t.Titulo, 120), "subtipo": t.Subtipo, "segundo_plano": t.SegundoPlano})
		}
		envia(e)
	})
	if s.Imputacion != nil {
		fin := map[string]any{"turno": turno, "turno_s": time.Since(inicioTurno).Seconds(), "cancelada": r.Context().Err() != nil,
			"procesos_vivos": vivosTurno}
		if err != nil {
			fin["error"] = recortaTexto(depuracion.Tapa(err.Error()), 600)
		}
		s.Imputacion.AnotaAgente(sesionImp, "agente-fin", fin)
	}
	if compacted {
		s.memPending.mark(agentSession)
	}
	if memRoot != "" {
		go saveMemoryTurn(memRoot, userText, turnMem)
	}
	if err != nil && r.Context().Err() == nil {
		envia(map[string]string{"error": err.Error()})
	}
	envia(map[string]bool{"fin": true})
}

// vigencia: la vigencia de cada intercambio y pieza de la sesión del agente, ahora (en segundo plano, sin barrer;
// ver llm.VigenciaReport). Espera a que acabe el marcado del último turno. Sin datos, listas vacías.
func (s *Servidor) vigencia(w http.ResponseWriter, r *http.Request, _ *sesiones.Sesion) {
	ctx, cancela := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancela()
	// «estado=1»: sin esperar al recolector (el indicador del chat mientras trabaja).
	rep := llm.VigenciaReport(ctx, r.URL.Query().Get("sesion_agente"), r.URL.Query().Get("estado") != "1")
	if rep == nil {
		rep = &llm.PruneReport{Exchanges: []llm.PruneExchange{}, Pieces: []llm.PrunePiece{}}
	}
	jsonOK(w, rep)
}

// vigenciaChat: la vigencia de la conversación de pgocode para los agentes que no dejan podar su historial
// (Antigravity; ver llm.ChatVigencia). La interfaz manda los intercambios desde la última conversación nueva del
// agente y, si va a enviar, el mensaje nuevo; con «pendiente» decide el relevo.
func (s *Servidor) vigenciaChat(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	if r.URL.Query().Get("barrer") == "1" {
		s.barrer(w, r, ses)
		return
	}
	var p struct {
		Clave        string             `json:"clave"`
		Carpeta      string             `json:"carpeta"`
		Intercambios []llm.ChatExchange `json:"intercambios"`
		Mensaje      string             `json:"mensaje"`
		Podar        *bool              `json:"podar"`
	}
	if err := lee(r, &p); err != nil {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	ctx, cancela := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancela()
	rep := llm.ChatVigencia(ctx, p.Clave, p.Carpeta, p.Intercambios, p.Mensaje, modoPoda(p.Podar) == llm.PruneAuto)
	if rep == nil {
		rep = &llm.PruneReport{Exchanges: []llm.PruneExchange{}, Pieces: []llm.PrunePiece{}}
	}
	jsonOK(w, rep)
}

// barrer: «Barrer ahora» desde la ventanita del hilo de limpieza (POST /api/chat/vigencia?barrer=1): retira ya lo
// que está fuera de vigencia de la sesión del agente, sin el mínimo ni la cuenta de la caché (ver llm.ForceSweep).
// Antigravity no deja tocar su historial: la interfaz fuerza el relevo en el próximo envío.
func (s *Servidor) barrer(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto     int    `json:"proyecto"`
		Agente       string `json:"agente"`
		SesionAgente string `json:"sesion_agente"`
		Carpeta      string `json:"carpeta"`
		Mensaje      string `json:"mensaje"`
		EnCurso      bool   `json:"en_curso"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	if p.Agente != "opencode" && p.Agente != "claude-code" {
		jsonError(w, http.StatusBadRequest, "este agente no deja podar su historial: se releva en el próximo envío")
		return
	}
	if _, ok := s.proyecto(w, r, ses, p.Proyecto); !ok {
		return
	}
	if p.Carpeta == "" || !s.Indice.Pertenece(p.Proyecto, p.Carpeta) {
		jsonError(w, http.StatusForbidden, "la carpeta no es del proyecto")
		return
	}
	ctx, cancela := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancela()
	var rep *llm.PruneReport
	var err error
	if r.URL.Query().Get("vaciar") == "1" {
		if p.EnCurso {
			jsonError(w, http.StatusConflict, "no se puede vaciar el contexto con el agente trabajando")
			return
		}
		rep, err = vaciado(ctx, p.Agente, p.SesionAgente, p.Carpeta, p.Mensaje)
		if err == nil {
			s.memPending.mark(p.SesionAgente) // el agente vuelve a empezar: con la memoria del proyecto
		}
	} else {
		rep, err = barrido(ctx, p.Agente, p.SesionAgente, p.Carpeta, p.Mensaje, p.EnCurso)
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, rep)
}

// barrido: el barrido forzado (en las pruebas, se sustituye).
var barrido = llm.ForceSweep

// vaciado: «Vaciar contexto» (en las pruebas, se sustituye).
var vaciado = llm.ForceEmpty

// modoPoda: la poda de la sesión (activada salvo que la sesión la desactive).
func modoPoda(podar *bool) string {
	if podar != nil && !*podar {
		return llm.PruneOff
	}
	return llm.PruneAuto
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
	if err := openWithSystem(u.String()); err != nil {
		jsonError(w, http.StatusInternalServerError, "no se pudo abrir el navegador: "+err.Error())
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}

// openWithSystem opens an already validated web address or local folder with the system's default program.
func openWithSystem(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
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

// carpetasAutorizadas: las carpetas de fuera del proyecto que la sesión autoriza, solo si son rutas completas de
// carpetas que existen (y nunca la raíz ni la carpeta personal entera, que equivaldría a no restringir nada).
func carpetasAutorizadas(l []string) []string {
	casa, _ := os.UserHomeDir()
	casa = filepath.Clean(casa)
	// Nunca: la raíz, el sistema, la carpeta personal entera ni sus padres, ni las de credenciales y ajustes.
	prohibida := func(c string) bool {
		if c == "/" || c == casa || dentroDe(c, casa) {
			return true
		}
		for _, x := range []string{"/etc", "/root", "/proc", "/sys", "/dev", "/boot", "/var", "/usr", "/bin", "/sbin", "/lib",
			filepath.Join(casa, ".ssh"), filepath.Join(casa, ".gnupg"), filepath.Join(casa, ".config"), filepath.Join(casa, ".aws"),
			filepath.Join(casa, ".local", "share", "pgocode"), filepath.Join(casa, ".claude")} {
			if dentroDe(x, c) {
				return true
			}
		}
		return false
	}
	var out []string
	for _, c := range l {
		c = filepath.Clean(strings.TrimSpace(c))
		if r, err := filepath.EvalSymlinks(c); err == nil {
			c = r
		}
		if !filepath.IsAbs(c) || prohibida(c) || slices.Contains(out, c) {
			continue
		}
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			out = append(out, c)
		}
	}
	return out
}

// servidoresMCP: los servidores MCP de Claude Code configurados para la carpeta (para elegirlos en la sesión).
func (s *Servidor) servidoresMCP(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	if _, ok := s.proyecto(w, r, ses, id); !ok {
		return
	}
	l := []string{}
	for n := range llm.ServidoresMCP(r.URL.Query().Get("carpeta")) {
		if n != "pgo" {
			l = append(l, n)
		}
	}
	sort.Strings(l)
	jsonOK(w, l)
}

// idTurno: un identificador corto y aleatorio para un turno.
func idTurno() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// diagnostico: un error de la interfaz (excepción o promesa sin atender) va al registro de depuración y, si es un
// fallo de pgocode, a su issue de GitHub (errores.go).
func (s *Servidor) diagnostico(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Mensaje string `json:"mensaje"`
		Pila    string `json:"pila"`
		URL     string `json:"url"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	depuracion.Log("interfaz", "%s %s: %s\n%s", s.Version, recortaTexto(p.URL, 200), recortaTexto(p.Mensaje, 500), recortaTexto(p.Pila, 2000))
	where, _, _ := strings.Cut(p.URL, "?") // the screen, not its parameters
	s.Errores.Report(bugreport.Report{Source: bugreport.FromInterface, Message: recortaTexto(p.Mensaje, 500), Stack: recortaTexto(p.Pila, 4000), Where: where})
	w.WriteHeader(http.StatusNoContent)
}
