package procesos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// Un programa sin salida devuelve una lista vacía, no nil (en JSON, null tumbaba la interfaz al hacer join).
func TestSalidaVaciaNoNil(t *testing.T) {
	g := Nuevo()
	p, err := g.Lanza("callado", 1, t.TempDir(), "true", "usuario", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-p.hecho
	l, _, err := g.Salida("callado", 10)
	if err != nil || l == nil {
		t.Fatalf("salida %v (%v)", l, err)
	}
	if b, _ := json.Marshal(l); string(b) != "[]" {
		t.Errorf("JSON %s", b)
	}
}

// El registro: la salida completa en <carpeta>/.pgocode/salidas, aunque en memoria solo queden las últimas líneas.
func TestRegistro(t *testing.T) {
	g := Nuevo()
	dir := t.TempDir()
	p, err := g.Lanza("largo", 1, dir, "seq 1 3000", "agente", nil)
	if err != nil {
		t.Fatal(err)
	}
	<-p.hecho
	for i := 0; i < 50; i++ { // lee acaba un poco después de Wait
		if c, _ := g.Busca("largo"); c.Registro != "" {
			if b, _ := os.ReadFile(c.Registro); strings.Count(string(b), "\n") == 3000 {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, _ := g.Busca("largo")
	if c.Registro != filepath.Join(DirRegistros(dir), "programa-largo.log") {
		t.Fatalf("registro %q", c.Registro)
	}
	b, err := os.ReadFile(c.Registro)
	if err != nil || !strings.HasPrefix(string(b), "1\n2\n") || !strings.HasSuffix(string(b), "\n3000\n") {
		t.Fatalf("registro completo: %v %d", err, len(b))
	}
	if l, total, _ := g.Salida("largo", 0); len(l) != maxLineas || total != 3000 {
		t.Fatalf("en memoria, las últimas: %d de %d", len(l), total)
	}
	// Más de las que hay en memoria: del registro.
	if l, total, _ := g.Salida("largo", 2000); len(l) != 2000 || total != 3000 || l[0] != "1001" || l[1999] != "3000" {
		t.Fatalf("del registro: %d de %d", len(l), total)
	}
}
