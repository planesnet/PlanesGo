package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/depuracion"
	"planesgo-code/internal/proceso"
)

// sesionValida: los ids de sesión de los agentes son UUID o similares. Se valida porque el valor llega del
// navegador y va como argumento del programa (un «--opcion» se tomaría como opción).
var sesionValida = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// Agente lanza un programa de agente (Claude Code, OpenCode, Gemini CLI…) en la carpeta del proyecto y
// retransmite su respuesta. Solo se le pasa el último mensaje: el agente lleva su propio historial.
type Agente struct {
	Clave, Nombre, Programa string
	// Args construye los argumentos a partir del mensaje y la petición (sesión anterior del agente, permisos,
	// modelo y esfuerzo, ya validados).
	Args func(msg string, p Peticion) []string
	// Modelos que se pueden elegir además de «por defecto», niveles de esfuerzo y si admite modos de permisos.
	Modelos, Esfuerzos []string
	Modos              bool
	// PorDefecto: el modelo que se usa si no se elige otro (vacío: el del propio agente).
	PorDefecto string
	// EntornoDefecto: variables «CLAVE=valor» que se ponen solo si el usuario no las tiene ya.
	EntornoDefecto []string
	// ModelosPara: lista de modelos que depende del equipo (p. ej. si hay clave de una API); sustituye a Modelos.
	ModelosPara func() []string
	// EntornoPara: variables según la petición (p. ej. los permisos de OpenCode según el modo).
	EntornoPara func(p Peticion) []string
	// Lee interpreta la salida; si es nil, la salida se pasa tal cual.
	Lee func(r io.Reader, emite func(Evento)) error
	// NoDisponible: el agente existe pero no se puede usar desde aquí (y por qué).
	NoDisponible string
}

func (a *Agente) ID() string { return a.Clave }

func (a *Agente) Info(ctx context.Context) Info {
	lista := a.Modelos
	if a.ModelosPara != nil {
		lista = a.ModelosPara()
	}
	modelos, defecto := append([]string{"por defecto"}, lista...), "por defecto"
	if a.PorDefecto != "" {
		modelos, defecto = lista, a.PorDefecto
	}
	i := Info{ID: a.Clave, Nombre: a.Nombre, Tipo: "agente", Modelos: modelos, PorDefecto: defecto,
		NecesitaDir: true, Esfuerzos: a.Esfuerzos, Modos: a.Modos}
	if a.NoDisponible != "" {
		i.Motivo = a.NoDisponible
		return i
	}
	if _, err := exec.LookPath(a.Programa); err != nil {
		i.Motivo = "no está instalado («" + a.Programa + "» no está en el PATH)"
		return i
	}
	i.Disponible = true
	return i
}

