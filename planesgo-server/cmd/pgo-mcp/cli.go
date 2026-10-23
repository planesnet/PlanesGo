package main

// Punto de entrada, opciones de línea de órdenes (para los hooks y el comando /pgo) y autoactualización.

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	codigo, servidor := ejecutaCLI(os.Args[1:], os.Stdout, os.Stderr)
	if servidor {
		// Sin órdenes: servidor MCP sobre stdio
		runMCPServer()
		return
	}
	os.Exit(codigo)
}

// autoactualizar se cambia en las pruebas para no lanzar procesos.
var autoactualizar = maybeSpawnBackgroundSelfUpdate

// ejecutaCLI atiende las opciones de línea de órdenes. Devuelve el código de salida o, si no se
// pidió ninguna orden, servidor=true para arrancar el servidor MCP.
func ejecutaCLI(argv []string, salida, errSalida io.Writer) (codigo int, servidor bool) {
	fs := flag.NewFlagSet("pgo-mcp", flag.ContinueOnError)
	fs.SetOutput(errSalida)

	// Imputación automática (hooks): no cambiar sin cambiar los hooks
	checkFlag := fs.Bool("check", false, "Comprueba token, proyecto vinculado y tarea")
	beatFlag := fs.Bool("beat", false, "Envía un latido de la tarea (--task)")
	stopFlag := fs.Bool("stop", false, "Cierra la tarea y consolida la imputación")
	taskFlag := fs.String("task", "", "Nombre de la tarea")
	typeFlag := fs.String("type", "", "Tipo de tarea: Análisis y diseño, Desarrollo o Pruebas (opcional)")
	descFlag := fs.String("desc", "", "Descripción del trabajo")
	pathFlag := fs.String("path", "", "Raíz del proyecto")
	projectIDFlag := fs.Int("project-id", 0, "ID del proyecto de Odoo (con --link lo vincula)")
	sessionIDFlag := fs.String("session-id", "", "Identificador de la sesión de IA (vincula por sesión y se guarda como ai_session_id)")
	modelFlag := fs.String("model", "", "Modelo de IA (p. ej. Claude Code)")
	tokensFlag := fs.Int("tokens", 0, "Tokens totales consumidos por la IA")
	tokensInFlag := fs.Int("tokens-in", 0, "Tokens de entrada")
	tokensOutFlag := fs.Int("tokens-out", 0, "Tokens de salida")

	// Proyecto y tiempo a mano (comando /pgo)
	projectFlag := fs.String("project", "", "Busca proyectos por texto y muestra las candidatas (no vincula)")
	linkFlag := fs.Bool("link", false, "Vincula el proyecto de --project-id a --path (o a la sesión con --session-id)")
	timeFlag := fs.Float64("time", 0, "Añade N minutos al proyecto vinculado (con --task opcional; si la tarea no existe, se crea)")

	versionFlag := fs.Bool("version", false, "Muestra la versión y sale")
	vFlag := fs.Bool("v", false, "Muestra la versión y sale")
	selfUpdateFlag := fs.Bool("self-update", false, "Reinstala pgo-mcp (y el hook y el comando /pgo) con la versión actual de master")

	fs.Usage = func() {
		fmt.Fprintf(errSalida, "pgo-mcp v%s: servidor MCP «pgo» de PlanesGo (sin opciones) y órdenes de imputación.\n\n", Version)
		fmt.Fprintln(errSalida, "Uso:")
		fmt.Fprintln(errSalida, "  pgo-mcp                                   servidor MCP sobre stdio")
		fmt.Fprintln(errSalida, "  pgo-mcp --project \"<texto>\"               busca proyectos (no vincula)")
		fmt.Fprintln(errSalida, "  pgo-mcp --project-id N --link --path DIR  vincula el proyecto")
		fmt.Fprintln(errSalida, "  pgo-mcp --time N [--task \"<tarea>\"]       añade N minutos")
		fmt.Fprintln(errSalida, "  pgo-mcp --check|--beat|--stop ...         imputación automática (hooks)")
		fmt.Fprintln(errSalida, "\nOpciones:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	timeDado := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "time" {
			timeDado = true
		}
	})

	if *versionFlag || *vFlag {
		fmt.Fprintf(salida, "pgo-mcp v%s\n", Version)
		return 0, false
	}
	if *selfUpdateFlag {
		runSelfUpdate(true)
		return 0, false
	}

	// Dentro de pgocode, ninguna orden que hable con PlanesGo (y con él, con Odoo): ni imputar, ni vincular, ni buscar.
	if dentroDePgocode() {
		fmt.Fprintln(errSalida, textoBloqueadoPgocode)
		return 3, false
	}

	// Al menos una vez cada selfUpdateCheckInterval, de forma no bloqueante
	autoactualizar()

	comunes := func(args map[string]interface{}) map[string]interface{} {
		if *pathFlag != "" {
			args["project_path"] = *pathFlag
		}
		if *sessionIDFlag != "" {
			args["session_id"] = *sessionIDFlag
			args["ai_session_id"] = *sessionIDFlag
		}
		if *projectIDFlag > 0 {
			args["project_id"] = float64(*projectIDFlag)
		}
		if *modelFlag != "" {
			args["ai_model"] = *modelFlag
		}
		if *typeFlag != "" {
			args["task_type"] = *typeFlag
		}
		return args
	}
	tokens := func(args map[string]interface{}) map[string]interface{} {
		if *tokensFlag > 0 {
			args["tokens_total"] = float64(*tokensFlag)
		}
		if *tokensInFlag > 0 {
			args["tokens_input"] = float64(*tokensInFlag)
		}
		if *tokensOutFlag > 0 {
			args["tokens_output"] = float64(*tokensOutFlag)
		}
		return args
	}
	imprime := func(res ToolCallResult) int {
		if len(res.Content) > 0 {
			fmt.Fprintln(salida, res.Content[0].Text)
		}
		if res.IsError {
			return 1
		}
		return 0
	}

	switch {
	case *checkFlag:
		args := comunes(map[string]interface{}{})
		if *taskFlag != "" {
			args["task_name"] = *taskFlag
		}
		if *descFlag != "" {
			args["description"] = *descFlag
		}
		return imprime(ordenCheck(args)), false

	case *beatFlag:
		if *taskFlag == "" {
			fmt.Fprintln(errSalida, "Error: debe indicar --task '<nombre>'")
			return 1, false
		}
		args := tokens(comunes(map[string]interface{}{"task_name": *taskFlag, "description": *descFlag}))
		return imprime(ordenBeat(args)), false

	case *stopFlag:
		args := tokens(comunes(map[string]interface{}{"task_name": *taskFlag, "description": *descFlag}))
		return imprime(ordenStop(args)), false

	case timeDado:
		args := comunes(map[string]interface{}{"minutes": *timeFlag})
		if *taskFlag != "" {
			args["task"] = *taskFlag
		}
		if *descFlag != "" {
			args["description"] = *descFlag
		}
		return imprime(anadeTiempo(args)), false

	case *linkFlag:
		if *projectIDFlag <= 0 {
			fmt.Fprintln(errSalida, "Error: --link necesita --project-id N (búscalo antes con --project \"<texto>\")")
			return 1, false
		}
		return imprime(vinculaProyecto(comunes(map[string]interface{}{}))), false

	case *projectFlag != "":
		res, n := buscaProyectos(comunes(map[string]interface{}{"query": *projectFlag}))
		if n > 0 {
			res.Content[0].Text += "\n\nNo se ha vinculado nada. Para vincular: pgo-mcp --project-id <id> --link --path \"<raíz del proyecto>\""
		}
		return imprime(res), false

	case *projectIDFlag > 0:
		fmt.Fprintln(errSalida, "Error: para vincular el proyecto "+strconv.Itoa(*projectIDFlag)+" añade --link")
		return 1, false
	}

	return 0, true
}

