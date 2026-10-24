package winrm

import (
	"fmt"
	"regexp"
	"strings"
)

// Read only, as far as it can be checked before running: PowerShell has no transactions, so a read-only script is
// limited to what reads. ReadOnlyCheck reads the script as PowerShell does (strings, here-strings, comments,
// subexpressions inside strings, script blocks, hashtables) and refuses it unless:
//
//   - every command is a query one: Get-*, Test-*, Select-*, Where-*, Measure-*, Format-*, Sort-*, Group-*,
//     Compare-*, ConvertTo-*, ConvertFrom-*, Resolve-*, Out-String/Null/Host/Default, Write-Output/Host/…, a few
//     more (Join-Path, Split-Path, Start-Sleep, Import-Csv, Set-Location…), their usual aliases and whoami,
//     hostname, systeminfo, tasklist and netstat; ForEach-Object only with a { block } (its member form calls
//     methods). No paths to scripts or programs, no «&» call operator, no dot-sourcing, no Invoke-Expression;
//   - .NET is only read: methods from a short list of harmless ones (ToString, Split, Substring, AddDays, GetValue…),
//     static methods only of value types ([math], [datetime], [convert], [string], [regex]…), no dynamic members;
//   - nothing is assigned but variables ($x = …, $h['k'] = …; never $obj.Prop = …) and nothing is redirected to a
//     file (only > $null and 2>&1);
//   - no class, enum, using, attributes or typographic quotes and dashes (PowerShell reads them as the ASCII ones,
//     which would hide words from this check).
//
// Then the script runs in ConstrainedLanguage mode. The limits: a Get-*/Test-* command of a third-party module may
// change something, a property getter could too, and reading is not limited (secrets the user can read, can be
// read). JEA endpoints would be the server-side guarantee, but they need the PowerShell remoting protocol (PSRP),
// which the WinRM library does not speak: for a guarantee, use a Windows user with read-only rights.

const maxDepth = 100

// ReadOnlyCheck: why a script cannot run in a read-only connection ("" if it can).
func ReadOnlyCheck(script string) string {
	if strings.ContainsAny(script, "‘’‚‛“”„–—―") {
		return "en solo lectura no se admiten comillas tipográficas ni guiones largos: usa ' \" y -"
	}
	p := &roScanner{s: script}
	return p.scan(0, false)
}

// token kinds, to know what a «.», «[», «=» or «++» applies to.
type tokenKind int

const (
	tNone  tokenKind = iota
	tValue           // a string, number, (…), {…}, [type] or member
	tVar             // a variable
	tOp              // an operator, a keyword, «|»
	tComma
	tWord // a bareword argument or command name
)

type roScanner struct {
	s     string
	i     int
	depth int
}

// roStatement: the state of the statement being read in one context ((…), {…}, @{…}, […] or the script).
type roStatement struct {
	cmdPos    bool // a bareword here is a command name
	argMode   bool // inside a command's arguments: barewords are strings
	start     int  // where the statement (or the list item) starts, for the assignment target
	hash      bool // a hashtable literal
	hashKey   bool // at a hashtable key
	last      tokenKind
	lastEnd   int
	simpleVar bool // the last token is a plain variable ($i, for $i++)
	skipName  bool // the next bareword is a function name
}

func (st *roStatement) reset(i int) {
	st.cmdPos, st.argMode, st.start, st.hashKey, st.last = !st.hash, false, i, st.hash, tNone
}

// value: a value has been read (in an expression it ends the command position).
func (st *roStatement) value(k tokenKind, end int) {
	if !st.argMode {
		st.cmdPos = false
	}
	st.last, st.lastEnd, st.simpleVar = k, end, false
}

func (p *roScanner) peek(n int) byte {
	if p.i+n < len(p.s) {
		return p.s[p.i+n]
	}
	return 0
}

