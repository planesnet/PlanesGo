package gworkspace

// Gmail en SOLO LECTURA: buscar con la sintaxis de Gmail, listar lo reciente de la bandeja de entrada y leer un
// mensaje o un hilo (cabeceras y cuerpo en texto; los adjuntos solo por nombre, tipo y tamaño, sin descargarlos).
// Solo GET a la API de Gmail: nada de enviar, borrar, marcar como leído ni tocar etiquetas (leer con format=full no
// marca nada). Nada se guarda en disco: lo leído va a quien lo pidió (la interfaz o el agente) y se olvida.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Límites de Gmail (lo leído va al contexto de un agente).
const (
	DefaultMaxMails     = 20
	MaxMails            = 50
	DefaultMaxMailChars = 20_000 // ~5k tokens por lectura
	minMailChars        = 600    // lo mínimo de cada mensaje de un hilo largo
	maxMailDownload     = 32 << 20
	mailFetchers        = 8 // cabeceras pedidas a la vez al buscar
	snippetChars        = 140
	addressChars        = 300 // Para/Cc largos se recortan
)

// ErrMailNotFound: Gmail no encuentra el mensaje ni el hilo.
var ErrMailNotFound = errors.New("no existe ese correo (ni mensaje ni hilo) o tu cuenta de Google no lo ve")

// Attachment: un adjunto (solo sus datos; nunca se descarga).
type Attachment struct {
	Name     string `json:"nombre"`
	MimeType string `json:"tipo"`
	Size     int    `json:"tamano"`
}

// Mail: un mensaje de Gmail (en búsquedas, sin cuerpo ni adjuntos).
type Mail struct {
	ID          string       `json:"id"`
	ThreadID    string       `json:"hilo"`
	Date        time.Time    `json:"fecha"`
	From        string       `json:"de,omitempty"`
	To          string       `json:"para,omitempty"`
	Cc          string       `json:"cc,omitempty"`
	Subject     string       `json:"asunto,omitempty"`
	Snippet     string       `json:"fragmento,omitempty"`
	Unread      bool         `json:"no_leido,omitempty"`
	Text        string       `json:"texto,omitempty"`
	FromHTML    bool         `json:"de_html,omitempty"` // el cuerpo era HTML (convertido a texto)
	Attachments []Attachment `json:"adjuntos,omitempty"`
	Truncated   bool         `json:"recortado,omitempty"`
	Quotes      bool         `json:"citas_omitidas,omitempty"` // se quitó lo citado de mensajes anteriores del hilo
}

// MailList: el resultado de una búsqueda o de lo reciente.
type MailList struct {
	Mails []Mail `json:"correos"`
	More  bool   `json:"hay_mas,omitempty"` // Gmail tiene más resultados
}

// MailContent: un mensaje o un hilo leído.
type MailContent struct {
	Thread    bool   `json:"es_hilo"`
	ThreadID  string `json:"hilo"`
	Subject   string `json:"asunto"`
	Messages  []Mail `json:"mensajes"`
	Truncated bool   `json:"recortado"`
	Max       int    `json:"-"`
}

// MailReadParams: qué leer.
type MailReadParams struct {
	Ref         string // ID de mensaje o de hilo (o URL de Gmail con el ID)
	MessageOnly bool   // solo ese mensaje, aunque el ID sea también el de su hilo
	Max         int    // caracteres como mucho (0: DefaultMaxMailChars)
}

// ---- formato de la API ----

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailPart struct {
	PartID   string        `json:"partId"`
	MimeType string        `json:"mimeType"`
	Filename string        `json:"filename"`
	Headers  []gmailHeader `json:"headers"`
	Body     struct {
		AttachmentID string `json:"attachmentId"`
		Size         int    `json:"size"`
		Data         string `json:"data"`
	} `json:"body"`
	Parts []gmailPart `json:"parts"`
}

type gmailMessage struct {
	ID           string    `json:"id"`
	ThreadID     string    `json:"threadId"`
	LabelIDs     []string  `json:"labelIds"`
	Snippet      string    `json:"snippet"`
	InternalDate string    `json:"internalDate"`
	Payload      gmailPart `json:"payload"`
}

func (p gmailPart) header(name string) string {
	for _, h := range p.Headers {
		if strings.EqualFold(h.Name, name) {
			return decodeHeader(h.Value)
		}
	}
	return ""
}

