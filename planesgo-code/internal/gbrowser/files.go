package gbrowser

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Los ficheros descargados: de qué tipo son y cómo se pasan a texto (o, si no son texto, dónde se guardan).

// baseType: el tipo MIME sin parámetros y en minúsculas («text/plain; charset=utf-8» → «text/plain»).
func baseType(ct string) string {
	t, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(t))
}

var textExts = map[string]bool{".txt": true, ".md": true, ".markdown": true, ".csv": true, ".tsv": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".log": true, ".ini": true, ".sql": true, ".html": true, ".htm": true,
	".js": true, ".ts": true, ".go": true, ".py": true, ".sh": true, ".css": true, ".toml": true, ".srt": true, ".vtt": true}

// fileType: xlsx | docx | eml | text | "" (lo demás), por la extensión del nombre o por el tipo.
func fileType(res Result) string {
	ext := strings.ToLower(filepath.Ext(res.FileName))
	ct := baseType(res.ContentType)
	switch {
	case ext == ".xlsx" || ct == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case ext == ".docx" || ct == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case ext == ".eml" || ct == "message/rfc822":
		return "eml"
	case textExts[ext] || strings.HasPrefix(ct, "text/") || ct == "application/json" || ct == "application/xml":
		return "text"
	}
	return ""
}

// decodeText: el texto de un fichero (sin BOM; si no es UTF-8, como Latin-1).
func decodeText(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return string(b)
	}
	return latin1(b)
}

// latin1: bytes ISO-8859-1 → texto.
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

var reUnsafeName = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// saveFile guarda un fichero que no es texto en saveDir (.pgocode/salidas de la carpeta de la sesión) para que el
// agente lo abra con sus herramientas; el contenido solo lleva su ruta, tipo y tamaño.
func saveFile(c *Content, res Result, saveDir string) error {
	c.Format, c.Text = "fichero", ""
	c.MIME, c.Size = baseType(res.ContentType), len(res.Data)
	if c.MIME == "" || c.MIME == "application/octet-stream" || c.MIME == "application/binary" {
		if t := mime.TypeByExtension(filepath.Ext(res.FileName)); t != "" {
			c.MIME = baseType(t)
		}
	}
	if saveDir == "" {
		c.Note = "no es texto y no se ha guardado (falta la carpeta de la sesión)"
		return nil
	}
	name := strings.TrimSpace(reUnsafeName.ReplaceAllString(filepath.Base(res.FileName), "_"))
	if name == "" || name == "." || name == ".." {
		name = c.ID
	}
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return err
	}
	f := filepath.Join(saveDir, "google-"+cutRunes(c.ID, 12)+"-"+name)
	if err := os.WriteFile(f, res.Data, 0o644); err != nil {
		return err
	}
	c.Path = f
	return nil
}

// ---- hojas de cálculo ----

// sheetsText: las hojas pedidas (todas, por nombre o número, con rango) en CSV, con «## hoja» delante si van varias.
func sheetsText(c *Content, all []sheetData, p ReadParams) error {
	c.Format = "csv"
	if len(all) == 0 {
		c.Note = "el libro no tiene hojas"
		return nil
	}
	chosen, err := pickSheets(all, p.Sheet, p.Range)
	if err != nil {
		return err
	}
	rng, err := parseRange(p.Range)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	multi := len(chosen) > 1
	for i, s := range chosen {
		if multi {
			if i > 0 {
				out.WriteString("\n")
			}
			fmt.Fprintf(&out, "## %s\n", s.Name)
		}
		w := csv.NewWriter(&out)
		for r, row := range s.Rows {
			if !rng.rowIn(r) {
				continue
			}
			w.Write(rng.cols(row))
			if out.Len() > 4*p.Max { // ya sobra: se recortará
				break
			}
		}
		w.Flush()
		if out.Len() > 4*p.Max {
			break
		}
	}
	if multi {
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		c.Note = "Hojas: " + strings.Join(names, ", ") + ". Para una sola: --hoja <nombre> [--rango A1:D20]."
	}
	c.Text = out.String()
	return nil
}

// pickSheets: las hojas que se leen (sin --hoja: todas; con --rango y sin --hoja: la primera).
func pickSheets(all []sheetData, sheet, rng string) ([]sheetData, error) {
	if sheet != "" {
		for _, s := range all {
			if strings.EqualFold(s.Name, sheet) {
				return []sheetData{s}, nil
			}
		}
		if n, err := strconv.Atoi(sheet); err == nil && n >= 1 && n <= len(all) {
			return []sheetData{all[n-1]}, nil
		}
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		return nil, fmt.Errorf("no hay ninguna hoja %q (hay: %s)", sheet, strings.Join(names, ", "))
	}
	if rng != "" {
		return all[:1], nil
	}
	return all, nil
}

// cellRange: un rango A1 (filas y columnas desde 0; -1: sin límite).
type cellRange struct{ r0, r1, c0, c1 int }

var reRange = regexp.MustCompile(`^([A-Za-z]*)(\d*)(?::([A-Za-z]*)(\d*))?$`)

// parseRange: «A1:D20», «B2», «A:C» o «2:10» (vacío: todo).
func parseRange(s string) (cellRange, error) {
	all := cellRange{-1, -1, -1, -1}
	s = strings.TrimSpace(s)
	if s == "" {
		return all, nil
	}
	if i := strings.LastIndexByte(s, '!'); i >= 0 { // «Hoja!A1:B2»: la hoja va con --hoja
		s = s[i+1:]
	}
	m := reRange.FindStringSubmatch(s)
	if m == nil || s == "" || (m[1] == "" && m[2] == "") {
		return all, fmt.Errorf("rango no válido %q (como A1:D20)", s)
	}
	r := all
	r.c0, r.r0 = colIndex(m[1]), rowIndex(m[2])
	if strings.Contains(s, ":") {
		r.c1, r.r1 = colIndex(m[3]), rowIndex(m[4])
	} else {
		r.c1, r.r1 = r.c0, r.r0
	}
	return r, nil
}

func colIndex(s string) int {
	if s == "" {
		return -1
	}
	n := 0
	for _, ch := range strings.ToUpper(s) {
		n = n*26 + int(ch-'A'+1)
	}
	return n - 1
}

func rowIndex(s string) int {
	if s == "" {
		return -1
	}
	n, _ := strconv.Atoi(s)
	return n - 1
}

func (r cellRange) rowIn(i int) bool { return (r.r0 < 0 || i >= r.r0) && (r.r1 < 0 || i <= r.r1) }

func (r cellRange) cols(row []string) []string {
	c0, c1 := max(r.c0, 0), len(row)-1
	if r.c1 >= 0 {
		c1 = r.c1
	}
	out := []string{}
	for i := c0; i <= c1; i++ {
		v := ""
		if i < len(row) {
			v = row[i]
		}
		out = append(out, v)
	}
	return out
}
