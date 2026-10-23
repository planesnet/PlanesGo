package gworkspace

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	maxDownload     = 10 << 20 // lo que se descarga como mucho (Drive no exporta más de 10 MB)
)

// Tipos de Google.
const (
	mimeFolder   = "application/vnd.google-apps.folder"
	mimeDoc      = "application/vnd.google-apps.document"
	mimeSheet    = "application/vnd.google-apps.spreadsheet"
	mimeSlides   = "application/vnd.google-apps.presentation"
	mimeShortcut = "application/vnd.google-apps.shortcut"
)

// ErrNotFound: Drive no encuentra el fichero (o la cuenta no lo ve).
var ErrNotFound = errors.New("no existe en Drive o tu cuenta de Google no tiene acceso")

// APIError: un error de las APIs de Google.
type APIError struct {
	Code    int
	Reason  string
	Message string
	Hint    string
}

func (e *APIError) Error() string {
	s := fmt.Sprintf("Google respondió %d", e.Code)
	if e.Message != "" {
		s += ": " + e.Message
	}
	if e.Hint != "" {
		s += ". " + e.Hint
	}
	return s
}

// File: los metadatos de un fichero de Drive.
type File struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MimeType     string `json:"mimeType"`
	ModifiedTime string `json:"modifiedTime,omitempty"`
	Size         string `json:"size,omitempty"`
	WebViewLink  string `json:"webViewLink,omitempty"`
	Owners       []struct {
		EmailAddress string `json:"emailAddress"`
	} `json:"owners,omitempty"`
	ShortcutDetails *struct {
		TargetID       string `json:"targetId"`
		TargetMimeType string `json:"targetMimeType"`
	} `json:"shortcutDetails,omitempty"`
}

const fileFields = "id,name,mimeType,modifiedTime,size,webViewLink,owners(emailAddress),shortcutDetails(targetId,targetMimeType)"

// ---- peticiones ----

