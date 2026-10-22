// consumo: banco de consumo de tokens. Lanza las pruebas de consumo/pruebas.json con cada agente de dos formas:
//
//   - directo (la calibración): el CLI del agente a pelo, como lo usaría alguien sin pgocode;
//   - pgocode: con la capa de pgocode que lanza los agentes (internal/llm), la misma que usa la app: sus
//     argumentos, los MCP que filtra, el idioma, su entorno y la entrada por stdin de Claude Code.
//
// Sin ninguna dependencia del ERP: ni Odoo (tampoco el simulado), ni login, ni proyectos.
//
// Cada prueba trabaja en una copia propia de consumo/proyecto (un repositorio git nuevo) y se comprueba sola
// (texto de la respuesta, una orden que ha de acabar bien, ficheros que no se pueden tocar…): gastar poco y
// hacerlo mal no cuenta. Gasta tokens de verdad: no entra en «make test».
//
//	go run ./cmd/consumo                                  # todo: 3 agentes × 5 pruebas × 3 repeticiones × 2 modos
//	go run ./cmd/consumo -pruebas 1,2,3 -repeticiones 1   # ronda rápida
//	go run ./cmd/consumo -modo pgocode                    # solo con pgocode, comparado con la última calibración
//
// Resultados, en el repositorio para seguir la evolución: consumo/resultados/<fecha>.jsonl (los datos de cada
// ejecución, según acaba), <fecha>.md (el informe) y evolucion.md (todas las rondas). Una ronda completa sin
// pgocode guarda además la calibración en consumo/calibracion.json.
package main

import (
	"bytes"
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
	modoF    = flag.String("modo", "ambos", "ambos | directo (calibración, sin pgocode) | pgocode")
	agentesF = flag.String("agentes", "claude-code,antigravity,opencode", "agentes, separados por comas")
	pruebasF = flag.String("pruebas", "1,2,3,4,5", "pruebas (sus id), separadas por comas")
	repsF    = flag.Int("repeticiones", 3, "repeticiones de cada prueba (se toma la mediana)")
	salidaF  = flag.String("salida", "", "carpeta de resultados (vacío: consumo/resultados, en el repositorio)")
	notaF    = flag.String("nota", "", "qué cambia en esta ronda (sale en el informe y en la evolución)")
	conserva = flag.Bool("conservar", false, "no borra las copias de trabajo al acabar")
	limiteF  = flag.Duration("limite", 20*time.Minute, "tiempo máximo de cada turno")
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
}

// resultadoTurno: lo que deja un turno (en cualquiera de los dos modos).
type resultadoTurno struct {
	Medida
	texto  string
	sesion string
	err    error
}

// recoge: junta los eventos de un turno. Antigravity da el consumo acumulado de su conversación: se resta el
// del turno anterior (previo).
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
	if h := e.Herramienta; h != nil && h.Estado == "curso" && h.Entrada != "" {
		c.r.Herramientas++
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
	}
}

func (c *recoge) fin() resultadoTurno {
	u := c.acumula
	if c.agente == "antigravity" && c.previo != nil {
		u.Entrada -= c.previo.Entrada
		u.Salida -= c.previo.Salida
		u.CacheLeida -= c.previo.CacheLeida
	}
	c.r.Entrada, c.r.Salida, c.r.CacheLeida, c.r.CacheEscrita = max(u.Entrada, 0), max(u.Salida, 0), max(u.CacheLeida, 0), u.CacheEscrita
	c.r.Llamadas, c.r.Coste = u.Llamadas, u.Coste
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
	errLee := llm.Lector(agente)(out, c.evento)
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
func conPgocode(ctx context.Context, prov llm.Proveedor, agente string, conf ConfAgente, carpeta, permisos, msg, sesion string) resultadoTurno {
	c := &recoge{inicio: time.Now(), agente: agente}
	var mu sync.Mutex
	err := prov.Chat(ctx, llm.Peticion{Modelo: conf.Modelo, Esfuerzo: conf.Esfuerzo, Carpeta: carpeta, Permisos: permisos,
		SesionAgente: sesion, Mensajes: []llm.Mensaje{{Rol: "user", Texto: msg}}, Idioma: llm.NombreIdioma("es-ES"),
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
		if out, _ := exec.Command("git", "-C", carpeta, "status", "--porcelain", "--", r).Output(); len(bytes.TrimSpace(out)) > 0 {
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

func ejecuta(ctx context.Context, modo, agente string, conf ConfAgente, pr Prueba, rep int, carpeta, datosOC string, prov llm.Proveedor) Ejecucion {
	e := Ejecucion{Modo: modo, Agente: agente, Prueba: pr.ID, Rep: rep, Carpeta: carpeta, Acierto: true}
	sesion := ""
	var previo *llm.Uso
	for i, t := range pr.Turnos {
		cx, cancela := context.WithTimeout(ctx, *limiteF)
		var r resultadoTurno
		if modo == "directo" {
			r = directo(cx, agente, conf, carpeta, pr.Permisos, t.Mensaje, sesion, previo, datosOC)
		} else {
			r = conPgocode(cx, prov, agente, conf, carpeta, pr.Permisos, t.Mensaje, sesion)
		}
		cancela()
		e.Medida.suma(r.Medida)
		if agente == "antigravity" && modo == "directo" {
			// El acumulado de la conversación, para restarlo en el turno siguiente (pgocode ya lo resta).
			a := llm.Uso{Entrada: r.Entrada, Salida: r.Salida, CacheLeida: r.CacheLeida}
			if previo != nil {
				a.Entrada += previo.Entrada
				a.Salida += previo.Salida
				a.CacheLeida += previo.CacheLeida
			}
			previo = &a
		}
		if r.sesion != "" {
			sesion = r.sesion
		}
		if r.err != nil {
			e.Acierto, e.Fallo = false, fmt.Sprintf("turno %d: %v", i+1, r.err)
			break
		}
		if f := comprueba(t, r.texto, carpeta); f != "" {
			e.Acierto, e.Fallo = false, fmt.Sprintf("turno %d: %s", i+1, f)
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
					if err := copia(filepath.Join(dirBanco, "proyecto"), carpeta(m, a, p.ID, r)); err != nil {
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
						e := ejecuta(ctx, m, a, banco.Agentes[a], p, r, carpeta(m, a, p.ID, r), datosOC, provs[a])
						e.Fecha, e.Nota, e.Carpeta = marca, *notaF, filepath.Base(e.Carpeta)
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
	informe := informe(banco, agentes, pruebas, modos, todas, dirBanco, *repsF)
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

func informe(banco Banco, agentes []string, pruebas []Prueba, modos []string, todas []Ejecucion, dirBanco string, reps int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Banco de consumo · %s\n\n", time.Now().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "Medianas de %d repeticiones. «Entrada» incluye la caché (leída y escrita). Tokens en miles (k).\n\n", reps)
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
		fecha, agente, modo string
		prueba              int
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
			k := clave{fecha, e.Agente, e.Modo, e.Prueba}
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
		modo := map[string]string{"directo": "sin pgocode", "pgocode": "pgocode"}[k.modo]
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d | %s | %.0f s | %d/%d |\n", k.agente, modo, k.fecha,
			miles(r.Entrada+r.Salida), miles(r.Entrada), miles(r.Salida), r.Llamadas, coste, r.Segundos, r.Aciertos, r.Total)
	}
	return os.WriteFile(filepath.Join(dir, "evolucion.md"), []byte(b.String()), 0o644)
}