// scan reads up to close (0: the end of the script); hash: a hashtable literal.
func (p *roScanner) scan(close byte, hash bool) string {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > maxDepth {
		return "la orden está demasiado anidada"
	}
	st := &roStatement{hash: hash}
	st.reset(p.i)
	for p.i < len(p.s) {
		c := p.s[p.i]
		var why string
		switch {
		case c == ' ' || c == '\t' || c == '\f' || c == '\v':
			p.i++
		case c == '`' && (p.peek(1) == '\n' || p.peek(1) == '\r'): // line continuation
			p.i += 2
			if p.s[p.i-1] == '\r' && p.peek(0) == '\n' {
				p.i++
			}
		case c == '\n' || c == '\r':
			p.i++
			if st.last != tOp && st.last != tComma && close != ']' {
				st.reset(p.i)
			}
		case c == ';':
			p.i++
			st.reset(p.i)
		case c == '#':
			for p.i < len(p.s) && p.s[p.i] != '\n' && p.s[p.i] != '\r' {
				p.i++
			}
		case c == '<' && p.peek(1) == '#':
			end := strings.Index(p.s[p.i+2:], "#>")
			if end < 0 {
				return "comentario <# sin cerrar"
			}
			p.i += end + 4
		case c == '|' || c == '&':
			if p.peek(1) == c { // && and ||: another pipeline
				p.i += 2
				st.reset(p.i)
				continue
			}
			if c == '&' {
				return "el operador & (llamada) no se admite en solo lectura"
			}
			p.i++
			st.cmdPos, st.argMode, st.start, st.last = true, false, p.i, tOp
		case c == ')' || c == '}' || c == ']':
			if c != close {
				return "paréntesis, corchetes o llaves sin emparejar"
			}
			p.i++
			return ""
		case c == '(':
			p.i++
			why = p.nested(st, ')', false)
		case c == '{':
			p.i++
			why = p.nested(st, '}', false)
		case c == '$':
			why = p.dollar(st)
		case c == '@':
			why = p.at(st)
		case c == '\'':
			why = p.singleQuoted(st)
		case c == '"':
			why = p.doubleQuoted(st, false)
		case c == '[':
			why = p.bracket(st)
		case c == '.':
			why = p.dot(st)
		case c == ':' && p.peek(1) == ':':
			return "«::» solo se admite tras un tipo: [Tipo]::Miembro"
		case c == '=':
			why = p.assign(st)
		case c == '-' || c == '+':
			why = p.sign(st)
		case c == '>':
			why = p.redirect(st)
		case c == ',':
			p.i++
			st.last = tComma
			if !st.argMode {
				st.start = p.i
			}
		case (c == '%' || c == '?') && st.cmdPos && !st.argMode && !st.hashKey: // ForEach-Object, Where-Object
			p.i++
			why = p.command(st, string(c))
		case isDigit(c):
			for p.i < len(p.s) && (isWordByte(p.s[p.i]) || p.s[p.i] == '.' && isDigit(p.peek(1))) {
				p.i++
			}
			st.value(tValue, p.i)
		case strings.IndexByte("*/!<%?:", c) >= 0:
			p.i++
			st.last = tOp
		default:
			why = p.word(st)
		}
		if why != "" {
			return why
		}
	}
	if close != 0 {
		return "paréntesis, corchetes o llaves sin cerrar"
	}
	return ""
}

// nested reads a (…), {…}, @(…), $(…), @{…} or index […] and counts it as a value of the outer statement.
func (p *roScanner) nested(st *roStatement, close byte, hash bool) string {
	if why := p.scan(close, hash); why != "" {
		return why
	}
	st.value(tValue, p.i)
	return ""
}

func (p *roScanner) dollar(st *roStatement) string {
	p.i++
	switch c := p.peek(0); {
	case c == '(':
		p.i++
		return p.nested(st, ')', false)
	case c == '{': // ${any name}: reading is harmless; it is never a valid assignment target
		for p.i++; p.i < len(p.s) && p.s[p.i] != '}'; p.i++ {
			if p.s[p.i] == '`' {
				p.i++
			}
		}
		if p.i >= len(p.s) {
			return "variable ${ sin cerrar"
		}
		p.i++
		st.value(tVar, p.i)
		return ""
	case c == '_' || c == '$' || c == '?' || c == '^':
		p.i++
	default:
		p.ident()
		if p.peek(0) == ':' && isWordByte(p.peek(1)) { // $env:X, $script:x
			p.i++
			p.ident()
		}
	}
	st.value(tVar, p.i)
	st.simpleVar = true
	return ""
}

