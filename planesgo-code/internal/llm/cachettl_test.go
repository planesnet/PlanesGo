package llm

import (
	"context"
	"io"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestClaudeCacheTTL(t *testing.T) {
	for in, want := range map[string]string{"": CacheTTL5m, "5m": CacheTTL5m, "1h": CacheTTL1h, "2h": CacheTTL5m, "1h; rm": CacheTTL5m} {
		if got := ClaudeCacheTTL(Peticion{CacheTTL: in}); got != want {
			t.Errorf("%q: %q, quería %q", in, got, want)
		}
	}
	if e := claudeCacheEnv(Peticion{CacheTTL: "1h"}); len(e) != 1 || e[0] != "CLAUDE_CODE_PROMPT_CACHE_TTL=1h" {
		t.Errorf("entorno: %v", e)
	}
	if claudeTTL(Peticion{}) != 5*time.Minute || claudeTTL(Peticion{CacheTTL: "1h"}) != time.Hour {
		t.Error("duraciones")
	}
	if claudeWriteFactor(5*time.Minute) != 1.25 || claudeWriteFactor(time.Hour) != 2 {
		t.Error("precios de escritura")
	}
}

// La caché que hay al empezar el turno es la que escribió el anterior; escribir cuesta lo del TTL de este.
func TestClaudeCostFor(t *testing.T) {
	if c := claudeCostFor(0, Peticion{CacheTTL: "1h"}); c.TTL != time.Hour || c.Write != 2 {
		t.Errorf("sin turno anterior: %+v", c)
	}
	if c := claudeCostFor(time.Hour, Peticion{CacheTTL: "5m"}); c.TTL != time.Hour || c.Write != 1.25 {
		t.Errorf("de 1 h a 5 min: %+v", c)
	}
}

// Con la caché a 5 min, a los 10 min ya está fría: podar sale gratis (a 1 h, no compensaría).
func TestClaudeCacheWorth5m(t *testing.T) {
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	s := &claudeSession{Pos: map[string]int{"a": 20000}, Cache: 95000, LastCall: now.Add(-10 * time.Minute)}
	d := pruneDiff{Saved: 30000, Update: map[string]string{"a": ""}}
	if ok, why := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 0, now, claudeCacheCost{TTL: 5 * time.Minute, Write: 1.25}); !ok || !strings.Contains(why, "fría") {
		t.Errorf("5 min: %v %s", ok, why)
	}
	if ok, _ := claudeCacheWorth(s, d, map[string]int{"a": 30000}, 0, now, claudeCacheCost{TTL: time.Hour, Write: 2}); ok {
		t.Error("1 h: caliente, no compensa")
	}
}

// fakeClaude: un Claude Code falso que guarda el mensaje y el entorno de cada petición y da un consumo.
func fakeClaude(msgs *[]string, envs *[][]string) *Agente {
	return &Agente{Clave: "claude-code", Nombre: "Prueba", Programa: "sh", EntornoPara: claudeEnv,
		Lee: func(r io.Reader, emite func(Evento)) error {
			io.Copy(io.Discard, r)
			emite(Evento{Texto: "ok", Uso: &Uso{Entrada: 10, Contexto: 10}})
			return nil
		}, Args: func(msg string, p Peticion) []string {
			*msgs = append(*msgs, msg)
			*envs = append(*envs, claudeEnv(p))
			return []string{"-c", "true"}
		}}
}

// El consumo dice cuánto dura en caché lo enviado (la interfaz avisa de cuándo caduca).
func TestCacheTTLInUsage(t *testing.T) {
	var msgs []string
	var envs [][]string
	a := fakeClaude(&msgs, &envs)
	for ttl, want := range map[string]int{"": 300, "1h": 3600} {
		var got int
		p := Peticion{Carpeta: t.TempDir(), CacheTTL: ttl, Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}
		if err := a.Chat(context.Background(), p, func(e Evento) {
			if e.Uso != nil {
				got = e.Uso.CacheTTL
			}
		}); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%q: %d s, quería %d", ttl, got, want)
		}
	}
	if !slices.Contains(envs[0], "CLAUDE_CODE_PROMPT_CACHE_TTL=5m") && !slices.Contains(envs[0], "CLAUDE_CODE_PROMPT_CACHE_TTL=1h") {
		t.Errorf("entorno: %v", envs)
	}
}

