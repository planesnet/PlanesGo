package gworkspace

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---- Gmail simulado ----

func b64(s string) string    { return base64.URLEncoding.EncodeToString([]byte(s)) }    // con relleno
func b64raw(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) } // sin relleno
func hdrs(kv ...string) []gmailHeader {
	var h []gmailHeader
	for i := 0; i+1 < len(kv); i += 2 {
		h = append(h, gmailHeader{Name: kv[i], Value: kv[i+1]})
	}
	return h
}

func part(mt, data string, h ...string) gmailPart {
	p := gmailPart{MimeType: mt, Headers: hdrs(h...)}
	p.Body.Data, p.Body.Size = data, len(data)
	return p
}

// fakeMails: los correos del Gmail simulado.
func fakeMails() map[string]gmailMessage {
	date := func(s string) string {
		t, _ := time.Parse(time.RFC3339, s)
		return strconvMs(t)
	}
	// Mensaje con texto y HTML alternativos, un PDF adjunto y en latin-1.
	mixed := gmailMessage{ID: "18f0000000000001", ThreadID: "18f0000000000001", LabelIDs: []string{"INBOX", "UNREAD"},
		Snippet: "Te paso la factura de octubre &amp; el resumen", InternalDate: date("2026-10-20T12:03:00Z")}
	mixed.Payload = gmailPart{MimeType: "multipart/mixed", Headers: hdrs("From", "Ana Pérez <ana@cliente.es>", "To", "luis@planesnet.com",
		"Subject", "Factura de octubre", "Date", "Mon, 20 Oct 2026 14:03:00 +0200"), Parts: []gmailPart{
		{MimeType: "multipart/alternative", Parts: []gmailPart{
			part("text/plain", b64("Hola Luis,\r\n\r\nTe paso la factura.\r\nUn saludo\r\n"), "Content-Type", `text/plain; charset="UTF-8"`),
			part("text/html", b64("<p>Hola <b>Luis</b></p>"), "Content-Type", "text/html; charset=UTF-8"),
		}},
	}}
	pdf := gmailPart{MimeType: "application/pdf", Filename: "factura-10.pdf", Headers: hdrs("Content-Disposition", `attachment; filename="factura-10.pdf"`)}
	pdf.Body.AttachmentID, pdf.Body.Size = "ATT-PDF", 123_456
	mixed.Payload.Parts = append(mixed.Payload.Parts, pdf)

	// Solo HTML (boletín), con estilos, guion, entidades y caracteres invisibles.
	htmlOnly := gmailMessage{ID: "18f0000000000002", ThreadID: "18f0000000000002", InternalDate: date("2026-10-19T08:00:00Z"),
		Payload: part("text/html", b64raw(`<html><head><style>p{color:red}</style><title>X</title></head><body>`+
			`<script>alert(1)</script><h1>Novedades&nbsp;de Odoo</h1><p>Precio: 10&euro; &amp; m&aacute;s</p>`+
			`<ul><li>Uno</li><li>Dos</li></ul><table><tr><td>A</td><td>B</td></tr></table>`+"‌‌"+`<a href="https://track.example/x?y=1">Leer más</a></body></html>`),
			"From", "Odoo <news@odoo.com>", "Subject", "Novedades", "Content-Type", "text/html; charset=utf-8")}

	// En latin-1 y con el cuerpo aparte (attachmentId sin nombre de fichero).
	latin := gmailMessage{ID: "18f0000000000003", ThreadID: "18f0000000000003", InternalDate: date("2026-10-18T08:00:00Z"),
		Payload: gmailPart{MimeType: "text/plain", Headers: hdrs("From", "=?ISO-8859-1?Q?Jos=E9?= <jose@x.es>", "Subject", "Año", "Content-Type", "text/plain; charset=ISO-8859-1")}}
	latin.Payload.Body.AttachmentID, latin.Payload.Body.Size = "ATT-BODY", 20

	// Un hilo de dos mensajes: la respuesta cita el primero.
	t1 := gmailMessage{ID: "18f00000000000a1", ThreadID: "18f00000000000a1", InternalDate: date("2026-10-10T08:00:00Z"),
		Payload: part("text/plain", b64("¿Quedamos el martes para revisar el presupuesto?"), "From", "Ana <ana@cliente.es>", "To", "luis@planesnet.com", "Subject", "Reunión")}
	t2 := gmailMessage{ID: "18f00000000000a2", ThreadID: "18f00000000000a1", InternalDate: date("2026-10-10T09:00:00Z"),
		Payload: part("text/plain", b64("Sí, el martes a las 10.\n\nEl vie, 10 oct 2026 a las 10:00, Ana (<ana@cliente.es>)\nescribió:\n> ¿Quedamos el martes para revisar el presupuesto?\n"),
			"From", "Luis <luis@planesnet.com>", "To", "Ana <ana@cliente.es>", "Subject", "Re: Reunión")}
	return map[string]gmailMessage{mixed.ID: mixed, htmlOnly.ID: htmlOnly, latin.ID: latin, t1.ID: t1, t2.ID: t2}
}

