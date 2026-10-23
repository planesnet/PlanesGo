package llm

import (
	"regexp"
	"strings"
)

// El agente no sabe que existe la orden «pgo» (programas del proyecto en pgocode: cmd/pgocode/pgo.go): si el
// mensaje habla de ella, se le añade una línea con su uso. Solo en ese mensaje: nada fijo en cada turno.

// pgoHint: la línea que se añade (el resumen de «pgo --help»).
const pgoHint = "(pgocode: «pgo» es una orden de terminal (ejecútala con tu herramienta de órdenes, no es una herramienta MCP): " +
	"`pgo ejecutar <nombre> <orden…>` arranca un programa del proyecto en la carpeta actual y sigue al acabar el turno; " +
	"`pgo parar <nombre>`; `pgo procesos`; `pgo salida <nombre> [n]` (últimas n líneas); `pgo abrir <url>` (navegador de pgocode).)"

// driveHint: el uso de «pgo drive» (Google Drive, Docs y Sheets en solo lectura).
const driveHint = "(pgocode: para Google Drive/Docs/Sheets usa la orden de terminal «pgo» (solo lectura, con la cuenta de Google del usuario): " +
	"`pgo drive buscar <texto> [--tipo doc|hoja|pdf|carpeta] [--carpeta <id|url>]`; `pgo drive listar [<carpeta>]`; " +
	"`pgo drive leer <id|url> [--hoja <nombre>] [--rango A1:D20] [--max N]` (Docs en Markdown, Sheets en CSV, recorta a 40000 caracteres).)"

// driveWords: el mensaje habla de Google Drive, Docs o Sheets (o lleva una URL suya). «docs» o «sheet» sueltos no
// cuentan (la carpeta docs/, las hojas de estilo).
var driveWords = regexp.MustCompile(`(?i)\bdrive\b|\bgoogle\s+(?:docs?|sheets?|slides)\b|\b(?:docs|drive|sheets)\.google\.com\b|\b(?:documentos?|docs?|hojas?|hojas?\s+de\s+c[aá]lculo)\s+de\s+google\b`)

var (
	pgoWord = regexp.MustCompile(`(?i)\bpgo\b`)
	pgoVerb = regexp.MustCompile(`(?i)\bpgo\s+(?:ejecutar|parar|procesos|salida|abrir)\b|\b(?:ejecut|arranc|lanz|levant|parar|det[eé]n|salida|abr[eií]|procesos?|programas?|servidor)`)
)

// pgoHintFor: la línea de uso de «pgo» si el mensaje la menciona (ejecutar, parar, salida, abrir…) y la de
// «pgo drive» si habla de Google Drive, Docs o Sheets; "" si nada. Las órdenes («/pgo …», la skill de imputación)
// no cuentan.
func pgoHintFor(text string) string {
	t := strings.TrimSpace(text)
	if strings.HasPrefix(t, "/") {
		return ""
	}
	var hs []string
	if pgoWord.MatchString(t) && pgoVerb.MatchString(t) {
		hs = append(hs, pgoHint)
	}
	if driveWords.MatchString(t) {
		hs = append(hs, driveHint)
	}
	return strings.Join(hs, "\n")
}

// hasPgo: el agente tiene la orden «pgo» (su entorno lleva el token de pgocode).
func hasPgo(entorno []string) bool {
	for _, kv := range entorno {
		if strings.HasPrefix(kv, "PGO_TOKEN=") {
			return true
		}
	}
	return false
}