func (a *Agente) Chat(ctx context.Context, p Peticion, emite func(Evento)) error {
	if p.Carpeta == "" {
		return errors.New("este proyecto no tiene carpeta local: falta un .planesgo.json con su odoo_project_id")
	}
	if len(p.Mensajes) == 0 {
		return errors.New("no hay mensaje")
	}
	if p.SesionAgente != "" && !sesionValida.MatchString(p.SesionAgente) {
		return errors.New("identificador de sesión del agente no válido")
	}
	msg := p.Mensajes[len(p.Mensajes)-1].Texto
	args := a.Args(msg, p)
	cmd := exec.CommandContext(ctx, a.Programa, args...)
	inicio := time.Now()
	id := fmt.Sprintf("%s-%d", a.Clave, inicio.UnixNano()%1e6)
	depuracion.Log(id, "lanza %s %q en %s (entorno añadido: %s)", a.Programa, args, p.Carpeta, clavesEntorno(p.Entorno)+" "+strings.Join(a.EntornoDefecto, ","))
	cmd.Dir = p.Carpeta
	entorno := append([]string(nil), p.Entorno...)
	if a.EntornoPara != nil {
		entorno = append(entorno, a.EntornoPara(p)...)
	}
	for _, kv := range a.EntornoDefecto {
		if k, _, _ := strings.Cut(kv, "="); os.Getenv(k) == "" {
			entorno = append(entorno, kv)
		}
	}
	if len(entorno) > 0 {
		cmd.Env = append(os.Environ(), entorno...)
	}
	// El agente va en su propio grupo de procesos: al cancelar se matan también sus hijos (servidores MCP,
	// shells…), y WaitDelay evita quedarse esperando a un nieto que conserve la salida abierta.
	proceso.GrupoPropio(cmd)
	cmd.WaitDelay = 5 * time.Second
	var errSalida strings.Builder
	cmd.Stderr = io.MultiWriter(&limitado{b: &errSalida, max: 8 << 10}, &lineasLog{id: id, flujo: "stderr"})
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		depuracion.Log(id, "no arranca: %v", err)
		return err
	}
	proceso.Vigila(cmd.Process.Pid)
	lee := a.Lee
	if lee == nil {
		lee = leeTexto
	}
	// Se cuenta lo que llega a la interfaz: si el agente acaba sin decir nada, es un fallo (no una respuesta vacía).
	respuesta := false
	errLee := lee(io.TeeReader(out, &lineasLog{id: id, flujo: "stdout"}), func(e Evento) {
		if e.Texto != "" || e.Herramienta != nil {
			respuesta = true
		}
		emite(e)
	})
	// Si la lectura acaba con error (p. ej. el agente se ha quedado esperando una respuesta que no puede
	// recibir), se para el agente: si no, Wait esperaría para siempre.
	if errLee != nil && cmd.Cancel != nil {
		cmd.Cancel()
	}
	errWait := cmd.Wait()
	depuracion.Log(id, "fin en %s: wait=%v lectura=%v respuesta=%v", time.Since(inicio).Round(time.Millisecond), errWait, errLee, respuesta)
	if errLee != nil && ctx.Err() == nil {
		return errLee
	}
	if errWait != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s terminó con error: %v %s", a.Programa, errWait, strings.TrimSpace(errSalida.String()))
	}
	if errLee == nil && !respuesta {
		detalle := strings.TrimSpace(errSalida.String())
		if detalle == "" {
			detalle = "sin mensajes de error"
		}
		return fmt.Errorf("%s acabó sin responder (%s)", a.Nombre, ultimas(detalle, 600))
	}
	return errLee
}

// permisosOpenCode: la configuración de permisos de OpenCode para cada modo de pgocode.
func permisosOpenCode(modo string) string {
	switch modo {
	case "auto":
		return `{"edit":"allow","bash":"allow","webfetch":"allow","external_directory":"allow","doom_loop":"allow"}`
	case "editar":
		return `{"edit":"allow","bash":"deny","webfetch":"allow","external_directory":"deny","doom_loop":"deny"}`
	default:
		return `{"edit":"deny","bash":"deny","webfetch":"allow","external_directory":"deny","doom_loop":"deny"}`
	}
}

// clavesEntorno: los nombres de las variables añadidas (sin sus valores: llevan tokens).
func clavesEntorno(e []string) string {
	l := make([]string, 0, len(e))
	for _, v := range e {
		k, _, _ := strings.Cut(v, "=")
		l = append(l, k)
	}
	return strings.Join(l, ",")
}

