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

// Gmail, leído de su vista web como lo leería el usuario: la bandeja o una búsqueda (las filas de la lista) y un
// hilo (su vista de impresión). Nada de clics: no se envía, borra, archiva ni etiqueta nada.

// Límites de Gmail (lo leído va al contexto de un agente).
const (
	DefaultMaxMails     = 20
	MaxMails            = 50
	DefaultMaxMailChars = 20_000 // ~5k tokens por lectura
	minMailChars        = 600    // lo mínimo de cada mensaje de un hilo largo
	snippetChars        = 140
)

// ErrMailNotFound: Gmail no encuentra el hilo.
var ErrMailNotFound = errors.New("no existe ese hilo de Gmail o tu cuenta de Google (la del navegador de pgocode) no lo ve")

// Mail: un correo de la lista (lo que enseña su fila).
type Mail struct {
	ID      string `json:"id"` // el del hilo (el que lleva la URL de Gmail)
	From    string `json:"from"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Snippet string `json:"snippet,omitempty"`
	Unread  bool   `json:"unread,omitempty"`
}

// MailList: una búsqueda o la bandeja.
type MailList struct {
	Mails []Mail
	More  bool // había más filas que las pedidas
}

// MailContent: un hilo.
type MailContent struct {
	ID        string
	Subject   string
	Messages  []string // el texto de cada mensaje (sin lo citado de los anteriores)
	Truncated bool
}

func clampMails(n int) int {
	if n <= 0 {
		return DefaultMaxMails
	}
	return min(n, MaxMails)
}

// mailBase: la cuenta de Gmail (la primera sesión del navegador).
func (r *Reader) mailBase() string { return r.Bases.Mail + "/mail/u/0/" }

// SearchMail: los correos de una búsqueda con la sintaxis de Gmail (from:ana newer_than:7d…).
func (r *Reader) SearchMail(ctx context.Context, project int, query string, max int) (MailList, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return MailList{}, errors.New("di qué buscar (con la sintaxis de Gmail: from:ana newer_than:7d…)")
	}
	return r.listMail(ctx, project, r.mailBase()+"#search/"+url.PathEscape(query), max)
}

// RecentMail: lo más reciente de la bandeja de entrada.
func (r *Reader) RecentMail(ctx context.Context, project int, max int) (MailList, error) {
	return r.listMail(ctx, project, r.mailBase()+"#inbox", max)
}

func (r *Reader) listMail(ctx context.Context, project int, u string, max int) (MailList, error) {
	max = clampMails(max)
	res, err := r.load(ctx, project, Job{URL: u, Mode: ModeGmailList, MaxChars: 1000})
	if err != nil {
		return MailList{}, err
	}
	var l MailList
	for _, it := range res.Items {
		if it.ID == "" {
			continue
		}
		if len(l.Mails) == max {
			l.More = true
			break
		}
		l.Mails = append(l.Mails, Mail{ID: it.ID, From: clean(it.From), Subject: clean(it.Name), Date: clean(it.Date),
			Snippet: strings.TrimLeft(clean(it.Snippet), "- –"), Unread: it.Unread})
	}
	return l, nil
}

var (
	reMailHexID = regexp.MustCompile(`^[0-9a-fA-F]{12,24}$`)
	reMailNewID = regexp.MustCompile(`^[A-Za-z]{6,}[A-Za-z0-9]{10,}$`)
)

// ParseMailRef: el ID de un hilo (tal cual, el que da «pgo gmail buscar», o al final de una URL de Gmail con el
// ID hexadecimal, #inbox/<id>).
func ParseMailRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if reMailHexID.MatchString(ref) {
		return strings.ToLower(ref), nil
	}
	if u, err := url.Parse(ref); err == nil && u.Hostname() == "mail.google.com" {
		f := u.Fragment
		if i := strings.LastIndexByte(f, '/'); i >= 0 {
			f = f[i+1:]
		}
		if reMailHexID.MatchString(f) {
			return strings.ToLower(f), nil
		}
		if reMailNewID.MatchString(f) {
			return "", errors.New("esa URL de Gmail no lleva el ID que se lee aquí: busca el correo con «pgo gmail buscar» y usa su ID")
		}
	}
	return "", fmt.Errorf("no es el ID de un hilo de Gmail (el que da «pgo gmail buscar»): %q", ref)
}

// ReadMail lee un hilo (su vista de impresión): cada mensaje sin lo citado de los anteriores.
func (r *Reader) ReadMail(ctx context.Context, project int, ref string, max int) (MailContent, error) {
	id, err := ParseMailRef(ref)
	if err != nil {
		return MailContent{}, err
	}
	if max <= 0 {
		max = DefaultMaxMailChars
	}
	max = min(max, MaxMaxChars)
	v := url.Values{"ui": {"2"}, "view": {"pt"}, "search": {"all"}, "th": {id}}
	res, err := r.load(ctx, project, Job{URL: r.mailBase() + "?" + v.Encode(), Mode: ModeGmailPrint, MaxChars: max * 4})
	if errors.Is(err, ErrNotFound) {
		return MailContent{}, ErrMailNotFound
	}
	if err != nil {
		return MailContent{}, err
	}
	c := MailContent{ID: id, Subject: clean(res.Title), Truncated: res.Truncated}
	var msgs []string
	for _, it := range res.Items {
		if t := tidyText(it.Text); t != "" {
			msgs = append(msgs, t)
		}
	}
	if len(msgs) == 0 {
		if t := tidyText(res.Text); t != "" {
			msgs = []string{t}
		}
	}
	if len(msgs) == 0 {
		return c, ErrMailNotFound
	}
	// Lo citado de los anteriores ya está en el hilo (el primero se deja entero).
	for i := 1; i < len(msgs); i++ {
		msgs[i], _ = stripQuoted(msgs[i])
	}
	// Reparto del máximo: los mensajes cortos enteros; lo que sobra, a partes iguales entre los largos.
	c.Messages, c.Truncated = share(msgs, max, c.Truncated)
	return c, nil
}

// share reparte max caracteres entre los mensajes (cada uno, al menos minMailChars si hay que recortar).
func share(msgs []string, limit int, truncated bool) ([]string, bool) {
	total := 0
	for _, m := range msgs {
		total += utf8.RuneCountInString(m)
	}
	if total <= limit {
		return msgs, truncated
	}
	each := max(limit/len(msgs), minMailChars)
	out := make([]string, len(msgs))
	for i, m := range msgs {
		if utf8.RuneCountInString(m) > each {
			m = cutRunes(m, each) + " […]"
		}
		out[i] = m
	}
	return out, true
}

// clean: en una línea y sin espacios de más.
func clean(s string) string { return strings.Join(strings.Fields(invisible.Replace(s)), " ") }

// ---- formato compacto (para el contexto de un agente) ----

// FormatMails: dos líneas por correo: «fecha · remitente · asunto · id · no leído» y el fragmento.
func FormatMails(l MailList) string {
	if len(l.Mails) == 0 {
		return "(ningún correo)\n"
	}
	var b strings.Builder
	for _, m := range l.Mails {
		fmt.Fprintf(&b, "%s · %s · %s · %s", firstNonEmpty(m.Date, "?"), firstNonEmpty(m.From, "?"), firstNonEmpty(m.Subject, "(sin asunto)"), m.ID)
		if m.Unread {
			b.WriteString(" · no leído")
		}
		b.WriteString("\n")
		if sn := m.Snippet; sn != "" {
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

// FormatMailContent: el hilo, con una línea por mensaje antes de su texto.
func FormatMailContent(c MailContent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s [hilo %s · %d mensajes]\n", firstNonEmpty(c.Subject, "(sin asunto)"), c.ID, len(c.Messages))
	for i, m := range c.Messages {
		fmt.Fprintf(&b, "\n## %d/%d\n%s\n", i+1, len(c.Messages), strings.TrimRight(m, "\n"))
	}
	if c.Truncated {
		fmt.Fprintf(&b, "[recortado; pide más con --max N, hasta %d]\n", MaxMaxChars)
	}
	return b.String()
}

func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if strings.TrimSpace(x) != "" {
			return x
		}
	}
	return ""
}
