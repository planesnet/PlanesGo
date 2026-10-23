package gbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Límites: lo que se lee va al contexto de un agente; mejor recortar (con aviso) que meter megas.
const (
	DefaultMaxChars = 40_000  // ~10k tokens
	MaxMaxChars     = 200_000 // tope aunque se pida más
)

// Tipos de fichero (los de los enlaces de Google).
const (
	KindDoc    = "doc"
	KindSheet  = "hoja"
	KindSlides = "presentación"
	KindFolder = "carpeta"
	KindFile   = "fichero"
)

// Errores de lectura.
var (
	ErrLogin = errors.New("no hay sesión de Google en el navegador de pgocode: inicia sesión en Google en el panel Web " +
		"de pgocode (se ha abierto accounts.google.com en él) y vuelve a pedirlo")
	ErrNoDownload = errors.New("no se ha podido descargar: ¿tienes acceso a ese fichero? ¿has iniciado sesión en Google " +
		"en el panel Web de pgocode?")
	ErrFolder = errors.New("una carpeta de Drive no se descarga: pide el enlace de un fichero de la carpeta")
)

// noDownload: ErrNoDownload con el detalle (si lo hay).
func noDownload(detail string) error {
	if detail = strings.TrimSpace(detail); detail != "" {
		return fmt.Errorf("%w (%s)", ErrNoDownload, detail)
	}
	return ErrNoDownload
}

// Bases: dónde está cada servicio de Google (fijas; las pruebas ponen un servidor local).
type Bases struct {
	Docs  string // https://docs.google.com
	Drive string // https://drive.google.com
	Mail  string // https://mail.google.com
}

// GoogleBases: las de verdad.
var GoogleBases = Bases{Docs: "https://docs.google.com", Drive: "https://drive.google.com", Mail: "https://mail.google.com"}

// Reader lee Google descargando ficheros a través del navegador de pgocode.
type Reader struct {
	Bridge *Bridge
	Bases  Bases
}

// New: un lector con las direcciones de Google.
func New(b *Bridge) *Reader { return &Reader{Bridge: b, Bases: GoogleBases} }

// download: el fichero de un enlace de descarga, con los errores comunes traducidos (sesión, sin descarga). Lo que
// no es una descarga de verdad (Google da una página: de error, de «solicitar acceso», de aviso…) no se lee.
func (r *Reader) download(ctx context.Context, project int, u string) (Result, error) {
	res, err := r.Bridge.Do(ctx, Job{URL: u, Project: project})
	switch {
	case err != nil:
		return res, err
	case res.Login:
		return res, ErrLogin
	case res.Error != "" || !res.Download:
		return res, noDownload(res.Error)
	case !isAttachment(res):
		return res, noDownload("Google ha respondido con una página, no con el fichero")
	}
	return res, nil
}

// isAttachment: la respuesta es un fichero que se descarga (Content-Disposition: attachment) y no una página.
func isAttachment(res Result) bool {
	d := strings.ToLower(strings.TrimSpace(res.Disposition))
	if strings.HasPrefix(d, "attachment") {
		return true
	}
	return d == "" && baseType(res.ContentType) != "text/html"
}

// ---- referencias ----

// Ref: un fichero de Google (ID y, si el enlace lo dice, tipo y hoja).
type Ref struct {
	ID   string
	Kind string // "" si no se sabe (un ID suelto)
	Gid  string
}

var (
	reKindPath = regexp.MustCompile(`/(document|spreadsheets|presentation|forms|file)(?:/u/\d+)?/d/(?:e/)?([A-Za-z0-9_-]{10,})`)
	reFolder   = regexp.MustCompile(`/folders/([A-Za-z0-9_-]{10,})`)
	reIDInPath = regexp.MustCompile(`/d/([A-Za-z0-9_-]{10,})`)
	reID       = regexp.MustCompile(`^[A-Za-z0-9_-]{10,}$`)
	reGid      = regexp.MustCompile(`[#&?]gid=(\d+)`)
)

var pathKinds = map[string]string{"document": KindDoc, "spreadsheets": KindSheet, "presentation": KindSlides, "forms": KindFile, "file": KindFile}

