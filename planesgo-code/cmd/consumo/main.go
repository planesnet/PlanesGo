// consumo: banco de consumo de tokens. Lanza las pruebas de consumo/pruebas.json con cada agente de dos formas:
//
//   - directo (la calibración): el CLI del agente a pelo, como lo usaría alguien sin pgocode;
//   - pgocode: con la capa de pgocode que lanza los agentes (internal/llm), la misma que usa la app: sus
//     argumentos, los MCP que filtra, el idioma, su entorno y la entrada por stdin de Claude Code.
//
// Sin ninguna dependencia del ERP: ni Odoo (tampoco el simulado), ni login, ni proyectos.
//
// Cada prueba trabaja en una copia propia de consumo/proyecto (o del que diga la prueba; un repositorio git nuevo) y se comprueba sola
// (texto de la respuesta, una orden que ha de acabar bien, ficheros que no se pueden tocar…): gastar poco y
// hacerlo mal no cuenta. Gasta tokens de verdad: no entra en «make test».
//
//	go run ./cmd/consumo                                  # todo: 3 agentes × 6 pruebas × 3 repeticiones × 2 modos
//	go run ./cmd/consumo -pruebas 1,2,3 -repeticiones 1   # ronda rápida
//	go run ./cmd/consumo -modo pgocode                    # solo con pgocode, comparado con la última calibración
//
// Resultados, en el repositorio para seguir la evolución: consumo/resultados/<fecha>.jsonl (los datos de cada
// ejecución, según acaba), <fecha>.md (el informe) y evolucion.md (todas las rondas). Una ronda completa sin
// pgocode guarda además la calibración en consumo/calibracion.json.
package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"planesgo-code/internal/llm"
)

var (
	modoF       = flag.String("modo", "ambos", "ambos | directo (calibración, sin pgocode) | pgocode")
	agentesF    = flag.String("agentes", "claude-code,antigravity,opencode", "agentes, separados por comas")
	pruebasF    = flag.String("pruebas", "1,2,3,4,5,6,7", "pruebas (sus id), separadas por comas")
	repsF       = flag.Int("repeticiones", 3, "repeticiones de cada prueba (se toma la mediana)")
	salidaF     = flag.String("salida", "", "carpeta de resultados (vacío: consumo/resultados, en el repositorio)")
	notaF       = flag.String("nota", "", "qué cambia en esta ronda (sale en el informe y en la evolución)")
	conserva    = flag.Bool("conservar", false, "no borra las copias de trabajo al acabar")
	limiteF     = flag.Duration("limite", 20*time.Minute, "tiempo máximo de cada turno")
	estrategiaF = flag.String("estrategia", "ninguna", "cómo se lleva la conversación de un turno al siguiente: "+strategyNames())
)

// ---- definición de las pruebas ----

type Minimo struct {
	Fichero string `json:"fichero"`
	Patron  string `json:"patron"`
	Veces   int    `json:"veces"`
}

type Turno struct {
	Mensaje    string   `json:"mensaje"`
	Texto      string   `json:"texto,omitempty"`       // la respuesta ha de cumplir esta expresión regular
	Orden      string   `json:"orden,omitempty"`       // ha de acabar bien en la carpeta de trabajo
	SinCambios []string `json:"sin_cambios,omitempty"` // rutas que no se pueden tocar
	Minimo     *Minimo  `json:"minimo,omitempty"`      // un patrón que ha de aparecer al menos N veces en un fichero
}

type Prueba struct {
	ID       int     `json:"id"`
	Nombre   string  `json:"nombre"`
	Permisos string  `json:"permisos"`
	Mide     string  `json:"mide"`
	Turnos   []Turno `json:"turnos"`
	// Proyecto: la carpeta de consumo/ que se copia para la prueba (por defecto, «proyecto»).
	Proyecto string `json:"proyecto,omitempty"`
}

type ConfAgente struct {
	Modelo   string `json:"modelo"`
	Esfuerzo string `json:"esfuerzo,omitempty"`
}

type Banco struct {
	Agentes map[string]ConfAgente `json:"agentes"`
	Pruebas []Prueba              `json:"pruebas"`
}

// ---- medidas ----

type Medida struct {
	Entrada      int     `json:"entrada"`
	Salida       int     `json:"salida"`
	CacheLeida   int     `json:"cache_leida"`
	CacheEscrita int     `json:"cache_escrita"`
	Llamadas     int     `json:"llamadas"`
	Herramientas int     `json:"herramientas"`
	Coste        float64 `json:"coste"`
	Segundos     float64 `json:"segundos"`
	Primera      float64 `json:"primera"` // segundos hasta lo primero que hace el agente (texto o herramienta)
	// Contexto: los tokens de la última llamada al modelo (todo lo que se reenvía); en una ejecución, el del
	// último turno.
	Contexto int `json:"contexto,omitempty"`
}

func (m *Medida) suma(o Medida) {
	m.Entrada += o.Entrada
	m.Salida += o.Salida
	m.CacheLeida += o.CacheLeida
	m.CacheEscrita += o.CacheEscrita
	m.Llamadas += o.Llamadas
	m.Herramientas += o.Herramientas
	m.Coste += o.Coste
	m.Segundos += o.Segundos
	if m.Primera == 0 {
		m.Primera = o.Primera
	}
	if o.Contexto > 0 {
		m.Contexto = o.Contexto
	}
}

type Ejecucion struct {
	Modo    string `json:"modo"`
	Agente  string `json:"agente"`
	Prueba  int    `json:"prueba"`
	Rep     int    `json:"rep"`
	Medida         // todos los turnos
	Acierto bool   `json:"acierto"`
	Fallo   string `json:"fallo,omitempty"` // por qué no acierta (comprobación o error del agente)
	Carpeta string `json:"carpeta"`
	Fecha   string `json:"fecha"`          // la de la ronda
	Nota    string `json:"nota,omitempty"` // qué cambia en la ronda
	// Estrategia: cómo se lleva la conversación de un turno al siguiente (vacía en las rondas antiguas: «ninguna»).
	Estrategia string `json:"estrategia,omitempty"`
	// Turnos: la medida de cada turno (con lo que gaste la estrategia antes de él), para ver crecer el contexto.
	Turnos []turnMeasure `json:"turnos,omitempty"`
}

