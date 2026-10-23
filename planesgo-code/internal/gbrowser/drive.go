package gbrowser

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Límites: lo que se lee va al contexto de un agente; mejor recortar (con aviso) que meter megas.
const (
	DefaultMaxChars = 40_000  // ~10k tokens
	MaxMaxChars     = 200_000 // tope aunque se pida más
	DefaultMaxFiles = 30
	MaxFiles        = 100
)

// Tipos de fichero (los de las URL de Google).
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
	ErrNotFound = errors.New("no existe en Google Drive o tu cuenta de Google (la del navegador de pgocode) no lo ve")
	ErrNoAccess = errors.New("tu cuenta de Google (la del navegador de pgocode) no tiene acceso a ese fichero")
)

// Bases: dónde está cada servicio de Google (fijas; las pruebas ponen un servidor local).
type Bases struct {
	Docs  string // https://docs.google.com
	Drive string // https://drive.google.com
	Mail  string // https://mail.google.com
}

// GoogleBases: las de verdad.
var GoogleBases = Bases{Docs: "https://docs.google.com", Drive: "https://drive.google.com", Mail: "https://mail.google.com"}

// Reader lee Google a través del navegador de pgocode.
type Reader struct {
	Bridge *Bridge
	Bases  Bases
}

// New: un lector con las direcciones de Google.
func New(b *Bridge) *Reader { return &Reader{Bridge: b, Bases: GoogleBases} }

// load: una carga, con los errores comunes traducidos (sesión, no existe, sin acceso).
func (r *Reader) load(ctx context.Context, project int, j Job) (Result, error) {
	j.Project = project
	res, err := r.Bridge.Do(ctx, j)
	if err != nil {
		return res, err
	}
	switch {
	case res.Login:
		return res, ErrLogin
	case res.Error != "":
		return res, errors.New("el navegador de pgocode no ha podido cargar la página de Google: " + res.Error)
	case res.Status == 404:
		return res, ErrNotFound
	case res.Status == 401 || res.Status == 403:
		return res, ErrNoAccess
	case res.Status >= 500:
		return res, fmt.Errorf("Google ha respondido con un error (%d): comprueba el ID o la URL", res.Status)
	case !res.Download && reNoAccess.MatchString(firstChars(res.Title+"\n"+res.Text, 2000)):
		return res, ErrNoAccess
	case !res.Download && reNotFound.MatchString(firstChars(res.Title+"\n"+res.Text, 2000)):
		return res, ErrNotFound
	}
	return res, nil
}

// Las páginas de Google de «sin acceso» y «no existe» (en español e inglés).
var (
	reNoAccess = regexp.MustCompile(`(?i)necesitas (?:acceso|permiso)|solicitar acceso|you need (?:access|permission)|request access`)
	reNotFound = regexp.MustCompile(`(?i)el archivo que has solicitado no existe|the file you have requested does not exist|no se puede abrir el archivo|página no encontrada|page not found`)
)

func firstChars(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---- referencias ----

// Ref: un fichero de Google (ID y, si la URL lo dice, tipo y hoja).
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

// ParseRef saca el ID (y el tipo y la hoja, si la URL los lleva) de un ID o una URL de Drive, Docs, Sheets o Slides.
func ParseRef(ref string) (Ref, error) {
	ref = strings.TrimSpace(ref)
	if reID.MatchString(ref) {
		return Ref{ID: ref}, nil
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.HasSuffix(u.Hostname(), "google.com") {
		return Ref{}, fmt.Errorf("no es un ID ni una URL de Google Drive/Docs/Sheets: %q", ref)
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
		return Ref{}, fmt.Errorf("no se encuentra el ID del fichero en la URL %q", ref)
	}
	return r, nil
}

// kindOf: el tipo de fichero de la URL a la que lleva Drive (ModeResolve).
func kindOf(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if m := reKindPath.FindStringSubmatch(p.Path); m != nil {
		return pathKinds[m[1]]
	}
	if reFolder.MatchString(p.Path) {
		return KindFolder
	}
	return ""
}

// ---- búsqueda y listados ----

// File: un fichero de una búsqueda o carpeta de Drive (lo que enseña la página).
type File struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Modified string `json:"modified,omitempty"`
}

// SearchParams: búsqueda en Drive.
type SearchParams struct {
	Text   string // texto (en el nombre o el contenido)
	Folder string // ID o URL de una carpeta (opcional: entonces se filtra su listado por nombre)
	Type   string // doc | hoja | presentacion | pdf | carpeta | imagen (opcional)
	Max    int
}

// typeKind: el tipo pedido → el operador de búsqueda de Drive y el tipo de los resultados.
func typeKind(t string) (op, kind string, err error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return "", "", nil
	case "doc", "docs", "documento", "documentos":
		return "type:document", KindDoc, nil
	case "hoja", "hojas", "sheet", "sheets", "hoja de calculo", "hoja de cálculo":
		return "type:spreadsheet", KindSheet, nil
	case "presentacion", "presentación", "presentaciones", "slides":
		return "type:presentation", KindSlides, nil
	case "pdf":
		return "type:pdf", "pdf", nil
	case "carpeta", "carpetas", "folder":
		return "type:folder", KindFolder, nil
	case "imagen", "imagenes", "imágenes":
		return "type:image", "imagen", nil
	}
	return "", "", fmt.Errorf("tipo desconocido %q (doc, hoja, presentacion, pdf, carpeta, imagen)", t)
}

