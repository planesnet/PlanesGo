package gbrowser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDesktop hace de Electron: recoge cada trabajo del canal y responde con h. Devuelve el lector y los trabajos
// recibidos (las URL que se han cargado).
func fakeDesktop(t *testing.T, h func(Job) Result) (*Reader, func() []Job) {
	t.Helper()
	b := NewBridge()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	var mu sync.Mutex
	var jobs []Job
	go func() {
		for {
			j, ok := b.Next(ctx)
			if !ok {
				return
			}
			mu.Lock()
			jobs = append(jobs, j)
			mu.Unlock()
			r := h(j)
			r.ID = j.ID
			b.Deliver(r)
		}
	}()
	for !b.Available() {
		time.Sleep(time.Millisecond)
	}
	return New(b), func() []Job {
		mu.Lock()
		defer mu.Unlock()
		return append([]Job(nil), jobs...)
	}
}

func urls(js []Job) []string {
	var u []string
	for _, j := range js {
		u = append(u, j.Mode+" "+j.URL)
	}
	return u
}

func TestBridge(t *testing.T) {
	b := NewBridge()
	if _, err := b.Do(context.Background(), Job{URL: "https://docs.google.com/x"}); !errors.Is(err, ErrNoDesktop) {
		t.Fatalf("sin Electron: %v", err)
	}
	if b.Deliver(Result{ID: "nadie"}) {
		t.Fatal("entrega sin espera")
	}
	// Un trabajo que ya nadie espera (se acabó su contexto) no llega a Electron.
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
		b.Deliver(Result{ID: j.ID, Text: "hola"})
	}()
	r, err := b.Do(context.Background(), Job{URL: "https://docs.google.com/nuevo"})
	if err != nil || r.Text != "hola" {
		t.Fatalf("%+v %v", r, err)
	}
	// Sin consultas de Electron en un rato, la app de escritorio no está.
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
		switch {
		case strings.Contains(j.URL, "/open?id="):
			return Result{URL: "https://docs.google.com/document/d/1DocIdXyzw/edit", Status: 200}
		case strings.HasSuffix(j.URL, "/export?format=md"):
			return Result{Download: true, FileName: "Informe de ventas.md", Text: "# Ventas\n\nTodo bien.\n"}
		}
		return Result{Error: "no esperada"}
	})
	c, err := r.Read(context.Background(), 7, ReadParams{Ref: "1DocIdXyzw"})
	if err != nil {
		t.Fatal(err)
	}
	got := FormatContent(c)
	if got != "# Informe de ventas [doc · markdown · 1DocIdXyzw]\n# Ventas\n\nTodo bien.\n" {
		t.Fatalf("%q", got)
	}
	js := jobs()
	if strings.Join(urls(js), "\n") != "resolve https://drive.google.com/open?id=1DocIdXyzw\ntext https://docs.google.com/document/d/1DocIdXyzw/export?format=md" {
		t.Fatalf("%v", urls(js))
	}
	if js[0].Project != 7 || js[1].MaxChars != DefaultMaxChars {
		t.Fatalf("%+v", js[1])
	}
}

// Si la exportación no da un fichero (una página HTML), se prueba la siguiente forma; la última, la versión móvil.
func TestReadDocFallback(t *testing.T) {
	r, jobs := fakeDesktop(t, func(j Job) Result {
		if strings.HasSuffix(j.URL, "/mobilebasic") {
			return Result{Status: 200, Title: "Acta - Documentos de Google", ContentType: "text/html", Text: "Acta\nPunto 1"}
		}
		return Result{Status: 200, ContentType: "text/html", Text: "<algo>"}
	})
	c, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/document/d/1DocIdXyzw/edit", Max: 8})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs()) != 3 || c.Name != "Acta" || c.Format != "texto" || c.Text != "Acta\nPun" || !c.Truncated || c.Total != 12 {
		t.Fatalf("%v %+v", urls(jobs()), c)
	}
	if f := FormatContent(c); !strings.Contains(f, "[recortado: 8 caracteres de 12;") {
		t.Fatalf("%q", f)
	}
}