// turnMeasure: lo que gasta un turno y si cumple su comprobación.
type turnMeasure struct {
	Medida
	Acierto bool `json:"acierto"`
	// Nota: lo que ha hecho la estrategia en el turno (relevo, compactación…).
	Nota string `json:"nota,omitempty"`
}

// ---- estrategias ----

// strategy: cómo pasa la conversación de un turno al siguiente. «ninguna» es lo que hacen los agentes (y
// pgocode) hoy: reanudar la sesión del agente, que reenvía todo el historial. Las otras (relevo, compactación,
// recorte…) se añaden en strategies.
type strategy interface {
	// before: antes de cada turno. Puede cambiar c.Session y c.Message (p. ej. empezar una sesión nueva con un
	// resumen de c.History), pedir la compactación al agente (c.CompactAt, c.LastContext) y lanzar turnos
	// propios con c.Run, cuyo consumo se suma al del turno. c.Note dice qué ha hecho (sale en el informe).
	before(ctx context.Context, c *conversation) error
}

// conversation: el estado de una ejecución que ve la estrategia.
type conversation struct {
	Agent, Mode string
	Turn        int    // el turno que va a empezar, desde 0
	Session     string // la sesión del agente que se reanudará («»: una nueva)
	Message     string // lo que se enviará
	History     []exchange
	// CompactAt y LastContext: los de llm.Peticion en el modo pgocode (ver internal/llm/compaction.go).
	CompactAt, LastContext int
	Note                   string // lo que ha hecho la estrategia en este turno
	// Run: un turno con el agente, fuera de las pruebas (su consumo cuenta en el turno que empieza).
	Run   func(ctx context.Context, msg, session string) resultadoTurno
	extra Medida
}

// exchange: un turno ya hecho (Message: el de la prueba, sin lo que le haya añadido la estrategia).
type exchange struct {
	Message, Reply string
	Usage          Medida
	Parts          []uiPart    // lo que ha hecho el agente, como lo guarda la interfaz
	Tasks          []llm.Tarea // sus tareas (la lista de tareas, subagentes…)
}

// noStrategy («ninguna»): reanudar la sesión y reenviarlo todo, sin compactar. pgocode compacta por defecto a partir
// de llm.CompactAtDefault (80k): aquí el umbral va al máximo (llm.CompactAtMax, 1M), que en la práctica no se
// alcanza; si no, «ninguna» sería «compactar:80» en cuanto la conversación pasara de 80k.
type noStrategy struct{}

func (noStrategy) before(_ context.Context, c *conversation) error {
	c.CompactAt = llm.CompactAtMax
	return nil
}

// compactStrategy («compactar[:N]»): la compactación gobernada por pgocode, como en la app: cada turno lleva el
// umbral (CompactAt) y el contexto de la última llamada del turno anterior (LastContext). Por defecto, 20k (el
// mínimo de pgocode), para que salte en una prueba corta.
type compactStrategy struct{ threshold int }

func (s compactStrategy) before(_ context.Context, c *conversation) error {
	c.CompactAt = s.threshold
	if n := len(c.History); n > 0 {
		c.LastContext = c.History[n-1].Usage.Contexto
	}
	return nil
}

// relayStrategy («relevo[:N]»): el relevo con memoria de trabajo de la interfaz. Si el contexto de la última
// llamada del turno anterior pasa de N (por defecto 20k), el turno va a una sesión nueva del agente con la memoria
// de trabajo delante del mensaje. La memoria y la decisión son las de web/src/workingMemory.ts (relay.mts, con
// node), sobre los mensajes del banco en el formato de la interfaz.
type relayStrategy struct {
	threshold int
	script    string // relay.mts
}

// relayResult: lo que devuelve relay.mts.
type relayResult struct {
	Reason      string `json:"reason"`
	Message     string `json:"message"`
	MemoryChars int    `json:"memory_chars"`
}

func (s relayStrategy) before(ctx context.Context, c *conversation) error {
	c.CompactAt = llm.CompactAtMax // solo el relevo: sin la compactación de pgocode (ver noStrategy)
	n := len(c.History)
	if n == 0 || c.Session == "" {
		return nil
	}
	in, err := json.Marshal(map[string]any{"agent": c.Agent, "threshold": s.threshold, "context": c.History[n-1].Usage.Contexto,
		"messages": uiMessages(c.History), "message": c.Message})
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "node", "--experimental-strip-types", "--no-warnings", s.script)
	cmd.Stdin = bytes.NewReader(in)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("relay.mts: %v: %s", err, recorta(errOut.String(), 400))
	}
	var r relayResult
	if err := json.Unmarshal(out, &r); err != nil {
		return fmt.Errorf("relay.mts: %v", err)
	}
	if r.Reason != "" {
		c.Session, c.Message = "", r.Message
		c.Note = fmt.Sprintf("relevo (memoria %s)", miles((r.MemoryChars+3)/4))
	}
	return nil
}

// uiPart y uiMessage: Parte y Mensaje de web/src/api.ts (lo que usa la memoria de trabajo).
type uiPart struct {
	Tipo  string           `json:"tipo"` // texto | pensamiento | herramienta
	Texto string           `json:"texto,omitempty"`
	H     *llm.Herramienta `json:"h,omitempty"`
}

type uiMessage struct {
	Rol    string      `json:"rol"`
	Texto  string      `json:"texto"`
	Partes []uiPart    `json:"partes,omitempty"`
	Tareas []llm.Tarea `json:"tareas,omitempty"`
}

// uiMessages: la conversación como la tiene la interfaz: cada petición y la respuesta del agente con sus partes.
func uiMessages(h []exchange) []uiMessage {
	var l []uiMessage
	for _, x := range h {
		l = append(l, uiMessage{Rol: "user", Texto: x.Message},
			uiMessage{Rol: "assistant", Texto: x.Reply, Partes: x.Parts, Tareas: x.Tasks})
	}
	return l
}

// addPart: como «parte» en Chat.tsx: el texto seguido se junta; la herramienta se actualiza por su id.
func addPart(ps []uiPart, p uiPart) []uiPart {
	if p.Tipo != "herramienta" {
		if n := len(ps); n > 0 && ps[n-1].Tipo == p.Tipo {
			ps[n-1].Texto += p.Texto
			return ps
		}
		return append(ps, p)
	}
	for i := range ps {
		if v := ps[i].H; v != nil && v.ID == p.H.ID {
			h := *p.H
			h.Nombre, h.Resumen, h.Entrada = cmp.Or(h.Nombre, v.Nombre), cmp.Or(v.Resumen, h.Resumen), cmp.Or(v.Entrada, h.Entrada)
			ps[i].H = &h
			return ps
		}
	}
	h := *p.H
	return append(ps, uiPart{Tipo: "herramienta", H: &h})
}

