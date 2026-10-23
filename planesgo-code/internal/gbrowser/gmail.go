package gbrowser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Gmail, solo por ENLACE a un correo: se descarga el mensaje original (.eml, como «Descargar mensaje» del menú del
// correo) y se lee aquí su MIME: el texto (o el HTML pasado a texto) sin lo citado y los adjuntos solo como nombre,
// tipo y tamaño. No se carga ninguna página de Gmail: ni la bandeja, ni búsquedas, ni el hilo.

// Límites de Gmail (lo leído va al contexto de un agente).
const DefaultMaxMailChars = 20_000 // ~5k tokens por lectura

// MailContent: un correo.
type MailContent struct {
	ID          string
	Subject     string
	From        string
	To          string
	Cc          string
	Date        string
	Text        string
	Attachments []Attachment
	Truncated   bool
	Total       int
}

// Attachment: un adjunto (no se descarga aparte ni se lee).
type Attachment struct {
	Name string
	Type string
	Size int
}

var (
	reMailHexID   = regexp.MustCompile(`^[0-9a-fA-F]{12,24}$`)
	reMailToken   = regexp.MustCompile(`^[B-DF-HJ-NP-TV-XZb-df-hj-np-tv-xz]{16,}$`)
	reMailAccount = regexp.MustCompile(`^/mail/u/(\d+)/?`)
	reMsgF        = regexp.MustCompile(`^(?:thread-|msg-)?f:(\d{1,20})$`)
)

// mailLink: la cuenta (/mail/u/N) y el ID hexadecimal del mensaje de un enlace de Gmail.
type mailLink struct {
	Account string
	ID      string
}

// ParseMailLink: el mensaje de un enlace de Gmail: …/mail/u/0/#inbox/<id> (el ID nuevo, FMfcgz…, o el
// hexadecimal antiguo), …?th=<id> o …?permmsgid=msg-f:<n>.
func ParseMailLink(link string) (mailLink, error) {
	link = strings.TrimSpace(link)
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Hostname() != "mail.google.com" {
		return mailLink{}, fmt.Errorf("no es un enlace de Gmail (https://mail.google.com/mail/u/0/#inbox/…): %q", link)
	}
	l := mailLink{Account: "0"}
	if m := reMailAccount.FindStringSubmatch(u.Path); m != nil {
		l.Account = m[1]
	}
	q := u.Query()
	if th := q.Get("th"); reMailHexID.MatchString(th) {
		l.ID = strings.ToLower(th)
		return l, nil
	}
	if id, ok := decimalID(q.Get("permmsgid")); ok {
		l.ID = id
		return l, nil
	}
	f := u.Fragment
	if i := strings.IndexByte(f, '?'); i >= 0 {
		f = f[:i]
	}
	if i := strings.LastIndexByte(f, '/'); i >= 0 {
		f = f[i+1:]
	}
	switch {
	case reMailHexID.MatchString(f):
		l.ID = strings.ToLower(f)
		return l, nil
	case reMailToken.MatchString(f):
		if id, ok := decimalID(decodeMailToken(f)); ok {
			l.ID = id
			return l, nil
		}
		return mailLink{}, errors.New("ese enlace de Gmail no lleva un ID de mensaje que se pueda descargar (los de enviados y " +
			"borradores no valen): abre el correo desde la bandeja o una búsqueda y copia ese enlace")
	}
	return mailLink{}, errors.New("el enlace no lleva el correo: ábrelo en Gmail y copia el enlace de la barra de direcciones " +
		"(…/mail/u/0/#inbox/<id>)")
}

// decimalID: «thread-f:1777…», «msg-f:1777…» o «f:1777…» → el ID hexadecimal.
func decimalID(s string) (string, bool) {
	m := reMsgF.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return "", false
	}
	n, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil || n == 0 {
		return "", false
	}
	return strconv.FormatUint(n, 16), true
}

// Los ID nuevos de las URL de Gmail (FMfcgz…, KtbxL…) son el texto «f:<número>» (o «a:r-…») en base64, escrito
// como número en base 40 con solo consonantes.
const (
	mailTokenChars = "BCDFGHJKLMNPQRSTVWXZbcdfghjklmnpqrstvwxz"
	base64Chars    = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
)

// convertBase: s, escrito con las cifras de in, escrito con las de out.
func convertBase(s, in, out string) string {
	n := new(big.Int)
	bi, bo := big.NewInt(int64(len(in))), big.NewInt(int64(len(out)))
	for _, ch := range s {
		n.Mul(n, bi)
		n.Add(n, big.NewInt(int64(strings.IndexRune(in, ch))))
	}
	var r []byte
	m := new(big.Int)
	for n.Sign() > 0 {
		n.DivMod(n, bo, m)
		r = append(r, out[m.Int64()])
	}
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// decodeMailToken: el texto que lleva un ID nuevo de Gmail ("" si no se puede).
func decodeMailToken(t string) string {
	b64 := convertBase(t, mailTokenChars, base64Chars)
	b, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(b64, "="))
	if err != nil || !utf8.Valid(b) {
		return ""
	}
	return string(b)
}

