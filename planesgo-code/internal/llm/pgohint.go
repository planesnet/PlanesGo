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

// driveHint: el uso de «pgo drive» (Google Drive, Docs y Sheets en solo lectura y por enlace).
const driveHint = "(pgocode: para Google Drive/Docs/Sheets/Slides usa la orden de terminal «pgo» (solo lectura, por enlace: descarga el fichero con la sesión de Google del navegador de pgocode; no busca ni lista): " +
	"`pgo drive leer <enlace o id> [--hoja <nombre>] [--rango A1:D20] [--max N]` (Docs en Markdown, Sheets en CSV; PDF y demás, a .pgocode/salidas; recorta a 40000 caracteres).)"

// driveWords: el mensaje habla de Google Drive, Docs o Sheets (o lleva una URL suya). «docs» o «sheet» sueltos no
// cuentan (la carpeta docs/, las hojas de estilo).
var driveWords = regexp.MustCompile(`(?i)\bdrive\b|\bgoogle\s+(?:docs?|sheets?|slides)\b|\b(?:docs|drive|sheets)\.google\.com\b|\b(?:documentos?|docs?|hojas?|hojas?\s+de\s+c[aá]lculo)\s+de\s+google\b`)

// gmailHint: el uso de «pgo gmail» (Gmail en solo lectura y por enlace).
const gmailHint = "(pgocode: para leer un correo de Gmail usa la orden de terminal «pgo» (solo lectura, con la sesión de Google del navegador de pgocode; no busca, envía ni borra nada): " +
	"`pgo gmail leer <enlace del correo> [--max N]` (el texto sin lo citado y los adjuntos; recorta a 20000 caracteres). Sin enlace, pídeselo al usuario.)"

// gmailWords: el mensaje habla del correo del usuario (Gmail, correos, email, la bandeja…).
var gmailWords = regexp.MustCompile(`(?i)\bgmail\b|\bcorreos?\b|\be-?mails?\b|\bbandeja\b|\binbox\b|\bbuz[oó]n\b|\bmail\.google\.com\b`)

// mailNoise: «email» o «correo» como dato de un programa (un campo, una columna, user.email…), no el correo del
// usuario: se quita antes de buscar gmailWords.
var mailNoise = regexp.MustCompile(`(?i)\b(?:campos?|field|columnas?|column|atributos?|propiedad|variables?|par[aá]metros?|validaci[oó]n|valida[rd]?|formato|tipo|plantillas?)\s+(?:(?:el|la|los|las|un|una|de|del|de\s+la)\s+)?(?:correo(?:\s+electr[oó]nico)?|e-?mail)\b|[\w)\]]\.e-?mail\b|\be-?mail\s*[(:=]`)

// sqlHint: el uso de «pgo sql» (el SQL Server de la sesión, si la persona ha definido su conexión).
const sqlHint = "(pgocode: para consultar SQL Server usa la orden de terminal «pgo» con la conexión de esta sesión (la define la persona; tú no ves la contraseña): " +
	"`pgo sql \"<T-SQL>\" [--filas N]` o `pgo sql -f <fichero.sql>` (resultados separados por tabuladores; solo lectura: lo que cambie se deshace; " +
	"`--escribir` solo si la persona ha permitido cambios). Si dice que la sesión no tiene conexión, pídesela al usuario.)"

// sqlWords: el mensaje habla de SQL Server, de una base de datos o de Sage.
var sqlWords = regexp.MustCompile(`(?i)\bsql\b|\bt-sql\b|\bmssql\b|\bsage\b|\bbases?\s+de\s+datos\b|\bconsultas?\s+sql\b`)

// desktopHint: how to use «pgo escritorio» (the session's agent desktop, if the person has started it).
const desktopHint = "(pgocode: para manejar el escritorio remoto (Windows por RDP) usa la orden de terminal «pgo» con el escritorio del agente de esta sesión (lo inicia la persona; tú no ves la contraseña): " +
	"`pgo escritorio captura [--escala 0.6]` (te da la ruta de la imagen: léela), `pgo escritorio clic X Y [--doble|--derecho]`, `mueve X Y`, `arrastra X1 Y1 X2 Y2` (coordenadas de la última captura), " +
	"`rueda arriba|abajo [N]`, `escribe \"texto\"`, `tecla ctrl+s Return`, `tamaño 1920x1080`, `graba inicio|fin`, `estado`. Si dice que la sesión no tiene escritorio, pídeselo al usuario.)"

// desktopWords: the message is about a remote desktop, RDP or the screen of a Windows computer.
var desktopWords = regexp.MustCompile(`(?is)\bescritorio\s+(?:remoto|del\s+agente)\b|\brdp\b|\bwindows\b.*\b(?:pantalla|captura|ventana|clic)|\b(?:pantalla|captura|clic)\b.*\bwindows\b`)

var (
	pgoWord = regexp.MustCompile(`(?i)\bpgo\b`)
	pgoVerb = regexp.MustCompile(`(?i)\bpgo\s+(?:ejecutar|parar|procesos|salida|abrir)\b|\b(?:ejecut|arranc|lanz|levant|parar|det[eé]n|salida|abr[eií]|procesos?|programas?|servidor)`)
)

// pgoHintFor: la línea de uso de «pgo» si el mensaje la menciona (ejecutar, parar, salida, abrir…) y la de
// «pgo drive» si habla de Google Drive, Docs o Sheets, la de «pgo gmail» si habla del correo y la de «pgo sql» si habla
// de SQL Server, bases de datos o Sage y la de «pgo escritorio» si habla de un escritorio remoto o de la pantalla de Windows; "" si nada. Las órdenes («/pgo …», la skill de imputación)
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
	if gmailWords.MatchString(mailNoise.ReplaceAllString(t, " ")) {
		hs = append(hs, gmailHint)
	}
	if sqlWords.MatchString(t) {
		hs = append(hs, sqlHint)
	}
	if desktopWords.MatchString(t) {
		hs = append(hs, desktopHint)
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