// mergeTask: como T.fusiona en tareas.ts: una actualización de una tarea (por id); lo vacío conserva lo anterior.
func mergeTask(l []llm.Tarea, t llm.Tarea) []llm.Tarea {
	i := slices.IndexFunc(l, func(x llm.Tarea) bool { return x.ID == t.ID })
	if i < 0 {
		return append(l, t)
	}
	var a, b map[string]any
	ba, _ := json.Marshal(l[i])
	bb, _ := json.Marshal(t)
	json.Unmarshal(ba, &a)
	json.Unmarshal(bb, &b)
	for k, v := range b {
		a[k] = v
	}
	bm, _ := json.Marshal(a)
	var n llm.Tarea
	json.Unmarshal(bm, &n)
	l[i] = n
	return l
}

// compactionNote: el aviso de pgocode de que el agente ha compactado su historial (ver compactionNotice).
var compactionNote = regexp.MustCompile(`ha compactado su historial(?:: (?:de \S+ a \S+|tenía \S+) tokens)?`)

// moduleDir: la carpeta del módulo planesgo-code (la pone main).
var moduleDir = "."

var strategies = map[string]func(n int) strategy{
	"ninguna":   func(int) strategy { return noStrategy{} },
	"compactar": func(n int) strategy { return compactStrategy{threshold: cmp.Or(n, 20) * 1000} },
	"relevo": func(n int) strategy {
		return relayStrategy{threshold: cmp.Or(n, 20) * 1000, script: filepath.Join(moduleDir, "cmd", "consumo", "relay.mts")}
	},
}

// pgocodeOnly: las estrategias que usan lo que hace pgocode (no tienen sentido sin él).
var pgocodeOnly = map[string]bool{"compactar": true, "relevo": true}

// parseStrategy: «nombre[:N]» (N en miles de tokens).
func parseStrategy(s string) (name string, n int, err error) {
	name, arg, hay := strings.Cut(strings.TrimSpace(s), ":")
	if _, ok := strategies[name]; !ok {
		return "", 0, fmt.Errorf("estrategia desconocida: %s (hay: %s)", name, strategyNames())
	}
	if hay {
		if n, err = strconv.Atoi(arg); err != nil || n <= 0 || name == "ninguna" {
			return "", 0, fmt.Errorf("estrategia %q: N ha de ser un número positivo (miles de tokens)", s)
		}
	}
	return name, n, nil
}

func strategyNames() string {
	var l []string
	for k := range strategies {
		if k != "ninguna" {
			k += "[:N]"
		}
		l = append(l, k)
	}
	sort.Strings(l)
	return strings.Join(l, " | ")
}

// resultadoTurno: lo que deja un turno (en cualquiera de los dos modos).
type resultadoTurno struct {
	Medida
	texto  string
	sesion string
	err    error
	bruto  llm.Uso // lo que da el agente, sin restar el acumulado anterior
	parts  []uiPart
	tasks  []llm.Tarea
}

// recoge: junta los eventos de un turno. Si el agente da el consumo acumulado de su conversación (ver
// cumulativeUsage), se resta el del turno anterior (previo); si baja (otra conversación), se toma entero.
type recoge struct {
	r       resultadoTurno
	inicio  time.Time
	previo  *llm.Uso
	agente  string
	acumula llm.Uso
}

func (c *recoge) evento(e llm.Evento) {
	if c.r.Primera == 0 && (e.Texto != "" || e.Herramienta != nil) {
		c.r.Primera = time.Since(c.inicio).Seconds()
	}
	c.r.texto += e.Texto
	if e.Texto != "" {
		c.r.parts = addPart(c.r.parts, uiPart{Tipo: "texto", Texto: e.Texto})
	}
	if e.Pensamiento != "" {
		c.r.parts = addPart(c.r.parts, uiPart{Tipo: "pensamiento", Texto: e.Pensamiento})
	}
	if h := e.Herramienta; h != nil {
		if h.Estado == "curso" && h.Entrada != "" {
			c.r.Herramientas++
		}
		c.r.parts = addPart(c.r.parts, uiPart{Tipo: "herramienta", H: h})
	}
	if e.Tarea != nil {
		c.r.tasks = mergeTask(c.r.tasks, *e.Tarea)
	}
	if e.SesionAgente != "" {
		c.r.sesion = e.SesionAgente
	}
	if u := e.Uso; u != nil {
		c.acumula.Entrada += u.Entrada
		c.acumula.Salida += u.Salida
		c.acumula.CacheLeida += u.CacheLeida
		c.acumula.CacheEscrita += u.CacheEscrita
		c.acumula.Llamadas += u.Llamadas
		c.acumula.Coste += u.Coste
		if u.Contexto > 0 {
			c.acumula.Contexto = u.Contexto
		}
	}
}

func (c *recoge) fin() resultadoTurno {
	u := c.acumula
	c.r.bruto = u
	if p := c.previo; p != nil && u.Entrada >= p.Entrada {
		u.Entrada -= p.Entrada
		u.Salida -= p.Salida
		u.CacheLeida -= p.CacheLeida
		u.CacheEscrita -= p.CacheEscrita
		u.Coste -= p.Coste
	}
	c.r.Entrada, c.r.Salida, c.r.CacheLeida, c.r.CacheEscrita = max(u.Entrada, 0), max(u.Salida, 0), max(u.CacheLeida, 0), max(u.CacheEscrita, 0)
	u.Coste = max(u.Coste, 0)
	c.r.Llamadas, c.r.Coste, c.r.Contexto = u.Llamadas, u.Coste, u.Contexto
	c.r.Segundos = time.Since(c.inicio).Seconds()
	return c.r
}

// ---- entorno ----

// limpiaEntorno: el del usuario sin lo que pone una sesión de Claude Code, pgocode o PlanesGo (si se lanza desde
// una, cambiaría lo que cargan los agentes), en el propio proceso: lo heredan los agentes en los dos modos.
// PLANESGO_GESTOR=pgocode: los hooks de PlanesGo se duermen (no imputan nada), en los dos modos por igual.
func limpiaEntorno() {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "CLAUDE") || strings.HasPrefix(k, "ANTHROPIC") || strings.HasPrefix(k, "PGO_") ||
			strings.HasPrefix(k, "PLANESGO") || strings.HasPrefix(k, "ODOO_") {
			os.Unsetenv(k)
		}
	}
	os.Setenv("PLANESGO_GESTOR", "pgocode")
}

