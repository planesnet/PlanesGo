package memory

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Reglas para sacar de la conversación lo que merece quedarse (sin modelo). Se prefiere la precisión: una memoria
// con ruido cuesta tokens en cada sesión nueva y despista al agente; lo que se escape, el usuario lo puede añadir.

// maxFactsPerTurn: lo más que entra de un mensaje (un texto pegado no llena la memoria).
const maxFactsPerTurn = 3

// durable: frases con una decisión, una convención o un dato del proyecto que sigue valiendo en otras sesiones.
var durable = []*regexp.Regexp{
	// Reglas explícitas: «nunca…», «siempre…», «importante: …», «a partir de ahora…», «hemos decidido…».
	regexp.MustCompile(`(?i)^(nunca|siempre|jam[aá]s|importante|recuerda|ten\s+en\s+cuenta|regla|convenci[oó]n|norma|a\s+partir\s+de\s+ahora|de\s+ahora\s+en\s+adelante|en\s+este\s+proyecto|hemos\s+decidido|he\s+decidido|decisi[oó]n|never|always|important|from\s+now\s+on|we\s+decided)\b`),
	regexp.MustCompile(`(?i)\b(siempre|nunca|jam[aá]s|always|never)\b`),
	regexp.MustCompile(`(?i)\bpor\s+defecto\b`),
	// Puertos y direcciones.
	regexp.MustCompile(`(?i)\b(puerto|port)\b[^.]{0,20}?\b\d{2,5}\b|\blocalhost:\d{2,5}\b`),
	// «el IVA se redondea con…», «se despliega con…», «se compila con…».
	regexp.MustCompile(`(?i)\bse\s+(redondea|calcula|ejecuta|lanza|arranca|compila|prueba|despliega|instala|configura|guarda|almacena|nombra)n?\b`),
	// «las pruebas: make test», «para probar, …», «los tests con pytest».
	regexp.MustCompile(`(?i)^(las\s+pruebas|los\s+tests?|el\s+test|para\s+(probar|compilar|ejecutar|arrancar|desplegar|lanzar))\b.*(:|\bcon\b|\bse\b|\bson\b)`),
	// «ejecuta odoo con odt -d intradema»: cómo se hace algo del proyecto.
	// Lo de después de «con» tiene que parecer una orden (entre comillas invertidas, con opciones o con una ruta):
	// «lanza el servidor con más memoria» no lo es.
	regexp.MustCompile(`(?i)\b(ejecuta|lanza|arranca|compila|prueba|despliega|instala)r?\s+(el\s+|la\s+|los\s+|las\s+)?[\wáéíóú.-]+(\s+[\wáéíóú.-]+)?\s+con\s+(` + "`[^`]+`" + `|[\w.-]+(\s+[\w.-]+)?\s+--?\w|\S*[./_]\S*)`),
	// «la base de datos de pruebas se llama intradema_test», «la rama principal es dev-lp»: el valor parece un
	// identificador (con _ . / : - o dígitos), no una opinión («la ruta es incorrecta»).
	regexp.MustCompile(`(?i)\b(base\s+de\s+datos|bd|rama|usuario|url|servidor|dominio|carpeta|ruta|fichero|entorno|versi[oó]n|tabla|m[oó]dulo)\b[^.]{0,40}?\b(es|son|se\s+llama|se\s+llaman|ser[aá])\s+[` + "`" + `"«]?[\w./:@-]*[_./:@\d-][\w./:@-]*`),
}

// transient: lo que parece un problema o una petición del momento (aunque diga «siempre»: «siempre falla el login»).
var transient = regexp.MustCompile(`(?i)\b(falla|fallan|fallo|no\s+funciona|error|errores|roto|rota|bug|peta|cuelga|ahora\s+mismo|de\s+momento|por\s+ahora|este\s+mensaje|esta\s+vez)\b`)

// question: preguntas (no son datos).
var question = regexp.MustCompile(`(?i)^(¿|qu[eé]\b|c[oó]mo\b|cu[aá]l|cu[aá]nt|d[oó]nde\b|por\s+qu[eé]\b|puedes\b|podr[ií]as\b|sabes\b|eres\s+capaz|hay\b|what\b|how\b|why\b|can\s+you\b|is\s+there\b)`)

var fence = regexp.MustCompile("(?s)```.*?```")

// Facts: las frases del mensaje del usuario que merecen ir a la memoria (como mucho maxFactsPerTurn), recortadas.
func Facts(text string) []string {
	text = fence.ReplaceAllString(text, "\n")
	var out []string
	for _, s := range sentences(text) {
		if len(out) >= maxFactsPerTurn {
			break
		}
		if f, ok := fact(s); ok {
			out = append(out, f)
		}
	}
	return out
}

