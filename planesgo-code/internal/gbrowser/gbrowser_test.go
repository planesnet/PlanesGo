package gbrowser

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDesktop hace de Electron: recoge cada descarga del canal y responde con h. Devuelve el lector y los enlaces
// pedidos.
func fakeDesktop(t *testing.T, h func(Job) Result) (*Reader, func() []string) {
	t.Helper()
	b := NewBridge()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var mu sync.Mutex
	var jobs []string
	go func() {
		for {
			j, ok := b.Next(ctx)
			if !ok {
				return
			}
			mu.Lock()
			jobs = append(jobs, j.URL)
			mu.Unlock()
			r := h(j)
			r.ID = j.ID
			b.Deliver(r)
		}
	}()
	for !b.Available() {
		time.Sleep(time.Millisecond)
	}
	return New(b), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), jobs...)
	}
}

// file: una descarga completa (como la da Google: Content-Disposition: attachment).
func file(name, ct string, data []byte) Result {
	return Result{Download: true, FileName: name, ContentType: ct, Disposition: `attachment; filename="` + name + `"`, Data: data}
}

// page: Google responde con una página (de error, de «solicitar acceso»…), no con un fichero.
var page = Result{Download: true, FileName: "export.html", ContentType: "text/html; charset=utf-8", Data: []byte("<html>Solicitar acceso</html>")}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for n, c := range files {
		f, _ := w.Create(n)
		f.Write([]byte(c))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// testXLSX: un libro con dos hojas (textos compartidos y en línea, números, booleanos y una fecha con estilo).
func testXLSX(t *testing.T) []byte {
	return zipOf(t, map[string]string{
		"xl/workbook.xml": `<?xml version="1.0"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` +
			`<sheet name="Ventas" sheetId="1" r:id="rId1"/><sheet name="Gastos, 2026" sheetId="2" r:id="rId2"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="x" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="x" Target="/xl/worksheets/hoja2.xml"/></Relationships>`,
		"xl/sharedStrings.xml": `<sst><si><t>Cliente</t></si><si><t>Importe</t></si><si><r><t>Acme</t></r><r><t xml:space="preserve">, S.A.</t></r></si></sst>`,
		"xl/styles.xml":        `<styleSheet><numFmts><numFmt numFmtId="164" formatCode="dd/mm/yyyy"/></numFmts><cellXfs><xf numFmtId="0"/><xf numFmtId="164"/></cellXfs></styleSheet>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData>` +
			`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Fecha</t></is></c></row>` +
			`<row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>1234.5</v></c><c r="C2" s="1"><v>46307</v></c></row>` +
			`<row r="4"><c r="B4" t="b"><v>1</v></c></row></sheetData></worksheet>`,
		"xl/worksheets/hoja2.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>Luz</t></is></c><c r="B1"><v>80</v></c></row></sheetData></worksheet>`,
	})
}

func TestBridge(t *testing.T) {
	b := NewBridge()
	if _, err := b.Do(context.Background(), Job{URL: "https://docs.google.com/x"}); !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("sin Electron: %v", err)
	}
	if b.Deliver(Result{ID: "nadie"}) {
		t.Fatal("entrega sin espera")
	}
	b.lastPoll = time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Do(ctx, Job{URL: "https://docs.google.com/viejo"}); err == nil {
		t.Fatal("contexto cancelado")
	}
	go func() {
		c, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		j, ok := b.Next(c)
		if !ok || j.URL != "https://docs.google.com/nuevo" || j.TimeoutMs <= 0 || j.ID == "" {
			t.Errorf("trabajo: %+v %v", j, ok)
			return
		}
		b.Deliver(Result{ID: j.ID, Data: []byte("hola")})
	}()
	r, err := b.Do(context.Background(), Job{URL: "https://docs.google.com/nuevo"})
	if err != nil || string(r.Data) != "hola" {
		t.Fatalf("%+v %v", r, err)
	}
	b.now = func() time.Time { return time.Now().Add(2 * presence) }
	if b.Available() {
		t.Fatal("presencia caducada")
	}
}

