package llm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
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
	// LeeNueva: el que se usa en vez de Lee cuando la sesión empieza (sin sesión del agente que retomar).
	LeeNueva func(r io.Reader, emite func(Evento)) error
	// NoDisponible: el agente existe pero no se puede usar desde aquí (y por qué).
	NoDisponible string
	// Entrada: si el agente lee los mensajes por su entrada (uno por línea) también a mitad de turno, cómo se
	// codifica cada uno. Entonces el primer mensaje va por la entrada y no como argumento.
	Entrada func(texto string) []byte
}

// ErrEntradaCerrada: el turno ya no admite mensajes (el agente ha acabado su respuesta).
var ErrEntradaCerrada = errors.New("el agente ya ha acabado su respuesta")

// porEntrada: el mensaje va por la entrada del agente (y se le pueden mandar más durante el turno).
func (a *Agente) porEntrada(p Peticion) bool {
	return a.Entrada != nil && p.AlInyectar != nil && !p.Aparte
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
		NecesitaDir: true, Esfuerzos: a.Esfuerzos, Modos: a.Modos, Inyecta: a.Entrada != nil}
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
	// «Continuar desde aquí»: el agente sigue desde un mensaje anterior. Si no puede cortar su sesión, o el corte
	// falla, sigue en una sesión nueva con el traspaso de pgocode.
	if !p.Aparte && (p.Corte != "" || p.Traspaso != "") {
		if p.Corte != "" && !puntoValido.MatchString(p.Corte) {
			return errors.New("punto de la conversación no válido")
		}
		if p.SesionAgente != "" && !sesionValida.MatchString(p.SesionAgente) {
			return errors.New("identificador de sesión del agente no válido")
		}
		return a.chatDesde(ctx, p, emite)
	}
	return a.chatCompleto(ctx, p, emite)
}

// puntoValido: los ids de los mensajes de los agentes (UUID en Claude, «msg_…» en OpenCode).
var puntoValido = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// cortaSesion: si el agente puede seguir su propia sesión desde un mensaje anterior (sin traspaso).
func (a *Agente) cortaSesion() bool { return a.Clave == "claude-code" || a.Clave == "opencode" }

// chatDesde: el turno de «Continuar desde aquí». Primero, con la sesión del agente cortada en el punto (Claude:
// una copia con --resume-session-at; OpenCode: se quitan los mensajes posteriores); si no se puede, con el traspaso.
func (a *Agente) chatDesde(ctx context.Context, p Peticion, emite func(Evento)) error {
	if a.cortaSesion() && p.Corte != "" && p.SesionAgente != "" {
		var err error
		if a.Clave == "opencode" {
			err = cortaOpenCode(ctx, a, p, p.SesionAgente, p.Corte)
		}
		if err == nil {
			// Si el agente falla sin haber respondido nada (p. ej. no encuentra el mensaje), se pasa al traspaso.
			respondio := false
			err = a.chatCompleto(ctx, p, func(e Evento) {
				if e.Texto != "" || e.Herramienta != nil || e.Pensamiento != "" {
					respondio = true
				}
				emite(e)
			})
			if err == nil || respondio || ctx.Err() != nil || p.Traspaso == "" {
				return err
			}
		}
		depuracion.Log(a.Clave, "no se puede seguir la sesión %s desde %s (%v): sesión nueva con el traspaso", p.SesionAgente, p.Corte, err)
	}
	q := conTraspaso(p)
	if q.Traspaso == "" {
		return errors.New("no se puede continuar desde ese mensaje: falta el resumen de la conversación")
	}
	emite(Evento{Texto: "_(Sigo desde el mensaje elegido en una conversación nueva del agente, con un resumen de lo anterior.)_\n\n"})
	return a.chatCompleto(ctx, q, emite)
}

// conTraspaso: la petición para una sesión nueva del agente con el traspaso delante del mensaje.
func conTraspaso(p Peticion) Peticion {
	q := p
	q.SesionAgente, q.Corte = "", ""
	if p.Traspaso != "" && len(p.Mensajes) > 0 {
		q.Mensajes = append([]Mensaje(nil), p.Mensajes...)
		u := &q.Mensajes[len(q.Mensajes)-1]
		u.Texto = p.Traspaso + "\n\n" + u.Texto
	}
	return q
}

