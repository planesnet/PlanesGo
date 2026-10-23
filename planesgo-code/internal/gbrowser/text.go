package gbrowser

import (
	"regexp"
	"strings"
)

var (
	reBlankLines = regexp.MustCompile(`\n{3,}`)
	invisible    = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\u034f", "", "\ufeff", "", "\u00ad", "", "\u00a0", " ")
)

// tidyText: sin \r, sin caracteres invisibles, sin espacios al final de las líneas y sin tandas de líneas en blanco.
func tidyText(s string) string {
	s = invisible.Replace(strings.ReplaceAll(s, "\r\n", "\n"))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.TrimSpace(reBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

var (
	reQuoteIntro  = regexp.MustCompile(`(?i)^(?:el|on)\s.{4,300}(?:escribió|wrote|a écrit|ha scritto)\s*:\s*$`)
	reQuoteStart  = regexp.MustCompile(`(?i)^(?:el|on)\s`)
	reQuoteEnd    = regexp.MustCompile(`(?i)(?:escribió|wrote|a écrit|ha scritto)\s*:\s*$`)
	reOriginalMsg = regexp.MustCompile(`(?i)^-{2,}\s*(?:original message|mensaje original)\s*-{2,}\s*$`)
	reOutlookFrom = regexp.MustCompile(`(?i)^\*?(?:de|from)\s*:\*?\s`)
	reForwarded   = regexp.MustCompile(`(?i)^-{2,}\s*(?:forwarded message|mensaje reenviado)\b`)
	reOutlookSent = regexp.MustCompile(`(?i)^\*?(?:enviado(?:\s+el)?|sent|fecha|date)\s*:\*?\s`)
)

// stripQuoted quita de un mensaje de un hilo lo que cita de los anteriores (que ya están en el hilo): desde «El …
// escribió:» / «On … wrote:», «-----Mensaje original-----» o la cabecera «De: … Enviado: …» de Outlook, y las líneas
// que empiezan por «>». Lo reenviado no se toca (es contenido nuevo).
func stripQuoted(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	cut := len(lines)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		next := ""
		if i+1 < len(lines) {
			next = strings.TrimSpace(lines[i+1])
		}
		if reForwarded.MatchString(t) { // lo de después es lo reenviado: se queda
			break
		}
		if reQuoteIntro.MatchString(t) || reOriginalMsg.MatchString(t) ||
			(reQuoteStart.MatchString(t) && !reQuoteEnd.MatchString(t) && len(t) < 300 && reQuoteEnd.MatchString(next) && len(next) < 120) {
			cut = i
			break
		}
		if reOutlookFrom.MatchString(t) && i > 0 {
			for j := i + 1; j < min(i+4, len(lines)); j++ {
				if reOutlookSent.MatchString(strings.TrimSpace(lines[j])) {
					cut = i
					break
				}
			}
			if cut != len(lines) {
				break
			}
		}
	}
	removed := cut < len(lines)
	var kept []string
	for _, l := range lines[:cut] {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return s, false
	}
	return tidyText(strings.Join(kept, "\n")), true
}