func ejecutaGit(dir string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=consumo", "-c", "user.email=consumo@prueba.invalid",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=master"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %v\n%s", args, err, out)
	}
	return nil
}

// copia: el proyecto de prueba en una carpeta nueva, como repositorio git.
func copia(origen, destino string) error {
	if err := os.MkdirAll(destino, 0o755); err != nil {
		return err
	}
	if out, err := exec.Command("cp", "-r", origen+"/.", destino).CombinedOutput(); err != nil {
		return fmt.Errorf("cp: %v %s", err, out)
	}
	for _, a := range [][]string{{"init", "-q"}, {"add", "."}, {"commit", "-q", "-m", "inicio"}} {
		if err := ejecutaGit(destino, a...); err != nil {
			return err
		}
	}
	return nil
}

// ---- modo directo (calibración) ----

func directo(ctx context.Context, agente string, conf ConfAgente, carpeta, permisos, msg, sesion string, previo *llm.Uso, datosOC string) resultadoTurno {
	var args, extra []string
	var prog string
	switch agente {
	case "claude-code":
		prog = "claude"
		args = []string{"-p", "--output-format", "stream-json", "--verbose", "--model", conf.Modelo}
		if conf.Esfuerzo != "" {
			args = append(args, "--effort", conf.Esfuerzo)
		}
		if permisos == "auto" {
			args = append(args, "--permission-mode", "auto")
		}
		if sesion != "" {
			args = append(args, "--resume", sesion)
		}
		args = append(args, "--", msg)
	case "antigravity":
		prog = "agy"
		// «--mode auto» no está en todas las cuentas (sin él no edita): accept-edits, que edita y ejecuta órdenes.
		modo := "plan"
		if permisos == "auto" {
			modo = "accept-edits"
		}
		args = []string{"--output-format", "stream-json", "--mode", modo, "--model", conf.Modelo}
		if conf.Esfuerzo != "" {
			args = append(args, "--effort", conf.Esfuerzo)
		}
		if sesion != "" {
			args = append(args, "--conversation", sesion)
		}
		args = append(args, "--print="+msg)
	case "opencode":
		prog = "opencode"
		args = []string{"run", "--format", "json", "--model", conf.Modelo, "--dir", carpeta}
		if sesion != "" {
			args = append(args, "--session", sesion)
		}
		args = append(args, "--", msg)
		// Lo mínimo para que funcione sin preguntar (en «run» nadie contestaría) y con Vertex; sin la
		// configuración de ahorro que pone pgocode. Datos propios: los de OpenCode Desktop no los abre la terminal.
		perm := `{"edit":"deny","bash":"deny","webfetch":"allow"}`
		if permisos == "auto" {
			perm = `{"edit":"allow","bash":"allow","webfetch":"allow"}`
		}
		extra = []string{"OPENCODE_PERMISSION=" + perm, "XDG_DATA_HOME=" + datosOC,
			"GOOGLE_CLOUD_LOCATION=global", "VERTEX_LOCATION=global", "GOOGLE_CLOUD_PROJECT=planes-dev"}
	}
	c := &recoge{inicio: time.Now(), previo: previo, agente: agente}
	cmd := exec.CommandContext(ctx, prog, args...)
	cmd.Dir = carpeta
	cmd.Env = append(append(os.Environ(), "PWD="+carpeta), extra...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	out, err := cmd.StdoutPipe()
	if err != nil {
		c.r.err = err
		return c.fin()
	}
	if err := cmd.Start(); err != nil {
		c.r.err = err
		return c.fin()
	}
	errLee := llm.Lector(agente, sesion == "")(out, c.evento)
	io.Copy(io.Discard, out)
	errCmd := cmd.Wait()
	r := c.fin()
	var fin *llm.FinConFondo
	switch {
	case errLee != nil && !errors.As(errLee, &fin):
		r.err = errLee
	case errCmd != nil && strings.TrimSpace(r.texto) == "":
		r.err = fmt.Errorf("%v: %s", errCmd, recorta(errOut.String(), 400))
	}
	return r
}

// ---- modo pgocode ----

// conPgocode: un turno con el proveedor de pgocode (internal/llm), con la petición que arma su servidor para la
// interfaz: idioma del navegador y mensajes a mitad de turno (Claude Code: la entrada por stdin).
func conPgocode(ctx context.Context, prov llm.Proveedor, agente string, conf ConfAgente, carpeta, permisos, msg, sesion string, previo *llm.Uso, compactAt, lastContext int) resultadoTurno {
	c := &recoge{inicio: time.Now(), previo: previo, agente: agente}
	var mu sync.Mutex
	err := prov.Chat(ctx, llm.Peticion{Modelo: conf.Modelo, Esfuerzo: conf.Esfuerzo, Carpeta: carpeta, Permisos: permisos,
		SesionAgente: sesion, Mensajes: []llm.Mensaje{{Rol: "user", Texto: msg}}, Idioma: llm.NombreIdioma("es-ES"),
		CompactAt: compactAt, LastContext: lastContext,
		AlInyectar: func(func(string) error) {}, Traza: "banco de consumo"}, func(e llm.Evento) {
		mu.Lock()
		c.evento(e)
		mu.Unlock()
	})
	r := c.fin()
	if err != nil {
		r.err = err
	}
	return r
}

// cumulativeUsage: si el consumo que da el agente al reanudar una sesión es el acumulado de toda la conversación
// y no el del turno. Antigravity sin pgocode (pgocode ya lo resta). Claude Code no: su «result» (modelUsage y
// total_cost_usd) suma la sesión entera tras --resume, pero el lector de internal/llm, que se usa en los dos
// modos, ya resta el acumulado anterior de la sesión (acumuladoClaude); restarlo aquí otra vez lo descontaba dos
// veces (las rondas del 22-10-2026 de 22:10 a 22:38).
func cumulativeUsage(agente, modo string) bool {
	return agente == "antigravity" && modo == "directo"
}

// limite: un error del agente que dice que no puede seguir (límite de gasto, de uso o sin crédito): no vale la
// pena lanzar más ejecuciones con él (saldrían vacías y estropearían las medianas).
var limite = regexp.MustCompile(`(?i)spend limit|usage limit|credit balance|out of credits|quota exceeded|rate.limit`)

// ---- comprobación ----

func comprueba(t Turno, texto, carpeta string) string {
	if t.Texto != "" && !regexp.MustCompile("(?is)"+t.Texto).MatchString(strings.TrimSpace(texto)) {
		return fmt.Sprintf("la respuesta no cumple %q: %q", t.Texto, recorta(texto, 160))
	}
	if t.Orden != "" {
		cmd := exec.Command("sh", "-c", t.Orden)
		cmd.Dir = carpeta
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Sprintf("«%s» falla: %s", t.Orden, recorta(string(out), 200))
		}
	}
	for _, r := range t.SinCambios {
		// Sin contar la caché de Python (__pycache__): la crea ejecutar las pruebas sin PYTHONDONTWRITEBYTECODE (CI).
		if out, _ := exec.Command("git", "-C", carpeta, "status", "--porcelain", "--", r, ":(exclude,glob)**/__pycache__/**").Output(); len(bytes.TrimSpace(out)) > 0 {
			return fmt.Sprintf("ha cambiado %s (no se podía tocar)", r)
		}
	}
	if m := t.Minimo; m != nil {
		b, _ := os.ReadFile(filepath.Join(carpeta, m.Fichero))
		if n := len(regexp.MustCompile(m.Patron).FindAll(b, -1)); n < m.Veces {
			return fmt.Sprintf("%s: %d veces «%s», al menos %d", m.Fichero, n, m.Patron, m.Veces)
		}
	}
	return ""
}