// Auditoría de la caché: lo que añade pgocode al mensaje (aviso de pgo, sugerencia de subagentes…) va al final: el
// principio del mensaje y todo lo anterior (sistema, herramientas, historial) no cambian entre turnos. Y el entorno
// y los argumentos de dos turnos seguidos de la misma sesión son iguales salvo el mensaje.
func TestPgocodeAdditionsKeepPrefix(t *testing.T) {
	var msgs []string
	var envs [][]string
	a := fakeClaude(&msgs, &envs)
	texto := "arranca el servidor con pgo ejecutar web npm run dev"
	p := Peticion{Carpeta: t.TempDir(), SesionAgente: "11111111-2222-4333-8444-555555555555", Entorno: []string{"PGO_TOKEN=x"},
		Mensajes: []Mensaje{{Rol: "user", Texto: texto}}}
	if err := a.Chat(context.Background(), p, func(Evento) {}); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || !strings.HasPrefix(msgs[0], texto) || msgs[0] == texto {
		t.Errorf("el aviso de pgo no va al final del mensaje: %q", msgs)
	}
	// Dos turnos de la misma sesión: mismos argumentos (salvo el mensaje, el último) y mismo entorno.
	c := Agentes()[0].(*Agente)
	q := Peticion{Carpeta: "/p", SesionAgente: p.SesionAgente, Permisos: "auto", Idioma: "español", Modelo: "opus", Esfuerzo: "high"}
	a1, a2 := c.Args("uno", q), c.Args("dos", q)
	if !slices.Equal(a1[:len(a1)-1], a2[:len(a2)-1]) || !slices.Equal(c.EntornoPara(q), c.EntornoPara(q)) {
		t.Errorf("los argumentos cambian entre turnos:\n%q\n%q", a1, a2)
	}
	// El sistema que añade pgocode (--append-system-prompt) es solo el idioma: nada que cambie de un turno a otro.
	i := slices.Index(a1, "--append-system-prompt")
	if i < 0 || a1[i+1] != InstruccionIdioma("español") {
		t.Errorf("sistema añadido: %q", a1)
	}
}

// OpenCode y Antigravity: el idioma va al principio del primer mensaje (queda en su historial); en los siguientes,
// nada delante del mensaje del usuario.
func TestLanguageOnlyInFirstMessage(t *testing.T) {
	var vistos []string
	a := &Agente{Clave: "antigravity", Nombre: "Prueba", Programa: "sh", Lee: func(r io.Reader, emite func(Evento)) error {
		io.Copy(io.Discard, r)
		emite(Evento{Texto: "ok"})
		return nil
	}, Args: func(msg string, p Peticion) []string {
		vistos = append(vistos, msg)
		return []string{"-c", "true"}
	}}
	for _, ses := range []string{"", "conv-1"} {
		p := Peticion{Carpeta: t.TempDir(), SesionAgente: ses, Idioma: "español", Mensajes: []Mensaje{{Rol: "user", Texto: "hola"}}}
		if err := a.Chat(context.Background(), p, func(Evento) {}); err != nil {
			t.Fatal(err)
		}
	}
	if len(vistos) != 2 || !strings.HasPrefix(vistos[0], "Idioma de las respuestas") || vistos[1] != "hola" {
		t.Errorf("mensajes: %q", vistos)
	}
}

func TestCacheTTLSeconds(t *testing.T) {
	for agent, want := range map[string]int{"claude-code": 300, "opencode": 300, "antigravity": 120, "otro": 0} {
		if got := cacheTTLSeconds(agent, Peticion{}); got != want {
			t.Errorf("%s: %d", agent, got)
		}
	}
}

// OpenCode poda solo si no poda pgocode (la suya no mira la caché).
func TestOpenCodeOwnPrune(t *testing.T) {
	for mode, want := range map[string]string{PruneOff: `"prune":true`, PruneAuto: `"prune":false`, PruneRules: `"prune":false`} {
		e := entornoOpenCode(Peticion{Modelo: "google-vertex/gemini-4-argon", Prune: mode})
		if !strings.Contains(e[len(e)-1], want) {
			t.Errorf("%q: %s", mode, e[len(e)-1])
		}
	}
}