func (p *roScanner) at(st *roStatement) string {
	switch p.peek(1) {
	case '(':
		p.i += 2
		return p.nested(st, ')', false)
	case '{':
		p.i += 2
		return p.nested(st, '}', true)
	case '\'':
		end := hereStringEnd(p.s, p.i+2, '\'')
		if end < 0 {
			return "texto @' sin cerrar"
		}
		p.i = end
		st.value(tValue, p.i)
		return ""
	case '"':
		return p.doubleQuoted(st, true)
	}
	p.i++ // @args: splatting
	p.ident()
	st.value(tVar, p.i)
	return ""
}

// hereStringEnd: the position after the '@ or "@ that closes a here-string whose text starts at from (-1: none).
func hereStringEnd(s string, from int, quote byte) int {
	for i := from; i+1 < len(s); i++ {
		if (s[i] == '\n' || s[i] == '\r') && s[i+1] == quote && i+2 < len(s) && s[i+2] == '@' {
			return i + 3
		}
	}
	return -1
}

func (p *roScanner) singleQuoted(st *roStatement) string {
	for p.i++; p.i < len(p.s); p.i++ {
		if p.s[p.i] == '\'' {
			if p.peek(1) == '\'' {
				p.i++
				continue
			}
			p.i++
			st.value(tValue, p.i)
			return ""
		}
	}
	return "texto entre comillas sin cerrar"
}

// doubleQuoted: "…" or @"…"@; their $(…) run code, so they are read as code.
func (p *roScanner) doubleQuoted(st *roStatement, here bool) string {
	end := -1
	if here {
		end = hereStringEnd(p.s, p.i+2, '"')
		if end < 0 {
			return "texto @\" sin cerrar"
		}
		p.i += 2
	} else {
		p.i++
	}
	for p.i < len(p.s) {
		switch c := p.s[p.i]; {
		case here && p.i == end-3:
			p.i = end
			st.value(tValue, p.i)
			return ""
		case c == '`':
			p.i += 2
		case c == '"' && !here:
			if p.peek(1) == '"' {
				p.i += 2
				continue
			}
			p.i++
			st.value(tValue, p.i)
			return ""
		case c == '$' && p.peek(1) == '(':
			p.i += 2
			if why := p.scan(')', false); why != "" {
				return why
			}
			if here && p.i > end-3 {
				return "texto @\" mal cerrado"
			}
		default:
			p.i++
		}
	}
	return "texto entre comillas sin cerrar"
}

// bracket: an index ($a[0], (…)[1]) or a type ([int], [math]::Round(…)).
func (p *roScanner) bracket(st *roStatement) string {
	if st.lastEnd == p.i && (st.last == tValue || st.last == tVar) {
		p.i++
		return p.nested(st, ']', false)
	}
	if st.argMode && st.last == tWord && st.lastEnd == p.i {
		return p.word(st)
	}
	depth, j := 0, p.i
	for ; j < len(p.s); j++ {
		if p.s[j] == '[' {
			depth++
		} else if p.s[j] == ']' {
			if depth--; depth == 0 {
				break
			}
		}
	}
	if j >= len(p.s) {
		return "corchete [ sin cerrar"
	}
	content := p.s[p.i+1 : j]
	if strings.ContainsAny(content, "({$'\"=") {
		return "los atributos [Nombre(…)] no se admiten en solo lectura"
	}
	typ := strings.TrimPrefix(strings.ToLower(strings.Join(strings.Fields(content), "")), "system.")
	p.i = j + 1
	if p.peek(0) == ':' && p.peek(1) == ':' {
		p.i += 2
		name := p.ident()
		if name == "" {
			return "falta el miembro tras «::»"
		}
		if p.peek(0) == '(' && !staticTypes[typ] {
			return fmt.Sprintf("[%s]::%s() no se admite en solo lectura (solo métodos de tipos de valor como [math] o [datetime])", content, name)
		}
	}
	st.value(tValue, p.i)
	return ""
}