// decodeHeader: Gmail ya da las cabeceras en UTF-8; por si alguna llega con palabras codificadas (RFC 2047).
func decodeHeader(s string) string {
	if strings.Contains(s, "=?") {
		if d, err := new(mime.WordDecoder).DecodeHeader(s); err == nil {
			s = d
		}
	}
	return strings.TrimSpace(strings.ToValidUTF8(s, ""))
}

func (m *Manager) gmailURL(path string, v url.Values) string {
	u := m.Endpoints.Gmail + "/users/me/" + path
	if len(v) > 0 {
		u += "?" + v.Encode()
	}
	return u
}

func (m *Manager) gmailJSON(ctx context.Context, u string, v any) error {
	b, trunc, err := m.getAPI(ctx, apiGmail, u, maxMailDownload)
	if err != nil {
		return err
	}
	if trunc {
		return errors.New("respuesta de Gmail demasiado grande")
	}
	return json.Unmarshal(b, v)
}

// ---- búsqueda ----

func clampMails(n int) int {
	if n <= 0 {
		return DefaultMaxMails
	}
	return min(n, MaxMails)
}

// SearchMail busca con la sintaxis de Gmail (from:ana newer_than:7d has:attachment…), lo más reciente primero.
func (m *Manager) SearchMail(ctx context.Context, query string, max int) (MailList, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return MailList{}, errors.New("di qué buscar (con la sintaxis de Gmail: from:ana newer_than:7d…)")
	}
	return m.listMail(ctx, url.Values{"q": {query}}, clampMails(max))
}

// RecentMail: lo más reciente de la bandeja de entrada.
func (m *Manager) RecentMail(ctx context.Context, max int) (MailList, error) {
	return m.listMail(ctx, url.Values{"labelIds": {"INBOX"}}, clampMails(max))
}

func (m *Manager) listMail(ctx context.Context, v url.Values, max int) (MailList, error) {
	v.Set("maxResults", strconv.Itoa(max))
	v.Set("fields", "messages(id,threadId),nextPageToken")
	var r struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := m.gmailJSON(ctx, m.gmailURL("messages", v), &r); err != nil {
		return MailList{}, err
	}
	out := MailList{Mails: make([]Mail, len(r.Messages)), More: r.NextPageToken != ""}
	errs := make([]error, len(r.Messages))
	sem := make(chan struct{}, mailFetchers)
	var wg sync.WaitGroup
	for i, x := range r.Messages {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			q := url.Values{"format": {"metadata"}, "metadataHeaders": {"From", "To", "Subject", "Date"},
				"fields": {"id,threadId,labelIds,snippet,internalDate,payload/headers"}}
			var gm gmailMessage
			if errs[i] = m.gmailJSON(ctx, m.gmailURL("messages/"+url.PathEscape(x.ID), q), &gm); errs[i] == nil {
				out.Mails[i] = mailFrom(gm)
			}
		}()
	}
	wg.Wait()
	kept := out.Mails[:0]
	for i, e := range errs {
		switch {
		case errors.Is(e, ErrMailNotFound): // borrado entre la búsqueda y la lectura
		case e != nil:
			return MailList{}, e
		default:
			kept = append(kept, out.Mails[i])
		}
	}
	out.Mails = kept
	return out, nil
}

// mailFrom: los datos de un mensaje (sin el cuerpo).
func mailFrom(gm gmailMessage) Mail {
	ml := Mail{ID: gm.ID, ThreadID: gm.ThreadID, From: gm.Payload.header("From"), To: gm.Payload.header("To"),
		Cc: gm.Payload.header("Cc"), Subject: gm.Payload.header("Subject"), Unread: contains(gm.LabelIDs, "UNREAD"),
		Snippet: strings.TrimSpace(html.UnescapeString(gm.Snippet))}
	if ms, err := strconv.ParseInt(gm.InternalDate, 10, 64); err == nil && ms > 0 {
		ml.Date = time.UnixMilli(ms)
	}
	return ml
}

// ---- lectura ----

var (
	reMailID    = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)
	reMailHexID = regexp.MustCompile(`^[0-9a-fA-F]{12,24}$`)
)