// encodeMailToken: lo contrario (para las pruebas).
func encodeMailToken(s string) string {
	return convertBase(base64.RawStdEncoding.EncodeToString([]byte(s)), base64Chars, mailTokenChars)
}

// ReadMail descarga el mensaje original de un enlace de Gmail (.eml) y lo lee.
func (r *Reader) ReadMail(ctx context.Context, project int, link string, max int) (MailContent, error) {
	l, err := ParseMailLink(link)
	if err != nil {
		return MailContent{}, err
	}
	if max <= 0 {
		max = DefaultMaxMailChars
	}
	max = min(max, MaxMaxChars)
	v := url.Values{"ui": {"2"}, "view": {"att"}, "th": {l.ID}, "attid": {"0"}, "disp": {"comp"}, "safe": {"1"}}
	res, err := r.download(ctx, project, r.Bases.Mail+"/mail/u/"+l.Account+"/?"+v.Encode()+"&zw")
	if err != nil {
		return MailContent{}, err
	}
	m, err := parseEML(res.Data)
	if err != nil {
		return MailContent{}, fmt.Errorf("lo descargado de Gmail no es un correo: %v", err)
	}
	m.ID = l.ID
	if n := utf8.RuneCountInString(m.Text); n > max {
		m.Text, m.Truncated, m.Total = cutRunes(m.Text, max)+" […]", true, n
	}
	return m, nil
}

// ---- MIME ----

var wordDecoder = &mime.WordDecoder{CharsetReader: func(cs string, in io.Reader) (io.Reader, error) {
	b, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(charsetText(cs, b)), nil
}}

// charsetText: texto en UTF-8 (lo que no es UTF-8 ni ASCII se toma como Latin-1).
func charsetText(cs string, b []byte) string {
	cs = strings.ToLower(strings.TrimSpace(cs))
	if cs == "" || cs == "utf-8" || cs == "utf8" || cs == "us-ascii" || utf8.Valid(b) && !strings.HasPrefix(cs, "iso-8859") && !strings.HasPrefix(cs, "windows-") {
		return strings.ToValidUTF8(string(b), "")
	}
	return latin1(b)
}

func header(h mail.Header, k string) string {
	v := h.Get(k)
	if d, err := wordDecoder.DecodeHeader(v); err == nil {
		v = d
	}
	return clean(v)
}

// parseEML: un correo .eml: cabeceras, el texto (sin lo citado) y los adjuntos.
func parseEML(data []byte) (MailContent, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return MailContent{}, err
	}
	if msg.Header.Get("From") == "" && msg.Header.Get("Subject") == "" && msg.Header.Get("Date") == "" {
		return MailContent{}, errors.New("sin cabeceras de correo")
	}
	m := MailContent{Subject: header(msg.Header, "Subject"), From: header(msg.Header, "From"), To: header(msg.Header, "To"),
		Cc: header(msg.Header, "Cc"), Date: header(msg.Header, "Date")}
	var w mimeWalk
	w.part(msg.Header, msg.Body, 0)
	text := tidyText(w.plain.String())
	if text == "" {
		text = htmlToText(w.html.String())
	}
	if t, ok := stripQuoted(text); ok {
		text = t
	}
	m.Text, m.Attachments = text, w.attachments
	return m, nil
}

// mimeWalk recorre las partes de un correo: los textos (sin nombre de fichero) y los adjuntos.
type mimeWalk struct {
	plain, html bytes.Buffer
	attachments []Attachment
}

func (w *mimeWalk) part(h map[string][]string, body io.Reader, depth int) {
	get := func(k string) string {
		if v := h[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	ct, params, err := mime.ParseMediaType(get("Content-Type"))
	if err != nil || ct == "" {
		ct, params = "text/plain", map[string]string{}
	}
	disp, dparams, _ := mime.ParseMediaType(get("Content-Disposition"))
	name := firstNonEmpty(dparams["filename"], params["name"])
	if d, err := wordDecoder.DecodeHeader(name); err == nil {
		name = d
	}
	if strings.HasPrefix(ct, "multipart/") && depth < 10 {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextRawPart()
			if err != nil {
				return
			}
			w.part(p.Header, p, depth+1)
		}
	}
	data, _ := io.ReadAll(io.LimitReader(decodeTransfer(get("Content-Transfer-Encoding"), body), MaxDownload))
	if disp == "attachment" || name != "" || ct == "message/rfc822" || !strings.HasPrefix(ct, "text/") {
		if name == "" && ct == "message/rfc822" {
			name = "(correo adjunto)"
		}
		w.attachments = append(w.attachments, Attachment{Name: firstNonEmpty(name, "(sin nombre)"), Type: ct, Size: len(data)})
		return
	}
	text := charsetText(params["charset"], data)
	switch ct {
	case "text/html":
		w.html.WriteString(text + "\n")
	default:
		w.plain.WriteString(text + "\n")
	}
}

func decodeTransfer(cte string, r io.Reader) io.Reader {
	switch strings.ToLower(strings.TrimSpace(cte)) {
	case "base64":
		return base64.NewDecoder(base64.StdEncoding, &base64Clean{r: r})
	case "quoted-printable":
		return quotedprintable.NewReader(r)
	}
	return r
}

// base64Clean: el base64 de un correo, sin los saltos de línea.
type base64Clean struct{ r io.Reader }

func (b *base64Clean) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	j := 0
	for _, c := range p[:n] {
		if c != '\r' && c != '\n' && c != ' ' && c != '\t' {
			p[j] = c
			j++
		}
	}
	return j, err
}

