package llm

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

// A1: el id de sesión llega del navegador y va como argumento: no puede ser una opción del programa.
func TestSesionAgenteValidada(t *testing.T) {
	a := &Agente{Clave: "x", Programa: "true", Args: func(string, Peticion) []string { return nil }}
	for _, mala := range []string{"--dangerously-skip-permissions", "-p", "a b", "../x", strings.Repeat("a", 200)} {
		err := a.Chat(context.Background(), Peticion{Carpeta: t.TempDir(), SesionAgente: mala, Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}, func(Evento) {})
		if err == nil || !strings.Contains(err.Error(), "sesión") {
			t.Errorf("sesión %q aceptada (err=%v)", mala, err)
		}
	}
	if !sesionValida.MatchString("0f8c6c2e-5d0b-4b5c-9a6e-1d2f3a4b5c6d") {
		t.Error("un UUID debe ser válido")
	}
}

// A1: el mensaje va después de «--», así que un texto que empiece por «-» no es una opción.
func TestMensajeNoEsOpcion(t *testing.T) {
	for _, p := range Agentes() {
		a := p.(*Agente)
		if a.NoDisponible != "" {
			continue
		}
		args := a.Args("- arregla el test", Peticion{})
		ultimo := args[len(args)-1]
		if ultimo == "- arregla el test" && (len(args) < 2 || args[len(args)-2] != "--") {
			t.Errorf("%s: el mensaje va sin «--» delante: %q", a.Clave, args)
		}
	}
}

// M3: leeTexto no parte caracteres UTF-8 aunque lleguen de byte en byte.
func TestLeeTextoUTF8(t *testing.T) {
	texto := "Añadir ñandú y tilde: canción 🚀 fin"
	var b strings.Builder
	if err := leeTexto(iotest.OneByteReader(strings.NewReader(texto)), func(e Evento) { b.WriteString(e.Texto) }); err != nil {
		t.Fatal(err)
	}
	if b.String() != texto {
		t.Fatalf("salida %q", b.String())
	}
}

// Solo hay agentes: Claude Code el primero, Antigravity y OpenCode.
func TestSoloAgentes(t *testing.T) {
	l := NuevoRegistro(Agentes()...).Lista(context.Background())
	for _, i := range l {
		if i.Tipo != "agente" {
			t.Errorf("%s no es un agente", i.ID)
		}
	}
	for _, id := range []string{"claude-code", "antigravity", "opencode"} {
		if _, ok := NuevoRegistro(Agentes()...).Busca(id); !ok {
			t.Error("falta " + id)
		}
	}
	if _, ok := NuevoRegistro(Agentes()...).Busca("gemini-cli"); ok {
		t.Error("Gemini CLI ya no es un agente")
	}
}

// Los permisos de la sesión llegan a Claude Code como --permission-mode.
func TestPermisosClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	for permisos, quiero := range map[string]string{"": "", "lectura": "", "editar": "acceptEdits", "auto": "auto"} {
		args := strings.Join(a.Args("hola", Peticion{Permisos: permisos}), " ")
		if quiero == "" && strings.Contains(args, "--permission-mode") || quiero != "" && !strings.Contains(args, "--permission-mode "+quiero) {
			t.Errorf("permisos %q: %s", permisos, args)
		}
	}
}

// El modelo y el esfuerzo llegan a Claude Code como --model y --effort («por defecto»: sin opción).
func TestModeloEsfuerzoClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	if args := strings.Join(a.Args("hola", Peticion{Modelo: "por defecto"}), " "); strings.Contains(args, "--model") || strings.Contains(args, "--effort") {
		t.Errorf("sin modelo ni esfuerzo: %s", args)
	}
	if args := strings.Join(a.Args("hola", Peticion{Modelo: "opus", Esfuerzo: "high"}), " "); !strings.Contains(args, "--model opus") || !strings.Contains(args, "--effort high") {
		t.Errorf("con modelo y esfuerzo: %s", args)
	}
}