// ---- ejecución ----

func ejecuta(ctx context.Context, modo, agente string, conf ConfAgente, pr Prueba, rep int, carpeta, datosOC string, prov llm.Proveedor, est strategy) Ejecucion {
	e := Ejecucion{Modo: modo, Agente: agente, Prueba: pr.ID, Rep: rep, Carpeta: carpeta, Acierto: true}
	// El acumulado de la conversación que da el agente, para restarlo en el turno siguiente de la misma sesión
	// (una estrategia que empieza otra sesión empieza de cero).
	var previo *llm.Uso
	previoSesion := ""
	acumula := cumulativeUsage(agente, modo)
	c := &conversation{Agent: agente, Mode: modo}
	run := func(cx context.Context, msg, sesion string) resultadoTurno {
		if !acumula || sesion == "" || sesion != previoSesion {
			previo = nil
		}
		var r resultadoTurno
		if modo == "directo" {
			r = directo(cx, agente, conf, carpeta, pr.Permisos, msg, sesion, previo, datosOC)
		} else {
			r = conPgocode(cx, prov, agente, conf, carpeta, pr.Permisos, msg, sesion, previo, c.CompactAt, c.LastContext)
		}
		if acumula {
			b := r.bruto
			previo, previoSesion = &b, cmp.Or(r.sesion, sesion)
		}
		return r
	}
	c.Run = func(cx context.Context, msg, sesion string) resultadoTurno {
		r := run(cx, msg, sesion)
		c.extra.suma(r.Medida)
		return r
	}
	for i, t := range pr.Turnos {
		cx, cancela := context.WithTimeout(ctx, *limiteF)
		c.Turn, c.Message, c.extra, c.Note = i, t.Mensaje, Medida{}, ""
		var r resultadoTurno
		if err := est.before(cx, c); err != nil {
			r.err = fmt.Errorf("estrategia: %w", err)
		} else {
			r = run(cx, c.Message, c.Session)
		}
		cancela()
		m := c.extra
		m.suma(r.Medida)
		m.Contexto = r.Contexto // el de la llamada del turno, no el de la estrategia
		e.Medida.suma(m)
		if r.sesion != "" {
			c.Session = r.sesion
		}
		c.History = append(c.History, exchange{Message: t.Mensaje, Reply: r.texto, Usage: m, Parts: r.parts, Tasks: r.tasks})
		nota := c.Note
		if x := compactionNote.FindString(r.texto); x != "" {
			nota = strings.TrimPrefix(strings.TrimSpace(nota+"; "+strings.Replace(x, "ha compactado su historial", "compacta", 1)), "; ")
		}
		f := ""
		if r.err != nil {
			f = fmt.Sprintf("turno %d: %v", i+1, r.err)
		} else if x := comprueba(t, r.texto, carpeta); x != "" {
			f = fmt.Sprintf("turno %d: %s", i+1, x)
		}
		e.Turnos = append(e.Turnos, turnMeasure{Medida: m, Acierto: f == "", Nota: nota})
		if f != "" {
			e.Acierto, e.Fallo = false, f
			break
		}
	}
	return e
}