func clampFiles(n int) int {
	if n <= 0 {
		return DefaultMaxFiles
	}
	return min(n, MaxFiles)
}

// Search busca en Drive como lo haría el usuario en su buscador (drive.google.com/drive/search?q=…).
func (r *Reader) Search(ctx context.Context, project int, p SearchParams) ([]File, error) {
	op, kind, err := typeKind(p.Type)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(p.Text)
	if p.Folder != "" {
		fs, err := r.List(ctx, project, p.Folder, MaxFiles)
		if err != nil {
			return nil, err
		}
		var out []File
		for _, f := range fs {
			if (text == "" || strings.Contains(strings.ToLower(f.Name), strings.ToLower(text))) && (kind == "" || f.Kind == kind) {
				out = append(out, f)
			}
		}
		return out[:min(len(out), clampFiles(p.Max))], nil
	}
	q := strings.TrimSpace(text + " " + op)
	if q == "" {
		return nil, errors.New("di qué buscar")
	}
	return r.driveRows(ctx, project, r.Bases.Drive+"/drive/search?q="+url.QueryEscape(q), kind, clampFiles(p.Max))
}

// List: el contenido de una carpeta (vacía: «Mi unidad»).
func (r *Reader) List(ctx context.Context, project int, folder string, max int) ([]File, error) {
	u := r.Bases.Drive + "/drive/my-drive"
	if strings.TrimSpace(folder) != "" {
		ref, err := ParseRef(folder)
		if err != nil {
			return nil, err
		}
		if ref.Kind != "" && ref.Kind != KindFolder {
			return nil, errors.New("eso no es una carpeta de Drive (para leerlo: pgo drive leer)")
		}
		u = r.Bases.Drive + "/drive/folders/" + url.PathEscape(ref.ID)
	}
	return r.driveRows(ctx, project, u, "", clampFiles(max))
}

func (r *Reader) driveRows(ctx context.Context, project int, u, kind string, max int) ([]File, error) {
	res, err := r.load(ctx, project, Job{URL: u, Mode: ModeDrive, MaxChars: 1000})
	if err != nil {
		return nil, err
	}
	var out []File
	for _, it := range res.Items {
		f := fileOf(it)
		if f.Name == "" || (kind != "" && f.Kind != kind && f.Kind != KindFile) {
			continue
		}
		out = append(out, f)
		if len(out) == max {
			break
		}
	}
	return out, nil
}

// reDate: una fecha u hora como las que enseña Drive («12 oct 2026», «12/10/2026», «Oct 12, 2026», «10:32», «ayer»).
var reDate = regexp.MustCompile(`(?i)^(?:\d{1,2}[ /.-](?:\d{1,2}|[a-zé]{3,10}\.?)(?:[ /.-]\d{2,4})?|[a-z]{3,10}\.? \d{1,2}(?:, \d{4})?|\d{1,2}:\d{2}(?: ?[ap]\.? ?m\.?)?|ayer|hoy|yesterday|today)$`)

// fileOf: una fila de Drive → fichero (nombre: la primera línea; fecha: la primera línea con pinta de fecha;
// tipo: por el enlace o por las etiquetas del icono).
func fileOf(it Item) File {
	f := File{ID: it.ID, Kind: KindFile}
	for _, l := range it.Lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if f.Name == "" {
			f.Name = l
			continue
		}
		if f.Modified == "" && reDate.MatchString(l) {
			f.Modified = l
		}
	}
	if k := kindOf(it.Href); k != "" {
		f.Kind = k
		return f
	}
	f.Kind = kindFromLabels(it.Labels)
	return f
}

// kindFromLabels: el tipo por las etiquetas (aria-label, title, alt) de la fila.
func kindFromLabels(labels []string) string {
	l := strings.ToLower(strings.Join(labels, " | "))
	switch {
	case strings.Contains(l, "carpeta") || strings.Contains(l, "folder"):
		return KindFolder
	case strings.Contains(l, "hoja de cálculo") || strings.Contains(l, "hojas de cálculo") || strings.Contains(l, "spreadsheet") || strings.Contains(l, "google sheets"):
		return KindSheet
	case strings.Contains(l, "presentación") || strings.Contains(l, "presentation") || strings.Contains(l, "google slides"):
		return KindSlides
	case strings.Contains(l, "documento de google") || strings.Contains(l, "documentos de google") || strings.Contains(l, "google docs") || strings.Contains(l, "document"):
		return KindDoc
	case strings.Contains(l, "pdf"):
		return "pdf"
	case strings.Contains(l, "imagen") || strings.Contains(l, "image"):
		return "imagen"
	}
	return KindFile
}