func TestReadErrors(t *testing.T) {
	for _, c := range []struct {
		res  Result
		want error
	}{
		{Result{Login: true, URL: "https://accounts.google.com/ServiceLogin"}, ErrLogin},
		{Result{Status: 404}, ErrNotFound},
		{Result{Status: 403}, ErrNoAccess},
		{Result{Status: 500}, nil},
		{Result{Status: 200, Title: "Google Docs", Text: "Necesitas acceso\nSolicitar acceso"}, ErrNoAccess},
		{Result{Status: 200, Text: "Lo sentimos, el archivo que has solicitado no existe."}, ErrNotFound},
	} {
		r, _ := fakeDesktop(t, func(Job) Result { return c.res })
		_, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/document/d/1DocIdXyzw/edit"})
		if c.want == nil && (err == nil || !strings.Contains(err.Error(), "(500)")) || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%+v: %v", c.res, err)
		}
	}
	r, _ := fakeDesktop(t, func(Job) Result { return Result{Error: "ERR_NAME_NOT_RESOLVED"} })
	if _, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/presentation/d/1SlidesIdXy/edit"}); err == nil || !strings.Contains(err.Error(), "ERR_NAME_NOT_RESOLVED") {
		t.Errorf("%v", err)
	}
}

func TestReadSheet(t *testing.T) {
	tabs := []Item{{ID: "0", Name: "Ventas"}, {ID: "55", Name: "Gastos"}}
	r, jobs := fakeDesktop(t, func(j Job) Result {
		switch {
		case strings.HasSuffix(j.URL, "/htmlview"):
			return Result{Status: 200, Title: "Cuentas 2026 - Hojas de cálculo de Google", Items: tabs}
		case strings.Contains(j.URL, "gid=0"):
			return Result{Download: true, FileName: "Cuentas 2026 - Ventas.csv", Text: "mes,total\nenero,10\n"}
		case strings.Contains(j.URL, "gid=55"):
			return Result{Download: true, Text: "mes,total\nenero,4\n"}
		}
		return Result{Error: "no esperada"}
	})
	c, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit"})
	if err != nil {
		t.Fatal(err)
	}
	want := "# Cuentas 2026 [hoja · csv · 1SheetIdXyz]\n## Ventas\nmes,total\nenero,10\n\n## Gastos\nmes,total\nenero,4\n[Hojas: Ventas, Gastos. Para una sola: --hoja <nombre> [--rango A1:D20].]\n"
	if got := FormatContent(c); got != want {
		t.Fatalf("%q", got)
	}
	// Una hoja por nombre, con rango.
	n := len(jobs())
	c, err = r.Read(context.Background(), 1, ReadParams{Ref: "1SheetIdXyz", Sheet: "gastos", Range: "A1:B2"})
	if err == nil {
		t.Fatalf("un ID suelto se resuelve primero: %+v", c)
	}
	c, err = r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit", Sheet: "gastos", Range: "A1:B2"})
	if err != nil || c.Text != "mes,total\nenero,4\n" || c.Note != "" {
		t.Fatalf("%+v %v", c, err)
	}
	last := jobs()[len(jobs())-1]
	if last.URL != "https://docs.google.com/spreadsheets/d/1SheetIdXyz/export?format=csv&gid=55&range=A1%3AB2" || len(jobs()) <= n {
		t.Fatalf("%s", last.URL)
	}
	// La hoja de la URL (#gid=55): sin pedir las pestañas.
	n = len(jobs())
	c, err = r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit#gid=55"})
	if err != nil || c.Text != "mes,total\nenero,4\n" || len(jobs()) != n+1 {
		t.Fatalf("%+v %v %v", c, err, urls(jobs()[n:]))
	}
	if _, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/spreadsheets/d/1SheetIdXyz/edit", Sheet: "Nada"}); err == nil || !strings.Contains(err.Error(), "hay: Ventas, Gastos") {
		t.Fatalf("%v", err)
	}
}