func main() {
	log.SetFlags(log.Ltime)
	log.SetPrefix("consumo ")
	flag.Parse()
	limpiaEntorno()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil || strings.TrimSpace(string(out)) == "" || strings.TrimSpace(string(out)) == os.DevNull {
		log.Fatal("ejecútalo dentro del módulo planesgo-code (go run ./cmd/consumo)")
	}
	modulo := filepath.Dir(strings.TrimSpace(string(out)))
	dirBanco := filepath.Join(modulo, "consumo")
	var banco Banco
	if b, err := os.ReadFile(filepath.Join(dirBanco, "pruebas.json")); err != nil || json.Unmarshal(b, &banco) != nil {
		log.Fatalf("no puedo leer %s: %v", filepath.Join(dirBanco, "pruebas.json"), err)
	}
	modos := []string{"directo", "pgocode"}
	if *modoF != "ambos" {
		modos = []string{*modoF}
	}
	var agentes []string
	for _, a := range strings.Split(*agentesF, ",") {
		a = strings.TrimSpace(a)
		if _, ok := banco.Agentes[a]; !ok {
			log.Fatalf("agente desconocido: %s", a)
		}
		agentes = append(agentes, a)
	}
	var pruebas []Prueba
	for _, s := range strings.Split(*pruebasF, ",") {
		id, _ := strconv.Atoi(strings.TrimSpace(s))
		i := slices.IndexFunc(banco.Pruebas, func(p Prueba) bool { return p.ID == id })
		if i < 0 {
			log.Fatalf("prueba desconocida: %s", s)
		}
		pruebas = append(pruebas, banco.Pruebas[i])
	}
	moduleDir = modulo
	nombreEst, nEst, err := parseStrategy(*estrategiaF)
	if err != nil {
		log.Fatal(err)
	}
	if pgocodeOnly[nombreEst] && slices.Contains(modos, "directo") {
		log.Fatalf("la estrategia %s solo tiene sentido con pgocode: usa -modo pgocode", nombreEst)
	}
	if nombreEst == "relevo" {
		if _, err := exec.LookPath("node"); err != nil {
			log.Fatal("la estrategia relevo necesita node (para web/src/workingMemory.ts)")
		}
	}
	nuevaEstrategia := func() strategy { return strategies[nombreEst](nEst) }
	casa, _ := os.UserHomeDir()
	salida := *salidaF
	if salida == "" {
		salida = filepath.Join(dirBanco, "resultados")
	}
	os.MkdirAll(salida, 0o755)
	marca := time.Now().Format("2006-01-02-150405")
	trabajo := filepath.Join(casa, ".cache", "pgocode-consumo", marca)
	raiz := filepath.Join(trabajo, "raiz")
	datosOC := filepath.Join(trabajo, "opencode-datos")
	os.MkdirAll(datosOC, 0o700)
	carpeta := func(modo, agente string, pr, rep int) string {
		return filepath.Join(raiz, fmt.Sprintf("%s-%s-p%d-r%d", modo, agente, pr, rep))
	}
	for _, m := range modos {
		for _, a := range agentes {
			for _, p := range pruebas {
				for r := 1; r <= *repsF; r++ {
					if err := copia(filepath.Join(dirBanco, cmp.Or(p.Proyecto, "proyecto")), carpeta(m, a, p.ID, r)); err != nil {
						log.Fatal(err)
					}
				}
			}
		}
	}
	// Los proveedores de pgocode (los mismos de la app), y que el agente esté instalado.
	provs := map[string]llm.Proveedor{}
	for _, p := range llm.Agentes() {
		provs[p.ID()] = p
	}
	for _, a := range agentes {
		if i := provs[a].Info(context.Background()); !i.Disponible {
			log.Fatalf("%s no está disponible: %s", a, i.Motivo)
		}
	}
	ctx, cancela := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancela()

	fj, err := os.Create(filepath.Join(salida, marca+".jsonl"))
	if err != nil {
		log.Fatal(err)
	}
	defer fj.Close()
	var mu sync.Mutex
	var todas []Ejecucion
	parado := map[string]bool{} // agentes que han dado con un límite
	// Un hilo por agente (no comparten límites ni caché); dentro, cada prueba alterna los modos repetición a
	// repetición, para que ninguno se lleve siempre la caché caliente.
	var wg sync.WaitGroup
	for _, a := range agentes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, p := range pruebas {
				for r := 1; r <= *repsF; r++ {
					orden := modos
					if r%2 == 0 {
						orden = slices.Clone(modos)
						slices.Reverse(orden)
					}
					for _, m := range orden {
						if ctx.Err() != nil {
							return
						}
						mu.Lock()
						fuera := parado[a]
						mu.Unlock()
						if fuera {
							return
						}
						e := ejecuta(ctx, m, a, banco.Agentes[a], p, r, carpeta(m, a, p.ID, r), datosOC, provs[a], nuevaEstrategia())
						e.Fecha, e.Nota, e.Carpeta, e.Estrategia = marca, *notaF, filepath.Base(e.Carpeta), *estrategiaF
						estado := "✓"
						if !e.Acierto {
							estado = "✗ " + e.Fallo
						}
						log.Printf("%-11s %-8s prueba %d rep %d: %s entrada, %s salida, %d llamadas, %.0f s %s",
							a, m, p.ID, r, miles(e.Entrada), miles(e.Salida), e.Llamadas, e.Segundos, estado)
						if !e.Acierto && limite.MatchString(e.Fallo) {
							log.Printf("%s: %s — no se lanza más con él (esta ejecución no cuenta)", a, e.Fallo)
							mu.Lock()
							parado[a] = true
							mu.Unlock()
							return
						}
						mu.Lock()
						todas = append(todas, e)
						b, _ := json.Marshal(e)
						fj.Write(append(b, '\n'))
						mu.Unlock()
					}
				}
			}
		}()
	}
	wg.Wait()
	informe := informe(banco, agentes, pruebas, modos, todas, dirBanco, *repsF, *estrategiaF)
	if len(parado) > 0 {
		var l []string
		for a := range parado {
			l = append(l, a)
		}
		sort.Strings(l)
		informe += "\n**Ronda incompleta:** " + strings.Join(l, ", ") + " dio con un límite de gasto o de uso y se paró.\n"
	}
	if *notaF != "" {
		informe = strings.Replace(informe, "\n\n", "\n\n**Nota:** "+*notaF+"\n\n", 1)
	}
	fmt.Println(informe)
	os.WriteFile(filepath.Join(salida, marca+".md"), []byte(informe), 0o644)
	if err := evolucion(salida, banco); err != nil {
		log.Printf("evolución: %v", err)
	}
	log.Printf("resultados: %s.jsonl y %s.md; evolución en %s", filepath.Join(salida, marca), filepath.Join(salida, marca), filepath.Join(salida, "evolucion.md"))
	// Calibración: la ronda completa sin pgocode (todas las pruebas, con 3 repeticiones o más).
	if slices.Contains(modos, "directo") && len(pruebas) == len(banco.Pruebas) && *repsF >= 3 && ctx.Err() == nil && len(parado) == 0 {
		guardaCalibracion(dirBanco, banco, agentes, todas, marca)
	}
	if !*conserva && ctx.Err() == nil {
		os.RemoveAll(trabajo)
	} else {
		log.Printf("copias de trabajo en %s", trabajo)
	}
}

// ---- informe ----

type resumen struct {
	Medida
	Aciertos, Total int
}