// get hace un GET autenticado y devuelve como mucho limit bytes (truncated: había más). Si Google rechaza el token,
// pide otro a gcloud una vez; si pide un proyecto de cuota, lo manda desde entonces.
func (m *Manager) get(ctx context.Context, u string, limit int64) (body []byte, truncated bool, err error) {
	retried, quotaTried := false, false
	for {
		tok, err := m.token(ctx)
		if err != nil {
			return nil, false, err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		m.mu.Lock()
		quota := m.useQuota && m.QuotaProject != ""
		m.mu.Unlock()
		if quota {
			req.Header.Set("x-goog-user-project", m.QuotaProject)
		}
		res, err := m.client().Do(req)
		if err != nil {
			return nil, false, fmt.Errorf("no se pudo hablar con Google: %w", err)
		}
		body, err = io.ReadAll(io.LimitReader(res.Body, limit+1))
		res.Body.Close()
		if err != nil {
			return nil, false, err
		}
		if res.StatusCode == http.StatusOK {
			if int64(len(body)) > limit {
				return body[:limit], true, nil
			}
			return body, false, nil
		}
		e := apiError(res.StatusCode, body)
		switch {
		case res.StatusCode == http.StatusUnauthorized && !retried:
			retried = true
			m.forget()
			continue
		case res.StatusCode == http.StatusForbidden && needsQuota(e) && !quota && !quotaTried && m.QuotaProject != "":
			quotaTried = true
			m.mu.Lock()
			m.useQuota = true
			m.mu.Unlock()
			continue
		case res.StatusCode == http.StatusNotFound:
			return nil, false, ErrNotFound
		case res.StatusCode == http.StatusForbidden && (e.Reason == "ACCESS_TOKEN_SCOPE_INSUFFICIENT" || e.Reason == "insufficientPermissions"):
			m.forget()
			return nil, false, ErrNoAccess
		case res.StatusCode == http.StatusForbidden && needsQuota(e):
			p := m.QuotaProject
			if p == "" {
				p = "<proyecto>"
			}
			e.Hint = "Habilita las APIs en el proyecto de cuota: «" + EnableServicesCommand(p) + "»"
		}
		return nil, false, e
	}
}

// needsQuota: el error es de API sin habilitar o sin proyecto de cuota.
func needsQuota(e *APIError) bool {
	switch e.Reason {
	case "SERVICE_DISABLED", "accessNotConfigured", "USER_PROJECT_DENIED", "projectNotLinked":
		return true
	}
	return strings.Contains(strings.ToLower(e.Message), "quota project")
}

// apiError traduce el cuerpo de error de Google.
func apiError(code int, body []byte) *APIError {
	var j struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	json.Unmarshal(body, &j)
	e := &APIError{Code: code, Message: firstLine(j.Error.Message)}
	for _, d := range j.Error.Details {
		if d.Reason != "" {
			e.Reason = d.Reason
		}
	}
	if e.Reason == "" && len(j.Error.Errors) > 0 {
		e.Reason = j.Error.Errors[0].Reason
	}
	return e
}

func (m *Manager) getJSON(ctx context.Context, u string, v any) error {
	b, trunc, err := m.get(ctx, u, maxDownload)
	if err != nil {
		return err
	}
	if trunc {
		return errors.New("respuesta de Google demasiado grande")
	}
	return json.Unmarshal(b, v)
}

// ---- referencias ----

var (
	reIDInPath = regexp.MustCompile(`/(?:d|folders|file/d)/([A-Za-z0-9_-]{10,})`)
	reID       = regexp.MustCompile(`^[A-Za-z0-9_-]{10,}$`)
	reGid      = regexp.MustCompile(`[#&?]gid=(\d+)`)
)

// ParseRef saca el ID de un ID o una URL de Drive, Docs, Sheets o Slides (y la hoja, gid, si la URL la lleva).
func ParseRef(ref string) (id, gid string, err error) {
	ref = strings.TrimSpace(ref)
	if reID.MatchString(ref) {
		return ref, "", nil
	}
	u, perr := url.Parse(ref)
	if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.HasSuffix(u.Hostname(), "google.com") {
		return "", "", fmt.Errorf("no es un ID ni una URL de Google Drive/Docs/Sheets: %q", ref)
	}
	if g := reGid.FindStringSubmatch(ref); g != nil {
		gid = g[1]
	}
	if mm := reIDInPath.FindStringSubmatch(u.Path); mm != nil {
		return mm[1], gid, nil
	}
	if q := u.Query().Get("id"); reID.MatchString(q) {
		return q, gid, nil
	}
	return "", "", fmt.Errorf("no se encuentra el ID del fichero en la URL %q", ref)
}

// ---- búsqueda y listados ----

// SearchParams: búsqueda en Drive.
type SearchParams struct {
	Text   string // texto (en el nombre o el contenido)
	Folder string // ID o URL de una carpeta (opcional)
	Type   string // doc | hoja | presentacion | pdf | carpeta | imagen (opcional)
	Max    int
}

// typeQuery: el filtro de Drive de cada tipo.
func typeQuery(t string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "":
		return "", nil
	case "doc", "docs", "documento", "documentos":
		return "mimeType = '" + mimeDoc + "'", nil
	case "hoja", "hojas", "sheet", "sheets", "hoja de calculo", "hoja de cálculo":
		return "mimeType = '" + mimeSheet + "'", nil
	case "presentacion", "presentación", "presentaciones", "slides":
		return "mimeType = '" + mimeSlides + "'", nil
	case "pdf":
		return "mimeType = 'application/pdf'", nil
	case "carpeta", "carpetas", "folder":
		return "mimeType = '" + mimeFolder + "'", nil
	case "imagen", "imagenes", "imágenes":
		return "mimeType contains 'image/'", nil
	}
	return "", fmt.Errorf("tipo desconocido %q (doc, hoja, presentacion, pdf, carpeta, imagen)", t)
}

// quote: una cadena entre comillas simples para las consultas de Drive.
func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

func clampFiles(n int) int {
	if n <= 0 {
		return DefaultMaxFiles
	}
	return min(n, MaxFiles)
}