func (a *Agente) chatCompleto(ctx context.Context, p Peticion, emite func(Evento)) error {
	// OpenCode: una conversación de otra carpeta no se retoma (trabajaría allí y lo de fuera le parecería dentro).
	if a.Clave == "opencode" && p.SesionAgente != "" && p.Carpeta != "" && !sesionOpenCodeEn(ctx, a, p) {
		depuracion.Log(a.Clave, "la sesión %s no es de %s: se empieza otra", p.SesionAgente, p.Carpeta)
		emite(Evento{Texto: "_(La conversación anterior de OpenCode era de otra carpeta; empiezo una nueva en la de esta sesión.)_\n\n"})
		p.SesionAgente = ""
	}
	// Antigravity da el consumo acumulado de la conversación (tokens y segundos): se emite solo lo de este turno.
	if a.Clave == "antigravity" {
		conv := p.SesionAgente
		// Su «duración» es el tiempo de reloj desde que empezó la conversación (también lo que estuvo parada): la
		// del turno la mide pgocode.
		inicioTurno := time.Now()
		previo := emite
		emite = func(e Evento) {
			if e.SesionAgente != "" {
				conv = e.SesionAgente
			}
			if e.Uso != nil && conv != "" {
				u := usoDelTurno(conv, *e.Uso)
				u.Segundos = time.Since(inicioTurno).Seconds()
				e.Uso = &u
			}
			previo(e)
		}
	}
	// OpenCode: poda del historial al empezar el turno (prune.go), con lo que su recolector ya ha marcado.
	if a.Clave == "opencode" {
		if rep := pruneOpenCode(ctx, a, p); rep != nil {
			emite(Evento{Prune: rep})
		}
	}
	// Claude Code: la misma poda, en su historial (prune_claude.go). Si el --resume falla tras podar, se restaura
	// la copia y se repite sin podar, antes de dar la sesión por perdida.
	if a.Clave == "claude-code" {
		return a.chatClaude(ctx, p, emite)
	}
	// OpenCode: se apunta su sesión (también la nueva) para repararla si el turno se corta.
	sesionOC := p.SesionAgente
	emiteOC := emite
	if a.Clave == "opencode" {
		pasos := 0
		emiteOC = func(e Evento) {
			if e.SesionAgente != "" {
				sesionOC = e.SesionAgente
			}
			// Marcado continuo: cada salida grande de una herramienta se embebe en cuanto llega.
			if h := e.Herramienta; h != nil && h.Estado == "ok" && p.Prune == PruneAuto && !p.Aparte && idSesionOC.MatchString(sesionOC) {
				collectorFor(sesionOC).notePiece(h.ID, h.Nombre, h.Resumen, h.Salida)
			}
			if e.Uso != nil {
				pasos++
				midTurnPrune(a, p, sesionOC, pasos)
			}
			emite(e)
		}
		defer func() { markOpenCode(a, p, sesionOC) }()
	}
	err := a.chat(ctx, p, emiteOC)
	// Pregunta aparte en OpenCode: su copia de la sesión (o la nueva) se borra; nada de reparar ni reintentar.
	if a.Clave == "opencode" && p.Aparte {
		if sesionOC != "" && sesionOC != p.SesionAgente {
			c, cancela := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancela()
			if e := borraOpenCode(c, a, p, sesionOC); e != nil {
				depuracion.Log(a.Clave, "borrar la copia %s de la pregunta aparte: %v", sesionOC, e)
			}
		}
		return err
	}
	if a.Clave == "opencode" && sesionOC != "" {
		switch {
		case ctx.Err() != nil:
			// Turno cortado: OpenCode deja una respuesta a medias (vacía) que Gemini rechaza en el siguiente.
			go func() {
				c, cancela := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancela()
				if e := reparaOpenCode(c, a, p, sesionOC); e != nil {
					depuracion.Log(a.Clave, "reparar la sesión %s tras cortar el turno: %v", sesionOC, e)
				}
			}()
		case errors.As(err, new(*errLimitePasos)):
			// Límite de pasos: el trabajo se conserva, pero el turno se quedaría sin respuesta. Se le pide (sin
			// herramientas, una sola llamada) que resuma lo hecho y lo que falta; con «sigue» continúa.
			depuracion.Log(a.Clave, "la sesión %s llegó al límite de %d pasos: se pide el resumen", sesionOC, PasosOpenCode)
			reparaOpenCode(ctx, a, p, sesionOC) // por si quedó una respuesta vacía
			emiteOC(Evento{Texto: fmt.Sprintf("\n\n_(OpenCode ha llegado al límite de %d pasos de esta petición: le pido que resuma.)_\n\n", PasosOpenCode)})
			q := p
			q.SesionAgente, q.Permisos, q.Adjuntos = sesionOC, "ninguna", nil
			q.Mensajes = []Mensaje{{Rol: "user", Texto: "Has llegado al límite de pasos de esta petición. Sin usar herramientas, resume en pocas líneas " +
				"qué has hecho y qué queda por hacer. Para seguir, te escribiré «sigue»."}}
			if e := a.chat(ctx, q, emiteOC); e != nil {
				return err
			}
			return nil
		case err != nil && historialRoto(err):
			depuracion.Log(a.Clave, "la sesión %s tiene una respuesta vacía: se repara y se repite", sesionOC)
			if reparaOpenCode(ctx, a, p, sesionOC) == nil {
				p.SesionAgente = sesionOC
				if err = a.chat(ctx, p, emiteOC); err == nil || !historialRoto(err) {
					return err
				}
			}
			emite(Evento{Texto: "_(La conversación anterior de OpenCode estaba dañada; empiezo una nueva.)_\n\n"})
			p.SesionAgente = ""
			return a.chat(ctx, p, emite)
		}
	}
	// La sesión propia del agente ya no existe (p. ej. OpenCode con otra carpeta de datos): se empieza otra.
	if err != nil && p.SesionAgente != "" && ctx.Err() == nil && sesionPerdida(err) {
		depuracion.Log(a.Clave, "la sesión %s ya no existe: se empieza otra", p.SesionAgente)
		p.SesionAgente = ""
		return a.chat(ctx, p, emite)
	}
	return err
}