// Perfil de herramientas de Claude Code (--tools): sin Edit ni Write en solo lectura y en las preguntas aparte;
// nunca las que pgocode no usa.
func TestHerramientasClaudeCode(t *testing.T) {
	a := Agentes()[0].(*Agente)
	herr := func(p Peticion) []string {
		args := a.Args("hola", p)
		i := slices.Index(args, "--tools")
		if i < 0 {
			t.Fatalf("sin --tools: %q", args)
		}
		return strings.Split(args[i+1], ",")
	}
	for _, c := range []struct {
		p     Peticion
		edita bool
	}{{Peticion{}, false}, {Peticion{Permisos: "lectura"}, false}, {Peticion{Permisos: "editar"}, true},
		{Peticion{Permisos: "auto"}, true}, {Peticion{Permisos: "auto", Aparte: true}, false}} {
		h := herr(c.p)
		if slices.Contains(h, "Edit") != c.edita || slices.Contains(h, "Write") != c.edita {
			t.Errorf("%+v: %v", c.p, h)
		}
		for _, x := range []string{"Bash", "Read", "Task", "Skill", "ToolSearch"} {
			if !slices.Contains(h, x) {
				t.Errorf("%+v: falta %s", c.p, x)
			}
		}
		for _, x := range []string{"CronCreate", "EnterWorktree", "PushNotification", "Workflow", "NotebookEdit"} {
			if slices.Contains(h, x) {
				t.Errorf("%+v: sobra %s", c.p, x)
			}
		}
	}
}

// Antigravity: «auto» va como accept-edits (--mode auto no está en todas las cuentas y sin él no edita).
func TestModoAutoAntigravity(t *testing.T) {
	a := Agentes()[1].(*Agente)
	if j := strings.Join(a.Args("hola", Peticion{Permisos: "auto"}), " "); !strings.Contains(j, "--mode accept-edits") {
		t.Errorf("auto: %s", j)
	}
}

// Antigravity: el mensaje va pegado a --print=, solo lectura es el modo plan y la conversación se continúa.
func TestArgsAntigravity(t *testing.T) {
	a := Agentes()[1].(*Agente)
	args := a.Args("--hola", Peticion{SesionAgente: "abc", Modelo: "gemini-4-argon-high"})
	j := strings.Join(args, " ")
	if args[len(args)-1] != "--print=--hola" || !strings.Contains(j, "--mode plan") || !strings.Contains(j, "--conversation abc") || !strings.Contains(j, "--model gemini-4-argon-high") {
		t.Errorf("args: %q", args)
	}
}

func TestLeeAntigravity(t *testing.T) {
	in := `{"event":"init","conversation_id":"c1"}
{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"tool","tool_name":"read_file"}}
{"event":"step_update","step_update":{"state":"ACTIVE","step_type":"agent_response","text_delta":"ho"}}
{"event":"step_update","step_update":{"state":"DONE","step_type":"agent_response","text_delta":"la"}}
{"event":"result","result":{"status":"SUCCESS"}}`
	var b strings.Builder
	sesion, herr := "", ""
	if err := leeAntigravity(strings.NewReader(in), func(e Evento) {
		b.WriteString(e.Texto)
		if e.SesionAgente != "" {
			sesion = e.SesionAgente
		}
		if e.Herramienta != nil {
			herr = e.Herramienta.Nombre
		}
	}); err != nil {
		t.Fatal(err)
	}
	if sesion != "c1" || herr != "read_file" || b.String() != "hola" {
		t.Errorf("sesión %q, herramienta %q, texto %q", sesion, herr, b.String())
	}
}

