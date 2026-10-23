package gbrowser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Lector mínimo de XLSX y DOCX (los dos son un ZIP con XML): solo la biblioteca estándar (archive/zip y
// encoding/xml), sin dependencias. De un XLSX, las hojas con sus nombres y celdas (texto, números, booleanos y
// fechas); de un DOCX, el texto de sus párrafos.

// maxXMLPart: lo que se descomprime como mucho de cada parte del ZIP (contra los ZIP bomba).
const maxXMLPart = 64 << 20

// sheetData: una hoja (su nombre y sus filas; las celdas vacías del final, fuera).
type sheetData struct {
	Name string
	Rows [][]string
}

type zipParts map[string]*zip.File

func openZip(data []byte) (zipParts, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	parts := zipParts{}
	for _, f := range zr.File {
		parts[strings.TrimPrefix(f.Name, "/")] = f
	}
	return parts, nil
}

// open: un decodificador XML de una parte (nil si no está).
func (z zipParts) open(name string) (*xml.Decoder, io.Closer, error) {
	f, ok := z[name]
	if !ok {
		return nil, nil, nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	return xml.NewDecoder(io.LimitReader(rc, maxXMLPart)), rc, nil
}

func attr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// relID: el atributo r:id (del espacio de nombres de las relaciones, sea cual sea su prefijo).
func relID(se xml.StartElement) string {
	for _, a := range se.Attr {
		if a.Name.Local == "id" && a.Name.Space != "" {
			return a.Value
		}
	}
	return ""
}

// readXLSX: las hojas de un libro XLSX, en su orden.
func readXLSX(data []byte) ([]sheetData, error) {
	z, err := openZip(data)
	if err != nil {
		return nil, err
	}
	type sheetRef struct{ name, rid string }
	var refs []sheetRef
	date1904 := false
	d, c, err := z.open("xl/workbook.xml")
	if err != nil || d == nil {
		return nil, errors.New("no es un libro XLSX")
	}
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		if se, ok := t.(xml.StartElement); ok {
			switch se.Name.Local {
			case "sheet":
				refs = append(refs, sheetRef{attr(se, "name"), relID(se)})
			case "workbookPr":
				v := attr(se, "date1904")
				date1904 = v == "1" || v == "true"
			}
		}
	}
	c.Close()
	targets := map[string]string{}
	if d, c, _ := z.open("xl/_rels/workbook.xml.rels"); d != nil {
		for {
			t, err := d.Token()
			if err != nil {
				break
			}
			if se, ok := t.(xml.StartElement); ok && se.Name.Local == "Relationship" {
				tg := attr(se, "Target")
				if strings.HasPrefix(tg, "/") {
					tg = strings.TrimPrefix(tg, "/")
				} else {
					tg = path.Join("xl", tg)
				}
				targets[attr(se, "Id")] = tg
			}
		}
		c.Close()
	}
	shared, err := sharedStrings(z)
	if err != nil {
		return nil, err
	}
	dates := dateStyles(z)
	var out []sheetData
	for i, r := range refs {
		tg := targets[r.rid]
		if tg == "" {
			tg = fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1)
		}
		rows, err := sheetRows(z, tg, shared, dates, date1904)
		if err != nil {
			return nil, err
		}
		out = append(out, sheetData{Name: r.name, Rows: rows})
	}
	return out, nil
}

// sharedStrings: los textos compartidos (el de cada <si>, con todas sus partes <t>, sin la fonética <rPh>).
func sharedStrings(z zipParts) ([]string, error) {
	d, c, err := z.open("xl/sharedStrings.xml")
	if err != nil || d == nil {
		return nil, err
	}
	defer c.Close()
	var out []string
	var cur strings.Builder
	inT, inPh := false, false
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch x := t.(type) {
		case xml.StartElement:
			switch x.Name.Local {
			case "si":
				cur.Reset()
			case "t":
				inT = true
			case "rPh":
				inPh = true
			}
		case xml.EndElement:
			switch x.Name.Local {
			case "si":
				out = append(out, cur.String())
			case "t":
				inT = false
			case "rPh":
				inPh = false
			}
		case xml.CharData:
			if inT && !inPh {
				cur.Write(x)
			}
		}
	}
	return out, nil
}

var reDateCode = regexp.MustCompile(`[dmyhs]`)
var reFmtNoise = regexp.MustCompile(`"[^"]*"|\[[^\]]*\]|\\.`)

// dateStyles: qué estilos de celda (índice de cellXfs) son fechas u horas (y si llevan hora).
func dateStyles(z zipParts) map[int]string {
	out := map[int]string{}
	d, c, _ := z.open("xl/styles.xml")
	if d == nil {
		return out
	}
	defer c.Close()
	custom := map[int]string{}
	inXfs, idx := false, 0
	for {
		t, err := d.Token()
		if err != nil {
			break
		}
		switch x := t.(type) {
		case xml.StartElement:
			switch x.Name.Local {
			case "numFmt":
				id, _ := strconv.Atoi(attr(x, "numFmtId"))
				custom[id] = attr(x, "formatCode")
			case "cellXfs":
				inXfs = true
			case "xf":
				if inXfs {
					id, _ := strconv.Atoi(attr(x, "numFmtId"))
					if k := dateKind(id, custom[id]); k != "" {
						out[idx] = k
					}
					idx++
				}
			}
		case xml.EndElement:
			if x.Name.Local == "cellXfs" {
				inXfs = false
			}
		}
	}
	return out
}