// chatClaude: el turno de Claude Code con la poda de su historial (barrido al empezar, marcado continuo de cada
// salida grande y marcado al acabar).
func (a *Agente) chatClaude(ctx context.Context, p Peticion, emite func(Evento)) error {
	rep, bak := pruneClaude(ctx, p)
	if rep != nil && (len(rep.Exchanges) > 0 || rep.Removed > 0 || rep.Podable) {
		emite(Evento{Prune: rep})
	}
	ses := p.SesionAgente
	respondio := false
	emiteC := func(e Evento) {
		if e.SesionAgente != "" {
			ses = e.SesionAgente
		}
		if e.Texto != "" || e.Herramienta != nil || e.Pensamiento != "" {
			respondio = true
		}
		if h := e.Herramienta; h != nil && h.Estado == "ok" && h.Padre == "" && p.Prune == PruneAuto && !p.Aparte && ses != "" {
			collectorFor(ses).notePiece(h.ID, claudeToolName(h.Nombre), h.Resumen, h.Salida)
		}
		emite(e)
	}
	defer func() { markClaude(p, ses) }()
	err := a.chat(ctx, p, emiteC)
	if err != nil && bak != nil && !respondio && ctx.Err() == nil {
		depuracion.Log(a.Clave, "la sesión %s no se retoma tras podar (%v): se restaura la copia y se repite", p.SesionAgente, err)
		if e := bak.restore(); e != nil {
			depuracion.Log(a.Clave, "restaurar la copia de %s: %v", p.SesionAgente, e)
		} else {
			err = a.chat(ctx, p, emiteC)
		}
	}
	if err != nil && p.SesionAgente != "" && ctx.Err() == nil && !respondio && sesionPerdida(err) {
		depuracion.Log(a.Clave, "la sesión %s ya no existe: se empieza otra", p.SesionAgente)
		p.SesionAgente = ""
		return a.chat(ctx, p, emiteC)
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
	msg := withAttachments(p.Mensajes[len(p.Mensajes)-1].Texto, p.Adjuntos)
	// Idioma: Claude Code lo recibe como instrucción de sistema (--append-system-prompt); los demás no tienen
	// cómo, así que va al principio del primer mensaje de la conversación (queda en su historial, sin repetirlo).
	if p.Idioma != "" && a.Clave != "claude-code" && p.SesionAgente == "" {
		msg = InstruccionIdioma(p.Idioma) + "\n\n" + msg
	}
	args := a.Args(msg, p)
	if a.porEntrada(p) {
		args = append(args, "--input-format", "stream-json")
	}
	cmd := exec.CommandContext(ctx, a.Programa, args...)
	inicio := time.Now()
	id := fmt.Sprintf("%s-%d", a.Clave, inicio.UnixNano()%1e6)
	cmd.Dir = p.Carpeta
	entorno := append([]string(nil), p.Entorno...)
	// PWD con la carpeta de la sesión: algunos agentes (OpenCode) toman su carpeta de PWD, no de la del proceso, y
	// heredarían la de pgocode (la personal), con lo que todo el equipo les parecería «dentro» del proyecto.
	if p.Carpeta != "" {
		entorno = append(entorno, "PWD="+p.Carpeta)
	}
	var propio []string
	if a.EntornoPara != nil {
		propio = a.EntornoPara(p)
		entorno = append(entorno, propio...)
	}
	for _, kv := range a.EntornoDefecto {
		if k, _, _ := strings.Cut(kv, "="); os.Getenv(k) == "" {
			entorno = append(entorno, kv)
		}
	}
	depuracion.Log(id, "lanza %s %q en %s [%s] (entorno añadido: %s; del agente: %s)", a.Programa, args, p.Carpeta, p.Traza,
		clavesEntorno(p.Entorno)+" "+strings.Join(a.EntornoDefecto, ","), clavesEntorno(propio))
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
	var entrada io.WriteCloser
	if a.porEntrada(p) {
		if entrada, err = cmd.StdinPipe(); err != nil {
			return err
		}
	}
	if err := cmd.Start(); err != nil {
		depuracion.Log(id, "no arranca: %v", err)
		return err
	}
	// Mensajes por la entrada: el primero ya, los demás cuando lleguen (a mitad de turno). La entrada se cierra al
	// acabar la respuesta (el primer consumo): si no, el agente esperaría otro mensaje para siempre; sus tareas en
	// segundo plano siguen igual (probado con Claude Code).
	if entrada != nil {
		var muE sync.Mutex
		abierta := true
		envia := func(t string) error {
			muE.Lock()
			defer muE.Unlock()
			if !abierta {
				return ErrEntradaCerrada
			}
			_, err := entrada.Write(a.Entrada(t))
			return err
		}
		cierra := func() {
			muE.Lock()
			fue := abierta
			if abierta {
				abierta = false
				entrada.Close()
			}
			muE.Unlock()
			if fue {
				p.AlInyectar(nil)
			}
		}
		defer cierra()
		if err := envia(msg); err != nil {
			return err
		}
		p.AlInyectar(func(t string) error {
			depuracion.Log(id, "mensaje a mitad de turno (%d caracteres)", len(t))
			return envia(t)
		})
		// Con subagentes en segundo plano la entrada sigue abierta tras la respuesta: lo que se mande entonces le
		// llega ya al agente principal (que puede pasárselo a ellos), y no al acabar todo. Se cierra con la
		// primera respuesta que acabe sin ninguno vivo (al terminar el último, el agente da otra).
		vivos := map[string]bool{}
		previo := emite
		emite = func(e Evento) {
			if t := e.Tarea; t != nil && t.Tipo != "lista" {
				if t.Estado == "curso" {
					if t.Tipo == "subagente" {
						vivos[t.ID] = true
					}
				} else {
					delete(vivos, t.ID)
				}
			}
			if e.Uso != nil && len(vivos) == 0 {
				cierra()
			}
			previo(e)
		}
	}
	proceso.Vigila(cmd.Process.Pid)
	lee := a.Lee
	if p.SesionAgente == "" && a.LeeNueva != nil {
		lee = a.LeeNueva
	}
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
	vivos := len(proceso.GruposConMarca(marca))
	emite(Evento{Vivos: &vivos})
	depuracion.Log(id, "fin en %s: wait=%v lectura=%v respuesta=%v vivos=%d", time.Since(inicio).Round(time.Millisecond), errWait, errLee, respuesta, vivos)
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
	// Lecturas que llenan el contexto sin aportar (registros, dependencias): denegadas (probado con OpenCode 1.19:
	// los patrones van relativos a la carpeta; «*» abarca subcarpetas).
	leer := map[string]string{"*": "allow", "*.log": "deny", "*.log.*": "deny", "node_modules/*": "deny", "*/node_modules/*": "deny"}
	// doom_loop denegado en todos los modos: la misma llamada repetida una y otra vez se corta (gasta tokens).
	p := map[string]any{"webfetch": "allow", "external_directory": fuera, "skill": sk, "read": leer, "doom_loop": "deny"}
	switch modo {
	case "ninguna": // sin herramientas: solo responde (el cierre tras el límite de pasos)
		for _, h := range []string{"edit", "bash", "read", "glob", "grep", "list", "webfetch", "websearch", "task", "todowrite", "skill", "patch", "codesearch"} {
			p[h] = "deny"
		}
	case "auto":
		p["edit"], p["bash"] = "allow", "allow"
	case "editar":
		p["edit"], p["bash"] = "allow", "deny"
	default:
		p["edit"], p["bash"] = "deny", "deny"
	}
	b, _ := json.Marshal(p)
	return string(b)
}

// herramientasClaude: el perfil de herramientas propias de Claude Code según el modo. Fuera: las que pgocode hace
// por su cuenta o no se pueden usar sin nadie delante (worktrees, cron, avisos, monitor, preguntas al usuario,
// equipos, plugins, flujos, LSP, cuadernos). En solo lectura (y en las preguntas aparte), sin Edit ni Write.
// Las de los servidores MCP no cuentan aquí (van por --mcp-config).
func herramientasClaude(permisos string, aparte bool) []string {
	h := []string{"Task", "Bash", "Read", "Skill", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TaskStop", "ToolSearch", "WebFetch", "WebSearch"}
	if !aparte && (permisos == "editar" || permisos == "auto") {
		h = append(h, "Edit", "Write")
	}
	return h
}

// clavesEntorno: los nombres de las variables añadidas (sin sus valores: llevan tokens).
func clavesEntorno(e []string) string {
	l := make([]string, 0, len(e))
	for _, v := range e {
		k, _, _ := strings.Cut(v, "=")
		if strings.Contains(k, "COMPACT") { // los umbrales de compactación, con su valor (no son secretos)
			k = v
		}
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
		&Agente{Clave: "claude-code", Nombre: "Claude Code", Programa: "claude", Lee: leeClaudeCode, LeeNueva: leeClaudeCodeNew, Modos: true,
			// Con los modelos actuales Claude Code no trae la lista de tareas (TaskCreate/TaskUpdate) si no se pide:
			// pgocode la pinta en el chat y la usa para el estado de las preguntas aparte.
			EntornoDefecto: []string{"CLAUDE_CODE_ENABLE_TODO_TOOLS=1"},
			// Compacta su historial al pasar del umbral de la sesión (compaction.go).
			EntornoPara: claudeCompactEnv,
			Modelos:     []string{"opus", "sonnet", "haiku"}, Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"-p", "--output-format", "stream-json", "--verbose"}
				// La imputación la hace pgocode: sus skills de imputación no se pueden usar.
				for _, sk := range skills.Sustituidas {
					a = append(a, "--disallowedTools", "Skill("+sk+")")
				}
				for _, d := range p.Autorizadas {
					a = append(a, "--add-dir", d)
				}
				// Solo las herramientas propias que usa pgocode: las definiciones de todas van en cada llamada al modelo
				// (con las 32 de Claude Code 2.1, un «OK» cuesta 20,5k tokens de entrada; con este perfil, 15k).
				a = append(a, "--tools", strings.Join(herramientasClaude(p.Permisos, p.Aparte), ","))
				// Solo los servidores MCP que pide la sesión (y «pgo»): el resto ni arranca ni gasta tokens.
				if f := configMCP(p.Carpeta, p.MCP); f != "" {
					a = append(a, "--strict-mcp-config", "--mcp-config", f)
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
				if p.Idioma != "" {
					a = append(a, "--append-system-prompt", InstruccionIdioma(p.Idioma))
				}
				if p.SesionAgente != "" {
					a = append(a, "--resume", p.SesionAgente)
					// Continuar desde un mensaje anterior: una copia de la sesión hasta ese mensaje (la original no se
					// toca); lo posterior no se reenvía (probado con Claude Code 2.1.308).
					if p.Corte != "" && !p.Aparte {
						a = append(a, "--resume-session-at", p.Corte, "--fork-session")
					}
				}
				// Pregunta aparte: sobre una copia de la conversación que no se guarda (la original no se entera).
				if p.Aparte {
					if p.SesionAgente != "" {
						a = append(a, "--fork-session")
					}
					a = append(a, "--no-session-persistence")
				}
				if p.AlInyectar != nil && !p.Aparte {
					return a // el mensaje va por la entrada (stream-json): se le pueden mandar más a mitad de turno
				}
				// «--» antes del mensaje: un texto que empiece por «-» no se toma como opción.
				return append(a, "--", msg)
			},
			Entrada: func(t string) []byte {
				b, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": t}})
				return append(b, '\n')
			}},
		// Antigravity: su CLI «agy» en modo print con eventos stream-json; --conversation continúa la sesión.
		&Agente{Clave: "antigravity", Nombre: "Antigravity", Programa: "agy", Lee: leeAntigravity, Modos: true,
			Modelos: []string{"gemini-4-argon-high", "gemini-4-argon-medium", "gemini-4-argon-low", "gemini-3.8-flash-high",
				"gemini-3.8-flash-medium", "gemini-3.8-flash-low", "gemini-3.1-pro-high", "gemini-3.1-pro-low"},
			Esfuerzos: []string{"low", "medium", "high", "xhigh", "max"},
			Args: func(msg string, p Peticion) []string {
				a := []string{"--output-format", "stream-json"}
				// Solo lectura: modo plan (no cambia ficheros); «editar» y «auto»: accept-edits. «--mode auto» no está
				// en todas las cuentas: sin él, agy vuelve al modo por defecto, que sin consola deniega las escrituras
				// (probado con agy 1.4.4); accept-edits edita y ejecuta órdenes.
				switch p.Permisos {
				case "editar", "auto":
					a = append(a, "--mode", "accept-edits")
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
				// El mensaje va pegado a la opción: así nunca se toma como otra opción. Con el contexto por encima del
				// umbral de la sesión, tras «/compact» (compaction.go).
				return append(a, "--print="+antigravityMessage(msg, p))
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
					if p.Aparte {
						a = append(a, "--fork") // pregunta aparte: una copia, que se borra al acabar
					}
				}
				// La carpeta de la sesión, explícita: OpenCode guarda con ella su «dentro» y su «fuera».
				if p.Carpeta != "" {
					a = append(a, "--dir", p.Carpeta)
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
		l = append(l, Adoptado{Nombre: nombre, Orden: o.Orden, Fichero: o.Fichero, Grupo: g, Tarea: o.ID, Marca: marca})
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

// Sesiones de OpenCode ya comprobadas (sesión → carpeta).
var (
	muSesionesOC sync.Mutex
	sesionesOC   = map[string]string{}
)

// sesionOpenCodeEn: si la sesión de OpenCode es de la carpeta (su listado de sesiones, que es por carpeta). Si no
// se puede comprobar, se da por buena.
func sesionOpenCodeEn(ctx context.Context, a *Agente, p Peticion) bool {
	muSesionesOC.Lock()
	ya := sesionesOC[p.SesionAgente] == p.Carpeta
	muSesionesOC.Unlock()
	if ya {
		return true
	}
	ctx, cancela := context.WithTimeout(ctx, 15*time.Second)
	defer cancela()
	cmd := exec.CommandContext(ctx, "opencode", "session", "list", "--format", "json", "-n", "1000")
	cmd.Dir = p.Carpeta
	cmd.Env = append(append(os.Environ(), a.EntornoDefecto...), "PWD="+p.Carpeta)
	if a.EntornoPara != nil {
		cmd.Env = append(cmd.Env, a.EntornoPara(p)...)
	}
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	var l []struct {
		ID  string `json:"id"`
		Dir string `json:"directory"`
	}
	if json.Unmarshal(out, &l) != nil {
		return true
	}
	for _, x := range l {
		if x.ID == p.SesionAgente && filepath.Clean(x.Dir) == filepath.Clean(p.Carpeta) {
			muSesionesOC.Lock()
			sesionesOC[p.SesionAgente] = p.Carpeta
			muSesionesOC.Unlock()
			return true
		}
	}
	return false
}

// historialRoto: el error del modelo cuando el historial lleva un mensaje vacío (una respuesta cortada).
func historialRoto(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "at least one parts field") || strings.Contains(s, "contents.parts must not be empty") ||
		strings.Contains(s, "must alternate between")
}

var idSesionOC = regexp.MustCompile(`^ses_[A-Za-z0-9]+$`)

// reparaOpenCode: quita de la sesión las respuestas sin contenido (un turno cortado) y la pregunta que quedó sin
// contestar, para que el historial vuelva a ser válido; la conversación conserva todo lo demás.
func reparaOpenCode(ctx context.Context, a *Agente, p Peticion, sesion string) error {
	if !idSesionOC.MatchString(sesion) {
		return errors.New("sesión no válida")
	}
	defer forgetSnapshot(sesion)
	rotas := `SELECT m.id FROM message m WHERE m.session_id='` + sesion + `' AND json_extract(m.data,'$.role')='assistant' AND NOT EXISTS (` +
		`SELECT 1 FROM part x WHERE x.message_id=m.id AND ((json_extract(x.data,'$.type') IN ('text','reasoning') AND ` +
		`length(coalesce(json_extract(x.data,'$.text'),''))>0) OR json_extract(x.data,'$.type')='tool'))`
	previas := `SELECT (SELECT u.id FROM message u WHERE u.session_id='` + sesion + `' AND json_extract(u.data,'$.role')='user' AND ` +
		`u.time_created<=r.time_created ORDER BY u.time_created DESC LIMIT 1) FROM message r WHERE r.id IN (` + rotas + `)`
	// Pasos vacíos dentro de una respuesta con contenido: a veces Gemini devuelve un paso sin nada (un razonamiento
	// en blanco) y OpenCode lo guarda; al reenviar el historial, Vertex lo rechaza («must include at least one parts
	// field») aunque el resto de la respuesta esté bien. Se quitan las partes de texto o razonamiento vacías y los
	// pasos que se quedan sin nada (un step-start sin contenido hasta el siguiente o hasta el final).
	en := `message_id IN (SELECT id FROM message WHERE session_id='` + sesion + `')`
	vacias := `DELETE FROM part WHERE ` + en + ` AND json_extract(data,'$.type') IN ('reasoning','text') AND ` +
		`length(coalesce(json_extract(data,'$.text'),''))=0`
	pasosVacios := `DELETE FROM part WHERE ` + en + ` AND json_extract(data,'$.type')='step-start' AND NOT EXISTS (` +
		`SELECT 1 FROM part q WHERE q.message_id=part.message_id AND q.id>part.id AND json_extract(q.data,'$.type')<>'step-start' AND ` +
		`q.id < coalesce((SELECT min(r.id) FROM part r WHERE r.message_id=part.message_id AND r.id>part.id AND ` +
		`json_extract(r.data,'$.type')='step-start'),'~'))`
	for _, q := range []string{
		vacias,
		pasosVacios,
		"DELETE FROM part WHERE message_id IN (" + previas + ") OR message_id IN (" + rotas + ")",
		"DELETE FROM message WHERE id IN (" + previas + ") OR id IN (" + rotas + ")",
	} {
		cmd := exec.CommandContext(ctx, "opencode", "db", q)
		cmd.Dir = p.Carpeta
		cmd.Env = append(append(os.Environ(), a.EntornoDefecto...), "PWD="+p.Carpeta)
		if a.EntornoPara != nil {
			cmd.Env = append(cmd.Env, a.EntornoPara(p)...)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// cortaOpenCode: «Continuar desde aquí» en OpenCode. Quita de la sesión los mensajes posteriores al del punto
// (con sus partes): el siguiente turno sigue desde él. OpenCode no permite copiar una sesión hasta un mensaje desde
// su CLI («run --fork» copia la sesión entera), así que se corta la propia sesión; pgocode conserva en su
// conversación lo de después (atenuado, fuera del contexto del agente).
func cortaOpenCode(ctx context.Context, a *Agente, p Peticion, sesion, punto string) error {
	if !idSesionOC.MatchString(sesion) || !idMensajeOC.MatchString(punto) {
		return errors.New("sesión o mensaje de OpenCode no válidos")
	}
	defer forgetSnapshot(sesion)
	for _, q := range consultasCorteOpenCode(sesion, punto) {
		cmd := exec.CommandContext(ctx, "opencode", "db", q)
		cmd.Dir = p.Carpeta
		cmd.Env = append(append(os.Environ(), a.EntornoDefecto...), "PWD="+p.Carpeta)
		if a.EntornoPara != nil {
			cmd.Env = append(cmd.Env, a.EntornoPara(p)...)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		// La primera consulta comprueba que el mensaje es de la sesión.
		if strings.HasPrefix(q, "SELECT") && !strings.Contains(string(out), punto) {
			return fmt.Errorf("el mensaje %s no está en la sesión %s", punto, sesion)
		}
	}
	return nil
}

var idMensajeOC = regexp.MustCompile(`^msg_[A-Za-z0-9]+$`)

// consultasCorteOpenCode: comprobar que el mensaje es de la sesión y quitar lo posterior (partes y mensajes; los
// ids de OpenCode crecen con el tiempo, así que «posterior» es un id mayor).
func consultasCorteOpenCode(sesion, punto string) []string {
	despues := `SELECT id FROM message WHERE session_id='` + sesion + `' AND id>'` + punto + `'`
	return []string{
		`SELECT id FROM message WHERE session_id='` + sesion + `' AND id='` + punto + `'`,
		`DELETE FROM part WHERE session_id='` + sesion + `' AND message_id IN (` + despues + `)`,
		`DELETE FROM message WHERE id IN (` + despues + `)`,
	}
}

// borraOpenCode borra una sesión de OpenCode (la copia de una pregunta aparte).
func borraOpenCode(ctx context.Context, a *Agente, p Peticion, sesion string) error {
	if !idSesionOC.MatchString(sesion) {
		return errors.New("sesión no válida")
	}
	cmd := exec.CommandContext(ctx, "opencode", "session", "delete", sesion)
	cmd.Dir = p.Carpeta
	cmd.Env = append(append(os.Environ(), a.EntornoDefecto...), "PWD="+p.Carpeta)
	if a.EntornoPara != nil {
		cmd.Env = append(cmd.Env, a.EntornoPara(p)...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Acumulados de consumo por conversación de Antigravity y por sesión de Claude Code (persisten entre ejecuciones
// de pgocode: los dos dan lo acumulado de toda la conversación, y lo del turno es la diferencia).
var (
	muAcumulados sync.Mutex
	acumulados   map[string]Uso
)

func ficheroAcumulados() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "pgocode", "antigravity-consumo.json")
}

// cargaAcumulados (con muAcumulados): el registro de acumulados, del fichero la primera vez.
func cargaAcumulados() {
	if acumulados == nil {
		acumulados = map[string]Uso{}
		if b, err := os.ReadFile(ficheroAcumulados()); err == nil {
			json.Unmarshal(b, &acumulados)
		}
	}
}

// guardaAcumulados (con muAcumulados).
func guardaAcumulados() {
	if f := ficheroAcumulados(); f != "" {
		os.MkdirAll(filepath.Dir(f), 0o700)
		if b, err := json.Marshal(acumulados); err == nil {
			os.WriteFile(f, b, 0o600)
		}
	}
}

// acumuladoClaude: el acumulado de una sesión de Claude Code la última vez que se vio (modelUsage y coste).
func acumuladoClaude(sesion string) (Uso, bool) {
	muAcumulados.Lock()
	defer muAcumulados.Unlock()
	cargaAcumulados()
	u, ok := acumulados["claude:"+sesion]
	return u, ok
}

func guardaAcumuladoClaude(sesion string, u Uso) {
	muAcumulados.Lock()
	defer muAcumulados.Unlock()
	cargaAcumulados()
	acumulados["claude:"+sesion] = u
	guardaAcumulados()
}

// usoDelTurno: lo de este turno = el acumulado que da el agente menos el anterior de la conversación. Si el
// acumulado baja (conversación nueva con el mismo id, o se perdió el registro), se toma entero.
func usoDelTurno(conv string, total Uso) Uso {
	muAcumulados.Lock()
	defer muAcumulados.Unlock()
	f := ficheroAcumulados()
	if acumulados == nil {
		acumulados = map[string]Uso{}
		if b, err := os.ReadFile(f); err == nil {
			json.Unmarshal(b, &acumulados)
		}
	}
	antes, ok := acumulados[conv]
	turno := total
	if ok && total.Entrada >= antes.Entrada && total.Salida >= antes.Salida && total.Segundos >= antes.Segundos {
		turno = Uso{Entrada: total.Entrada - antes.Entrada, Salida: total.Salida - antes.Salida,
			Coste: max(total.Coste-antes.Coste, 0), Segundos: total.Segundos - antes.Segundos,
			CacheLeida: max(total.CacheLeida-antes.CacheLeida, 0), Llamadas: total.Llamadas, Contexto: total.Contexto}
	}
	acumulados[conv] = total
	if f != "" {
		os.MkdirAll(filepath.Dir(f), 0o700)
		if b, err := json.Marshal(acumulados); err == nil {
			os.WriteFile(f, b, 0o600)
		}
	}
	return turno
}

// ServidoresMCP: los servidores MCP de Claude Code configurados (los del usuario y los de la carpeta), por nombre.
func ServidoresMCP(carpeta string) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	casa, err := os.UserHomeDir()
	if err != nil {
		return m
	}
	var cfg struct {
		MCP       map[string]json.RawMessage `json:"mcpServers"`
		Proyectos map[string]struct {
			MCP map[string]json.RawMessage `json:"mcpServers"`
		} `json:"projects"`
	}
	if b, err := os.ReadFile(filepath.Join(casa, ".claude.json")); err == nil && json.Unmarshal(b, &cfg) == nil {
		for k, v := range cfg.MCP {
			m[k] = v
		}
		for k, v := range cfg.Proyectos[carpeta].MCP {
			m[k] = v
		}
	}
	var proyecto struct {
		MCP map[string]json.RawMessage `json:"mcpServers"`
	}
	if b, err := os.ReadFile(filepath.Join(carpeta, ".mcp.json")); err == nil && json.Unmarshal(b, &proyecto) == nil {
		for k, v := range proyecto.MCP {
			m[k] = v
		}
	}
	return m
}

// configMCP: un fichero de configuración MCP con «pgo» y los servidores pedidos (para --strict-mcp-config).
func configMCP(carpeta string, pedidos []string) string {
	todos := ServidoresMCP(carpeta)
	sel := map[string]json.RawMessage{}
	for _, n := range append([]string{"pgo"}, pedidos...) {
		if v, ok := todos[n]; ok {
			sel[n] = v
		}
	}
	b, err := json.Marshal(map[string]any{"mcpServers": sel})
	if err != nil {
		return ""
	}
	// En la caché del usuario (0700), no en /tmp: lleva el entorno de los servidores (puede haber tokens) y otro
	// usuario del equipo podría dejar ahí una configuración con su propia orden.
	unaVezMCP.Do(func() { os.RemoveAll(filepath.Join(os.TempDir(), "pgocode-mcp")) }) // la de versiones anteriores
	cache, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	d := filepath.Join(cache, "pgocode", "mcp")
	if os.MkdirAll(d, 0o700) != nil {
		return ""
	}
	if st, err := os.Lstat(d); err != nil || !st.IsDir() || st.Mode().Perm()&0o077 != 0 {
		return ""
	}
	f := filepath.Join(d, fmt.Sprintf("%x.json", sha256.Sum256(b)))
	if _, err := os.Stat(f); err != nil {
		tmp := f + ".tmp"
		if os.WriteFile(tmp, b, 0o600) != nil || os.Rename(tmp, f) != nil {
			return ""
		}
	}
	return f
}

var unaVezMCP sync.Once

// InstruccionIdioma: lo que se le dice al agente para que responda en el idioma del usuario.
func InstruccionIdioma(idioma string) string {
	// Probado con Claude: «el idioma del usuario» lo entiende como el del mensaje; así obedece al configurado.
	return "Idioma de las respuestas: " + idioma + ". Escribe siempre en " + idioma + " todo lo que respondas (explicaciones, " +
		"resúmenes, preguntas y planes), aunque el mensaje, el código, los ficheros o las herramientas estén en otro idioma, " +
		"salvo que el usuario te pida expresamente otro idioma."
}

// idiomas: el nombre de cada idioma por su código (BCP 47, el del navegador).
var idiomas = map[string]string{"es": "español", "en": "English", "ca": "català", "gl": "galego", "eu": "euskara",
	"fr": "français", "pt": "português", "it": "italiano", "de": "Deutsch", "nl": "Nederlands"}

var codigoIdioma = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// NombreIdioma: el nombre del idioma de un código del navegador («es-ES» → «español»); vacío o raro: español.
func NombreIdioma(codigo string) string {
	if !codigoIdioma.MatchString(codigo) {
		return "español"
	}
	base, _, _ := strings.Cut(strings.ToLower(codigo), "-")
	if n, ok := idiomas[base]; ok {
		return n
	}
	return codigo
}

// withAttachments: el mensaje con la lista de ficheros adjuntos detrás. Sin texto (solo adjuntos), un texto mínimo
// para que el agente sepa que son para él.
func withAttachments(msg string, files []string) string {
	if len(files) == 0 {
		return msg
	}
	if strings.TrimSpace(msg) == "" {
		msg = "Te paso estos ficheros."
	}
	msg += "\n\nFicheros adjuntos (léelos con tus herramientas):"
	for _, f := range files {
		msg += "\n- " + f
	}
	return msg
}