func TestReadSlidesFolderFile(t *testing.T) {
	r, _ := fakeDesktop(t, func(j Job) Result {
		switch {
		case strings.HasSuffix(j.URL, "/export/txt"):
			return Result{Download: true, FileName: "Plan.txt", Text: "Diapositiva 1"}
		case j.Mode == ModeDrive:
			return Result{Status: 200, Items: []Item{{ID: "1DocIdXyzw", Lines: []string{"Acta", "12 oct 2026", "yo"}, Href: "https://docs.google.com/document/d/1DocIdXyzw/edit"}}}
		}
		return Result{Error: "no esperada"}
	})
	c, err := r.Read(context.Background(), 1, ReadParams{Ref: "https://docs.google.com/presentation/d/1SlidesIdXy/edit"})
	if err != nil || FormatContent(c) != "# Plan [presentación · texto · 1SlidesIdXy]\nDiapositiva 1\n" {
		t.Fatalf("%q %v", FormatContent(c), err)
	}
	c, err = r.Read(context.Background(), 1, ReadParams{Ref: "https://drive.google.com/drive/folders/1FolderIdXy"})
	if err != nil || c.Text != "[doc] Acta · 12 oct 2026 · 1DocIdXyzw\n" {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = r.Read(context.Background(), 1, ReadParams{Ref: "https://drive.google.com/file/d/1FileIdXyzw/view"})
	if err != nil || c.Format != "enlace" || !strings.Contains(c.Note, "https://drive.google.com/file/d/1FileIdXyzw/view") {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestSearch(t *testing.T) {
	rows := []Item{
		{ID: "1FolderIdXy", Lines: []string{"Clientes", "yo", "3 sept 2026"}, Labels: []string{"Carpeta compartida"}},
		{ID: "1SheetIdXyz", Lines: []string{"Ventas 2026", "yo", "10:32"}, Labels: []string{"Hoja de cálculo de Google"}},
		{ID: "1DocIdXyzw", Lines: []string{"Ventas: informe", "Oct 12, 2026"}, Href: "https://docs.google.com/document/d/1DocIdXyzw/edit"},
		{ID: "1SinNombre1", Lines: nil},
	}
	r, jobs := fakeDesktop(t, func(j Job) Result { return Result{Status: 200, Items: rows} })
	fs, err := r.Search(context.Background(), 3, SearchParams{Text: "ventas"})
	if err != nil {
		t.Fatal(err)
	}
	want := "[carpeta] Clientes · 3 sept 2026 · 1FolderIdXy\n[hoja] Ventas 2026 · 10:32 · 1SheetIdXyz\n[doc] Ventas: informe · Oct 12, 2026 · 1DocIdXyzw\n"
	if got := FormatFiles(fs); got != want {
		t.Fatalf("%q", got)
	}
	if u := jobs()[0].URL; u != "https://drive.google.com/drive/search?q=ventas" {
		t.Fatal(u)
	}
	fs, err = r.Search(context.Background(), 3, SearchParams{Text: "ventas", Type: "hoja", Max: 5})
	if err != nil || len(fs) != 1 || fs[0].ID != "1SheetIdXyz" || jobs()[1].URL != "https://drive.google.com/drive/search?q=ventas+type%3Aspreadsheet" {
		t.Fatalf("%v %v %s", fs, err, jobs()[1].URL)
	}
	// En una carpeta: su listado, filtrado por nombre.
	fs, err = r.Search(context.Background(), 3, SearchParams{Text: "informe", Folder: "1FolderIdXy"})
	if err != nil || len(fs) != 1 || fs[0].ID != "1DocIdXyzw" || jobs()[2].URL != "https://drive.google.com/drive/folders/1FolderIdXy" {
		t.Fatalf("%v %v", fs, err)
	}
	if _, err := r.Search(context.Background(), 3, SearchParams{Type: "vídeo"}); err == nil {
		t.Fatal("tipo desconocido")
	}
	if _, err := r.Search(context.Background(), 3, SearchParams{}); err == nil {
		t.Fatal("búsqueda vacía")
	}
	fs, err = r.List(context.Background(), 3, "", 2)
	if err != nil || len(fs) != 2 || jobs()[len(jobs())-1].URL != "https://drive.google.com/drive/my-drive" {
		t.Fatalf("%v %v", fs, err)
	}
	if _, err := r.List(context.Background(), 3, "https://docs.google.com/document/d/1DocIdXyzw/edit", 0); err == nil {
		t.Fatal("un documento no es una carpeta")
	}
}

func TestGmail(t *testing.T) {
	var rows []Item
	for i := 0; i < 3; i++ {
		rows = append(rows, Item{ID: "18f0a1b2c3d4e5f" + string(rune('0'+i)), From: "Ana  Pérez", Name: "Factura octubre", Date: "12 oct", Snippet: " - Te mando la factura de octubre", Unread: i == 0})
	}
	thread := []Item{
		{Text: "Ana Pérez <ana@x.com>\t12 oct 2026\nHola, ¿me mandas la factura?"},
		{Text: "Luis <luis@x.com>\n13 oct 2026\nAquí la tienes.\n\nEl 12 oct 2026, Ana Pérez escribió:\n> Hola, ¿me mandas la factura?"},
	}
	r, jobs := fakeDesktop(t, func(j Job) Result {
		if j.Mode == ModeGmailPrint {
			return Result{Status: 200, Title: "Factura octubre", Items: thread}
		}
		return Result{Status: 200, Items: rows}
	})
	l, err := r.SearchMail(context.Background(), 2, "from:ana newer_than:7d", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := "12 oct · Ana Pérez · Factura octubre · 18f0a1b2c3d4e5f0 · no leído\n  Te mando la factura de octubre\n" +
		"12 oct · Ana Pérez · Factura octubre · 18f0a1b2c3d4e5f1\n  Te mando la factura de octubre\n[hay más: afina la consulta o pide más con --max N, hasta 50]\n"
	if got := FormatMails(l); got != want {
		t.Fatalf("%q", got)
	}
	if u := jobs()[0].URL; u != "https://mail.google.com/mail/u/0/#search/from:ana%20newer_than:7d" || jobs()[0].Mode != ModeGmailList {
		t.Fatal(u)
	}
	if _, err := r.RecentMail(context.Background(), 2, 0); err != nil || jobs()[1].URL != "https://mail.google.com/mail/u/0/#inbox" {
		t.Fatal(err, jobs()[1].URL)
	}
	c, err := r.ReadMail(context.Background(), 2, "https://mail.google.com/mail/u/0/#inbox/18F0A1B2C3D4E5F0", 0)
	if err != nil {
		t.Fatal(err)
	}
	want = "# Factura octubre [hilo 18f0a1b2c3d4e5f0 · 2 mensajes]\n\n## 1/2\nAna Pérez <ana@x.com>\t12 oct 2026\nHola, ¿me mandas la factura?\n\n## 2/2\nLuis <luis@x.com>\n13 oct 2026\nAquí la tienes.\n"
	if got := FormatMailContent(c); got != want {
		t.Fatalf("%q", got)
	}
	if u := jobs()[2].URL; u != "https://mail.google.com/mail/u/0/?search=all&th=18f0a1b2c3d4e5f0&ui=2&view=pt" {
		t.Fatal(u)
	}
	for _, bad := range []string{"", "hola", "https://mail.google.com/mail/u/0/#inbox/FMfcgzQXJWDsKmGgQkLwzrbPbDdcQqKx"} {
		if _, err := ParseMailRef(bad); err == nil {
			t.Errorf("%q debería fallar", bad)
		}
	}
	if _, err := r.SearchMail(context.Background(), 2, " ", 0); err == nil {
		t.Fatal("búsqueda vacía")
	}
}

func TestShare(t *testing.T) {
	long := strings.Repeat("a", 5000)
	out, cut := share([]string{"corto", long, long}, 2000, false)
	if !cut || out[0] != "corto" || len([]rune(out[1])) != 2000/3+len([]rune(" […]")) {
		t.Fatalf("%v %d", cut, len(out[1]))
	}
	// Con muchos mensajes, cada uno al menos minMailChars.
	if out, _ := share([]string{long, long, long, long, long}, 2000, false); len([]rune(out[0])) != minMailChars+len([]rune(" […]")) {
		t.Fatal(len([]rune(out[0])))
	}
	if out, cut := share([]string{"a", "b"}, 10, false); cut || len(out) != 2 {
		t.Fatal("sin recorte")
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
	// Con algo que no es suyo dentro, la carpeta se queda (y lo de dentro también).
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
