package llm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Salidas enormes a fichero (OpenCode y Claude Code).
//
// Una salida grande (una lectura de un fichero largo, una orden con miles de líneas, una búsqueda) se reenvía entera
// en cada paso del agente mientras siga en su historial. El agente casi nunca vuelve a necesitarla entera: le basta
// el principio, el final y saber dónde está el resto para leer con offset/limit (o grep) solo el trozo que le haga
// falta. Así que, pasados los pasos protegidos (pruneProtectSteps: los de lo que está haciendo ahora), la poda deja
// en el historial solo el principio y el final con un aviso con la RUTA de la salida completa:
//   - lecturas: la ruta es el propio fichero (con las líneas que faltan, para releerlas con offset/limit);
//   - lo demás (órdenes, búsquedas, MCP…): <carpeta de la sesión>/.pgocode/salidas/<llamada>.txt, excluida de git.
//
// El aviso va con el texto nativo de cada agente («Full output saved to: …» de OpenCode; el de <persisted-output>
// de Claude Code): con textos propios el modelo se desdecía de lo deducido (prune.go, ClearedText).
//
// Lo de antes de llegar al historial lo hace el propio agente, sin coste de caché (la salida nunca entra entera):
// OpenCode con su «tool_output» (offloadWriteBytes, configOpenCode) y Claude Code con «bashOutputMaxChars»
// (claudeOffloadSettings). Lo de aquí es lo que ya entró: se aplica en el barrido, con su economía de caché
// (decideSweep, pays, claudeCacheWorth): cuenta como lo demás que sale.

const (
	// offloadChars: a partir de cuántos caracteres una salida de los intercambios jóvenes (pasados los pasos
	// protegidos) o una lectura de los anteriores se deja en principio y final. Medido en las sesiones reales de
	// Luis (OpenCode, 10 036 partes; Claude Code, 16 679 órdenes): el 1-2 % de las salidas pasa de 12 000 y
	// son las que más se arrastran (las órdenes de Claude Code de más de 12 000, 47 pasos de mediana en el mismo
	// turno; las lecturas de OpenCode de más de 12 000 suman 16 M de tokens·paso más allá de los protegidos).
	offloadChars = 12000
	// offloadWriteBytes y offloadWriteLines: el límite de las salidas al llegar (OpenCode, tool_output: lo que pasa,
	// a su fichero; deja la cola de las órdenes y el principio de lo demás). El de OpenCode es 51 200 y 2000.
	offloadWriteBytes = 12288
	offloadWriteLines = 500
	// offloadBashChars: lo mismo en Claude Code (bashOutputMaxChars; el suyo es 30 000; lo admite entre 4000 y
	// 128 000): por encima, la salida va a un fichero y el modelo recibe el principio y la ruta.
	offloadBashChars = 12000
	// offloadKeepDays: lo que se guardan los ficheros de .pgocode/salidas.
	offloadKeepDays = 30
)

// offloadOnWrite: los límites de las salidas al llegar (OpenCode tool_output, Claude Code bashOutputMaxChars). En
// las pruebas de consumo se quita para medir la diferencia (PGOCODE_OFFLOAD_WRITE=0).
var offloadOnWrite = os.Getenv("PGOCODE_OFFLOAD_WRITE") != "0"

// offloadInSweep: las salidas ya recibidas, a fichero en el barrido (false: como antes; para medir la diferencia).
var offloadInSweep = os.Getenv("PGOCODE_OFFLOAD_SWEEP") != "0"

// claudeOffloadSettings: los ajustes de Claude Code para las salidas al llegar («» sin ellos).
func claudeOffloadSettings() string {
	if !offloadOnWrite {
		return ""
	}
	return fmt.Sprintf(`{"bashOutputMaxChars":%d}`, offloadBashChars)
}

