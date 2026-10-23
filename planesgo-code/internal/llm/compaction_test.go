package llm

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestCompactThreshold(t *testing.T) {
	for _, c := range []struct{ in, want int }{{0, CompactAtDefault}, {-5, CompactAtDefault}, {5000, CompactAtMin},
		{50000, 50000}, {5000000, CompactAtMax}} {
		if got := compactThreshold(Peticion{CompactAt: c.in}); got != c.want {
			t.Errorf("compactThreshold(%d) = %d, quiero %d", c.in, got, c.want)
		}
	}
}

// claudeThreshold: el umbral al que compacta Claude Code 2.1 con esas variables (su fórmula, para un modelo con
// 64k de salida y 200k de contexto).
func claudeThreshold(t *testing.T, env []string) int {
	t.Helper()
	var w int
	var pct float64
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "CLAUDE_CODE_AUTO_COMPACT_WINDOW":
			w, _ = strconv.Atoi(v)
		case "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE":
			pct, _ = strconv.ParseFloat(v, 64)
		}
	}
	if w < 100000 || w > 1000000 || pct <= 0 || pct > 100 {
		t.Fatalf("variables fuera de rango: %v", env)
	}
	e := min(w, 200000) - 20000
	return min(int(float64(e)*pct/100), e-13000)
}

func TestClaudeCompactEnv(t *testing.T) {
	a := Agentes()[0].(*Agente)
	for _, n := range []int{0, 20000, 30000, 80000, 150000} {
		env := a.EntornoPara(Peticion{CompactAt: n})
		want := compactThreshold(Peticion{CompactAt: n})
		if got := claudeThreshold(t, env); got < want-50 || got > want {
			t.Errorf("umbral %d: Claude compactaría a %d (%v)", want, got, env)
		}
	}
	// Las variables van al proceso de Claude (las de la petición se añaden después del entorno del usuario); con
	// ellas, el TTL de la caché (cachettl.go).
	if env := a.EntornoPara(Peticion{}); len(env) != 3 {
		t.Errorf("entorno: %v", env)
	}
	// Sin gestión del contexto: la compactación de Claude Code, sin tocar (el TTL sí).
	if env := a.EntornoPara(Peticion{CompactAt: CompactAgentOwn}); len(env) != 1 || env[0] != "CLAUDE_CODE_PROMPT_CACHE_TTL=5m" {
		t.Errorf("sin gestión: %v", env)
	}
}

func TestOpenCodeCompactConfig(t *testing.T) {
	a := Agentes()[2].(*Agente)
	conf := func(p Peticion) map[string]any {
		t.Helper()
		e := a.EntornoPara(p)
		var cfg map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(e[len(e)-1], "OPENCODE_CONFIG_CONTENT=")), &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	limite := func(cfg map[string]any, prov, id string) float64 {
		l, _ := cfg["provider"].(map[string]any)[prov].(map[string]any)["models"].(map[string]any)[id].(map[string]any)["limit"].(map[string]any)
		return l["context"].(float64)
	}
	// OpenCode compacta a limit.context − min(limit.output, 32k): al umbral de la sesión.
	cfg := conf(Peticion{Modelo: "google-vertex/gemini-4-argon", CompactAt: 30000})
	if c := limite(cfg, "google-vertex", "gemini-4-argon"); c-openCodeOutput != 30000 {
		t.Errorf("gemini: contexto %v", c)
	}
	if cmp := cfg["compaction"].(map[string]any); cmp["auto"] != true || cmp["prune"] != true {
		t.Errorf("compaction: %v", cmp)
	}
	if c := limite(conf(Peticion{Modelo: "deepseek/deepseek-v4-pro"}), "deepseek", "deepseek-v4-pro"); c-openCodeOutput != CompactAtDefault {
		t.Errorf("deepseek: contexto %v", c)
	}
	// Sin gestión del contexto: el límite del catálogo de OpenCode (pgocode no declara ninguno).
	if p := conf(Peticion{Modelo: "google-vertex/gemini-4-argon", CompactAt: CompactAgentOwn})["provider"].(map[string]any); len(p) != 0 {
		t.Errorf("sin gestión: %v", p)
	}
}