// dot: a member ($x.Name, $x.Method(…)), a range (1..9), a path (.\x) or dot-sourcing (. script).
func (p *roScanner) dot(st *roStatement) string {
	if p.peek(1) == '.' {
		p.i += 2
		st.last = tOp
		return ""
	}
	if st.lastEnd == p.i && (st.last == tValue || st.last == tVar) {
		p.i++
		if c := p.peek(0); c == '$' || c == '(' || c == '\'' || c == '"' {
			return "el acceso dinámico a miembros ($x.$nombre) no se admite en solo lectura"
		}
		name := p.ident()
		if name == "" {
			return "falta el miembro tras «.»"
		}
		if p.peek(0) == '(' && !allowedMethods[strings.ToLower(name)] {
			return fmt.Sprintf("el método .%s() no se admite en solo lectura (solo los que leen, como .ToString() o .Split())", name)
		}
		st.value(tValue, p.i)
		return ""
	}
	if c := p.peek(1); st.argMode || isWordByte(c) || c == '\\' || c == '/' {
		return p.word(st)
	}
	return "ejecutar un script con «.» (dot-sourcing) no se admite en solo lectura"
}

// assignTarget: what a read-only script can assign: variables (with a type, an index or several at once).
var assignTarget = regexp.MustCompile(`(?i)^(?:\[[\w.\[\], ` + "`" + `]+\]\s*)*\$(?:(?:global|script|local|private|env|using):)?\w+(?:\s*\[[^\]]*\])?$`)

func (p *roScanner) assign(st *roStatement) string {
	p.i++
	switch {
	case st.hashKey:
		st.hashKey = false
	case st.argMode: // in a command's arguments «=» is just text
		return ""
	default:
		target := strings.TrimRight(strings.TrimSpace(p.s[st.start:p.i-1]), "+-*/%? \t")
		if !assignTarget.MatchString(target) {
			return "en solo lectura solo se asigna a variables ($x = …), no a propiedades ni a otros destinos"
		}
	}
	st.cmdPos, st.last = true, tOp
	return ""
}

var prefixVar = regexp.MustCompile(`^\$\w+`)

// sign: «-» and «+»: parameters, operators, numbers, ++ and --.
func (p *roScanner) sign(st *roStatement) string {
	c := p.s[p.i]
	if c == '-' && st.argMode && p.peek(1) == '-' && p.peek(2) == '%' { // --%: the rest of the line is text
		for p.i < len(p.s) && p.s[p.i] != '\n' && p.s[p.i] != '\r' {
			p.i++
		}
		return ""
	}
	if p.peek(1) == c && !st.argMode { // ++ and --: only on a plain variable
		post := st.last == tVar && st.simpleVar && st.lastEnd == p.i
		m := prefixVar.FindString(p.s[p.i+2:])
		pre := m != "" && !strings.ContainsRune(".[", rune(at(p.s, p.i+2+len(m))))
		if !post && !pre {
			return "en solo lectura «++» y «--» solo se usan con variables ($i++)"
		}
		p.i += 2
		return ""
	}
	if c == '-' && isLetter(p.peek(1)) {
		if st.argMode { // a parameter
			return p.word(st)
		}
		p.i++
		p.ident() // an operator: -eq, -not, -f…
		st.last = tOp
		return ""
	}
	p.i++
	st.last = tOp
	return ""
}