// Claude Code: la herramienta llega al empezar (con su resumen) y al acabar (con su salida); y el consumo.
func TestLeeClaudeCode(t *testing.T) {
	in := `{"type":"system","subtype":"init","session_id":"s1"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls -la\npwd"}}]}}
{"type":"assistant","parent_tool_use_id":"t9","message":{"content":[{"type":"text","text":"del subagente"}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a.txt"}]}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Listo."}]}}
{"type":"result","total_cost_usd":0.5,"duration_ms":2000,"usage":{"input_tokens":3,"cache_read_input_tokens":7,"output_tokens":4}}`
	var evs []Evento
	if err := leeClaudeCode(strings.NewReader(in), func(e Evento) { evs = append(evs, e) }); err != nil {
		t.Fatal(err)
	}
	if len(evs) != 5 || evs[0].SesionAgente != "s1" || evs[1].Herramienta.Resumen != "ls -la" || evs[1].Herramienta.Estado != "curso" ||
		evs[2].Herramienta.Salida != "a.txt" || evs[2].Herramienta.Nombre != "Bash" || evs[3].Texto != "Listo." || evs[4].Uso.Entrada != 10 || evs[4].Uso.Coste != 0.5 {
		t.Errorf("eventos: %+v", evs)
	}
}

// OpenCode --format json: texto, herramienta terminada, consumo y sesión; un evento error es un fallo.
func TestLeeOpenCode(t *testing.T) {
	in := `{"type":"step_start","sessionID":"ses_1","part":{"type":"step-start"}}
{"type":"tool_use","sessionID":"ses_1","part":{"callID":"c1","tool":"read","state":{"status":"completed","input":{"filePath":"/x/a.txt"},"output":"x","title":"a.txt"}}}
{"type":"text","sessionID":"ses_1","part":{"type":"text","text":"hola"}}
{"type":"step_finish","sessionID":"ses_1","part":{"cost":0.03,"tokens":{"input":10,"output":2,"reasoning":1,"cache":{"read":5,"write":0}}}}`
	var evs []Evento
	if err := leeOpenCode(strings.NewReader(in), func(e Evento) { evs = append(evs, e) }); err != nil {
		t.Fatal(err)
	}
	if len(evs) != 4 || evs[0].SesionAgente != "ses_1" || evs[1].Herramienta.Nombre != "read" || evs[1].Herramienta.Resumen != "/x/a.txt" ||
		evs[2].Texto != "hola" || evs[3].Uso.Entrada != 15 || evs[3].Uso.Salida != 3 {
		t.Errorf("eventos: %+v", evs)
	}
	err := leeOpenCode(strings.NewReader(`{"type":"error","sessionID":"s","error":{"name":"APIError","data":{"message":"API key not valid"}}}`), func(Evento) {})
	if err == nil || !strings.Contains(err.Error(), "API key not valid") {
		t.Errorf("error: %v", err)
	}
}

// Si OpenCode pregunta por un permiso, la lectura acaba con un error claro (y Chat lo para).
func TestOpenCodePermiso(t *testing.T) {
	in := "{\"type\":\"step_start\",\"sessionID\":\"s\"}\n\x1b[?25l│\n◆  Permission required: external_directory (/tmp/*)\n│  ● Allow once\n"
	err := leeOpenCode(strings.NewReader(in), func(Evento) {})
	if err == nil || !strings.Contains(err.Error(), "external_directory (/tmp/*)") {
		t.Errorf("error: %v", err)
	}
	a := Agentes()[2].(*Agente)
	e := a.EntornoPara(Peticion{Permisos: "auto", Modelo: "google-vertex/gemini-4-argon"})
	if len(e) != 3 || !strings.Contains(e[0], `"bash":"allow"`) || !strings.HasPrefix(e[1], "XDG_DATA_HOME=") {
		t.Errorf("auto: %v", e)
	}
	// Ahorro de tokens: lecturas de registros y dependencias denegadas, bucles cortados, compactación, límite de
	// pasos y de contexto para el modelo de Google.
	for _, x := range []string{`"*.log":"deny"`, `"node_modules/*":"deny"`, `"doom_loop":"deny"`} {
		if !strings.Contains(e[0], x) {
			t.Errorf("permisos sin %s: %s", x, e[0])
		}
	}
	for _, x := range []string{`"compaction":{"auto":true,"prune":true}`, `"steps":40`, `"gemini-4-argon":{"limit":{"context":160000`} {
		if !strings.HasPrefix(e[2], "OPENCODE_CONFIG_CONTENT=") || !strings.Contains(e[2], x) {
			t.Errorf("configuración sin %s: %s", x, e[2])
		}
	}
	if e := a.EntornoPara(Peticion{}); !strings.Contains(e[0], `"edit":"deny"`) || strings.Contains(e[0], "ask") {
		t.Errorf("lectura: %v", e)
	}
}