// ---- lectura ----

// ReadParams: qué leer.
type ReadParams struct {
	Ref   string // ID o URL
	Sheet string // hoja (por nombre o número, desde 1); vacía: todas (o la de la URL)
	Range string // rango A1 (A1:D20)
	Max   int    // caracteres como mucho (0: DefaultMaxChars)
}

// Content: lo leído.
type Content struct {
	ID        string
	Name      string
	Kind      string
	Format    string // markdown | texto | csv | listado | enlace
	Text      string
	Truncated bool
	Total     int    // caracteres que había (si se conoce) cuando se recorta
	Note      string // aviso
}

func clampChars(n int) int {
	if n <= 0 {
		return DefaultMaxChars
	}
	return min(n, MaxMaxChars)
}

// Read lee un fichero: Docs en Markdown (o texto), Sheets en CSV, Slides en texto, carpetas como listado; de lo
// demás, solo el enlace.
func (r *Reader) Read(ctx context.Context, project int, p ReadParams) (Content, error) {
	ref, err := ParseRef(p.Ref)
	if err != nil {
		return Content{}, err
	}
	max := clampChars(p.Max)
	if ref.Kind == "" {
		// Un ID suelto: Drive lleva a la página de su tipo (documento, hoja, carpeta…).
		res, err := r.load(ctx, project, Job{URL: r.Bases.Drive + "/open?id=" + url.QueryEscape(ref.ID), Mode: ModeResolve, MaxChars: 2000})
		if err != nil {
			return Content{}, err
		}
		if ref.Kind = kindOf(res.URL); ref.Kind == "" {
			ref.Kind = KindFile
		}
	}
	c := Content{ID: ref.ID, Kind: ref.Kind}
	switch ref.Kind {
	case KindFolder:
		fs, err := r.List(ctx, project, ref.ID, MaxFiles)
		if err != nil {
			return c, err
		}
		c.Format, c.Text = "listado", FormatFiles(fs)
	case KindDoc:
		err = r.readDoc(ctx, project, &c, max)
	case KindSheet:
		err = r.readSheet(ctx, project, &c, ref.Gid, p.Sheet, p.Range, max)
	case KindSlides:
		var res Result
		res, err = r.load(ctx, project, Job{URL: r.Bases.Docs + "/presentation/d/" + ref.ID + "/export/txt", Mode: ModeText, MaxChars: max})
		if err == nil {
			c.Format, c.Name = "texto", nameOf(res)
			err = takeText(&c, res)
		}
	default:
		c.Format = "enlace"
		c.Note = "Desde el navegador solo se lee el texto de Docs, Sheets y Slides. Ábrelo con su enlace: " + r.Bases.Drive + "/file/d/" + ref.ID + "/view"
	}
	if err != nil {
		return c, err
	}
	c.Text = strings.ToValidUTF8(c.Text, "")
	if n := utf8.RuneCountInString(c.Text); n > max {
		if !c.Truncated {
			c.Total = n
		}
		c.Text, c.Truncated = cutRunes(c.Text, max), true
	}
	return c, nil
}

// readDoc: el documento exportado a Markdown (como «Archivo → Descargar → Markdown»); si no, a texto; si tampoco,
// el texto de su versión para móviles.
func (r *Reader) readDoc(ctx context.Context, project int, c *Content, max int) error {
	base := r.Bases.Docs + "/document/d/" + c.ID
	var last error
	for _, try := range []struct{ url, format string }{{base + "/export?format=md", "markdown"}, {base + "/export?format=txt", "texto"}, {base + "/mobilebasic", "texto"}} {
		res, err := r.load(ctx, project, Job{URL: try.url, Mode: ModeText, MaxChars: max})
		if err != nil {
			if errors.Is(err, ErrLogin) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrNoAccess) || errors.Is(err, ErrNoDesktop) {
				return err
			}
			last = err
			continue
		}
		if !res.Download && !isPlain(res.ContentType) && !strings.HasSuffix(try.url, "/mobilebasic") {
			continue // la exportación no ha dado un fichero: siguiente forma
		}
		c.Format, c.Name = try.format, nameOf(res)
		return takeText(c, res)
	}
	if last == nil {
		last = errors.New("Google no ha dado el texto del documento")
	}
	return last
}