func strconvMs(t time.Time) string { return strconv.FormatInt(t.UnixMilli(), 10) }

func (g *fakeGoogle) gmailRoutes(t *testing.T, mux *http.ServeMux, auth func(http.ResponseWriter, *http.Request) bool) {
	mails := fakeMails()
	threads := map[string][]string{"18f00000000000a1": {"18f00000000000a1", "18f00000000000a2"}, "18f0000000000001": {"18f0000000000001"}}
	gauth := func(w http.ResponseWriter, r *http.Request) bool {
		if !auth(w, r) {
			return false
		}
		g.mu.Lock()
		sc := g.scopes[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		noGmail, deny := g.noGmail, g.gmailDeny
		g.mu.Unlock()
		switch {
		case noGmail:
			w.WriteHeader(403)
			w.Write([]byte(`{"error":{"code":403,"message":"Gmail API has not been used in project 1","details":[{"reason":"SERVICE_DISABLED"}]}}`))
			return false
		case deny || !strings.Contains(sc, ScopeGmailReadonly):
			w.WriteHeader(403)
			w.Write([]byte(`{"error":{"code":403,"message":"Request had insufficient authentication scopes.","details":[{"reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}]}}`))
			return false
		}
		return true
	}
	notFound := func(w http.ResponseWriter) {
		w.WriteHeader(404)
		w.Write([]byte(`{"error":{"code":404,"message":"Requested entity was not found."}}`))
	}
	mux.HandleFunc("GET /gmail/v1/users/me/messages", func(w http.ResponseWriter, r *http.Request) {
		if !gauth(w, r) {
			return
		}
		q := r.URL.Query()
		type ref struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		}
		var out []ref
		next := ""
		switch {
		case q.Get("labelIds") == "INBOX":
			out = []ref{{"18f0000000000001", "18f0000000000001"}, {"18f0000000000002", "18f0000000000002"}}
			if q.Get("maxResults") == "2" {
				next = "pagina2"
			}
		case q.Get("q") == "from:ana newer_than:30d":
			// El 18f…dead se ha borrado entre la búsqueda y la lectura.
			out = []ref{{"18f0000000000001", "18f0000000000001"}, {"18f00000000000a2", "18f00000000000a1"}, {"18f000000000dead", "18f000000000dead"}}
		case q.Get("q") != "":
			out = nil
		default:
			t.Errorf("búsqueda sin consulta ni etiqueta: %s", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"messages": out, "nextPageToken": next, "resultSizeEstimate": len(out)})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !gauth(w, r) {
			return
		}
		m, ok := mails[r.PathValue("id")]
		if !ok {
			notFound(w)
			return
		}
		q := r.URL.Query()
		switch q.Get("format") {
		case "metadata":
			if len(q["metadataHeaders"]) == 0 {
				t.Error("metadata sin metadataHeaders")
			}
			m.Payload = gmailPart{Headers: m.Payload.Headers} // sin partes ni cuerpo
		case "full":
		default:
			t.Errorf("formato %q", q.Get("format"))
		}
		json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("GET /gmail/v1/users/me/messages/{id}/attachments/{att}", func(w http.ResponseWriter, r *http.Request) {
		if !gauth(w, r) {
			return
		}
		if r.PathValue("att") != "ATT-BODY" {
			t.Errorf("se ha descargado un adjunto: %s", r.PathValue("att"))
			notFound(w)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"size": 20, "data": base64.URLEncoding.EncodeToString([]byte("Feliz a\xf1o, Jos\xe9"))})
	})
	mux.HandleFunc("GET /gmail/v1/users/me/threads/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !gauth(w, r) {
			return
		}
		ids, ok := threads[r.PathValue("id")]
		if !ok {
			notFound(w)
			return
		}
		if r.URL.Query().Get("format") != "full" {
			t.Errorf("hilo con formato %q", r.URL.Query().Get("format"))
		}
		var ms []gmailMessage
		for _, id := range ids {
			ms = append(ms, mails[id])
		}
		json.NewEncoder(w).Encode(map[string]any{"id": r.PathValue("id"), "messages": ms})
	})
}

