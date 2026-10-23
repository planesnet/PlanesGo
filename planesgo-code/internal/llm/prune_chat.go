package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Vigencia desde la conversación de pgocode, para los agentes que no dejan tocar su historial (Antigravity: guarda
// la conversación cifrada en ~/.gemini/antigravity-cli/conversations/<id>.pb y la copia SQLite de al lado la
// rehace a partir de ella; medido: vaciar una lectura en la copia no baja el contexto). Con el mismo núcleo
// (planPrune, vigencia y embeddings locales) se calcula la vigencia de lo que ve pgocode en su chat; nada se quita
// del historial del agente: cuando lo que está fuera de vigencia pesa bastante (Pending), la interfaz manda el
// siguiente mensaje como relevo, a una conversación nueva con la memoria de trabajo hecha solo con lo vigente.

// ChatExchange: un intercambio de la conversación de pgocode (mensaje del usuario y respuesta del agente).
type ChatExchange struct {
	Point  string     `json:"punto"` // el id con que la interfaz lo reconoce
	User   string     `json:"usuario"`
	Answer string     `json:"respuesta"`
	At     int64      `json:"en,omitempty"` // cuándo acabó (ms)
	Tools  []ChatTool `json:"herramientas,omitempty"`
	// Compacted: anterior a la última compactación del agente («/compact»): ya solo tiene su resumen.
	Compacted bool `json:"compactado,omitempty"`
}

// ChatTool: una herramienta del intercambio, como la ve pgocode.
type ChatTool struct {
	ID      string `json:"id"`
	Name    string `json:"nombre"`
	Summary string `json:"resumen,omitempty"`
	Output  string `json:"salida,omitempty"`
	Images  int    `json:"imagenes,omitempty"`
	Error   bool   `json:"error,omitempty"`
	Size    int    `json:"tamano,omitempty"` // la salida entera, si la interfaz solo tiene un trozo (caracteres)
}

// chatToolName: el nombre de la herramienta con el de su equivalente en OpenCode (las reglas de prune.go).
func chatToolName(n string) string {
	l := strings.ToLower(n)
	switch {
	case l == "view_file" || l == "read_file" || l == "read" || l == "open_file" || l == "view":
		return "read"
	case strings.Contains(l, "write") || strings.Contains(l, "replace") || strings.Contains(l, "edit") || strings.Contains(l, "create_file"):
		return "edit"
	case l == "run_command" || l == "bash" || l == "shell" || l == "terminal" || l == "exec":
		return "bash"
	case strings.Contains(l, "todo") || strings.Contains(l, "task_list"):
		return "todowrite"
	}
	return l
}

// agyLines: la salida de una lectura de Antigravity, que solo trae su tamaño («1401 lines, 61315 bytes»): el
// fichero entero va al modelo, pero no a la salida que ve pgocode.
var agyLines = regexp.MustCompile(`^\s*\d+ lines?, (\d+) bytes\s*$`)

func agyReadSize(out string) int {
	if m := agyLines.FindStringSubmatch(out); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// chatWindow: los intercambios en mensajes y partes de la poda común.
func chatWindow(xs []ChatExchange) []*ocMessage {
	var win []*ocMessage
	for _, x := range xs {
		u := &ocMessage{ID: "u:" + x.Point, Role: "user", Time: x.At}
		d, _ := marshalNoEscape(map[string]any{"type": "text", "text": x.User})
		u.Parts = append(u.Parts, newPart(u.ID+":t", u.ID, string(d), ""))
		win = append(win, u)
		// Cada herramienta, un paso del agente (su llamada al modelo); la respuesta, el último (con el punto).
		for k, t := range x.Tools {
			a := &ocMessage{ID: fmt.Sprintf("%s#%d", x.Point, k), Role: "assistant", Time: x.At}
			tool := chatToolName(t.Name)
			clave := "description"
			switch tool {
			case "read", "edit":
				clave = "file_path"
			case "bash":
				clave = "command"
			}
			in, _ := json.Marshal(map[string]string{clave: t.Summary})
			status := "completed"
			if t.Error {
				status = "error"
			}
			out := t.Output
			if n := agyReadSize(out); n > t.Size {
				t.Size = n
			}
			if t.Size > len(out) {
				// Para estimar lo que pesa: la salida entera tenía Size caracteres (el relleno no se embebe: va al final).
				out += strings.Repeat(" ", t.Size-len(out))
			}
			a.Parts = append(a.Parts, newPart(t.ID, a.ID, claudePartData(tool, t.ID, in, status, out, t.Images), ""))
			win = append(win, a)
		}
		a := &ocMessage{ID: x.Point, Role: "assistant", Time: x.At}
		if x.Answer != "" {
			d, _ := marshalNoEscape(map[string]any{"type": "text", "text": x.Answer})
			a.Parts = append(a.Parts, newPart(a.ID+":t", a.ID, string(d), ""))
		}
		win = append(win, a)
	}
	return win
}

// chatPending: lo que está fuera de vigencia (3 o más) y sigue en el contexto del agente: los intercambios enteros
// y, de los demás, las piezas.
func chatPending(plan prunePlan) int {
	n := 0
	for _, x := range plan.Exchanges {
		if x.Tier != TierL3 {
			continue
		}
		for _, m := range x.Messages {
			for _, p := range m.Parts {
				n += tokensOf(p.Data)
			}
		}
	}
	for _, pv := range plan.Pieces {
		if pv.X.Tier == TierL3 || pv.V < vigCut {
			continue
		}
		for _, m := range pv.X.Messages {
			for _, p := range m.Parts {
				if p.ID == pv.PartID {
					n += tokensOf(p.Data)
				}
			}
		}
	}
	return n
}

// ChatVigencia: la vigencia de una conversación de pgocode (clave: la de su recolector, «chat-<sesión>»), frente al
// mensaje nuevo (o, sin él, al último). auto: con embeddings (Ollama local); si no, solo reglas.
// folder: la carpeta del proyecto (sus pesos de la vigencia).
func ChatVigencia(ctx context.Context, key, folder string, xs []ChatExchange, msg string, auto bool) *PruneReport {
	if !sesionValida.MatchString(key) || len(xs) == 0 {
		return nil
	}
	c := collectorFor(key)
	var rep *PruneReport
	c.do(ctx, func() {
		defer c.setPhase("", 0)
		c.setPhase("marcando", len(xs))
		// Lo anterior a la última compactación del agente: fuera del contexto (solo para la vigencia).
		var antes, dentro []ChatExchange
		for _, x := range xs {
			if x.Compacted {
				antes = append(antes, x)
			} else {
				dentro = append(dentro, x)
			}
		}
		c.setFolder(folder)
		win := chatWindow(dentro)
		var s *pruneSims
		if auto {
			ce, cancela := context.WithTimeout(ctx, 20*time.Second)
			c.embedAll(ce, win)
			if strings.TrimSpace(msg) == "" {
				msg = xs[len(xs)-1].User
			}
			s = c.sims(ce, win, msg)
			cancela()
		}
		c.setRulesOnly(s == nil)
		now := pruneNow()
		plan := planPruneK(win, s, pruneYoung, now, c.params())
		for _, pv := range plan.Pieces {
			pv.Note, pv.Out = "", false // nada se aplica: lo decide el relevo
		}
		c.state.Vig = vigSnapshot(plan, func(*pruneExchange) bool { return false }, func(*pieceVig) bool { return false })
		c.state.Vig.addCompacted(chatWindow(antes))
		c.save()
		rep = &PruneReport{Pending: chatPending(plan), Calibration: calibrationInfo(folder)}
		rep.Exchanges, rep.Pieces = c.state.Vig.report(now)
		c.publish(rep)
	})
	if rep != nil {
		st := c.status()
		rep.Status = &st
	}
	return rep
}