// offloadDirOf: la carpeta de las salidas completas de una sesión («» sin carpeta).
func offloadDirOf(carpeta string) string {
	if carpeta == "" {
		return ""
	}
	return filepath.Join(carpeta, ".pgocode", "salidas")
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// offloadPath: el fichero de la salida completa de una llamada.
func offloadPath(dir, callID string) string {
	return filepath.Join(dir, unsafeName.ReplaceAllString(callID, "_")+".txt")
}

// offloadable: si la salida de la herramienta se puede dejar en principio y final: no las ediciones (pequeñas),
// la lista de tareas, los informes de los subagentes, las skills (instrucciones) ni las imágenes.
func offloadable(p *ocPart) bool {
	switch {
	case !p.isTool(), isEditTool(p.v.Tool), len(p.v.State.Attachments) > 0:
		return false
	}
	switch p.v.Tool {
	case "todowrite", "task", "skill", "image":
		return false
	}
	return true
}

// lineNum: el número de línea con que empieza una línea de una lectura («12: …» en OpenCode, «    12→…» o
// «12\t…» en Claude Code); 0 si no lleva.
var lineNumRe = regexp.MustCompile(`^\s*(\d+)(?:: |\t|→)`)

func lineNum(l string) int {
	if m := lineNumRe.FindStringSubmatch(l); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// headTail: el principio y el final de una salida (como cutLong) y las líneas que quedan en medio; ok si es larga.
func headTail(s string) (head, tail string, mid []string, ok bool) {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) <= pruneLongLines && len(s) <= pruneLongChars {
		return "", "", nil, false
	}
	if len(ls) < 2*pruneKeepLines+5 { // pocas líneas muy largas: por caracteres
		return strings.ToValidUTF8(s[:pruneKeepChars], ""), strings.ToValidUTF8(s[len(s)-pruneKeepChars:], ""), nil, true
	}
	head = recortaBytes(strings.Join(ls[:pruneKeepLines], "\n"), pruneKeepChars, false)
	tail = recortaBytes(strings.Join(ls[len(ls)-pruneKeepLines:], "\n"), pruneKeepChars, true)
	return head, tail, ls[pruneKeepLines : len(ls)-pruneKeepLines], true
}

// kb: el tamaño en KB con un decimal («40.1KB», como lo escriben los agentes).
func kb(n int) string { return fmt.Sprintf("%.1fKB", float64(n)/1024) }

// offloadText: la salida larga de una herramienta en principio y final con el aviso de dónde está entera (file: el
// fichero que hay que escribir con la salida completa; «» si es una lectura, cuyo resto está en el propio fichero).
// dir: la carpeta de las salidas («»: sin ella solo se recortan las lecturas). claude: el aviso de Claude Code.
func offloadText(p *ocPart, dir string, claude bool) (text, file string, ok bool) {
	out := p.output()
	head, tail, mid, largo := headTail(out)
	if !largo {
		return "", "", false
	}
	if p.v.Tool == "read" {
		f := p.filePath()
		if f == "" {
			return "", "", false
		}
		desde, hasta := 0, 0
		if len(mid) > 0 {
			desde, hasta = lineNum(mid[0]), lineNum(mid[len(mid)-1])
		}
		var nota string
		if desde > 0 && hasta >= desde {
			nota = fmt.Sprintf("\n\n(Lines %d-%d of %s omitted to save context. Use Read with offset=%d and limit=%d to view them again.)\n\n",
				desde, hasta, f, desde, hasta-desde+1)
		} else {
			nota = fmt.Sprintf("\n\n(%s omitted to save context. Read %s with offset/limit to view specific sections.)\n\n", kb(len(out)-len(head)-len(tail)), f)
		}
		return head + nota + tail, "", true
	}
	if dir == "" {
		return "", "", false
	}
	file = offloadPath(dir, p.v.CallID)
	lineas := strings.Count(strings.TrimRight(out, "\n"), "\n") + 1
	if claude {
		// El aviso de Claude Code (<persisted-output>), con el final además del principio.
		return fmt.Sprintf("<persisted-output>\nOutput too large (%s, %d lines). Full output saved to: %s\n\nPreview (first %s):\n%s\n...\n\nEnd (last %s):\n%s\n</persisted-output>",
			kb(len(out)), lineas, file, kb(len(head)), head, kb(len(tail)), tail), file, true
	}
	// El de OpenCode (su Truncate.output), con el principio y el final.
	omitido := len(out) - len(head) - len(tail)
	return fmt.Sprintf("%s\n\n...%d bytes truncated (%d lines, %s in total)...\n\nThe tool call succeeded but the output was truncated. Full output saved to: %s\nUse Grep to search the full content or Read with offset/limit to view specific sections.\n\n%s",
		head, omitido, lineas, kb(len(out)), file, tail), file, true
}

// offloadPlan: en el plan, las salidas largas en principio y final con la ruta de la completa (offloadText):
//   - de los intercambios jóvenes, las de más de offloadChars pasados los pasos protegidos;
//   - de los anteriores, las lecturas de más de offloadChars y las demás salidas largas (las que antes se recortaban
//     sin ruta, cutLong).
//
// Solo lo que sigue en el contexto con el plan (lo que sale entero, sale; lo superado, igual). dir: la carpeta de
// las salidas completas («»: las salidas que no son lecturas se recortan como antes, sin ruta).
func offloadPlan(plan *prunePlan, win []*ocMessage, dir string, claude bool) {
	if !offloadInSweep {
		return
	}
	partes := map[string]*ocPart{}
	for _, m := range win {
		for _, p := range m.Parts {
			partes[p.ID] = p
		}
	}
	for _, pv := range plan.Pieces {
		p := partes[pv.PartID]
		if p == nil || pv.Out || !offloadable(p) {
			continue
		}
		e, hay := plan.Edits[p.ID]
		if hay && (e.Clear || e.TrimInput || e.Delete || e.Keep) {
			continue
		}
		joven := pv.X != nil && pv.X.Tier == TierL1
		if joven && pv.F.Steps < pruneProtectSteps {
			continue
		}
		n := len(p.output())
		if (joven || p.v.Tool == "read") && n < offloadChars {
			continue
		}
		t, f, ok := offloadText(p, dir, claude)
		if !ok {
			continue // sin carpeta: se queda como estaba (recortada sin ruta si ya lo estaba)
		}
		plan.Edits[p.ID] = pruneEdit{Output: &t, Offload: true, File: f}
		pv.Note = "salida a fichero"
		if p.v.Tool == "read" {
			pv.Note = "lectura recortada"
		}
	}
}

// writeOffloads: guarda la salida completa de lo que va a fichero con el barrido (antes de cambiar el historial:
// si no se puede escribir, no se barre). Excluye .pgocode/ de git y borra las salidas de más de offloadKeepDays.
func writeOffloads(plan prunePlan, win []*ocMessage, ids map[string]string) error {
	escritas := 0
	for _, m := range win {
		for _, p := range m.Parts {
			if _, cambia := ids[p.ID]; !cambia {
				continue
			}
			e := plan.Edits[p.ID]
			if !e.Offload || e.File == "" {
				continue
			}
			if _, err := os.Stat(e.File); err == nil {
				continue
			}
			if escritas == 0 {
				if err := os.MkdirAll(filepath.Dir(e.File), 0o700); err != nil {
					return err
				}
				excludeFromGit(filepath.Dir(filepath.Dir(filepath.Dir(e.File))))
				cleanOffloads(filepath.Dir(e.File))
			}
			if err := os.WriteFile(e.File, []byte(p.output()), 0o600); err != nil {
				return err
			}
			escritas++
		}
	}
	return nil
}

// cleanOffloads: borra las salidas de más de offloadKeepDays.
func cleanOffloads(dir string) {
	l, _ := os.ReadDir(dir)
	limite := time.Now().AddDate(0, 0, -offloadKeepDays)
	for _, f := range l {
		if i, err := f.Info(); err == nil && !f.IsDir() && i.ModTime().Before(limite) {
			os.Remove(filepath.Join(dir, f.Name()))
		}
	}
}

// excludeFromGit añade «.pgocode/» a .git/info/exclude del repositorio de la carpeta (como los adjuntos).
func excludeFromGit(carpeta string) {
	out, err := exec.Command("git", "-C", carpeta, "rev-parse", "--git-common-dir").Output()
	if err != nil {
		return
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(carpeta, dir)
	}
	f := filepath.Join(dir, "info", "exclude")
	if b, _ := os.ReadFile(f); strings.Contains(string(b), "\n.pgocode/") || strings.HasPrefix(string(b), ".pgocode/") {
		return
	}
	os.MkdirAll(filepath.Dir(f), 0o755)
	if fh, err := os.OpenFile(f, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintln(fh, ".pgocode/")
		fh.Close()
	}
}