// dateKind: «date», «datetime», «time» o "" según el formato de número.
func dateKind(id int, code string) string {
	switch {
	case id >= 14 && id <= 17:
		return "date"
	case id == 22:
		return "datetime"
	case (id >= 18 && id <= 21) || (id >= 45 && id <= 47):
		return "time"
	case code == "":
		return ""
	}
	c := strings.ToLower(reFmtNoise.ReplaceAllString(code, ""))
	if !reDateCode.MatchString(c) {
		return ""
	}
	hasDate, hasTime := strings.ContainsAny(c, "dy") || (strings.Contains(c, "m") && !strings.ContainsAny(c, "hs")), strings.ContainsAny(c, "hs")
	switch {
	case hasDate && hasTime:
		return "datetime"
	case hasTime:
		return "time"
	}
	return "date"
}

// excelDate: un número de serie de Excel como fecha (ISO).
func excelDate(v string, kind string, date1904 bool) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(f) || f < 0 || f > 2958466 {
		return v
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	t := base.Add(time.Duration(math.Round(f*86400)) * time.Second)
	switch {
	case kind == "time" && f < 1:
		return t.Format("15:04:05")
	case kind == "date" || t.Hour()+t.Minute()+t.Second() == 0:
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04:05")
}

// sheetRows: las filas de una hoja (cada celda en su columna, por su referencia A1).
func sheetRows(z zipParts, name string, shared []string, dates map[int]string, date1904 bool) ([][]string, error) {
	d, c, err := z.open(name)
	if err != nil || d == nil {
		return nil, err
	}
	defer c.Close()
	var rows [][]string
	var row []string
	rowN := -1
	var cellRef, cellType, val string
	var cellStyle int
	var inV, inT bool
	var inline strings.Builder
	set := func(col int, v string) {
		for len(row) <= col {
			row = append(row, "")
		}
		row[col] = v
	}
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch x := t.(type) {
		case xml.StartElement:
			switch x.Name.Local {
			case "row":
				n, err := strconv.Atoi(attr(x, "r"))
				if err != nil {
					n = rowN + 2
				}
				for len(rows) < n-1 {
					rows = append(rows, nil)
				}
				rowN, row = n-1, nil
			case "c":
				cellRef, cellType, val = attr(x, "r"), attr(x, "t"), ""
				cellStyle, _ = strconv.Atoi(attr(x, "s"))
				inline.Reset()
			case "v":
				inV = true
			case "t":
				inT = true
			}
		case xml.EndElement:
			switch x.Name.Local {
			case "v":
				inV = false
			case "t":
				inT = false
			case "c":
				v := val
				switch cellType {
				case "s":
					if i, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && i >= 0 && i < len(shared) {
						v = shared[i]
					}
				case "inlineStr":
					v = inline.String()
				case "b":
					v = map[string]string{"1": "TRUE", "0": "FALSE"}[strings.TrimSpace(val)]
				case "", "n":
					if k := dates[cellStyle]; k != "" && v != "" {
						v = excelDate(v, k, date1904)
					}
				}
				col := colIndex(strings.TrimRight(cellRef, "0123456789"))
				if col < 0 {
					col = len(row)
				}
				set(col, v)
			case "row":
				for len(row) > 0 && row[len(row)-1] == "" {
					row = row[:len(row)-1]
				}
				for len(rows) <= rowN {
					rows = append(rows, nil)
				}
				rows[rowN] = row
			}
		case xml.CharData:
			if inV {
				val += string(x)
			} else if inT && cellType == "inlineStr" {
				inline.Write(x)
			}
		}
	}
	for len(rows) > 0 && len(rows[len(rows)-1]) == 0 {
		rows = rows[:len(rows)-1]
	}
	return rows, nil
}

// docxText: el texto de un documento de Word (un párrafo por línea; las celdas de una tabla, separadas por « | »).
func docxText(data []byte) (string, error) {
	z, err := openZip(data)
	if err != nil {
		return "", err
	}
	d, c, err := z.open("word/document.xml")
	if err != nil || d == nil {
		return "", errors.New("no es un documento de Word")
	}
	defer c.Close()
	var b bytes.Buffer
	trim := func(suffix string) {
		for bytes.HasSuffix(b.Bytes(), []byte(suffix)) {
			b.Truncate(b.Len() - len(suffix))
		}
	}
	inT := false
	for {
		t, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch x := t.(type) {
		case xml.StartElement:
			switch x.Name.Local {
			case "t":
				inT = true
			case "tab":
				b.WriteString("\t")
			case "br", "cr":
				b.WriteString("\n")
			}
		case xml.EndElement:
			switch x.Name.Local {
			case "t":
				inT = false
			case "p":
				b.WriteString("\n")
			case "tc":
				trim("\n")
				b.WriteString(" | ")
			case "tr":
				trim(" | ")
				b.WriteString("\n")
			}
		case xml.CharData:
			if inT {
				b.Write(x)
			}
		}
	}
	return tidyText(b.String()), nil
}