func at(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

var harmlessRedirect = regexp.MustCompile(`(?i)^>{1,2}(?:&[12]|\s*\$null\b)`)

func (p *roScanner) redirect(st *roStatement) string {
	m := harmlessRedirect.FindString(p.s[p.i:])
	if m == "" {
		return "redirigir la salida a un fichero (>) no se admite en solo lectura (sí > $null y 2>&1)"
	}
	p.i += len(m)
	st.last = tOp
	return ""
}

// word: a bareword: a command name, a keyword, a hashtable key or a command's argument.
func (p *roScanner) word(st *roStatement) string {
	w := p.bareword(st.argMode)
	if w == "" {
		p.i++ // a byte that is nothing else: skip it
		return ""
	}
	end := p.i
	switch {
	case st.argMode:
		st.last, st.lastEnd = tWord, end
		return ""
	case st.hashKey:
		st.value(tValue, end)
		return ""
	case st.skipName:
		st.skipName = false
		st.last, st.lastEnd = tWord, end
		return ""
	}
	lw := strings.ToLower(w)
	if lw == "foreach" && p.next() == '(' {
		lw = "foreach-statement"
	}
	if strings.HasPrefix(lw, ":") && len(lw) > 1 { // a loop label
		return ""
	}
	if kw, ok := keywords[lw]; ok {
		if !kw {
			return fmt.Sprintf("«%s» no se admite en solo lectura", w)
		}
		st.cmdPos, st.argMode, st.last = true, false, tOp
		st.skipName = lw == "function" || lw == "filter"
		return ""
	}
	return p.command(st, w)
}

// command checks a command name and starts its arguments.
func (p *roScanner) command(st *roStatement, name string) string {
	if why := checkCommand(name); why != "" {
		return why
	}
	if canon := canonical(name); canon == "foreach-object" && !blockFollows.MatchString(p.s[p.i:]) {
		return "ForEach-Object solo se admite con un bloque { … } en solo lectura"
	}
	st.cmdPos, st.argMode, st.last, st.lastEnd = false, true, tWord, p.i
	return ""
}

var blockFollows = regexp.MustCompile(`(?i)^\s*(?:-(?:begin|process|end)\s*)?\{`)

// next: the next character that is not a space.
func (p *roScanner) next() byte {
	for j := p.i; j < len(p.s); j++ {
		if p.s[j] != ' ' && p.s[j] != '\t' {
			return p.s[j]
		}
	}
	return 0
}

// bareword reads a word as PowerShell does (a backtick escapes the next character). It stops where code can start
// ($, (, {), at quotes and separators; outside arguments also at = [ ].
func (p *roScanner) bareword(arg bool) string {
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '`' {
			if n := p.peek(1); n == 0 || n == '\n' || n == '\r' {
				break
			}
			b.WriteByte(p.s[p.i+1])
			p.i += 2
			continue
		}
		if strings.IndexByte(" \t\f\v\r\n;|&(){}'\",$><", c) >= 0 || !arg && strings.IndexByte("=[]", c) >= 0 {
			break
		}
		b.WriteByte(c)
		p.i++
	}
	return b.String()
}