// Search busca en Drive (también en las unidades compartidas).
func (m *Manager) Search(ctx context.Context, p SearchParams) ([]File, error) {
	var conds []string
	if t := strings.TrimSpace(p.Text); t != "" {
		conds = append(conds, "(name contains "+quote(t)+" or fullText contains "+quote(t)+")")
	}
	if p.Folder != "" {
		id, _, err := ParseRef(p.Folder)
		if err != nil {
			return nil, err
		}
		conds = append(conds, quote(id)+" in parents")
	}
	tq, err := typeQuery(p.Type)
	if err != nil {
		return nil, err
	}
	if tq != "" {
		conds = append(conds, tq)
	}
	if len(conds) == 0 {
		return nil, errors.New("di qué buscar")
	}
	conds = append(conds, "trashed = false")
	// Con texto, Drive ordena por relevancia (no admite orderBy con fullText).
	order := ""
	if strings.TrimSpace(p.Text) == "" {
		order = "modifiedTime desc"
	}
	return m.listFiles(ctx, strings.Join(conds, " and "), order, clampFiles(p.Max))
}

// List: el contenido de una carpeta (vacía: «Mi unidad»), carpetas primero.
func (m *Manager) List(ctx context.Context, folder string, max int) ([]File, error) {
	id := "root"
	if strings.TrimSpace(folder) != "" {
		var err error
		if id, _, err = ParseRef(folder); err != nil {
			return nil, err
		}
	}
	return m.listFiles(ctx, quote(id)+" in parents and trashed = false", "folder,name", clampFiles(max))
}

func (m *Manager) listFiles(ctx context.Context, q, order string, max int) ([]File, error) {
	v := url.Values{"q": {q}, "pageSize": {strconv.Itoa(max)}, "fields": {"files(" + fileFields + ")"},
		"supportsAllDrives": {"true"}, "includeItemsFromAllDrives": {"true"}, "corpora": {"allDrives"}}
	if order != "" {
		v.Set("orderBy", order)
	}
	var r struct {
		Files []File `json:"files"`
	}
	if err := m.getJSON(ctx, m.Endpoints.Drive+"/files?"+v.Encode(), &r); err != nil {
		return nil, err
	}
	return r.Files, nil
}

// Get: los metadatos de un fichero (ID o URL).
func (m *Manager) Get(ctx context.Context, ref string) (File, error) {
	id, _, err := ParseRef(ref)
	if err != nil {
		return File{}, err
	}
	return m.get1(ctx, id)
}

func (m *Manager) get1(ctx context.Context, id string) (File, error) {
	var f File
	v := url.Values{"fields": {fileFields}, "supportsAllDrives": {"true"}}
	err := m.getJSON(ctx, m.Endpoints.Drive+"/files/"+url.PathEscape(id)+"?"+v.Encode(), &f)
	return f, err
}

// ---- lectura ----

// ReadParams: qué leer.
type ReadParams struct {
	Ref   string // ID o URL
	Sheet string // hoja (por nombre o número, desde 1); vacía: todas (o la de la URL)
	Range string // rango A1 (A1:D20); con hoja o sin ella (entonces, la primera)
	Max   int    // caracteres como mucho (0: DefaultMaxChars)
}

// Content: lo leído.
type Content struct {
	File      File
	Format    string // markdown | texto | csv | listado | metadatos
	Text      string
	Truncated bool
	Total     int    // caracteres que había (si se conoce) cuando se recorta
	Note      string // aviso (p. ej. por qué no hay texto)
}