func ultimas(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// lineasLog escribe en el registro de depuración cada línea que pasa por él.
type lineasLog struct {
	id, flujo string
	resto     []byte
}

func (l *lineasLog) Write(p []byte) (int, error) {
	if !depuracion.Activo() {
		return len(p), nil
	}
	b := append(l.resto, p...)
	for {
		i := strings.IndexByte(string(b), '\n')
		if i < 0 {
			break
		}
		linea := string(b[:i])
		if len(linea) > 4000 {
			linea = linea[:4000] + "…"
		}
		depuracion.Log(l.id, "%s: %s", l.flujo, linea)
		b = b[i+1:]
	}
	l.resto = append([]byte(nil), b...)
	return len(p), nil
}

type limitado struct {
	b   *strings.Builder
	max int
}

func (l *limitado) Write(p []byte) (int, error) {
	if l.b.Len() < l.max {
		l.b.Write(p[:min(len(p), l.max-l.b.Len())])
	}
	return len(p), nil
}

// leeTexto pasa la salida tal cual, sin partir caracteres UTF-8 entre dos lecturas.
func leeTexto(r io.Reader, emite func(Evento)) error {
	b := make([]byte, 4096)
	var resto []byte
	for {
		n, err := r.Read(b)
		if n > 0 {
			datos := append(resto, b[:n]...)
			corte := len(datos)
			// Si el final es un carácter incompleto, se guarda para la siguiente lectura.
			for i := len(datos) - 1; i >= 0 && i >= len(datos)-utf8.UTFMax; i-- {
				if utf8.RuneStart(datos[i]) {
					if !utf8.FullRune(datos[i:]) {
						corte = i
					}
					break
				}
			}
			if corte > 0 {
				emite(Evento{Texto: string(datos[:corte])})
			}
			resto = append([]byte(nil), datos[corte:]...)
		}
		if err == io.EOF && len(resto) > 0 {
			emite(Evento{Texto: string(resto)})
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Agentes devuelve los agentes conocidos: Claude Code, Antigravity y OpenCode.
func Agentes() []Proveedor {
	return []Proveedor{
		&Agente{Clave: "claude-code", Nombre: "Claude Code", Programa: "claude", Lee: leeClaudeCode, Modos: true,
			Modelos: []string{"opus", "sonnet", "haiku"}, Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"-p", "--output-format", "stream-json", "--verbose"}
				// Sin permiso solo lee; «editar» cambia ficheros; «auto» ejecuta comandos con su control de seguridad.
				switch p.Permisos {
				case "editar":
					a = append(a, "--permission-mode", "acceptEdits")
				case "auto":
					a = append(a, "--permission-mode", "auto")
				}
				if p.Modelo != "" && p.Modelo != "por defecto" {
					a = append(a, "--model", p.Modelo)
				}
				if p.Esfuerzo != "" {
					a = append(a, "--effort", p.Esfuerzo)
				}
				if p.SesionAgente != "" {
					a = append(a, "--resume", p.SesionAgente)
				}
				// «--» antes del mensaje: un texto que empiece por «-» no se toma como opción.
				return append(a, "--", msg)
			}},
		// Antigravity: su CLI «agy» en modo print con eventos stream-json; --conversation continúa la sesión.
		&Agente{Clave: "antigravity", Nombre: "Antigravity", Programa: "agy", Lee: leeAntigravity, Modos: true,
			Modelos: []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-3.8-flash-high",
				"gemini-3.8-flash-medium", "gemini-3.8-flash-low", "gemini-3.1-pro-high", "gemini-3.1-pro-low"},
			Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"--output-format", "stream-json"}
				// Solo lectura: modo plan (no cambia ficheros); «editar»: accept-edits; «auto»: auto.
				switch p.Permisos {
				case "editar":
					a = append(a, "--mode", "accept-edits")
				case "auto":
					a = append(a, "--mode", "auto")
				default:
					a = append(a, "--mode", "plan")
				}
				if p.Modelo != "" && p.Modelo != "por defecto" {
					a = append(a, "--model", p.Modelo)
				}
				if p.Esfuerzo != "" {
					a = append(a, "--effort", p.Esfuerzo)
				}
				if p.SesionAgente != "" {
					a = append(a, "--conversation", p.SesionAgente)
				}
				// El mensaje va pegado a la opción: así nunca se toma como otra opción.
				return append(a, "--print="+msg)
			}},
		// OpenCode: «run --format json» (eventos de texto, herramientas, consumo y errores); -s continúa la sesión.
		&Agente{Clave: "opencode", Nombre: "OpenCode", Programa: "opencode", Lee: leeOpenCode, Modos: true,
			// Permisos según el modo, sin «ask»: en «run» la pregunta se queda esperando sin nadie que conteste.
			EntornoPara: entornoOpenCode, ModelosPara: modelosOpenCode,
			PorDefecto: "google-vertex/gemini-4-argon", Modelos: []string{"google-vertex/gemini-4-argon"},
			// Vertex AI: los modelos Gemini 4 están en la región «global» (en una regional, «Permission denied»).
			EntornoDefecto: []string{"GOOGLE_CLOUD_LOCATION=global", "VERTEX_LOCATION=global", "GOOGLE_CLOUD_PROJECT=planes-dev"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"run", "--format", "json"}
				modelo := p.Modelo
				if modelo == "" || modelo == "por defecto" {
					modelo = "google-vertex/gemini-4-argon"
				}
				a = append(a, "--model", modelo)
				if p.SesionAgente != "" {
					a = append(a, "--session", p.SesionAgente)
				}
				return append(a, "--", msg)
			}},
	}
}