// ParseRef saca el ID (y el tipo y la hoja, si el enlace los lleva) de un ID o un enlace de Drive, Docs, Sheets o
// Slides.
func ParseRef(ref string) (Ref, error) {
	ref = strings.TrimSpace(ref)
	if reID.MatchString(ref) {
		return Ref{ID: ref}, nil
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.HasSuffix(u.Hostname(), "google.com") {
		return Ref{}, fmt.Errorf("no es un ID ni un enlace de Google Drive/Docs/Sheets: %q", ref)
	}
	var r Ref
	if g := reGid.FindStringSubmatch(ref); g != nil {
		r.Gid = g[1]
	}
	switch {
	case reKindPath.MatchString(u.Path):
		m := reKindPath.FindStringSubmatch(u.Path)
		r.Kind, r.ID = pathKinds[m[1]], m[2]
	case reFolder.MatchString(u.Path):
		r.Kind, r.ID = KindFolder, reFolder.FindStringSubmatch(u.Path)[1]
	case reIDInPath.MatchString(u.Path):
		r.ID = reIDInPath.FindStringSubmatch(u.Path)[1]
	case reID.MatchString(u.Query().Get("id")):
		r.ID = u.Query().Get("id")
	default:
		return Ref{}, fmt.Errorf("no se encuentra el ID del fichero en el enlace %q", ref)
	}
	return r, nil
}

// ---- lectura ----

// ReadParams: qué leer.
type ReadParams struct {
	Ref     string // ID o enlace
	Sheet   string // hoja (por nombre o número, desde 1); vacía: todas (o la del enlace)
	Range   string // rango A1 (A1:D20)
	Max     int    // caracteres como mucho (0: DefaultMaxChars)
	SaveDir string // dónde se guardan los ficheros que no son texto (.pgocode/salidas de la carpeta de la sesión)
}

// Content: lo leído.
type Content struct {
	ID        string
	Name      string
	Kind      string
	Format    string // markdown | texto | csv | correo | fichero
	Text      string
	Truncated bool
	Total     int    // caracteres que había cuando se recorta
	Note      string // aviso
	Path      string // el fichero guardado (los que no son texto)
	MIME      string
	Size      int
}

func clampChars(n int) int {
	if n <= 0 {
		return DefaultMaxChars
	}
	return min(n, MaxMaxChars)
}

// Read descarga un fichero por su enlace de descarga o exportación y lo lee: Docs en Markdown (o texto), Sheets en
// CSV, Slides en texto y los ficheros de Drive según su tipo (texto, Word, Excel, correo .eml; lo demás se guarda y
// se da su ruta). Un ID suelto: se prueban las descargas de cada tipo, en orden.
func (r *Reader) Read(ctx context.Context, project int, p ReadParams) (Content, error) {
	ref, err := ParseRef(p.Ref)
	if err != nil {
		return Content{}, err
	}
	if ref.Kind == KindFolder {
		return Content{}, ErrFolder
	}
	p.Max = clampChars(p.Max)
	var c Content
	if ref.Kind != "" {
		c, err = r.readKind(ctx, project, ref, p)
	} else {
		for _, k := range []string{KindDoc, KindSheet, KindSlides, KindFile} {
			ref.Kind = k
			c, err = r.readKind(ctx, project, ref, p)
			if err == nil || !errors.Is(err, ErrNoDownload) {
				break
			}
		}
		if errors.Is(err, ErrNoDownload) {
			err = ErrNoDownload // el detalle del último intento no dice nada del fichero
		}
	}
	if err != nil {
		return c, err
	}
	c.Text = strings.ToValidUTF8(c.Text, "")
	if n := utf8.RuneCountInString(c.Text); n > p.Max {
		if !c.Truncated {
			c.Total = n
		}
		c.Text, c.Truncated = cutRunes(c.Text, p.Max), true
	}
	return c, nil
}

func (r *Reader) readKind(ctx context.Context, project int, ref Ref, p ReadParams) (Content, error) {
	c := Content{ID: ref.ID, Kind: ref.Kind}
	var err error
	switch ref.Kind {
	case KindDoc:
		err = r.readDoc(ctx, project, &c)
	case KindSheet:
		err = r.readSheet(ctx, project, &c, ref.Gid, p)
	case KindSlides:
		var res Result
		if res, err = r.download(ctx, project, r.Bases.Docs+"/presentation/d/"+ref.ID+"/export/txt"); err == nil {
			c.Format, c.Name, c.Text = "texto", nameOf(res), decodeText(res.Data)
		}
	default:
		var res Result
		if res, err = r.download(ctx, project, r.Bases.Drive+"/uc?export=download&id="+url.QueryEscape(ref.ID)); err == nil {
			err = readFile(&c, res, p)
		}
	}
	return c, err
}

// readDoc: el documento exportado a Markdown (como «Archivo → Descargar → Markdown»); si no, a texto.
func (r *Reader) readDoc(ctx context.Context, project int, c *Content) error {
	base := r.Bases.Docs + "/document/d/" + c.ID + "/export?format="
	var err error
	for _, f := range []struct{ ext, format string }{{"md", "markdown"}, {"txt", "texto"}} {
		var res Result
		if res, err = r.download(ctx, project, base+f.ext); err == nil {
			c.Format, c.Name, c.Text = f.format, nameOf(res), decodeText(res.Data)
			return nil
		}
		if !errors.Is(err, ErrNoDownload) {
			return err
		}
	}
	return err
}

// readSheet: la hoja de cálculo en CSV. Con la hoja del enlace (#gid=…) y sin --hoja, esa hoja exportada a CSV;
// si no, el libro entero descargado como XLSX y convertido aquí a CSV por pestañas (así se saben sus nombres sin
// leer ninguna página).
func (r *Reader) readSheet(ctx context.Context, project int, c *Content, gid string, p ReadParams) error {
	c.Format = "csv"
	base := r.Bases.Docs + "/spreadsheets/d/" + c.ID + "/export?format="
	if gid != "" && p.Sheet == "" {
		u := base + "csv&gid=" + url.QueryEscape(gid)
		if p.Range != "" {
			u += "&range=" + url.QueryEscape(p.Range)
		}
		res, err := r.download(ctx, project, u)
		if err != nil {
			return err
		}
		c.Name, c.Text = nameOf(res), decodeText(res.Data)
		return nil
	}
	res, err := r.download(ctx, project, base+"xlsx")
	if err != nil {
		return err
	}
	c.Name = nameOf(res)
	sheets, err := readXLSX(res.Data)
	if err != nil {
		return fmt.Errorf("no se ha podido leer el libro descargado (XLSX): %v", err)
	}
	return sheetsText(c, sheets, p)
}

// readFile: un fichero de Drive descargado, según su tipo.
func readFile(c *Content, res Result, p ReadParams) error {
	c.Name, c.MIME, c.Size = nameOf(res), baseType(res.ContentType), len(res.Data)
	switch fileType(res) {
	case "xlsx":
		c.Format = "csv"
		sheets, err := readXLSX(res.Data)
		if err != nil {
			return saveFile(c, res, p.SaveDir)
		}
		return sheetsText(c, sheets, p)
	case "docx":
		t, err := docxText(res.Data)
		if err != nil {
			return saveFile(c, res, p.SaveDir)
		}
		c.Format, c.Text = "texto", t
	case "eml":
		m, err := parseEML(res.Data)
		if err != nil {
			return saveFile(c, res, p.SaveDir)
		}
		c.Format, c.Text = "correo", FormatMailContent(m)
	case "text":
		c.Format, c.Text = "texto", decodeText(res.Data)
	default:
		return saveFile(c, res, p.SaveDir)
	}
	return nil
}

// nameOf: el nombre del fichero descargado sin su extensión (y sin la del original: «Informe.docx.md»).
func nameOf(res Result) string {
	n := strings.TrimSpace(res.FileName)
	for range 2 {
		if i := strings.LastIndexByte(n, '.'); i > 0 && len(n)-i <= 6 {
			n = n[:i]
		}
	}
	return n
}

// cutRunes: los primeros n caracteres.
func cutRunes(s string, n int) string {
	i := 0
	for j := range s {
		if i == n {
			return s[:j]
		}
		i++
	}
	return s
}

// ---- formato compacto (para el contexto de un agente) ----

// FormatContent: cabecera de una línea, el texto (o dónde está el fichero) y, si se ha recortado, cómo pedir menos.
func FormatContent(c Content) string {
	var b strings.Builder
	name := c.Name
	if name == "" {
		name = "(sin nombre)"
	}
	fmt.Fprintf(&b, "# %s [%s · %s · %s]\n", name, c.Kind, c.Format, c.ID)
	if c.Text != "" {
		b.WriteString(strings.TrimRight(c.Text, "\n"))
		b.WriteString("\n")
	}
	if c.Path != "" {
		fmt.Fprintf(&b, "Descargado en %s (%s, %s): ábrelo con tus herramientas.\n", c.Path, firstNonEmpty(c.MIME, "?"), humanSize(c.Size))
	}
	if c.Truncated {
		total := ""
		if c.Total > 0 {
			total = fmt.Sprintf(" de %d", c.Total)
		}
		fmt.Fprintf(&b, "[recortado: %d caracteres%s; pide menos (--hoja, --rango) o más con --max N, hasta %d]\n", utf8.RuneCountInString(c.Text), total, MaxMaxChars)
	}
	if c.Note != "" {
		fmt.Fprintf(&b, "[%s]\n", c.Note)
	}
	return b.String()
}

// humanSize: 1234 → «1,2 KB».
func humanSize(n int) string {
	switch {
	case n >= 1<<20:
		return strings.Replace(fmt.Sprintf("%.1f MB", float64(n)/(1<<20)), ".", ",", 1)
	case n >= 1<<10:
		return strings.Replace(fmt.Sprintf("%.1f KB", float64(n)/(1<<10)), ".", ",", 1)
	}
	return fmt.Sprintf("%d B", n)
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