// ---------------------------------------------------------------------------
// Autoactualización: en vez de duplicar en Go la lógica de compilación que ya vive en
// scripts/install-claude-hook.sh, el binario relanza ese mismo instalador (curl | bash) de vez en
// cuando. Un único camino de actualización, el mismo que se usa a mano. El instalador compila
// pgo-mcp y deja planesgo-mcp como enlace a él.
// ---------------------------------------------------------------------------

const (
	selfUpdateCheckInterval = 6 * time.Hour
	selfUpdateInstallURL    = DefaultServer + "/install/claude-hook.sh"
)

func selfUpdateMarkerPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "planesgo-mcp-last-self-update")
}

// selfUpdateDue indica si ya toca volver a autoactualizar (como mucho cada selfUpdateCheckInterval).
func selfUpdateDue() bool {
	p := selfUpdateMarkerPath()
	if p == "" {
		return false
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return true
	}
	last, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return true
	}
	return time.Since(time.Unix(last, 0)) > selfUpdateCheckInterval
}

func touchSelfUpdateMarker() {
	p := selfUpdateMarkerPath()
	if p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	_ = os.WriteFile(p, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0644)
}

// runSelfUpdate reutiliza el instalador de siempre (curl | bash): reconstruye el hook, el comando /pgo y
// el binario pgo-mcp con la versión actual de master. No falla nunca de forma ruidosa (ni bloquea nada):
// cualquier problema (sin bash, sin red, instalación fallida) solo se imprime si verbose=true.
func runSelfUpdate(verbose bool) {
	touchSelfUpdateMarker()
	if _, err := exec.LookPath("bash"); err != nil {
		if verbose {
			fmt.Println("AVISO: 'bash' no está disponible; no se puede autoactualizar.")
		}
		return
	}
	cmd := exec.Command("bash", "-c", "curl -fsSL "+selfUpdateInstallURL+" | bash")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if verbose {
			fmt.Printf("AVISO: fallo al autoactualizar: %v\n%s\n", err, string(out))
		}
		return
	}
	if verbose {
		fmt.Println(string(out))
		fmt.Println("✅ pgo-mcp y el hook de PlanesGo actualizados a la versión actual de master.")
	}
}

// maybeSpawnBackgroundSelfUpdate lanza la actualización en un proceso aparte y no bloqueante, como mucho una
// vez cada selfUpdateCheckInterval, para no añadir latencia ni dependencia de red a la invocación actual.
func maybeSpawnBackgroundSelfUpdate() {
	if !selfUpdateDue() {
		return
	}
	touchSelfUpdateMarker()
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exePath, "--self-update")
	separaProceso(cmd)
	_ = cmd.Start()
}