// Un agente que se queda bloqueado tras un error de lectura se para (no se espera para siempre).
func TestParaSiLecturaFalla(t *testing.T) {
	a := &Agente{Clave: "x", Nombre: "X", Programa: "sh", Args: func(string, Peticion) []string {
		return []string{"-c", "echo 'Permission required: bash (x)'; sleep 30"}
	},
		Lee: leeOpenCode}
	inicio := time.Now()
	err := a.Chat(context.Background(), Peticion{Carpeta: t.TempDir(), Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}, func(Evento) {})
	if err == nil || time.Since(inicio) > 10*time.Second {
		t.Errorf("err=%v en %s", err, time.Since(inicio))
	}
}

// OpenCode ofrece DeepSeek en Vertex AI junto a Gemini.
func TestModelosOpenCode(t *testing.T) {
	i := Agentes()[2].(*Agente).Info(context.Background())
	if !slices.Contains(i.Modelos, "vertex-deepseek/deepseek-ai/deepseek-v3.2-maas") || i.PorDefecto != "google-vertex/gemini-4-argon" {
		t.Errorf("modelos %v, defecto %s", i.Modelos, i.PorDefecto)
	}
}

// Se reconoce la skill que carga cada agente.
func TestSkillDe(t *testing.T) {
	for _, c := range []struct{ h, e, q string }{
		{"Skill", `{"skill":"prueba-madera"}`, "prueba-madera"},
		{"skill", `{"name":"odoo-18"}`, "odoo-18"},
		{"read_file", `{"Path":"/home/x/.gemini/config/skills/golang-pro/SKILL.md"}`, "golang-pro"},
		{"Read", `{"file_path":"/p/.claude/skills/a/README.md"}`, ""},
	} {
		if g := skillDe(c.h, json.RawMessage(c.e)); g != c.q {
			t.Errorf("%s %s: %q", c.h, c.e, g)
		}
	}
}

// Los adjuntos llegan a OpenCode como --file (antes del «--» y el mensaje).
func TestAdjuntosOpenCode(t *testing.T) {
	a := Agentes()[2].(*Agente)
	args := a.Args("hola", Peticion{Adjuntos: []string{"/p/.pgocode/adjuntos/s/a.png"}})
	j := strings.Join(args, " ")
	if !strings.Contains(j, "--file /p/.pgocode/adjuntos/s/a.png --") {
		t.Errorf("args: %q", args)
	}
}

// OpenCode y Antigravity: la instrucción de idioma va en el primer mensaje de la conversación, no en los siguientes.
func TestIdiomaEnElPrimerMensaje(t *testing.T) {
	var visto string
	a := &Agente{Clave: "opencode", Nombre: "Prueba", Programa: "true",
		Args: func(msg string, p Peticion) []string { visto = msg; return nil }, Lee: func(io.Reader, func(Evento)) error { return nil }}
	p := Peticion{Carpeta: t.TempDir(), Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}, Idioma: "English"}
	a.chat(context.Background(), p, func(Evento) {})
	if !strings.HasPrefix(visto, InstruccionIdioma("English")) || !strings.HasSuffix(visto, "hola") {
		t.Errorf("primer mensaje: %q", visto)
	}
	p.SesionAgente = "ses_1"
	a.chat(context.Background(), p, func(Evento) {})
	if visto != "hola" {
		t.Errorf("siguiente mensaje: %q", visto)
	}
}

