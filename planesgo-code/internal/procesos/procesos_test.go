package procesos

import (
	"testing"
	"time"
)

func TestURL(t *testing.T) {
	for l, quiero := range map[string]string{
		"  VITE ready: ➜  Local:   http://localhost:5173/":                            "http://localhost:5173/",
		"INFO ? odoo.service.server: HTTP service (werkzeug) running on 0.0.0.0:8069": "http://localhost:8069/",
		"Serving on http://0.0.0.0:8000.":                                             "http://localhost:8000",
		"nada que ver":                                                                "",
	} {
		if u := urlDe(l); u != quiero {
			t.Errorf("%q: %q, quiero %q", l, u, quiero)
		}
	}
}

func TestLanzaYPara(t *testing.T) {
	g := Nuevo()
	if _, err := g.Lanza("../x", 1, t.TempDir(), "true", "usuario", nil); err == nil {
		t.Error("nombre no válido aceptado")
	}
	p, err := g.Lanza("web", 1, t.TempDir(), "echo listo en http://127.0.0.1:9999/; sleep 30", "agente", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		if c, _ := g.Busca(p.Nombre); c.URL != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c, _ := g.Busca("web"); c.URL != "http://127.0.0.1:9999/" || c.Estado != "curso" {
		t.Fatalf("proceso: %+v", c)
	}
	if !g.Para("web") {
		t.Fatal("no se paró")
	}
	if c, _ := g.Busca("web"); c.Estado != "parado" {
		t.Errorf("estado %q", c.Estado)
	}
	if l, _, _ := g.Salida("web", 10); len(l) != 1 {
		t.Errorf("salida %q", l)
	}
}