// ParseMailRef: el ID de un mensaje o hilo (tal cual o al final de una URL de Gmail antigua, #inbox/<id>).
func ParseMailRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if reMailID.MatchString(ref) {
		return ref, nil
	}
	u, err := url.Parse(ref)
	if err == nil && u.Hostname() == "mail.google.com" {
		f := u.Fragment
		if i := strings.LastIndexByte(f, '/'); i >= 0 {
			f = f[i+1:]
		}
		if reMailHexID.MatchString(f) {
			return f, nil
		}
		return "", errors.New("esa URL de Gmail no lleva el ID que entiende la API: busca el correo con «pgo gmail buscar» y usa su ID")
	}
	return "", fmt.Errorf("no es el ID de un mensaje o hilo de Gmail: %q", ref)
}

// ReadMail lee un hilo (si el ID es de un hilo) o un mensaje: cabeceras, cuerpo en texto y adjuntos (sin
// descargarlos). En los hilos se quita lo citado de los mensajes anteriores.
func (m *Manager) ReadMail(ctx context.Context, p MailReadParams) (MailContent, error) {
	id, err := ParseMailRef(p.Ref)
	if err != nil {
		return MailContent{}, err
	}
	limit := p.Max
	if limit <= 0 {
		limit = DefaultMaxMailChars
	}
	limit = min(limit, MaxMaxChars)
	full := url.Values{"format": {"full"}}
	var msgs []gmailMessage
	c := MailContent{Max: limit}
	if !p.MessageOnly {
		var th struct {
			ID       string         `json:"id"`
			Messages []gmailMessage `json:"messages"`
		}
		err := m.gmailJSON(ctx, m.gmailURL("threads/"+url.PathEscape(id), full), &th)
		switch {
		case err == nil && len(th.Messages) > 0:
			msgs, c.Thread, c.ThreadID = th.Messages, len(th.Messages) > 1, th.ID
		case err != nil && !errors.Is(err, ErrMailNotFound):
			return c, err
		}
	}
	if msgs == nil {
		var gm gmailMessage
		if err := m.gmailJSON(ctx, m.gmailURL("messages/"+url.PathEscape(id), full), &gm); err != nil {
			return c, err
		}
		msgs, c.ThreadID = []gmailMessage{gm}, gm.ThreadID
	}
	budget := limit
	if len(msgs) > 1 {
		budget = max(limit/len(msgs), minMailChars)
	}
	for _, gm := range msgs {
		ml := mailFrom(gm)
		ml.Snippet = ""
		text, isHTML, atts := m.mailBody(ctx, gm)
		ml.FromHTML, ml.Attachments = isHTML, atts
		if c.Thread {
			text, ml.Quotes = stripQuoted(text)
		}
		if utf8.RuneCountInString(text) > budget {
			text, ml.Truncated, c.Truncated = cutRunes(text, budget), true, true
		}
		ml.Text = text
		c.Messages = append(c.Messages, ml)
	}
	c.Subject = c.Messages[0].Subject
	return c, nil
}

// mailBody: el cuerpo en texto (text/plain o, si no hay, el HTML convertido) y los adjuntos.
func (m *Manager) mailBody(ctx context.Context, gm gmailMessage) (string, bool, []Attachment) {
	var plain, htm []gmailPart
	var atts []Attachment
	var walk func(p gmailPart)
	walk = func(p gmailPart) {
		mt := strings.ToLower(p.MimeType)
		disp := strings.ToLower(p.header("Content-Disposition"))
		switch {
		case strings.HasPrefix(mt, "multipart/"):
			for _, c := range p.Parts {
				walk(c)
			}
		case p.Filename != "" || strings.HasPrefix(disp, "attachment"):
			atts = append(atts, Attachment{Name: firstNonEmpty(p.Filename, "(sin nombre)"), MimeType: p.MimeType, Size: p.Body.Size})
		case mt == "text/plain":
			plain = append(plain, p)
		case mt == "text/html":
			htm = append(htm, p)
		case mt == "message/rfc822":
			atts = append(atts, Attachment{Name: "(mensaje adjunto)", MimeType: p.MimeType, Size: p.Body.Size})
		case p.Body.AttachmentID != "" || p.Body.Size > 0: // imágenes en línea y demás
			atts = append(atts, Attachment{Name: firstNonEmpty(p.Filename, "(en línea)"), MimeType: p.MimeType, Size: p.Body.Size})
		}
	}
	walk(gm.Payload)
	parts, isHTML := plain, false
	if len(parts) == 0 {
		parts, isHTML = htm, true
	}
	var texts []string
	for _, p := range parts {
		raw := p.Body.Data
		if raw == "" && p.Body.AttachmentID != "" {
			// Gmail deja fuera los cuerpos grandes: se piden (es el texto del correo, no un adjunto).
			raw = m.bodyData(ctx, gm.ID, p.Body.AttachmentID)
		}
		b := decodeBase64URL(raw)
		if b == nil {
			continue
		}
		s := toUTF8(b, contentCharset(p))
		if isHTML {
			s = HTMLToText(s)
		} else {
			s = tidyText(s)
		}
		if s != "" {
			texts = append(texts, s)
		}
	}
	return strings.Join(texts, "\n\n"), isHTML && len(texts) > 0, atts
}