// mediana de cada medida (por separado) y aciertos.
func resume(l []Ejecucion) resumen {
	r := resumen{Total: len(l)}
	if len(l) == 0 {
		return r
	}
	med := func(f func(Ejecucion) float64) float64 {
		v := make([]float64, len(l))
		for i, e := range l {
			v[i] = f(e)
		}
		sort.Float64s(v)
		if len(v)%2 == 1 {
			return v[len(v)/2]
		}
		return (v[len(v)/2-1] + v[len(v)/2]) / 2
	}
	r.Entrada = int(med(func(e Ejecucion) float64 { return float64(e.Entrada) }))
	r.Salida = int(med(func(e Ejecucion) float64 { return float64(e.Salida) }))
	r.CacheLeida = int(med(func(e Ejecucion) float64 { return float64(e.CacheLeida) }))
	r.CacheEscrita = int(med(func(e Ejecucion) float64 { return float64(e.CacheEscrita) }))
	r.Llamadas = int(med(func(e Ejecucion) float64 { return float64(e.Llamadas) }))
	r.Herramientas = int(med(func(e Ejecucion) float64 { return float64(e.Herramientas) }))
	r.Coste = med(func(e Ejecucion) float64 { return e.Coste })
	r.Segundos = med(func(e Ejecucion) float64 { return e.Segundos })
	r.Primera = med(func(e Ejecucion) float64 { return e.Primera })
	r.Contexto = int(med(func(e Ejecucion) float64 { return float64(e.Contexto) }))
	for _, e := range l {
		if e.Acierto {
			r.Aciertos++
		}
	}
	return r
}

func filtra(l []Ejecucion, modo, agente string, prueba int) []Ejecucion {
	var r []Ejecucion
	for _, e := range l {
		if e.Modo == modo && e.Agente == agente && e.Prueba == prueba {
			r = append(r, e)
		}
	}
	return r
}

// Calibración guardada: las medianas sin pgocode por agente y prueba.
type Calibracion struct {
	Fecha   string                                   `json:"fecha"`
	Agentes map[string]ConfAgente                    `json:"agentes"`
	Medidas map[string]map[string]resumenCalibracion `json:"medidas"` // agente → prueba → mediana
}

type resumenCalibracion struct {
	Medida
	Aciertos int `json:"aciertos"`
	Total    int `json:"total"`
}

func guardaCalibracion(dir string, banco Banco, agentes []string, todas []Ejecucion, marca string) {
	c := Calibracion{Fecha: marca, Agentes: map[string]ConfAgente{}, Medidas: map[string]map[string]resumenCalibracion{}}
	if b, err := os.ReadFile(filepath.Join(dir, "calibracion.json")); err == nil {
		json.Unmarshal(b, &c) // se conservan los agentes que no se han vuelto a calibrar
		c.Fecha = marca
	}
	for _, a := range agentes {
		c.Agentes[a] = banco.Agentes[a]
		c.Medidas[a] = map[string]resumenCalibracion{}
		for _, p := range banco.Pruebas {
			r := resume(filtra(todas, "directo", a, p.ID))
			c.Medidas[a][strconv.Itoa(p.ID)] = resumenCalibracion{Medida: r.Medida, Aciertos: r.Aciertos, Total: r.Total}
		}
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "calibracion.json"), append(b, '\n'), 0o644); err == nil {
		log.Printf("calibración guardada en %s", filepath.Join(dir, "calibracion.json"))
	}
}

func informe(banco Banco, agentes []string, pruebas []Prueba, modos []string, todas []Ejecucion, dirBanco string, reps int, estrategia string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Banco de consumo · %s\n\n", time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "Medianas de %d repeticiones. «Entrada» incluye la caché (leída y escrita). Tokens en miles (k). Estrategia: `%s`.\n\n", reps, estrategia)
	for _, a := range agentes {
		c := banco.Agentes[a]
		fmt.Fprintf(&b, "- **%s**: modelo `%s`%s\n", a, c.Modelo, map[bool]string{true: ", esfuerzo `" + c.Esfuerzo + "`", false: ""}[c.Esfuerzo != ""])
	}
	// Sin pgocode en esta ronda: la referencia es la calibración guardada.
	var cal *Calibracion
	if !slices.Contains(modos, "directo") {
		if bs, err := os.ReadFile(filepath.Join(dirBanco, "calibracion.json")); err == nil {
			cal = &Calibracion{}
			if json.Unmarshal(bs, cal) != nil {
				cal = nil
			} else {
				fmt.Fprintf(&b, "\nSin pgocode: la calibración del %s.\n", cal.Fecha)
			}
		}
	}
	directoDe := func(a string, p int) (resumen, bool) {
		if slices.Contains(modos, "directo") {
			return resume(filtra(todas, "directo", a, p)), true
		}
		if cal != nil {
			if m, ok := cal.Medidas[a][strconv.Itoa(p)]; ok {
				return resumen{Medida: m.Medida, Aciertos: m.Aciertos, Total: m.Total}, true
			}
		}
		return resumen{}, false
	}
	for _, p := range pruebas {
		fmt.Fprintf(&b, "\n## %d · %s (%s)\n\n%s\n\n", p.ID, p.Nombre, p.Permisos, p.Mide)
		b.WriteString("| Agente | Modo | Entrada | Salida | Caché leída | Caché escrita | Llamadas | Herram. | Coste | Tiempo | 1.ª resp. | Aciertos |\n")
		b.WriteString("|---|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|\n")
		for _, a := range agentes {
			fila := func(modo string, r resumen) {
				coste := "—"
				if r.Coste > 0 {
					coste = fmt.Sprintf("%.3f $", r.Coste)
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %d | %s | %.0f s | %.1f s | %d/%d |\n", a, modo,
					miles(r.Entrada), miles(r.Salida), miles(r.CacheLeida), miles(r.CacheEscrita), r.Llamadas, r.Herramientas, coste, r.Segundos, r.Primera, r.Aciertos, r.Total)
			}
			d, hayD := directoDe(a, p.ID)
			if hayD {
				fila("sin pgocode", d)
			}
			if slices.Contains(modos, "pgocode") {
				g := resume(filtra(todas, "pgocode", a, p.ID))
				fila("pgocode", g)
				if hayD && d.Entrada+d.Salida > 0 {
					fmt.Fprintf(&b, "| | **pgocode / sin** | **%s** | | | | | | | **%s** | | |\n",
						porcentaje(g.Entrada+g.Salida, d.Entrada+d.Salida), porcentajeF(g.Segundos, d.Segundos))
				}
			}
		}
		if len(p.Turnos) > 1 {
			for _, a := range agentes {
				b.WriteString(turnEvolution(p, a, modos, todas))
			}
		}
	}
	// Fallos, para mirar qué pasó.
	var fallos []string
	for _, e := range todas {
		if !e.Acierto {
			fallos = append(fallos, fmt.Sprintf("- %s · %s · prueba %d rep %d: %s", e.Agente, e.Modo, e.Prueba, e.Rep, e.Fallo))
		}
	}
	if len(fallos) > 0 {
		sort.Strings(fallos)
		b.WriteString("\n## Fallos\n\n" + strings.Join(fallos, "\n") + "\n")
	}
	return b.String()
}