// Read lee un fichero: Docs en Markdown (o texto), Sheets en CSV, Slides en texto, ficheros de texto tal cual,
// carpetas como listado; de lo demás (PDF, imágenes…), solo los metadatos.
func (m *Manager) Read(ctx context.Context, p ReadParams) (Content, error) {
	id, gid, err := ParseRef(p.Ref)
	if err != nil {
		return Content{}, err
	}
	max := p.Max
	if max <= 0 {
		max = DefaultMaxChars
	}
	max = min(max, MaxMaxChars)
	f, err := m.get1(ctx, id)
	if err != nil {
		return Content{}, err
	}
	if f.MimeType == mimeShortcut && f.ShortcutDetails != nil && f.ShortcutDetails.TargetID != "" {
		if f, err = m.get1(ctx, f.ShortcutDetails.TargetID); err != nil {
			return Content{}, err
		}
	}
	c := Content{File: f}
	switch {
	case f.MimeType == mimeFolder:
		fs, err := m.List(ctx, f.ID, MaxFiles)
		if err != nil {
			return c, err
		}
		c.Format, c.Text = "listado", FormatFiles(fs)
	case f.MimeType == mimeDoc:
		c.Format = "markdown"
		b, trunc, err := m.export(ctx, f.ID, "text/markdown", max)
		if err != nil {
			// Drive exporta a Markdown desde 2024; si no, texto.
			c.Format = "texto"
			if b, trunc, err = m.export(ctx, f.ID, "text/plain", max); err != nil {
				return c, err
			}
		}
		c.Text, c.Truncated = string(b), trunc
	case f.MimeType == mimeSheet:
		if err := m.readSheet(ctx, &c, gid, p.Sheet, p.Range, max); err != nil {
			return c, err
		}
	case f.MimeType == mimeSlides:
		b, trunc, err := m.export(ctx, f.ID, "text/plain", max)
		if err != nil {
			return c, err
		}
		c.Format, c.Text, c.Truncated = "texto", string(b), trunc
	case isText(f.MimeType):
		v := url.Values{"alt": {"media"}, "supportsAllDrives": {"true"}}
		b, trunc, err := m.get(ctx, m.Endpoints.Drive+"/files/"+url.PathEscape(f.ID)+"?"+v.Encode(), int64(max)*4)
		if err != nil {
			return c, err
		}
		c.Format, c.Text, c.Truncated = "texto", string(b), trunc
	default:
		c.Format = "metadatos"
		c.Note = "Drive no da el texto de este tipo de fichero (" + f.MimeType + "): solo los metadatos. Ábrelo con su enlace."
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

// export descarga un documento de Google en otro formato (con un poco de margen sobre max caracteres).
func (m *Manager) export(ctx context.Context, id, mime string, max int) ([]byte, bool, error) {
	v := url.Values{"mimeType": {mime}}
	return m.get(ctx, m.Endpoints.Drive+"/files/"+url.PathEscape(id)+"/export?"+v.Encode(), min(int64(max)*4, maxDownload))
}

func isText(mime string) bool {
	if strings.HasPrefix(mime, "text/") {
		return true
	}
	switch mime {
	case "application/json", "application/xml", "application/x-yaml", "application/yaml", "application/javascript", "application/sql":
		return true
	}
	return false
}

type sheetProps struct {
	SheetID int    `json:"sheetId"`
	Title   string `json:"title"`
	Hidden  bool   `json:"hidden"`
}

// readSheet: la hoja de cálculo en CSV (todas las hojas, la pedida o la de la URL; con rango, solo ese).
func (m *Manager) readSheet(ctx context.Context, c *Content, gid, sheet, rng string, max int) error {
	c.Format = "csv"
	var meta struct {
		Sheets []struct {
			Properties sheetProps `json:"properties"`
		} `json:"sheets"`
	}
	base := m.Endpoints.Sheets + "/spreadsheets/" + url.PathEscape(c.File.ID)
	if err := m.getJSON(ctx, base+"?"+url.Values{"fields": {"sheets.properties(sheetId,title,hidden)"}}.Encode(), &meta); err != nil {
		var ae *APIError
		if errors.As(err, &ae) && ae.Code == http.StatusForbidden && sheet == "" && rng == "" {
			// Sin la API de Sheets: Drive exporta la primera hoja en CSV.
			b, trunc, err2 := m.export(ctx, c.File.ID, "text/csv", max)
			if err2 != nil {
				return err
			}
			c.Text, c.Truncated = string(b), trunc
			c.Note = "Solo la primera hoja (la API de Sheets no responde: " + ae.Error() + ")."
			return nil
		}
		return err
	}
	var all []sheetProps
	for _, s := range meta.Sheets {
		all = append(all, s.Properties)
	}
	if len(all) == 0 {
		return errors.New("la hoja de cálculo no tiene hojas")
	}
	chosen, err := pickSheets(all, gid, sheet, rng)
	if err != nil {
		return err
	}
	ranges := make([]string, len(chosen))
	for i, s := range chosen {
		ranges[i] = "'" + strings.ReplaceAll(s.Title, "'", "''") + "'"
		if rng != "" {
			ranges[i] += "!" + rng
		}
	}
	if strings.Contains(rng, "!") { // el rango ya dice la hoja
		ranges = []string{rng}
	}
	v := url.Values{"ranges": ranges, "valueRenderOption": {"FORMATTED_VALUE"}, "majorDimension": {"ROWS"}}
	var vals struct {
		ValueRanges []struct {
			Range  string  `json:"range"`
			Values [][]any `json:"values"`
		} `json:"valueRanges"`
	}
	if err := m.getJSON(ctx, base+"/values:batchGet?"+v.Encode(), &vals); err != nil {
		return err
	}
	var out strings.Builder
	multi := len(vals.ValueRanges) > 1
	for i, vr := range vals.ValueRanges {
		if multi {
			if i > 0 {
				out.WriteString("\n")
			}
			fmt.Fprintf(&out, "## %s (%d filas)\n", vr.Range, len(vr.Values))
		}
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		for _, row := range vr.Values {
			rec := make([]string, len(row))
			for j, x := range row {
				rec[j] = fmt.Sprint(x)
			}
			w.Write(rec)
			if out.Len()+buf.Len() > max*4 {
				break
			}
		}
		w.Flush()
		out.Write(buf.Bytes())
		if out.Len() > max*4 {
			c.Truncated = true
			break
		}
	}
	if multi && len(chosen) > 1 {
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Title
		}
		c.Note = "Hojas: " + strings.Join(names, ", ") + ". Para una sola: --hoja <nombre> [--rango A1:D20]."
	}
	c.Text = out.String()
	return nil
}

// pickSheets: las hojas que se leen.
func pickSheets(all []sheetProps, gid, sheet, rng string) ([]sheetProps, error) {
	if sheet != "" {
		for _, s := range all {
			if strings.EqualFold(s.Title, sheet) {
				return []sheetProps{s}, nil
			}
		}
		if n, err := strconv.Atoi(sheet); err == nil && n >= 1 && n <= len(all) {
			return []sheetProps{all[n-1]}, nil
		}
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Title
		}
		return nil, fmt.Errorf("no hay ninguna hoja %q (hay: %s)", sheet, strings.Join(names, ", "))
	}
	if gid != "" {
		for _, s := range all {
			if strconv.Itoa(s.SheetID) == gid {
				return []sheetProps{s}, nil
			}
		}
	}
	if rng != "" {
		return all[:1], nil
	}
	var visible []sheetProps
	for _, s := range all {
		if !s.Hidden {
			visible = append(visible, s)
		}
	}
	if len(visible) == 0 {
		return all, nil
	}
	return visible, nil
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

// Kind: el tipo de un fichero en una palabra.
func Kind(mime string) string {
	switch mime {
	case mimeFolder:
		return "carpeta"
	case mimeDoc:
		return "doc"
	case mimeSheet:
		return "hoja"
	case mimeSlides:
		return "presentación"
	case mimeShortcut:
		return "acceso"
	case "application/pdf":
		return "pdf"
	}
	if strings.HasPrefix(mime, "application/vnd.google-apps.") {
		return strings.TrimPrefix(mime, "application/vnd.google-apps.")
	}
	if i := strings.IndexByte(mime, '/'); i >= 0 {
		if strings.HasPrefix(mime, "image/") {
			return "imagen"
		}
		return mime[i+1:]
	}
	return mime
}

// FormatFiles: una línea por fichero: tipo, nombre, fecha e ID (con el ID se lee: pgo drive leer <id>).
func FormatFiles(fs []File) string {
	if len(fs) == 0 {
		return "(nada)\n"
	}
	var b strings.Builder
	for _, f := range fs {
		d := f.ModifiedTime
		if len(d) >= 10 {
			d = d[:10]
		}
		fmt.Fprintf(&b, "[%s] %s · %s · %s\n", Kind(f.MimeType), f.Name, d, f.ID)
	}
	return b.String()
}

// FormatContent: cabecera de una línea, el texto y, si se ha recortado, cómo pedir menos.
func FormatContent(c Content) string {
	var b strings.Builder
	d := c.File.ModifiedTime
	if len(d) >= 10 {
		d = d[:10]
	}
	fmt.Fprintf(&b, "# %s [%s · %s · %s · %s]\n", c.File.Name, Kind(c.File.MimeType), c.Format, d, c.File.ID)
	if c.Format == "metadatos" {
		if c.File.Size != "" {
			fmt.Fprintf(&b, "tamaño: %s bytes\n", c.File.Size)
		}
		if len(c.File.Owners) > 0 {
			fmt.Fprintf(&b, "propietario: %s\n", c.File.Owners[0].EmailAddress)
		}
		if c.File.WebViewLink != "" {
			fmt.Fprintf(&b, "enlace: %s\n", c.File.WebViewLink)
		}
	}
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