// bodyData: un cuerpo que Gmail da aparte (attachments.get); "" si no se puede.
func (m *Manager) bodyData(ctx context.Context, msgID, attID string) string {
	var r struct {
		Data string `json:"data"`
	}
	if m.gmailJSON(ctx, m.gmailURL("messages/"+url.PathEscape(msgID)+"/attachments/"+url.PathEscape(attID), nil), &r) != nil {
		return ""
	}
	return r.Data
}

// decodeBase64URL: Gmail codifica en base64url, a veces con relleno y a veces sin él. nil si no vale.
func decodeBase64URL(s string) []byte {
	s = strings.TrimRight(strings.TrimSpace(s), "=")
	if s == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		if b, err = base64.RawStdEncoding.DecodeString(s); err != nil {
			return nil
		}
	}
	return b
}

func contentCharset(p gmailPart) string {
	if _, params, err := mime.ParseMediaType(p.header("Content-Type")); err == nil {
		return strings.ToLower(params["charset"])
	}
	return ""
}

// cp1252: los caracteres de windows-1252 entre 0x80 y 0x9F (el resto coincide con latin-1).
var cp1252 = [32]rune{'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8d, 'Ž', 0x8f,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9d, 'ž', 'Ÿ'}

// toUTF8: el texto en UTF-8 (UTF-8, ASCII, latin-1, latin-9 y windows-1252; lo demás, lo que sea UTF-8 válido).
func toUTF8(b []byte, charset string) string {
	switch charset {
	case "iso-8859-1", "latin1", "iso8859-1", "iso-8859-15", "latin-9", "windows-1252", "cp1252":
		var sb strings.Builder
		for _, c := range b {
			r := rune(c)
			if c >= 0x80 && c < 0xa0 && (charset == "windows-1252" || charset == "cp1252") {
				r = cp1252[c-0x80]
			} else if c == 0xa4 && (charset == "iso-8859-15" || charset == "latin-9") {
				r = '€'
			}
			sb.WriteRune(r)
		}
		return sb.String()
	}
	if !utf8.Valid(b) && charset == "" {
		return toUTF8(b, "windows-1252") // sin charset y sin UTF-8 válido: lo más probable
	}
	return strings.ToValidUTF8(string(b), "")
}

// ---- texto ----

var (
	reBlankLines = regexp.MustCompile(`\n{3,}`)
	reSpaces     = regexp.MustCompile(`[ \t\f\v\r]+`)
	invisible    = strings.NewReplacer("\u200b", "", "\u200c", "", "\u200d", "", "\u034f", "", "\ufeff", "", "\u00ad", "", "\u00a0", " ")
)

// tidyText: sin \r, sin caracteres invisibles, sin espacios al final de las líneas y sin tandas de líneas en blanco.
func tidyText(s string) string {
	s = invisible.Replace(strings.ReplaceAll(s, "\r\n", "\n"))
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.TrimSpace(reBlankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

var (
	reSkipBlock = regexp.MustCompile(`(?is)<!--.*?-->|<(script|style|head|title|noscript)\b.*?</(?:script|style|head|title|noscript)\s*>`)
	reTag       = regexp.MustCompile(`(?s)<(/?)([a-zA-Z][a-zA-Z0-9]*)\b[^>]*>`)
	reAnyTag    = regexp.MustCompile(`(?s)<[^>]*>`)
)

// HTMLToText convierte el HTML de un correo en texto: bloques en líneas, elementos de lista con «- », celdas
// separadas, sin estilos, guiones, imágenes ni enlaces (las URL de seguimiento gastan mucho y dicen poco).
func HTMLToText(s string) string {
	s = reSkipBlock.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllStringFunc(s, func(t string) string {
		mm := reTag.FindStringSubmatch(t)
		closing, name := mm[1] == "/", strings.ToLower(mm[2])
		switch name {
		case "br":
			return "\n"
		case "li":
			if closing {
				return ""
			}
			return "\n- "
		case "td", "th":
			if closing {
				return " "
			}
			return " | "
		case "p", "div", "tr", "table", "ul", "ol", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "section", "article", "header", "footer", "dl", "dt", "dd", "center":
			return "\n"
		case "hr":
			return "\n---\n"
		}
		return ""
	})
	s = reAnyTag.ReplaceAllString(s, "")
	// Los espacios del HTML no cuentan (salvo los saltos que se acaban de poner).
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = invisible.Replace(html.UnescapeString(reSpaces.ReplaceAllString(l, " ")))
		l = strings.TrimSpace(reSpaces.ReplaceAllString(l, " "))
		l = strings.TrimSpace(strings.TrimPrefix(l, "|"))
		lines[i] = l
	}
	return tidyText(strings.Join(lines, "\n"))
}

var (
	reQuoteIntro  = regexp.MustCompile(`(?i)^(?:el|on)\s.{4,300}(?:escribió|wrote|a écrit|ha scritto)\s*:\s*$`)
	reQuoteStart  = regexp.MustCompile(`(?i)^(?:el|on)\s`)
	reQuoteEnd    = regexp.MustCompile(`(?i)(?:escribió|wrote|a écrit|ha scritto)\s*:\s*$`)
	reOriginalMsg = regexp.MustCompile(`(?i)^-{2,}\s*(?:original message|mensaje original)\s*-{2,}\s*$`)
	reOutlookFrom = regexp.MustCompile(`(?i)^\*?(?:de|from)\s*:\*?\s`)
	reForwarded   = regexp.MustCompile(`(?i)^-{2,}\s*(?:forwarded message|mensaje reenviado)\b`)
	reOutlookSent = regexp.MustCompile(`(?i)^\*?(?:enviado(?:\s+el)?|sent|fecha|date)\s*:\*?\s`)
)

// stripQuoted quita de un mensaje de un hilo lo que cita de los anteriores (que ya están en el hilo): desde «El …
// escribió:» / «On … wrote:», «-----Mensaje original-----» o la cabecera «De: … Enviado: …» de Outlook, y las líneas
// que empiezan por «>». Lo reenviado no se toca (es contenido nuevo).
func stripQuoted(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	cut := len(lines)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		next := ""
		if i+1 < len(lines) {
			next = strings.TrimSpace(lines[i+1])
		}
		if reForwarded.MatchString(t) { // lo de después es lo reenviado: se queda
			break
		}
		if reQuoteIntro.MatchString(t) || reOriginalMsg.MatchString(t) ||
			(reQuoteStart.MatchString(t) && !reQuoteEnd.MatchString(t) && len(t) < 300 && reQuoteEnd.MatchString(next) && len(next) < 120) {
			cut = i
			break
		}
		if reOutlookFrom.MatchString(t) && i > 0 {
			for j := i + 1; j < min(i+4, len(lines)); j++ {
				if reOutlookSent.MatchString(strings.TrimSpace(lines[j])) {
					cut = i
					break
				}
			}
			if cut != len(lines) {
				break
			}
		}
	}
	removed := cut < len(lines)
	var kept []string
	for _, l := range lines[:cut] {
		if strings.HasPrefix(strings.TrimSpace(l), ">") {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if !removed {
		return s, false
	}
	return tidyText(strings.Join(kept, "\n")), true
}

// ---- formato compacto (para el contexto de un agente) ----

func mailDate(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// shortAddresses: Para/Cc largos, recortados con cuántos más había.
func shortAddresses(s string) string {
	if utf8.RuneCountInString(s) <= addressChars {
		return s
	}
	c := cutRunes(s, addressChars)
	if i := strings.LastIndexByte(c, ','); i > 0 {
		c = c[:i]
	}
	rest := strings.Count(s[len(c):], "@")
	return fmt.Sprintf("%s… (+%d)", c, rest)
}

func sizeText(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d B", n)
}

// FormatMails: dos líneas por correo: fecha · remitente · asunto · id (y el hilo si es otro), y el fragmento.
func FormatMails(l MailList) string {
	if len(l.Mails) == 0 {
		return "(ningún correo)\n"
	}
	var b strings.Builder
	for _, ml := range l.Mails {
		fmt.Fprintf(&b, "%s · %s · %s · %s", mailDate(ml.Date), firstNonEmpty(ml.From, "?"), firstNonEmpty(ml.Subject, "(sin asunto)"), ml.ID)
		if ml.ThreadID != "" && ml.ThreadID != ml.ID {
			fmt.Fprintf(&b, " (hilo %s)", ml.ThreadID)
		}
		if ml.Unread {
			b.WriteString(" · no leído")
		}
		b.WriteString("\n")
		if sn := strings.Join(strings.Fields(ml.Snippet), " "); sn != "" {
			if utf8.RuneCountInString(sn) > snippetChars {
				sn = cutRunes(sn, snippetChars) + "…"
			}
			fmt.Fprintf(&b, "  %s\n", sn)
		}
	}
	if l.More {
		fmt.Fprintf(&b, "[hay más: afina la consulta o pide más con --max N, hasta %d]\n", MaxMails)
	}
	return b.String()
}

// FormatMailContent: el mensaje o el hilo, con una cabecera corta por mensaje.
func FormatMailContent(c MailContent) string {
	var b strings.Builder
	subject := firstNonEmpty(c.Subject, "(sin asunto)")
	if c.Thread {
		fmt.Fprintf(&b, "# %s [hilo %s · %d mensajes]\n", subject, c.ThreadID, len(c.Messages))
	} else if len(c.Messages) == 1 {
		ml := c.Messages[0]
		fmt.Fprintf(&b, "# %s [mensaje %s · hilo %s · %s]\n", subject, ml.ID, ml.ThreadID, mailDate(ml.Date))
	}
	for i, ml := range c.Messages {
		if c.Thread {
			fmt.Fprintf(&b, "\n## %d/%d · %s · %s\n", i+1, len(c.Messages), mailDate(ml.Date), ml.ID)
			if ml.Subject != "" && ml.Subject != c.Subject && !strings.HasSuffix(ml.Subject, c.Subject) {
				fmt.Fprintf(&b, "Asunto: %s\n", ml.Subject)
			}
		}
		fmt.Fprintf(&b, "De: %s\n", firstNonEmpty(ml.From, "?"))
		if ml.To != "" {
			fmt.Fprintf(&b, "Para: %s\n", shortAddresses(ml.To))
		}
		if ml.Cc != "" {
			fmt.Fprintf(&b, "Cc: %s\n", shortAddresses(ml.Cc))
		}
		if len(ml.Attachments) > 0 {
			as := make([]string, len(ml.Attachments))
			for j, a := range ml.Attachments {
				as[j] = fmt.Sprintf("%s (%s, %s)", a.Name, a.MimeType, sizeText(a.Size))
			}
			fmt.Fprintf(&b, "Adjuntos: %s\n", strings.Join(as, "; "))
		}
		if ml.Unread {
			b.WriteString("(no leído)\n")
		}
		b.WriteString("\n")
		if ml.Text != "" {
			b.WriteString(ml.Text)
			b.WriteString("\n")
		} else {
			b.WriteString("(sin texto)\n")
		}
		var notes []string
		if ml.Quotes {
			notes = append(notes, "citas de mensajes anteriores omitidas")
		}
		if ml.Truncated {
			notes = append(notes, "recortado")
		}
		if len(notes) > 0 {
			fmt.Fprintf(&b, "[%s]\n", strings.Join(notes, "; "))
		}
	}
	if c.Truncated {
		hint := ""
		if c.Thread {
			hint = " o lee un solo mensaje: pgo gmail leer <id> --mensaje"
		}
		fmt.Fprintf(&b, "[recortado a %d caracteres en total; pide más con --max N, hasta %d%s]\n", c.Max, MaxMaxChars, hint)
	}
	return b.String()
}
