package llm

import (
	"context"
	"encoding/json"
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
	"planesgo-code/internal/skills"
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
	err := a.chat(ctx, p, emite)
	// La sesión propia del agente ya no existe (p. ej. OpenCode con otra carpeta de datos): se empieza otra.
	if err != nil && p.SesionAgente != "" && ctx.Err() == nil && sesionPerdida(err) {
		depuracion.Log(a.Clave, "la sesión %s ya no existe: se empieza otra", p.SesionAgente)
		p.SesionAgente = ""
		return a.chat(ctx, p, emite)
	}
	return err
}

func sesionPerdida(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "session") && (strings.Contains(s, "not found") || strings.Contains(s, "no existe") || strings.Contains(s, "notfound"))
}

func (a *Agente) chat(ctx context.Context, p Peticion, emite func(Evento)) error {
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
	if len(p.Adjuntos) > 0 {
		msg += "\n\nFicheros adjuntos (léelos con tus herramientas):"
		for _, f := range p.Adjuntos {
			msg += "\n- " + f
		}
	}
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
	// El agente va en su propio grupo y con una marca en el entorno que heredan todos sus descendientes: al
	// cancelar se le pide que termine (cierra sus tareas) y, tras unos segundos, se mata todo lo marcado, también
	// lo que lanzó en otro grupo (Bash en segundo plano). WaitDelay evita esperar a un nieto con la salida abierta.
	marca := fmt.Sprintf("%d-%s", os.Getpid(), id)
	proceso.Agente(cmd, marca)
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
	// Respuesta acabada y el agente solo espera a sus órdenes en segundo plano (el programa que se prueba): se
	// termina el agente con lo de su grupo (servidores MCP…) y pgocode se queda esas órdenes, que van en otros grupos.
	var fin *FinConFondo
	if errors.As(errLee, &fin) {
		proceso.SenalGrupo(cmd.Process.Pid, true)
		cmd.Wait()
		if ad := adopta(marca, cmd.Process.Pid, fin.Ordenes); len(ad) > 0 {
			emite(Evento{Adoptados: ad})
		}
		depuracion.Log(id, "fin de la respuesta en %s; órdenes en segundo plano adoptadas: %d de %d", time.Since(inicio).Round(time.Millisecond), len(fin.Ordenes), len(fin.Ordenes))
		return nil
	}
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
func permisosOpenCode(modo string, autorizadas []string) string {
	// La imputación la hace pgocode: sus skills de imputación, denegadas.
	sk := map[string]string{"*": "allow"}
	for _, n := range skills.Sustituidas {
		sk[n] = "deny"
	}
	// Fuera de la carpeta de la sesión, solo las carpetas autorizadas en ella (en todos los modos).
	fuera := map[string]string{"*": "deny"}
	for _, d := range autorizadas {
		fuera[strings.TrimRight(d, "/")+"/**"] = "allow"
	}
	p := map[string]any{"webfetch": "allow", "external_directory": fuera, "skill": sk}
	switch modo {
	case "auto":
		p["edit"], p["bash"], p["doom_loop"] = "allow", "allow", "allow"
	case "editar":
		p["edit"], p["bash"], p["doom_loop"] = "allow", "deny", "deny"
	default:
		p["edit"], p["bash"], p["doom_loop"] = "deny", "deny", "deny"
	}
	b, _ := json.Marshal(p)
	return string(b)
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
				// La imputación la hace pgocode: sus skills de imputación no se pueden usar.
				for _, sk := range skills.Sustituidas {
					a = append(a, "--disallowedTools", "Skill("+sk+")")
				}
				for _, d := range p.Autorizadas {
					a = append(a, "--add-dir", d)
				}
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
				for _, d := range p.Autorizadas {
					a = append(a, "--add-dir", d)
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
				// Los adjuntos van además como ficheros del mensaje (imágenes incluidas).
				for _, f := range p.Adjuntos {
					a = append(a, "--file", f)
				}
				return append(a, "--", msg)
			}},
	}
}

// adopta: empareja cada orden en segundo plano con su grupo de procesos (los que siguen vivos con la marca del
// agente) por su línea de órdenes; si solo hay una de cada, van juntas.
func adopta(marca string, agente int, ordenes []Fondo) []Adoptado {
	grupos := proceso.GruposConMarca(marca)
	delete(grupos, agente)
	var l []Adoptado
	usados := map[int]bool{}
	for i, o := range ordenes {
		g := 0
		for pg, pids := range grupos {
			if usados[pg] {
				continue
			}
			for _, pid := range pids {
				if o.Orden != "" && strings.Contains(proceso.Orden(pid), primeraLinea(o.Orden)) {
					g = pg
				}
			}
		}
		if g == 0 && len(ordenes) == 1 && len(grupos) == 1 {
			for pg := range grupos {
				g = pg
			}
		}
		if g == 0 {
			continue
		}
		usados[g] = true
		nombre := "agente-" + nombreDe(o.Titulo, o.Orden)
		if i > 0 {
			nombre += fmt.Sprintf("-%d", i+1)
		}
		l = append(l, Adoptado{Nombre: nombre, Orden: o.Orden, Fichero: o.Fichero, Grupo: g, Tarea: o.ID})
	}
	return l
}

var noNombre = regexp.MustCompile(`[^a-z0-9]+`)

// nombreDe: un nombre corto de programa a partir de su descripción u orden.
func nombreDe(titulo, orden string) string {
	t := titulo
	if t == "" {
		t = orden
	}
	n := strings.Trim(noNombre.ReplaceAllString(strings.ToLower(t), "-"), "-")
	if len(n) > 40 {
		n = strings.Trim(n[:40], "-")
	}
	if n == "" {
		n = "programa"
	}
	return n
}