func TestParseRef(t *testing.T) {
	for in, want := range map[string]Ref{
		"1AbCdEfGhIjKlMnOp": {ID: "1AbCdEfGhIjKlMnOp"},
		"https://docs.google.com/document/d/1AbCdEfGhIjK/edit?tab=t.0":             {ID: "1AbCdEfGhIjK", Kind: KindDoc},
		"https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit#gid=123":          {ID: "1SheetIdXyz", Kind: KindSheet, Gid: "123"},
		"https://docs.google.com/presentation/u/1/d/1SlidesIdXy/edit":              {ID: "1SlidesIdXy", Kind: KindSlides},
		"https://drive.google.com/drive/u/0/folders/1FolderIdXy":                   {ID: "1FolderIdXy", Kind: KindFolder},
		"https://drive.google.com/file/d/1FileIdXyzw/view":                         {ID: "1FileIdXyzw", Kind: KindFile},
		"https://drive.google.com/open?id=1OpenIdXyzw":                             {ID: "1OpenIdXyzw"},
		"https://docs.google.com/spreadsheets/d/e/2PACX-1vPublicado/pubhtml?gid=7": {ID: "2PACX-1vPublicado", Kind: KindSheet, Gid: "7"},
	} {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("%s: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "corto", "https://example.com/document/d/1AbCdEfGhIjK", "https://docs.google.com/"} {
		if _, err := ParseRef(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

func TestReadDoc(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result {
		return file("Informe de ventas.docx.md", "text/x-markdown; charset=utf-8", []byte("\xef\xbb\xbf# Ventas\n\nTodo bien.\n"))
	})
	c, err := r.Read(context.Background(), 3, ReadParams{Ref: "https://docs.google.com/document/d/1DocIdXyzw/edit"})
	if err != nil || c.Format != "markdown" || c.Name != "Informe de ventas" || c.Text != "# Ventas\n\nTodo bien.\n" {
		t.Fatalf("%+v %v", c, err)
	}
	if got := jobs(); len(got) != 1 || got[0] != "https://docs.google.com/document/d/1DocIdXyzw/export?format=md" {
		t.Fatalf("%v", got)
	}
	if out := FormatContent(c); !strings.HasPrefix(out, "# Informe de ventas [doc · markdown · 1DocIdXyzw]\n# Ventas") {
		t.Fatalf("%q", out)
	}
}

// Sin Markdown, el texto; si tampoco se descarga (Google da una página), error claro: nunca se lee la página.
func TestReadDocFallbackAndNoDownload(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result {
		if strings.HasSuffix(j.URL, "format=txt") {
			return file("Notas.txt", "text/plain", []byte("hola"))
		}
		return Result{Error: "descarga interrupted"}
	})
	c, err := r.Read(context.Background(), 0, ReadParams{Ref: "https://docs.google.com/document/d/1DocIdXyzw/edit"})
	if err != nil || c.Format != "texto" || c.Text != "hola" || len(jobs()) != 2 {
		t.Fatalf("%+v %v %v", c, err, jobs())
	}
	r, jobs = fakeDesktop(t, func(Job) Result { return page })
	_, err = r.Read(context.Background(), 0, ReadParams{Ref: "https://docs.google.com/document/d/1DocIdXyzw/edit"})
	if !errors.Is(err, ErrNoDownload) || !strings.Contains(err.Error(), "¿tienes acceso") || !strings.Contains(err.Error(), "panel Web") {
		t.Fatalf("%v", err)
	}
	if got := jobs(); len(got) != 2 || !strings.HasSuffix(got[1], "/export?format=txt") {
		t.Fatalf("solo descargas: %v", got)
	}
}

func TestReadLoginAndFolder(t *testing.T) {
	r, jobs := fakeDesktop(t, func(Job) Result { return Result{Login: true, URL: "https://accounts.google.com/ServiceLogin"} })
	if _, err := r.Read(context.Background(), 0, ReadParams{Ref: "1DocIdXyzwAbc"}); !errors.Is(err, ErrLogin) || len(jobs()) != 1 {
		t.Fatalf("%v %v", err, jobs())
	}
	if _, err := r.Read(context.Background(), 0, ReadParams{Ref: "https://drive.google.com/drive/folders/1FolderIdXy"}); !errors.Is(err, ErrFolder) || len(jobs()) != 1 {
		t.Fatalf("carpeta: %v", err)
	}
}

func TestReadSheetXLSX(t *testing.T) {
	x := testXLSX(t)
	r, jobs := fakeDesktop(t, func(j Job) Result {
		if strings.HasSuffix(j.URL, "/export?format=xlsx") {
			return file("Cuentas.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", x)
		}
		return page
	})
	ctx := context.Background()
	c, err := r.Read(ctx, 0, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit"})
	if err != nil || c.Name != "Cuentas" || c.Text != "## Ventas\nCliente,Importe,Fecha\n\"Acme, S.A.\",1234.5,2026-10-12\n\n,TRUE\n\n## Gastos, 2026\nLuz,80\n" ||
		c.Note != "Hojas: Ventas, Gastos, 2026. Para una sola: --hoja <nombre> [--rango A1:D20]." {
		t.Fatalf("%q %q %v", c.Text, c.Note, err)
	}
	if got := jobs(); len(got) != 1 || got[0] != "https://docs.google.com/spreadsheets/d/1SheetIdXyz/export?format=xlsx" {
		t.Fatalf("%v", got)
	}
	c, err = r.Read(ctx, 0, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit", Sheet: "gastos, 2026"})
	if err != nil || c.Text != "Luz,80\n" || c.Note != "" {
		t.Fatalf("hoja: %q %v", c.Text, err)
	}
	// Con --hoja manda la hoja pedida (aunque el enlace lleve otra), y el rango se aplica aquí.
	c, err = r.Read(ctx, 0, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit#gid=5", Sheet: "1", Range: "B1:C2"})
	if err != nil || c.Text != "Importe,Fecha\n1234.5,2026-10-12\n" {
		t.Fatalf("rango: %q %v", c.Text, err)
	}
	if _, err := r.Read(ctx, 0, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit", Sheet: "Nada"}); err == nil || !strings.Contains(err.Error(), "hay: Ventas, Gastos, 2026") {
		t.Fatalf("hoja que no hay: %v", err)
	}
}

// Con la hoja del enlace (#gid=), esa hoja en CSV por su enlace de exportación (con el rango, si se pide).
func TestReadSheetGid(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result { return file("Cuentas - Gastos.csv", "text/csv", []byte("a,b\n1,2\n")) })
	c, err := r.Read(context.Background(), 0, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit#gid=42", Range: "A1:B2"})
	if err != nil || c.Text != "a,b\n1,2\n" || c.Format != "csv" {
		t.Fatalf("%+v %v", c, err)
	}
	if got := jobs(); got[0] != "https://docs.google.com/spreadsheets/d/1SheetIdXyz/export?format=csv&gid=42&range=A1%3AB2" {
		t.Fatalf("%v", got)
	}
}

func TestReadSlidesAndTruncate(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result { return file("Plan.txt", "text/plain", []byte(strings.Repeat("ñ", 50))) })
	c, err := r.Read(context.Background(), 0, ReadParams{Ref: "https://docs.google.com/presentation/d/1SlidesIdXy/edit", Max: 10})
	if err != nil || c.Text != strings.Repeat("ñ", 10) || !c.Truncated || c.Total != 50 || jobs()[0] != "https://docs.google.com/presentation/d/1SlidesIdXy/export/txt" {
		t.Fatalf("%+v %v %v", c, err, jobs())
	}
	if out := FormatContent(c); !strings.Contains(out, "[recortado: 10 caracteres de 50;") {
		t.Fatalf("%q", out)
	}
}

// Un ID suelto: se prueban las descargas de cada tipo; un PDF de Drive se guarda en la carpeta de la sesión.
func TestReadBareIDFile(t *testing.T) {
	pdf := []byte("%PDF-1.4 binario\x00\x01")
	r, jobs := fakeDesktop(t, func(j Job) Result {
		if strings.Contains(j.URL, "/uc?export=download&id=") {
			return file("Contrato firmado.pdf", "application/pdf", pdf)
		}
		return Result{Error: "descarga interrupted"}
	})
	dir := filepath.Join(t.TempDir(), ".pgocode", "salidas")
	c, err := r.Read(context.Background(), 0, ReadParams{Ref: "1FileIdXyzwAbc", SaveDir: dir})
	if err != nil || c.Kind != KindFile || c.Format != "fichero" || c.MIME != "application/pdf" || c.Size != len(pdf) || c.Name != "Contrato firmado" {
		t.Fatalf("%+v %v", c, err)
	}
	want := []string{
		"https://docs.google.com/document/d/1FileIdXyzwAbc/export?format=md",
		"https://docs.google.com/document/d/1FileIdXyzwAbc/export?format=txt",
		"https://docs.google.com/spreadsheets/d/1FileIdXyzwAbc/export?format=xlsx",
		"https://docs.google.com/presentation/d/1FileIdXyzwAbc/export/txt",
		"https://drive.google.com/uc?export=download&id=1FileIdXyzwAbc",
	}
	if got := jobs(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%v", got)
	}
	if b, err := os.ReadFile(c.Path); err != nil || !bytes.Equal(b, pdf) || filepath.Dir(c.Path) != dir {
		t.Fatalf("%s %v", c.Path, err)
	}
	if out := FormatContent(c); !strings.Contains(out, "Descargado en "+c.Path+" (application/pdf, 18 B)") {
		t.Fatalf("%q", out)
	}
	// Ninguna descarga: error claro.
	r, _ = fakeDesktop(t, func(Job) Result { return page })
	if _, err := r.Read(context.Background(), 0, ReadParams{Ref: "1FileIdXyzwAbc"}); err != ErrNoDownload {
		t.Fatalf("%v", err)
	}
}

// Ficheros de Drive que son texto, Word o Excel: su texto.
func TestReadDriveFiles(t *testing.T) {
	docx := zipOf(t, map[string]string{"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Hola</w:t></w:r><w:r><w:tab/><w:t xml:space="preserve">mundo</w:t></w:r></w:p>` +
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>a</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>b</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:body></w:document>`})
	files := map[string]Result{
		"1TextoIdXyz": file("notas.md", "application/octet-stream", []byte("# Notas")),
		"1WordIdXyzw": file("Acta.docx", "application/octet-stream", docx),
		"1ExcelIdXyz": file("Datos.xlsx", "application/octet-stream", testXLSX(t)),
	}
	r, _ := fakeDesktop(t, func(j Job) Result { return files[j.URL[strings.LastIndex(j.URL, "=")+1:]] })
	ctx := context.Background()
	c, err := r.Read(ctx, 0, ReadParams{Ref: "https://drive.google.com/file/d/1TextoIdXyz/view"})
	if err != nil || c.Text != "# Notas" || c.Format != "texto" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = r.Read(ctx, 0, ReadParams{Ref: "https://drive.google.com/file/d/1WordIdXyzw/view"})
	if err != nil || c.Text != "Hola\tmundo\na | b" {
		t.Fatalf("%q %v", c.Text, err)
	}
	c, err = r.Read(ctx, 0, ReadParams{Ref: "https://drive.google.com/file/d/1ExcelIdXyz/view", Sheet: "2"})
	if err != nil || c.Text != "Luz,80\n" || c.Format != "csv" {
		t.Fatalf("%q %v", c.Text, err)
	}
}

func TestParseRange(t *testing.T) {
	for in, want := range map[string]cellRange{"": {-1, -1, -1, -1}, "A1:D20": {0, 19, 0, 3}, "B2": {1, 1, 1, 1}, "A:C": {-1, -1, 0, 2},
		"2:10": {1, 9, -1, -1}, "Hoja!AA1:AB2": {0, 1, 26, 27}} {
		if got, err := parseRange(in); err != nil || got != want {
			t.Errorf("%q: %+v %v", in, got, err)
		}
	}
	if _, err := parseRange("¿?"); err == nil {
		t.Error("rango no válido")
	}
}

func TestParseMailLink(t *testing.T) {
	// Un ID nuevo de verdad empieza por FMfcgz… (el texto «f:<número>»).
	tok := encodeMailToken("f:1777712345678901234")
	if !strings.HasPrefix(tok, "FMfcgz") || decodeMailToken(tok) != "f:1777712345678901234" {
		t.Fatalf("%s %q", tok, decodeMailToken(tok))
	}
	const id = "18abb3f5ad046ff2"
	for in, want := range map[string]mailLink{
		"https://mail.google.com/mail/u/0/#inbox/" + tok:                                {"0", id},
		"https://mail.google.com/mail/u/1/#search/factura/" + tok + "?compose=new":      {"1", id},
		"https://mail.google.com/mail/u/0/#inbox/18ABB3F5AD046FF2":                      {"0", id},
		"https://mail.google.com/mail/u/2/?ui=2&view=pt&th=" + id:                       {"2", id},
		"https://mail.google.com/mail/u/0/?view=om&permmsgid=msg-f:1777712345678901234": {"0", id},
	} {
		if got, err := ParseMailLink(in); err != nil || got != want {
			t.Errorf("%s: %+v %v", in, got, err)
		}
	}
	for _, bad := range []string{id, "https://mail.google.com/mail/u/0/#inbox", "https://evil.com/mail/u/0/#inbox/" + id,
		"https://mail.google.com/mail/u/0/#sent/" + encodeMailToken("a:r-123456789")} {
		if _, err := ParseMailLink(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

const testEML = "From: =?UTF-8?Q?Ana_Mart=C3=ADnez?= <ana@x.com>\r\nTo: luis@x.com\r\nSubject: =?UTF-8?B?RmFjdHVyYSBkZSBvY3R1YnJl?=\r\n" +
	"Date: Mon, 12 Oct 2026 10:00:00 +0200\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=\"MIX\"\r\n\r\n" +
	"--MIX\r\nContent-Type: multipart/alternative; boundary=\"ALT\"\r\n\r\n" +
	"--ALT\r\nContent-Type: text/html; charset=\"iso-8859-1\"\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
	"<html><head><style>p{}</style></head><body><p>Hola Luis,</p><p>Te env=EDo la factura &amp; el albar=E1n.</p>" +
	"<div class=3D\"gmail_quote\"><div>El lun, Ana escribi=F3:</div><blockquote>lo de antes</blockquote></div></body></html>\r\n" +
	"--ALT--\r\n" +
	"--MIX\r\nContent-Type: application/pdf; name=\"factura.pdf\"\r\nContent-Disposition: attachment; filename=\"factura.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
	"JVBERi0xLjQK\r\nAAAA\r\n--MIX--\r\n"

func TestReadMail(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result { return file("Factura de octubre.eml", "message/rfc822", []byte(testEML)) })
	m, err := r.ReadMail(context.Background(), 0, "https://mail.google.com/mail/u/1/#inbox/18abb3f5ad046ff2", 0)
	if err != nil || m.Subject != "Factura de octubre" || m.From != "Ana Martínez <ana@x.com>" || m.Text != "Hola Luis,\n\nTe envío la factura & el albarán." ||
		len(m.Attachments) != 1 || m.Attachments[0] != (Attachment{"factura.pdf", "application/pdf", 12}) {
		t.Fatalf("%+v %v", m, err)
	}
	if got := jobs(); got[0] != "https://mail.google.com/mail/u/1/?attid=0&disp=comp&safe=1&th=18abb3f5ad046ff2&ui=2&view=att&zw" {
		t.Fatalf("%v", got)
	}
	out := FormatMailContent(m)
	for _, s := range []string{"# Factura de octubre [correo 18abb3f5ad046ff2]\n", "De: Ana Martínez <ana@x.com>\n", "Adjuntos: factura.pdf (application/pdf, 12 B)"} {
		if !strings.Contains(out, s) {
			t.Fatalf("%q sin %q", out, s)
		}
	}
	// Recorte y texto sin formato con lo citado.
	plain := "From: a@x.com\r\nSubject: Re: hola\r\n\r\nVale, de acuerdo.\r\n\r\nOn Mon, Oct 12, 2026 at 10:00 Ana <ana@x.com> wrote:\r\n> antes\r\n"
	r, _ = fakeDesktop(t, func(j Job) Result { return file("x.eml", "message/rfc822", []byte(plain)) })
	if m, err = r.ReadMail(context.Background(), 0, "https://mail.google.com/mail/u/0/#inbox/18abb3f5ad046ff2", 4); err != nil || m.Text != "Vale […]" || !m.Truncated {
		t.Fatalf("%+v %v", m, err)
	}
	// Sin sesión o si Gmail no da el mensaje (una página): errores claros.
	r, _ = fakeDesktop(t, func(j Job) Result { return Result{Login: true} })
	if _, err := r.ReadMail(context.Background(), 0, "https://mail.google.com/mail/u/0/#inbox/18abb3f5ad046ff2", 0); !errors.Is(err, ErrLogin) {
		t.Fatal(err)
	}
	r, _ = fakeDesktop(t, func(j Job) Result { return page })
	if _, err := r.ReadMail(context.Background(), 0, "https://mail.google.com/mail/u/0/#inbox/18abb3f5ad046ff2", 0); !errors.Is(err, ErrNoDownload) {
		t.Fatal(err)
	}
}

func TestHTMLToText(t *testing.T) {
	got := htmlToText(`<div>Uno<br>dos</div><script>x()</script><table><tr><td>a</td><td>b</td></tr></table><!-- nada --><p>&lt;fin&gt;</p>`)
	if got != "Uno\ndos\n\na b\n\n<fin>" {
		t.Fatalf("%q", got)
	}
}

func TestStripQuoted(t *testing.T) {
	s, ok := stripQuoted("Vale.\n\nOn Mon, Oct 12, 2026 at 10:00 Ana <ana@x.com> wrote:\n> antes")
	if !ok || s != "Vale." {
		t.Fatalf("%q", s)
	}
	s, ok = stripQuoted("Mira esto.\n---------- Forwarded message ---------\nDe: X")
	if ok || !strings.Contains(s, "Forwarded") {
		t.Fatalf("lo reenviado se queda: %q", s)
	}
}

func TestRemoveOAuthLeftovers(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "google-oauth-client.json"), []byte("{}"), 0o600)
	os.MkdirAll(filepath.Join(d, "google"), 0o700)
	os.WriteFile(filepath.Join(d, "google", "luis.json"), []byte("{}"), 0o600)
	os.WriteFile(filepath.Join(d, "studio.json"), []byte("{}"), 0o600)
	got := RemoveOAuthLeftovers(d)
	if len(got) != 3 {
		t.Fatalf("%v", got)
	}
	if _, err := os.Stat(filepath.Join(d, "google")); !os.IsNotExist(err) {
		t.Fatal("la carpeta google/ sigue")
	}
	if _, err := os.Stat(filepath.Join(d, "studio.json")); err != nil {
		t.Fatal("ha borrado otra cosa")
	}
	os.MkdirAll(filepath.Join(d, "google"), 0o700)
	os.WriteFile(filepath.Join(d, "google", "nota.txt"), []byte("x"), 0o600)
	if got := RemoveOAuthLeftovers(d); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if _, err := os.Stat(filepath.Join(d, "google", "nota.txt")); err != nil {
		t.Fatal("ha borrado lo que no era suyo")
	}
	if RemoveOAuthLeftovers("") != nil {
		t.Fatal("sin carpeta")
	}
}