// Mensajes a mitad de turno: el primero va por la entrada, los que llegan durante el turno también, y al acabar
// la respuesta (el consumo) la entrada se cierra: el agente termina y ya no se admiten más.
func TestMensajesAMitadDeTurno(t *testing.T) {
	// Agente falso: lee una línea, la repite; lee otra (la de mitad de turno), la repite; da su consumo y espera
	// a que se cierre su entrada para terminar.
	guion := `read a; echo "texto:$a"; read b; echo "texto:$b"; echo "fin"; cat >/dev/null; echo "cerrada"`
	a := &Agente{Clave: "falso", Nombre: "Falso", Programa: "sh",
		Args:    func(msg string, p Peticion) []string { return []string{"-c", guion} },
		Entrada: func(t string) []byte { return []byte(t + "\n") },
		Lee: func(r io.Reader, emite func(Evento)) error {
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				switch l := sc.Text(); {
				case strings.HasPrefix(l, "texto:"):
					emite(Evento{Texto: strings.TrimPrefix(l, "texto:") + "|"})
				case l == "fin":
					emite(Evento{Uso: &Uso{Entrada: 1}})
				default:
					emite(Evento{Texto: l + "|"})
				}
			}
			return nil
		}}
	var enviar func(string) error
	var mu sync.Mutex
	listo := make(chan struct{}, 1)
	p := Peticion{Carpeta: t.TempDir(), Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}, AlInyectar: func(f func(string) error) {
		mu.Lock()
		enviar = f
		mu.Unlock()
		if f != nil {
			listo <- struct{}{}
		}
	}}
	var texto strings.Builder
	hecho := make(chan error, 1)
	go func() {
		hecho <- a.chat(context.Background(), p, func(e Evento) { mu.Lock(); texto.WriteString(e.Texto); mu.Unlock() })
	}()
	<-listo
	mu.Lock()
	f := enviar
	mu.Unlock()
	if err := f("KIWI"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-hecho:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el agente no termina: la entrada no se cerró al acabar la respuesta")
	}
	if g := texto.String(); g != "hola|KIWI|cerrada|" {
		t.Errorf("texto %q", g)
	}
	if err := f("tarde"); !errors.Is(err, ErrEntradaCerrada) {
		t.Errorf("tras la respuesta: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if enviar != nil {
		t.Error("al cerrar la entrada se avisa con nil")
	}
}

// Con un subagente en segundo plano vivo al acabar la respuesta, la entrada sigue abierta (lo que se manda llega
// ya al agente principal); se cierra con la respuesta que acaba cuando ya no queda ninguno.
func TestMensajesConSubagenteEnSegundoPlano(t *testing.T) {
	guion := `read a; echo "sub"; echo "fin"; read b; echo "texto:$b"; echo "subfin"; echo "fin"; cat >/dev/null; echo "cerrada"`
	a := &Agente{Clave: "falso", Nombre: "Falso", Programa: "sh",
		Args:    func(msg string, p Peticion) []string { return []string{"-c", guion} },
		Entrada: func(t string) []byte { return []byte(t + "\n") },
		Lee: func(r io.Reader, emite func(Evento)) error {
			sc := bufio.NewScanner(r)
			for sc.Scan() {
				switch l := sc.Text(); {
				case l == "sub":
					emite(Evento{Tarea: &Tarea{ID: "t1", Tipo: "subagente", Estado: "curso", SegundoPlano: true}})
				case l == "subfin":
					emite(Evento{Tarea: &Tarea{ID: "t1", Estado: "ok"}})
				case l == "fin":
					emite(Evento{Uso: &Uso{Entrada: 1}})
				default:
					emite(Evento{Texto: strings.TrimPrefix(l, "texto:") + "|"})
				}
			}
			return nil
		}}
	var mu sync.Mutex
	var enviar func(string) error
	listo := make(chan struct{}, 1)
	p := Peticion{Carpeta: t.TempDir(), Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}, AlInyectar: func(f func(string) error) {
		mu.Lock()
		enviar = f
		mu.Unlock()
		if f != nil {
			listo <- struct{}{}
		}
	}}
	var texto strings.Builder
	usos := make(chan struct{}, 2)
	hecho := make(chan error, 1)
	go func() {
		hecho <- a.chat(context.Background(), p, func(e Evento) {
			mu.Lock()
			texto.WriteString(e.Texto)
			mu.Unlock()
			if e.Uso != nil {
				usos <- struct{}{}
			}
		})
	}()
	<-listo
	<-usos // la primera respuesta ha acabado, con el subagente aún vivo
	mu.Lock()
	f := enviar
	mu.Unlock()
	if f == nil {
		t.Fatal("la entrada se cerró con un subagente en segundo plano vivo")
	}
	if err := f("KIWI"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-hecho:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("el agente no termina: la entrada no se cerró al acabar el subagente")
	}
	if g := texto.String(); g != "KIWI|cerrada|" {
		t.Errorf("texto %q", g)
	}
}

