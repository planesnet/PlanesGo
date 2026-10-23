package servidor

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/memory"
	"planesgo-code/internal/sesiones"
)

// Memoria persistente del proyecto (internal/memory): se le da al agente solo cuando empieza de cero, al final de
// su mensaje (no rompe el prefijo en caché):
//   - sesión nueva del agente (primer mensaje, relevo, cambio de rama o de agente);
//   - tras «Vaciar contexto» (su historial queda vacío aunque siga la misma sesión);
//   - tras una compactación (el resumen del agente puede haber perdido lo acordado);
//   - Antigravity: en el mismo mensaje que va tras «/compact».
// Nunca en cada turno ni en las preguntas aparte.

// memoryPending: las sesiones del agente que tienen que volver a recibir la memoria en su próximo mensaje.
type memoryPending struct {
	mu sync.Mutex
	m  map[string]bool
}

func (p *memoryPending) mark(agentSession string) {
	if agentSession == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]bool{}
	}
	p.m[agentSession] = true
}

func (p *memoryPending) take(agentSession string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.m[agentSession] {
		return false
	}
	delete(p.m, agentSession)
	return true
}

// memoryReason: por qué este mensaje lleva la memoria del proyecto («» si no la lleva).
func (s *Servidor) memoryReason(provider, msg string, p llm.Peticion) string {
	switch {
	case p.Aparte || p.Corte != "":
		return ""
	case p.SesionAgente == "":
		return "sesión nueva del agente"
	case s.memPending.take(p.SesionAgente):
		return "contexto vaciado o compactado"
	case provider == "antigravity" && llm.AntigravityCompacts(msg, p):
		return "compactación"
	}
	return ""
}

// memoryTurn: lo que un turno aporta a la memoria: las frases del usuario y las órdenes del proyecto que han
// funcionado (por el id de la herramienta: el evento de «ok» no siempre trae la orden).
type memoryTurn struct {
	mu       sync.Mutex
	started  map[string]llm.Herramienta
	commands []memory.CommandEntry
}

func (t *memoryTurn) tool(h *llm.Herramienta) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started == nil {
		t.started = map[string]llm.Herramienta{}
	}
	prev := t.started[h.ID]
	if h.Nombre == "" {
		h.Nombre = prev.Nombre
	}
	if h.Resumen == "" {
		h.Resumen = prev.Resumen
	}
	t.started[h.ID] = llm.Herramienta{Nombre: h.Nombre, Resumen: h.Resumen}
	if h.Estado != "ok" || h.Padre != "" {
		return
	}
	if c, ok := memory.Command(h.Nombre, h.Resumen); ok {
		t.commands = append(t.commands, c)
	}
}

// saveMemoryTurn: al acabar el turno, lo nuevo a la memoria del proyecto (sin bloquear la respuesta).
func saveMemoryTurn(root, userText string, t *memoryTurn) {
	t.mu.Lock()
	cmds := t.commands
	t.mu.Unlock()
	if root == "" {
		return
	}
	memory.Update(root, memory.Facts(userText), cmds)
}

// memoryRoot: la carpeta de la memoria de una carpeta de la sesión del proyecto («» si no es del proyecto).
func (s *Servidor) memoryRoot(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion, proyecto int, carpeta string) (string, bool) {
	if _, ok := s.proyecto(w, r, ses, proyecto); !ok {
		return "", false
	}
	if carpeta == "" || !s.Indice.Pertenece(proyecto, carpeta) {
		jsonError(w, http.StatusForbidden, "la carpeta no es del proyecto")
		return "", false
	}
	return memory.Root(carpeta), true
}

type memoryView struct {
	Texto  string `json:"texto"`
	Tokens int    `json:"tokens"`
	Max    int    `json:"max"`
	Ruta   string `json:"ruta"`
}

func memoryViewOf(root string) memoryView {
	t := memory.Load(root)
	return memoryView{Texto: t, Tokens: memory.Tokens(t), Max: memory.MaxTokens, Ruta: memory.Path(root)}
}

// memoriaLee: GET /api/memoria?proyecto=&carpeta= (la memoria del proyecto de esa carpeta).
func (s *Servidor) memoriaLee(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	id, _ := strconv.Atoi(r.URL.Query().Get("proyecto"))
	root, ok := s.memoryRoot(w, r, ses, id, r.URL.Query().Get("carpeta"))
	if !ok {
		return
	}
	jsonOK(w, memoryViewOf(root))
}

// memoriaGuarda: POST /api/memoria {proyecto, carpeta, texto}: la corrige el usuario (vacía: la borra).
func (s *Servidor) memoriaGuarda(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Proyecto int    `json:"proyecto"`
		Carpeta  string `json:"carpeta"`
		Texto    string `json:"texto"`
	}
	if err := lee(r, &p); err != nil {
		errorLectura(w, err)
		return
	}
	root, ok := s.memoryRoot(w, r, ses, p.Proyecto, p.Carpeta)
	if !ok {
		return
	}
	if err := memory.Save(root, p.Texto); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, memory.ErrTooLong) {
			code = http.StatusBadRequest
		}
		jsonError(w, code, err.Error())
		return
	}
	jsonOK(w, memoryViewOf(root))
}

// withoutRelayMemory: el mensaje sin la memoria de trabajo del relevo (workingMemory.ts, relayMessage), que no es
// lo que ha dicho el usuario ahora (para los clientes que no mandan texto_usuario).
func withoutRelayMemory(t string) string {
	i := strings.Index(t, "[Memoria de trabajo:")
	j := strings.Index(t, "[Fin de la memoria de trabajo]")
	if i < 0 || j < i {
		return t
	}
	return t[:i] + t[j+len("[Fin de la memoria de trabajo]"):]
}