func (p *roScanner) ident() string {
	start := p.i
	for p.i < len(p.s) && isWordByte(p.s[p.i]) {
		p.i++
	}
	return p.s[start:p.i]
}

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isLetter(c byte) bool   { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isWordByte(c byte) bool { return isLetter(c) || isDigit(c) || c == '_' }

// keywords: true, allowed (their next word is checked again as a command); false, refused.
var keywords = map[string]bool{
	"if": true, "elseif": true, "else": true, "switch": true, "foreach-statement": true, "for": true, "while": true,
	"do": true, "until": true, "try": true, "catch": true, "finally": true, "return": true, "break": true,
	"continue": true, "throw": true, "exit": true, "param": true, "begin": true, "process": true, "end": true,
	"in": true, "default": true, "trap": true, "function": true, "filter": true,
	"class": false, "enum": false, "using": false, "data": false, "dynamicparam": false, "workflow": false,
	"configuration": false, "parallel": false, "sequence": false, "inlinescript": false,
}

// queryVerbs: the verbs of commands that only read (their nouns are free).
var queryVerbs = []string{"get-", "test-", "select-", "where-", "measure-", "format-", "sort-", "group-", "compare-",
	"convertto-", "convertfrom-", "resolve-"}

// queryCommands: other harmless commands, and the native programs that only show information.
var queryCommands = map[string]bool{
	"out-string": true, "out-null": true, "out-host": true, "out-default": true, "write-output": true,
	"write-host": true, "write-verbose": true, "write-warning": true, "write-error": true, "write-debug": true,
	"write-information": true, "write-progress": true, "join-path": true, "split-path": true, "start-sleep": true,
	"new-timespan": true, "import-csv": true, "set-location": true, "push-location": true, "pop-location": true,
	"foreach-object": true,
	"whoami":         true, "hostname": true, "systeminfo": true, "tasklist": true, "netstat": true,
}

// deniedCommands: query verbs that can change things anyway.
var deniedCommands = map[string]bool{"test-computersecurechannel": true}

// aliases: the usual short names of the allowed commands (any other alias is refused).
var aliases = map[string]string{
	"ls": "get-childitem", "dir": "get-childitem", "gci": "get-childitem", "cat": "get-content", "gc": "get-content",
	"type": "get-content", "gi": "get-item", "gp": "get-itemproperty", "gpv": "get-itempropertyvalue",
	"gps": "get-process", "ps": "get-process", "gsv": "get-service", "gwmi": "get-wmiobject", "gcim": "get-ciminstance",
	"gcm": "get-command", "gm": "get-member", "gv": "get-variable", "gal": "get-alias", "gdr": "get-psdrive",
	"gu": "get-unique", "gl": "get-location", "pwd": "get-location", "h": "get-history", "history": "get-history",
	"ghy": "get-history", "gin": "get-computerinfo", "gtz": "get-timezone", "select": "select-object",
	"where": "where-object", "?": "where-object", "sort": "sort-object", "ft": "format-table", "fl": "format-list",
	"fw": "format-wide", "fc": "format-custom", "measure": "measure-object", "group": "group-object",
	"sls": "select-string", "diff": "compare-object", "compare": "compare-object", "echo": "write-output",
	"write": "write-output", "foreach": "foreach-object", "%": "foreach-object", "cd": "set-location",
	"chdir": "set-location", "sl": "set-location", "pushd": "push-location", "popd": "pop-location",
	"sleep": "start-sleep",
}

func canonical(name string) string {
	n := strings.TrimSuffix(strings.ToLower(name), ".exe")
	if a, ok := aliases[n]; ok {
		return a
	}
	return n
}

// checkCommand: why a command cannot run in a read-only connection ("" if it can).
func checkCommand(name string) string {
	if strings.ContainsAny(name, `\/`) || strings.HasPrefix(name, ".") {
		return fmt.Sprintf("«%s»: en solo lectura no se ejecutan scripts ni programas por su ruta", name)
	}
	n := canonical(name)
	if strings.Contains(n, ".") {
		return fmt.Sprintf("«%s»: en solo lectura no se ejecutan scripts ni programas por su ruta", name)
	}
	if deniedCommands[n] {
		return fmt.Sprintf("«%s» puede cambiar el equipo: no se admite en solo lectura", name)
	}
	if queryCommands[n] {
		return ""
	}
	for _, v := range queryVerbs {
		if strings.HasPrefix(n, v) && len(n) > len(v) {
			return ""
		}
	}
	return fmt.Sprintf("«%s» no se admite en solo lectura: solo órdenes de consulta (Get-*, Test-*, Select-*, Where-*, Measure-*, Format-*, Sort-*, ConvertTo-*…). Para cambiar algo, la conexión tiene que ser actualizable y la orden, de escritura", name)
}

// allowedMethods: .NET methods that only compute or read (lower case). Anything that adds, removes, sets, saves,
// deletes, kills or invokes is out.
var allowedMethods = setOf("tostring", "tolower", "toupper", "tolowerinvariant", "toupperinvariant", "trim",
	"trimstart", "trimend", "split", "replace", "substring", "contains", "startswith", "endswith", "indexof",
	"indexofany", "lastindexof", "padleft", "padright", "equals", "compareto", "gettype", "gethashcode", "getbytes",
	"getstring", "toshortdatestring", "tolongdatestring", "toshorttimestring", "tolongtimestring", "adddays",
	"addhours", "addminutes", "addseconds", "addmilliseconds", "addmonths", "addyears", "addticks", "subtract",
	"touniversaltime", "tolocaltime", "tochararray", "toarray", "normalize", "getvalue", "getvaluenames",
	"getsubkeynames", "getvaluekind", "containskey", "containsvalue", "getenumerator", "getfiles", "getdirectories",
	"getfilesysteminfos", "getaccessrules", "getowner", "match", "matches", "ismatch", "isnullorempty",
	"isnullorwhitespace", "format", "join", "concat", "compare", "round", "floor", "ceiling", "abs", "max", "min",
	"pow", "sqrt", "truncate", "parse", "tryparse", "frombase64string", "tobase64string", "toint32", "toint64",
	"todouble", "todecimal", "toboolean", "todatetime", "isdigit", "isletter", "iswhitespace", "getlength",
	"getupperbound", "getlowerbound")

// staticTypes: the types whose static methods can be called when read only ([math]::Round, [datetime]::Parse…).
var staticTypes = setOf("math", "datetime", "timespan", "string", "char", "int", "int32", "int64", "long", "double",
	"decimal", "bool", "boolean", "guid", "convert", "regex", "bitconverter", "text.encoding", "uri", "version")

func setOf(l ...string) map[string]bool {
	m := make(map[string]bool, len(l))
	for _, x := range l {
		m[x] = true
	}
	return m
}