func fact(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimLeft(s, "-*•> \t"))
	n := utf8.RuneCountInString(s)
	if n < 10 || n > 240 || strings.Contains(s, "?") || question.MatchString(s) || transient.MatchString(s) || looksPasted(s) {
		return "", false
	}
	for _, r := range durable {
		if r.MatchString(s) {
			// «Contexto del proyecto: las pruebas…»: la etiqueta sobra si lo de detrás ya es un dato.
			if m := label.FindStringSubmatch(s); m != nil {
				if f, ok := fact(m[1]); ok {
					return f, true
				}
			}
			s = strings.TrimRight(s, ".;: ")
			r0, size := utf8.DecodeRuneInString(s)
			return string(unicode.ToUpper(r0)) + s[size:], true
		}
	}
	return "", false
}

var label = regexp.MustCompile(`^[\pL ]{3,30}:\s+(.+)$`)

// looksPasted: un trozo de registro o de código pegado (muchos símbolos), no una frase.
func looksPasted(s string) bool {
	sym := 0
	for _, r := range s {
		if strings.ContainsRune("{}[]()<>=;|\\$#", r) {
			sym++
		}
	}
	return sym > 6
}

var sentenceEnd = regexp.MustCompile(`([.!?])\s+`)

func sentences(t string) []string {
	var out []string
	for _, l := range strings.Split(t, "\n") {
		for _, s := range strings.Split(sentenceEnd.ReplaceAllString(l, "$1\n"), "\n") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// ---- Órdenes del proyecto ----

// CommandEntry: una orden del proyecto que ha funcionado (pruebas, compilación, comprobaciones…).
type CommandEntry struct {
	Kind string // pruebas | compilar | comprobar
	Cmd  string
}

func (c CommandEntry) String() string { return c.Kind + ": `" + c.Cmd + "`" }

// head: el programa y su primera palabra («go test», «npm run»): las órdenes con la misma cabeza se sustituyen.
func (c CommandEntry) head() string {
	f := strings.Fields(c.Cmd)
	if len(f) > 2 {
		f = f[:2]
	}
	return strings.Join(f, " ")
}

var (
	commandTool = regexp.MustCompile(`(?i)^(bash|shell|run|exec|run_command|terminal)`)
	kindTest    = regexp.MustCompile(`(?i)(\b(test|tests|prueba|pruebas|check|pytest|jest|vitest|mocha|phpunit|rspec|unittest)\b|--test-enable)`)
	kindCheck   = regexp.MustCompile(`(?i)\b(lint|eslint|ruff|mypy|vet|flake8|golangci-lint|tsc|typecheck|prettier|gofmt)\b`)
	kindBuild   = regexp.MustCompile(`(?i)\b(build|compile|make|cargo\s+build|mvn|gradle)\b`)
	// Una prueba concreta (de un fichero, una función), no la orden del proyecto.
	specific = regexp.MustCompile(`(?i)(\s-run[\s=]|\s-k\s|--grep|::|\.test\.|\.spec\.|_test\.go|test_\w+\.py|--testNamePattern|\s-t\s)`)
	cdPrefix = regexp.MustCompile(`^(cd\s+("[^"]*"|'[^']*'|\S+)\s*(&&|;)\s*)+`)
	// Lo que va detrás y no es la orden: redirecciones, tuberías a head/tail/grep, «; echo …».
	tail     = regexp.MustCompile(`(\s*(2>&1|>\s*/dev/null|\|\s*(tail|head|grep|tee|less|cat)\b.*|;\s*echo\b.*|&&\s*echo\b.*))+$`)
	envOrTim = regexp.MustCompile(`^(timeout\s+\d+[smh]?\s+|(\w+=\S+\s+)+)`)
)

// Command: si una orden que el agente ha ejecutado con éxito es de las del proyecto (pruebas, compilación,
// comprobaciones), su entrada en la memoria; si es de un solo uso o demasiado concreta, no.
func Command(tool, cmd string) (CommandEntry, bool) {
	if !commandTool.MatchString(tool) {
		return CommandEntry{}, false
	}
	c := strings.TrimSpace(cmd)
	if strings.ContainsAny(c, "\n") || strings.Contains(c, "<<") || strings.Contains(c, "$(") {
		return CommandEntry{}, false
	}
	c = cdPrefix.ReplaceAllString(c, "")
	c = strings.TrimSpace(tail.ReplaceAllString(c, ""))
	c = strings.TrimSpace(envOrTim.ReplaceAllString(c, ""))
	if c == "" || utf8.RuneCountInString(c) > 80 || specific.MatchString(" "+c) || strings.ContainsAny(c, "|;") || strings.Count(c, "&&") > 1 {
		return CommandEntry{}, false
	}
	kind := ""
	switch {
	case kindTest.MatchString(c):
		kind = "pruebas"
	case kindCheck.MatchString(c):
		kind = "comprobar"
	case kindBuild.MatchString(c):
		kind = "compilar"
	default:
		return CommandEntry{}, false
	}
	return CommandEntry{Kind: kind, Cmd: c}, true
}