func TestAntigravityCompact(t *testing.T) {
	a := Agentes()[1].(*Agente)
	msg := func(p Peticion) string {
		args := a.Args("hola", p)
		return strings.TrimPrefix(args[len(args)-1], "--print=")
	}
	for _, c := range []struct {
		p    Peticion
		want string
	}{
		{Peticion{SesionAgente: "c1", LastContext: 160000}, "/compact hola"},
		{Peticion{SesionAgente: "c1", LastContext: 140000}, "hola"},
		{Peticion{SesionAgente: "c1", LastContext: 160000, CompactAt: CompactAgentOwn}, "hola"}, // sin gestión
		{Peticion{SesionAgente: "c1", LastContext: 30000, CompactAt: 25000}, "/compact hola"},
		{Peticion{LastContext: 160000}, "hola"},                                   // conversación nueva
		{Peticion{SesionAgente: "c1", LastContext: 160000, Aparte: true}, "hola"}, // pregunta aparte
	} {
		if got := msg(c.p); got != c.want {
			t.Errorf("%+v: %q, quiero %q", c.p, got, c.want)
		}
	}
	if got := antigravityMessage("/plan algo", Peticion{SesionAgente: "c1", LastContext: 160000}); got != "/plan algo" {
		t.Errorf("orden: %q", got)
	}
}

func TestCompactionEvents(t *testing.T) {
	var textos []string
	var uso *Uso
	emite := func(e Evento) {
		if e.Texto != "" {
			textos = append(textos, e.Texto)
		}
		if e.Uso != nil {
			uso = e.Uso
		}
	}
	claude := `{"type":"system","subtype":"compact_boundary","session_id":"s","compact_metadata":{"trigger":"auto","pre_tokens":45210,"post_tokens":9800}}`
	if err := leeClaudeCode(strings.NewReader(claude+"\n"), emite); err != nil {
		t.Fatal(err)
	}
	if len(textos) != 1 || !strings.Contains(textos[0], "de 45k a 10k tokens") {
		t.Errorf("claude: %q", textos)
	}
	textos = nil
	agy := strings.Join([]string{
		`{"event":"step_update","step_update":{"step_index":13,"state":"DONE","step_type":"checkpoint"}}`,
		`{"event":"step_update","step_update":{"step_index":14,"state":"DONE","step_type":"agent_response","text_delta":"ok","usage":{"input_tokens":2900,"cache_read_tokens":15000}}}`,
		`{"event":"result","result":{"status":"SUCCESS","usage":{"input_tokens":20000,"cache_read_tokens":15000,"output_tokens":3}}}`,
	}, "\n")
	if err := leeAntigravity(strings.NewReader(agy), emite); err != nil {
		t.Fatal(err)
	}
	if len(textos) != 2 || !strings.Contains(textos[0], "Antigravity ha compactado") || uso == nil || uso.Contexto != 17900 {
		t.Errorf("antigravity: %q %+v", textos, uso)
	}

	// OpenCode: la respuesta, el resumen (fuera), el aviso con el contexto antes y después y el paso de después.
	textos = nil
	var usos []Uso
	emiteOC := func(e Evento) {
		if e.Texto != "" {
			textos = append(textos, e.Texto)
		}
		if e.Uso != nil {
			usos = append(usos, *e.Uso)
		}
	}
	paso := func(texto string, input int) string {
		return `{"type":"step_start","sessionID":"s","part":{}}` + "\n" +
			`{"type":"text","sessionID":"s","part":{"type":"text","text":"` + texto + `"}}` + "\n" +
			`{"type":"step_finish","sessionID":"s","part":{"reason":"stop","tokens":{"input":` + strconv.Itoa(input) + `,"output":5}}}` + "\n"
	}
	oc := paso("respuesta", 50246) + paso("## Objective resumen", 2483) +
		`{"type":"text","sessionID":"s","part":{"type":"text","synthetic":true,"metadata":{"compaction_continue":true},"text":"Continue"}}` + "\n" +
		paso("sigo", 14381)
	if err := leeOpenCode(strings.NewReader(oc), emiteOC); err != nil {
		t.Fatal(err)
	}
	if len(textos) != 3 || textos[0] != "respuesta" || !strings.Contains(textos[1], "OpenCode ha compactado su historial: de 50k a 14k") || textos[2] != "sigo" {
		t.Errorf("opencode: %q", textos)
	}
	if len(usos) != 3 || usos[1].Entrada != 2483 || usos[1].Contexto != 0 || usos[2].Contexto != 14381 {
		t.Errorf("opencode, consumos: %+v", usos)
	}
}