// Las imágenes que leen los agentes salen en el chat: OpenCode las adjunta (data: URL); Antigravity solo da la
// ruta y se toman del disco.
func TestImagenesLeidas(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfalso")
	b64 := base64.StdEncoding.EncodeToString(png)
	oc := `{"type":"tool_use","sessionID":"s","part":{"tool":"read","callID":"c1","state":{"status":"completed","input":{"filePath":"/x/a.png"},"output":"Image read successfully","attachments":[{"type":"file","mime":"image/png","url":"data:image/png;base64,` + b64 + `"}]}}}`
	var imgs []Imagen
	leeOpenCode(strings.NewReader(oc), func(e Evento) {
		if e.Herramienta != nil {
			imgs = append(imgs, e.Herramienta.Imagenes...)
		}
	})
	if len(imgs) != 1 || imgs[0].Tipo != "image/png" || string(imgs[0].Datos) != string(png) {
		t.Fatalf("OpenCode: %+v", imgs)
	}

	f := filepath.Join(t.TempDir(), "captura.png")
	os.WriteFile(f, png, 0o644)
	ruta, _ := json.Marshal(f)
	agy := `{"event":"step_update","step_update":{"step_index":3,"state":"ACTIVE","step_type":"tool","tool_name":"read_file","tool_info":{"parameters":{"Path":` + string(ruta) + `}}}}
{"event":"step_update","step_update":{"step_index":3,"state":"DONE","step_type":"tool","tool_name":"read_file","tool_info":{"output":"ok"}}}
{"event":"result","result":{"status":"SUCCESS"}}`
	imgs = nil
	leeAntigravity(strings.NewReader(agy), func(e Evento) {
		if e.Herramienta != nil {
			imgs = append(imgs, e.Herramienta.Imagenes...)
		}
	})
	if len(imgs) != 1 || string(imgs[0].Datos) != string(png) {
		t.Fatalf("Antigravity: %+v", imgs)
	}
	if imagenDeFichero(json.RawMessage(`{"Path":"/etc/hostname"}`)) != nil {
		t.Error("un fichero que no es una imagen no se muestra")
	}
}

// El error de Gemini al llegar OpenCode al límite de pasos se explica (y cómo seguir).
func TestLimitePasosOpenCode(t *testing.T) {
	in := `{"type":"error","sessionID":"s","error":{"name":"APIError","data":{"message":"Requests ending with a model turn are not supported."}}}`
	err := leeOpenCode(strings.NewReader(in), func(Evento) {})
	if err == nil || !strings.Contains(err.Error(), "límite de 40 pasos") || !strings.Contains(err.Error(), "sigue") {
		t.Errorf("%v", err)
	}
}