// isPlain: la página es un texto tal cual (una exportación que el navegador enseña en vez de descargar).
func isPlain(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "text/plain") || strings.HasPrefix(ct, "text/csv") || strings.HasPrefix(ct, "text/markdown")
}

func takeText(c *Content, res Result) error {
	c.Text, c.Truncated, c.Total = res.Text, res.Truncated, res.Total
	return nil
}

// nameOf: el nombre del fichero (el de la descarga sin extensión o el título de la página sin « - Google …»).
func nameOf(res Result) string {
	if n := strings.TrimSpace(res.FileName); n != "" {
		if i := strings.LastIndexByte(n, '.'); i > 0 {
			n = n[:i]
		}
		return n
	}
	t := strings.TrimSpace(res.Title)
	if i := strings.LastIndex(t, " - "); i > 0 {
		t = t[:i]
	}
	return t
}

// readSheet: la hoja de cálculo en CSV (todas las hojas, la pedida o la de la URL; con rango, solo ese).
func (r *Reader) readSheet(ctx context.Context, project int, c *Content, gid, sheet, rng string, max int) error {
	c.Format = "csv"
	base := r.Bases.Docs + "/spreadsheets/d/" + c.ID
	var tabs []Item
	if gid == "" || sheet != "" {
		res, err := r.load(ctx, project, Job{URL: base + "/htmlview", Mode: ModeSheets, MaxChars: 1000})
		if err != nil {
			return err
		}
		tabs, c.Name = res.Items, nameOf(res)
	}
	var chosen []Item
	switch {
	case len(tabs) == 0 && sheet != "":
		return fmt.Errorf("no se ven las hojas de la hoja de cálculo para elegir %q: pide la hoja con su URL (la que lleva #gid=…)", sheet)
	case len(tabs) == 0:
		chosen = []Item{{ID: gid}} // la de la URL o, sin gid, la primera
	default:
		var err error
		if chosen, err = pickSheets(tabs, gid, sheet, rng); err != nil {
			return err
		}
	}
	var out strings.Builder
	multi := len(chosen) > 1
	for i, s := range chosen {
		u := base + "/export?format=csv"
		if s.ID != "" {
			u += "&gid=" + url.QueryEscape(s.ID)
		}
		if rng != "" {
			u += "&range=" + url.QueryEscape(rng)
		}
		left := max - utf8.RuneCountInString(out.String())
		if left <= 0 {
			c.Truncated = true
			break
		}
		res, err := r.load(ctx, project, Job{URL: u, Mode: ModeText, MaxChars: left})
		if err != nil {
			return err
		}
		if !res.Download && !isPlain(res.ContentType) {
			return errors.New("Google no ha dado la hoja en CSV")
		}
		if c.Name == "" {
			c.Name = nameOf(res)
		}
		if multi {
			if i > 0 {
				out.WriteString("\n")
			}
			fmt.Fprintf(&out, "## %s\n", s.Name)
		}
		out.WriteString(strings.TrimRight(res.Text, "\n") + "\n")
		if res.Truncated {
			c.Truncated = true
			break
		}
	}
	if len(tabs) > 1 && len(chosen) > 1 {
		names := make([]string, len(tabs))
		for i, s := range tabs {
			names[i] = s.Name
		}
		c.Note = "Hojas: " + strings.Join(names, ", ") + ". Para una sola: --hoja <nombre> [--rango A1:D20]."
	}
	c.Text = out.String()
	return nil
}

// pickSheets: las hojas que se leen.
func pickSheets(all []Item, gid, sheet, rng string) ([]Item, error) {
	if sheet != "" {
		for _, s := range all {
			if strings.EqualFold(s.Name, sheet) {
				return []Item{s}, nil
			}
		}
		if n, err := strconv.Atoi(sheet); err == nil && n >= 1 && n <= len(all) {
			return []Item{all[n-1]}, nil
		}
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		return nil, fmt.Errorf("no hay ninguna hoja %q (hay: %s)", sheet, strings.Join(names, ", "))
	}
	if gid != "" {
		for _, s := range all {
			if s.ID == gid {
				return []Item{s}, nil
			}
		}
	}
	if rng != "" {
		return all[:1], nil
	}
	return all, nil
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

// FormatFiles: una línea por fichero: tipo, nombre, fecha e ID (con el ID se lee: pgo drive leer <id>).
func FormatFiles(fs []File) string {
	if len(fs) == 0 {
		return "(nada)\n"
	}
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "[%s] %s", f.Kind, f.Name)
		if f.Modified != "" {
			fmt.Fprintf(&b, " · %s", f.Modified)
		}
		fmt.Fprintf(&b, " · %s\n", f.ID)
	}
	return b.String()
}

// FormatContent: cabecera de una línea, el texto y, si se ha recortado, cómo pedir menos.
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