// turnEvolution: en una prueba de varios turnos, la mediana de cada turno por modo: el contexto (lo que se
// reenvía en cada llamada), la entrada (con la caché) y el coste, para ver cómo crece la conversación.
func turnEvolution(p Prueba, agente string, modos []string, todas []Ejecucion) string {
	porModo := map[string][]Ejecucion{}
	var conDatos []string
	for _, m := range modos {
		for _, e := range filtra(todas, m, agente, p.ID) {
			if len(e.Turnos) > 0 {
				porModo[m] = append(porModo[m], e)
			}
		}
		if len(porModo[m]) > 0 {
			conDatos = append(conDatos, m)
		}
	}
	if len(conDatos) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n**%s, turno a turno** (medianas; «Contexto»: tokens de la última llamada del turno, lo que se reenvía)\n\n| Turno | Mensaje |", agente)
	sep := "|--:|---|"
	for _, m := range conDatos {
		n := nombreModo(m)
		fmt.Fprintf(&b, " Contexto %s | Entrada %s | Coste %s | Bien %s |", n, n, n, n)
		sep += "--:|--:|--:|--:|"
	}
	b.WriteString("\n" + sep + "\n")
	for i, t := range p.Turnos {
		// Lo que ha hecho la estrategia en el turno (relevo, compactación), en alguna de las ejecuciones.
		var notas []string
		for _, m := range conDatos {
			for _, e := range porModo[m] {
				if i < len(e.Turnos) && e.Turnos[i].Nota != "" && !slices.Contains(notas, e.Turnos[i].Nota) {
					notas = append(notas, e.Turnos[i].Nota)
				}
			}
		}
		celda := recorta(t.Mensaje, 50)
		if len(notas) > 0 {
			celda += " **[" + strings.Join(notas, "; ") + "]**"
		}
		fmt.Fprintf(&b, "| %d | %s |", i+1, strings.ReplaceAll(celda, "|", "/"))
		for _, m := range conDatos {
			var l []Ejecucion
			for _, e := range porModo[m] {
				if i < len(e.Turnos) {
					l = append(l, Ejecucion{Medida: e.Turnos[i].Medida, Acierto: e.Turnos[i].Acierto})
				}
			}
			if len(l) == 0 {
				b.WriteString(" — | — | — | — |")
				continue
			}
			r := resume(l)
			fmt.Fprintf(&b, " %s | %s | %.3f $ | %d/%d |", miles(r.Contexto), miles(r.Entrada), r.Coste, r.Aciertos, len(porModo[m]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func nombreModo(m string) string {
	return map[string]string{"directo": "sin pgocode", "pgocode": "pgocode"}[m]
}

func porcentaje(a, b int) string { return porcentajeF(float64(a), float64(b)) }
func porcentajeF(a, b float64) string {
	if b == 0 {
		return "—"
	}
	return fmt.Sprintf("%+.0f %%", (a/b-1)*100)
}

func miles(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return strconv.Itoa(n)
}

func recorta(t string, n int) string {
	t = strings.Join(strings.Fields(t), " ")
	if len(t) > n {
		return t[:n] + "…"
	}
	return t
}

// evolucion: todas las rondas de la carpeta de resultados, prueba a prueba: mediana de tokens, coste y tiempo por
// fecha, agente y modo (para ver si cada mejora ahorra de verdad).
func evolucion(dir string, banco Banco) error {
	fs, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return err
	}
	sort.Strings(fs)
	type clave struct {
		fecha, agente, modo, estrategia string
		prueba                          int
	}
	grupos := map[clave][]Ejecucion{}
	notas := map[string]string{}
	var claves []clave
	for _, f := range fs {
		fecha := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		bs, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		for _, l := range bytes.Split(bs, []byte("\n")) {
			var e Ejecucion
			if json.Unmarshal(l, &e) != nil {
				continue
			}
			if e.Nota != "" {
				notas[fecha] = e.Nota
			}
			k := clave{fecha, e.Agente, e.Modo, e.Estrategia, e.Prueba}
			if _, ok := grupos[k]; !ok {
				claves = append(claves, k)
			}
			grupos[k] = append(grupos[k], e)
		}
	}
	sort.Slice(claves, func(i, j int) bool {
		a, b := claves[i], claves[j]
		if a.prueba != b.prueba {
			return a.prueba < b.prueba
		}
		if a.agente != b.agente {
			return a.agente < b.agente
		}
		if a.modo != b.modo {
			return a.modo < b.modo
		}
		if a.estrategia != b.estrategia {
			return a.estrategia < b.estrategia
		}
		return a.fecha < b.fecha
	})
	var b strings.Builder
	b.WriteString("# Evolución del consumo\n\nMedianas de cada ronda (`<fecha>.jsonl`: los datos originales). Tokens = entrada (con la caché) + salida.\n")
	if len(notas) > 0 {
		b.WriteString("\n## Rondas con nota\n\n")
		var ks []string
		for k := range notas {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			fmt.Fprintf(&b, "- **%s**: %s\n", k, notas[k])
		}
	}
	prueba := -1
	for _, k := range claves {
		if k.prueba != prueba {
			prueba = k.prueba
			nombre := ""
			if i := slices.IndexFunc(banco.Pruebas, func(p Prueba) bool { return p.ID == k.prueba }); i >= 0 {
				nombre = banco.Pruebas[i].Nombre
			}
			fmt.Fprintf(&b, "\n## %d · %s\n\n| Agente | Modo | Ronda | Tokens | Entrada | Salida | Llamadas | Coste | Tiempo | Aciertos |\n|---|---|---|--:|--:|--:|--:|--:|--:|--:|\n", k.prueba, nombre)
		}
		r := resume(grupos[k])
		coste := "—"
		if r.Coste > 0 {
			coste = fmt.Sprintf("%.3f $", r.Coste)
		}
		modo := nombreModo(k.modo)
		if k.estrategia != "" && k.estrategia != "ninguna" {
			modo += " · " + k.estrategia
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %s | %.0f s | %d/%d |\n", k.agente, modo, k.fecha,
			miles(r.Entrada+r.Salida), miles(r.Entrada), miles(r.Salida), r.Llamadas, coste, r.Segundos, r.Aciertos, r.Total)
	}
	return os.WriteFile(filepath.Join(dir, "evolucion.md"), []byte(b.String()), 0o644)
}