// ---- HTML → texto ----

var (
	reTag      = regexp.MustCompile(`(?s)<!--.*?-->|<(/?)([a-zA-Z][a-zA-Z0-9]*)([^>]*)>`)
	reSpaces   = regexp.MustCompile(`[ \t\f\r]+`)
	blockTags  = map[string]bool{"p": true, "div": true, "br": true, "tr": true, "li": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "table": true, "ul": true, "ol": true, "blockquote": true, "hr": true, "pre": true}
	skipTags   = map[string]bool{"script": true, "style": true, "head": true, "title": true, "blockquote": true}
	reQuoteCls = regexp.MustCompile(`(?i)class\s*=\s*["'][^"']*\b(?:gmail_quote|gmail_extra|moz-cite-prefix|yahoo_quoted)\b`)
)

// htmlToText: el texto de un correo en HTML (sin guiones, estilos ni lo citado: <blockquote>, .gmail_quote).
func htmlToText(s string) string {
	var b strings.Builder
	skip, skipTag := 0, ""
	last := 0
	for _, m := range reTag.FindAllStringSubmatchIndex(s, -1) {
		if skip == 0 {
			b.WriteString(reSpaces.ReplaceAllString(strings.ReplaceAll(s[last:m[0]], "\n", " "), " "))
		}
		last = m[1]
		if m[4] < 0 { // comentario
			continue
		}
		closing, tag, attrs := s[m[2]:m[3]] == "/", strings.ToLower(s[m[4]:m[5]]), s[m[6]:m[7]]
		if skip > 0 {
			if tag == skipTag {
				if closing {
					skip--
				} else if !strings.HasSuffix(attrs, "/") {
					skip++
				}
			}
			continue
		}
		if !closing && (skipTags[tag] || reQuoteCls.MatchString(attrs)) && !strings.HasSuffix(attrs, "/") {
			skip, skipTag = 1, tag
			continue
		}
		if blockTags[tag] {
			b.WriteString("\n")
		} else if tag == "td" || tag == "th" {
			b.WriteString(" ")
		}
	}
	if skip == 0 && last < len(s) {
		b.WriteString(reSpaces.ReplaceAllString(strings.ReplaceAll(s[last:], "\n", " "), " "))
	}
	lines := strings.Split(html.UnescapeString(b.String()), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(reSpaces.ReplaceAllString(l, " "))
	}
	return tidyText(strings.Join(lines, "\n"))
}

// clean: en una línea y sin espacios de más.
func clean(s string) string { return strings.Join(strings.Fields(invisible.Replace(s)), " ") }

// ---- formato compacto (para el contexto de un agente) ----

// FormatMailContent: cabecera del correo (asunto, de, para, fecha), su texto y sus adjuntos.
func FormatMailContent(c MailContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s", firstNonEmpty(c.Subject, "(sin asunto)"))
	if c.ID != "" {
		fmt.Fprintf(&b, " [correo %s]", c.ID)
	}
	b.WriteString("\n")
	for _, h := range [][2]string{{"De", c.From}, {"Para", c.To}, {"Cc", c.Cc}, {"Fecha", c.Date}} {
		if h[1] != "" {
			fmt.Fprintf(&b, "%s: %s\n", h[0], h[1])
		}
	}
	if t := strings.TrimRight(c.Text, "\n"); t != "" {
		b.WriteString("\n" + t + "\n")
	}
	if len(c.Attachments) > 0 {
		var as []string
		for _, a := range c.Attachments {
			as = append(as, fmt.Sprintf("%s (%s, %s)", a.Name, a.Type, humanSize(a.Size)))
		}
		fmt.Fprintf(&b, "Adjuntos: %s\n", strings.Join(as, "; "))
	}
	if c.Truncated {
		fmt.Fprintf(&b, "[recortado: %d de %d caracteres; pide más con --max N, hasta %d]\n", utf8.RuneCountInString(c.Text), c.Total, MaxMaxChars)
	}
	return b.String()
}