const fullScopes = "openid https://www.googleapis.com/auth/userinfo.email " + ScopeDriveReadonly + " " + ScopeGmailReadonly

// gmailTest: un usuario conectado con Drive y Gmail.
func gmailTest(t *testing.T) (*Manager, *fakeOAuth, context.Context) {
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	o.scope = fullScopes
	connect(t, m, o, "luis")
	return m, o, WithUser(context.Background(), "luis")
}

func (g *fakeGoogle) gmailCalls() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	var l []string
	for _, c := range g.calls {
		if strings.Contains(c, "/gmail/") {
			l = append(l, c)
		}
	}
	return l
}

// ---- pruebas ----

func TestGmailSearch(t *testing.T) {
	m, o, ctx := gmailTest(t)
	st := m.Status(ctx, "luis", false)
	if !st.Connected || !st.Drive || !st.Gmail {
		t.Fatalf("%+v", st)
	}
	l, err := m.SearchMail(ctx, "from:ana newer_than:30d", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Mails) != 2 || l.More {
		t.Fatalf("%+v", l)
	}
	if a := l.Mails[0]; a.ID != "18f0000000000001" || a.From != "Ana Pérez <ana@cliente.es>" || a.Subject != "Factura de octubre" || !a.Unread ||
		a.Snippet != "Te paso la factura de octubre & el resumen" || a.Date.IsZero() || a.Text != "" {
		t.Fatalf("%+v", a)
	}
	out := FormatMails(l)
	for _, want := range []string{" · Ana Pérez <ana@cliente.es> · Factura de octubre · 18f0000000000001 · no leído\n",
		"  Te paso la factura de octubre & el resumen\n", "· Re: Reunión · 18f00000000000a2 (hilo 18f00000000000a1)\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	if strings.Contains(out, "18f0000000000001 (hilo") {
		t.Error("el hilo solo se dice si es otro")
	}
	// Lo reciente: la bandeja de entrada, con aviso de que hay más.
	l, err = m.RecentMail(ctx, 2)
	if err != nil || len(l.Mails) != 2 || !l.More || !strings.Contains(FormatMails(l), "[hay más") {
		t.Fatalf("%+v %v", l, err)
	}
	if _, err := m.SearchMail(ctx, "  ", 0); err == nil {
		t.Error("búsqueda vacía")
	}
	if l, _ := m.SearchMail(ctx, "nada", 0); FormatMails(l) != "(ningún correo)\n" {
		t.Error(FormatMails(l))
	}
	for _, c := range o.g.gmailCalls() {
		if !strings.HasPrefix(c, "GET ") {
			t.Errorf("petición que no es GET: %s", c)
		}
	}
}

func TestGmailReadMessage(t *testing.T) {
	m, o, ctx := gmailTest(t)
	// Un mensaje que es su propio hilo de uno: se lee como mensaje.
	c, err := m.ReadMail(ctx, MailReadParams{Ref: "18f0000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Thread || len(c.Messages) != 1 {
		t.Fatalf("%+v", c)
	}
	ml := c.Messages[0]
	if ml.Text != "Hola Luis,\n\nTe paso la factura.\nUn saludo" || ml.FromHTML {
		t.Fatalf("texto %q", ml.Text)
	}
	if len(ml.Attachments) != 1 || ml.Attachments[0] != (Attachment{Name: "factura-10.pdf", MimeType: "application/pdf", Size: 123_456}) {
		t.Fatalf("%+v", ml.Attachments)
	}
	out := FormatMailContent(c)
	for _, want := range []string{"# Factura de octubre [mensaje 18f0000000000001 · hilo 18f0000000000001 · ", "De: Ana Pérez <ana@cliente.es>\n",
		"Para: luis@planesnet.com\n", "Adjuntos: factura-10.pdf (application/pdf, 120 KB)\n", "(no leído)\n", "\nHola Luis,"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	// Solo HTML: convertido a texto, sin estilos, guiones, enlaces ni invisibles.
	c, err = m.ReadMail(ctx, MailReadParams{Ref: "18f0000000000002"})
	if err != nil {
		t.Fatal(err)
	}
	want := "Novedades de Odoo\n\nPrecio: 10€ & más\n\n- Uno\n- Dos\n\nA | B\n\nLeer más"
	if ml := c.Messages[0]; ml.Text != want || !ml.FromHTML {
		t.Fatalf("html: %q", ml.Text)
	}
	// latin-1, con el cuerpo aparte (se pide: es el texto, no un adjunto) y el remitente codificado.
	c, err = m.ReadMail(ctx, MailReadParams{Ref: "18f0000000000003", MessageOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if ml := c.Messages[0]; ml.Text != "Feliz año, José" || ml.From != "José <jose@x.es>" {
		t.Fatalf("%+v", ml)
	}
	// El PDF nunca se ha descargado (la ruta de adjuntos solo ha visto ATT-BODY).
	for _, call := range o.g.gmailCalls() {
		if strings.Contains(call, "ATT-PDF") {
			t.Error("se ha descargado el adjunto")
		}
	}
	if _, err := m.ReadMail(ctx, MailReadParams{Ref: "18f0000000000999"}); !errors.Is(err, ErrMailNotFound) {
		t.Fatalf("no existe: %v", err)
	}
	if _, err := m.ReadMail(ctx, MailReadParams{Ref: "no vale/../x"}); err == nil {
		t.Error("referencia mala")
	}
}

func TestGmailReadThread(t *testing.T) {
	m, _, ctx := gmailTest(t)
	c, err := m.ReadMail(ctx, MailReadParams{Ref: "18f00000000000a1"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Thread || len(c.Messages) != 2 || c.Subject != "Reunión" {
		t.Fatalf("%+v", c)
	}
	if r := c.Messages[1]; r.Text != "Sí, el martes a las 10." || !r.Quotes {
		t.Fatalf("respuesta %q", r.Text)
	}
	out := FormatMailContent(c)
	for _, want := range []string{"# Reunión [hilo 18f00000000000a1 · 2 mensajes]", "## 1/2 · ", "## 2/2 · ", "[citas de mensajes anteriores omitidas]"} {
		if !strings.Contains(out, want) {
			t.Errorf("falta %q en:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Asunto: Re: Reunión") || strings.Count(out, "revisar el presupuesto") != 1 {
		t.Errorf("repetido:\n%s", out)
	}
	// El ID de un mensaje que no es el de su hilo: Gmail no lo encuentra como hilo y se lee el mensaje (con lo citado).
	c, err = m.ReadMail(ctx, MailReadParams{Ref: "18f00000000000a2"})
	if err != nil || c.Thread || !strings.Contains(c.Messages[0].Text, "> ¿Quedamos") {
		t.Fatalf("%+v %v", c, err)
	}
	// Con --mensaje, solo ese mensaje aunque sea también el ID del hilo.
	c, err = m.ReadMail(ctx, MailReadParams{Ref: "18f00000000000a1", MessageOnly: true})
	if err != nil || c.Thread || len(c.Messages) != 1 {
		t.Fatalf("%+v %v", c, err)
	}
	// Recorte: cada mensaje del hilo tiene su parte, con aviso.
	c, err = m.ReadMail(ctx, MailReadParams{Ref: "18f0000000000001", Max: 10})
	if err != nil || !c.Truncated || c.Messages[0].Text != "Hola Luis," || !strings.Contains(FormatMailContent(c), "[recortado a 10 caracteres") {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestGmailScopeNotGranted(t *testing.T) {
	// Conectado antes de Gmail (solo Drive): Drive sigue y Gmail sale como no autorizado, sin llamar a Gmail.
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	connect(t, m, o, "luis")
	ctx := WithUser(context.Background(), "luis")
	st := m.Status(ctx, "luis", false)
	if !st.Connected || !st.Drive || st.Gmail {
		t.Fatalf("%+v", st)
	}
	if _, err := m.SearchMail(ctx, "x", 0); !errors.Is(err, ErrNoGmail) {
		t.Fatalf("%v", err)
	}
	if _, err := m.List(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	if n := len(o.g.gmailCalls()); n != 0 {
		t.Fatalf("llamadas a Gmail: %d", n)
	}
	// «Volver a conectar»: pide los dos (con include_granted_scopes) y ya tiene Gmail.
	o.scope = fullScopes
	connect(t, m, o, "luis")
	if st := m.Status(ctx, "luis", true); !st.Drive || !st.Gmail {
		t.Fatalf("%+v", st)
	}
	if _, err := m.SearchMail(ctx, "x", 0); err != nil {
		t.Fatal(err)
	}
	// Sin cuenta conectada: el mismo aviso (el de Gmail, no el de Drive).
	if _, err := m.RecentMail(WithUser(context.Background(), "ana"), 0); !errors.Is(err, ErrNoGmail) {
		t.Fatalf("%v", err)
	}
}

func TestGmailOnlyGranted(t *testing.T) {
	// En la pantalla de Google se desmarca Drive: Gmail funciona y Drive sale como no autorizado (no falla todo).
	m, o := newOAuthTest(t)
	m.SaveClientFile([]byte(testClientFile))
	o.scope = "openid https://www.googleapis.com/auth/userinfo.email " + ScopeGmailReadonly
	connect(t, m, o, "luis")
	ctx := WithUser(context.Background(), "luis")
	st := m.Status(ctx, "luis", false)
	if !st.Connected || st.Drive || !st.Gmail {
		t.Fatalf("%+v", st)
	}
	if _, err := m.List(ctx, "", 0); !errors.Is(err, ErrNoAccess) || !strings.Contains(err.Error(), "permiso de Drive") {
		t.Fatalf("%v", err)
	}
	if _, err := m.RecentMail(ctx, 0); err != nil {
		t.Fatal(err)
	}
}

func TestGmailAPIErrors(t *testing.T) {
	m, o, ctx := gmailTest(t)
	// Gmail rechaza el ámbito aunque el token diga tenerlo: no autorizado (y se olvida el token).
	o.g.mu.Lock()
	o.g.gmailDeny = true
	o.g.mu.Unlock()
	if _, err := m.RecentMail(ctx, 0); !errors.Is(err, ErrNoGmail) {
		t.Fatalf("%v", err)
	}
	o.g.mu.Lock()
	o.g.gmailDeny, o.g.noGmail = false, true
	o.g.mu.Unlock()
	var ae *APIError
	if _, err := m.RecentMail(ctx, 0); !errors.As(err, &ae) || !strings.Contains(err.Error(), "API de Gmail") {
		t.Fatalf("%v", err)
	}
}

func TestGmailRefresh(t *testing.T) {
	m, o, ctx := gmailTest(t)
	// Google rechaza el token en memoria (401): se renueva y se repite, una vez.
	o.g.mu.Lock()
	for k := range o.g.scopes {
		o.g.expired[k] = true
	}
	o.g.mu.Unlock()
	n := o.refreshes
	if _, err := m.ReadMail(ctx, MailReadParams{Ref: "18f00000000000a1"}); err != nil || o.refreshes != n+1 {
		t.Fatalf("%v %d", err, o.refreshes-n)
	}
	// La renovación conserva los dos permisos.
	if st := m.Status(ctx, "luis", false); !st.Gmail || !st.Drive {
		t.Fatalf("%+v", st)
	}
}

func TestParseMailRef(t *testing.T) {
	for in, want := range map[string]string{
		"18f0a1b2c3d4e5f6":   "18f0a1b2c3d4e5f6",
		" 18f0a1b2c3d4e5f6 ": "18f0a1b2c3d4e5f6",
		"https://mail.google.com/mail/u/0/#inbox/18f0a1b2c3d4e5f6": "18f0a1b2c3d4e5f6",
	} {
		if got, err := ParseMailRef(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	if _, err := ParseMailRef("https://mail.google.com/mail/u/0/#inbox/FMfcgzQXJWbcdefghijklmn"); err == nil || !strings.Contains(err.Error(), "pgo gmail buscar") {
		t.Errorf("URL nueva: %v", err)
	}
	for _, bad := range []string{"", "corto", "../../etc", "https://example.com/18f0a1b2c3d4e5f6"} {
		if _, err := ParseMailRef(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
}

func TestMailText(t *testing.T) {
	if string(decodeBase64URL(b64("hola?>"))) != "hola?>" || string(decodeBase64URL(b64raw("hola?>"))) != "hola?>" || decodeBase64URL("") != nil || decodeBase64URL("%%%") != nil {
		t.Error("base64url")
	}
	if toUTF8([]byte("\x93hola\x94 \x80"), "windows-1252") != "“hola” €" || toUTF8([]byte("ni\xf1o"), "") != "niño" || toUTF8([]byte("niño"), "utf-8") != "niño" {
		t.Error("juegos de caracteres")
	}
	for in, want := range map[string]string{
		"Vale.\n\nOn Fri, Oct 10, 2026 at 10:00 AM Ana <ana@x.es> wrote:\n> hola": "Vale.",
		"Vale.\n-----Mensaje original-----\nDe: Ana":                              "Vale.",
		"Vale.\n\nDe: Ana <ana@x.es>\nEnviado: viernes\nPara: Luis\n\nhola":       "Vale.",
		"Mira esto:\n> cita\nsigue":                                               "Mira esto:\nsigue",
	} {
		if got, ok := stripQuoted(in); got != want || !ok {
			t.Errorf("%q: %q %v", in, got, ok)
		}
	}
	// Lo reenviado no es una cita.
	fwd := "Te lo reenvío.\n---------- Forwarded message ---------\nDe: Ana <ana@x.es>\nDate: hoy\nhola"
	if got, ok := stripQuoted(fwd); ok || got != fwd {
		t.Errorf("reenviado: %q", got)
	}
	if shortAddresses(strings.Repeat("persona@ejemplo.es, ", 40)) == strings.Repeat("persona@ejemplo.es, ", 40) {
		t.Error("Para largo sin recortar")
	}
	if s := shortAddresses(strings.Repeat("persona@ejemplo.es, ", 40)); !strings.Contains(s, "… (+") {
		t.Error(s)
	}
}
