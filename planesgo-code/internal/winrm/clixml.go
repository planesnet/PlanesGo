package winrm

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
)

// A powershell.exe without a console writes its error, warning and information streams to stderr as CLIXML
// («#< CLIXML» and an <Objs> document). cleanErrors turns it into the plain text PowerShell would have shown.

var clixmlEscape = regexp.MustCompile(`_x([0-9A-Fa-f]{4})_`)

// streamPrefix: how each stream is shown (the error one, as it is; progress records are dropped).
var streamPrefix = map[string]string{"error": "", "warning": "ADVERTENCIA: ", "verbose": "DETALLE: ", "debug": "DEPURACIÓN: ", "information": ""}

func cleanErrors(s string) string {
	body, ok := strings.CutPrefix(strings.TrimLeft(s, "\ufeff \r\n"), "#< CLIXML")
	if !ok {
		return s
	}
	var b strings.Builder
	d := xml.NewDecoder(strings.NewReader(body))
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		st, ok := tok.(xml.StartElement)
		if !ok || st.Name.Local != "S" {
			continue
		}
		stream := ""
		for _, a := range st.Attr {
			if a.Name.Local == "S" {
				stream = strings.ToLower(a.Value)
			}
		}
		var text string
		if d.DecodeElement(&text, &st) != nil {
			break
		}
		prefix, shown := streamPrefix[stream]
		if !shown {
			continue
		}
		text = clixmlEscape.ReplaceAllStringFunc(text, func(m string) string {
			n, _ := strconv.ParseUint(m[2:6], 16, 32)
			return string(rune(n))
		})
		if prefix != "" && strings.TrimSpace(text) != "" {
			text = prefix + text
		}
		b.WriteString(text)
	}
	return strings.ReplaceAll(b.String(), "\r\n", "\n")
}
