package github

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestDeRemoto(t *testing.T) {
	for r, q := range map[string]string{
		"https://github.com/planesnet/madpla.git": "planesnet/madpla",
		"git@github.com:planesnet/PlanesGo.git":   "planesnet/PlanesGo",
		"https://gitlab.com/a/b":                  "",
		"https://github.com/a/b/c":                "",
	} {
		if g := DeRemoto(r); g != q {
			t.Errorf("%s: %q", r, g)
		}
	}
}

func TestClaseYFase(t *testing.T) {
	if clase("Feature", nil) != "peticion" || clase("", []Etiqueta{{Nombre: "corrección"}}) != "mantenimiento" || clase("", nil) != "sin-clasificar" {
		t.Error("clase")
	}
	if fase("OPEN", []Etiqueta{{Nombre: "en-test"}}, nil) != "test" ||
		fase("OPEN", nil, []PRCorta{{Estado: "OPEN"}}) != "revision" ||
		fase("OPEN", nil, []PRCorta{{Estado: "OPEN", Borrador: true}}) != "desarrollo" ||
		fase("OPEN", nil, nil) != "por-empezar" || fase("CLOSED", nil, nil) != "cerrada" {
		t.Error("fase")
	}
}

// Con red y «gh»: PGO_PRUEBA_GITHUB=planesnet/madpla go test ./internal/github
func TestLeeReal(t *testing.T) {
	repo := os.Getenv("PGO_PRUEBA_GITHUB")
	if _, err := exec.LookPath("gh"); repo == "" || err != nil {
		t.Skip("sin PGO_PRUEBA_GITHUB")
	}
	e, err := (&Lector{}).Lee(context.Background(), repo, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d issues abiertas, %d PRs abiertas, %d leídas, %d PRs leídas", e.Repo, e.Abiertas, e.PRsAbiert, len(e.Issues), len(e.PRs))
	for _, i := range e.Issues[:min(5, len(e.Issues))] {
		t.Logf("#%d %s [%s/%s] %v", i.Numero, i.Titulo, i.Clase, i.Fase, i.PRs)
	}
	for _, p := range e.PRs[:min(3, len(e.PRs))] {
		t.Logf("PR #%d %s→%s %s checks=%s rev=%s +%d-%d", p.Numero, p.Rama, p.Base, p.Estado, p.Checks, p.Revision, p.Mas, p.Menos)
	}
}

func TestCuerpoReal(t *testing.T) {
	repo := os.Getenv("PGO_PRUEBA_GITHUB")
	if repo == "" {
		t.Skip("sin PGO_PRUEBA_GITHUB")
	}
	c, err := (&Lector{}).Cuerpo(context.Background(), repo, 582)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("cuerpo (%d bytes): %.120s", len(c), c)
}

func TestAccionNoValida(t *testing.T) {
	var l Lector
	for _, c := range []struct {
		repo, tipo, accion string
		n                  int
	}{{"a/b", "issue", "fusionar", 1}, {"a/b", "pr", "borrar", 1}, {"a/b", "otro", "cerrar", 1}, {"a/b; rm", "issue", "cerrar", 1}, {"a/b", "issue", "cerrar", 0}} {
		if err := l.Accion(context.Background(), c.repo, c.tipo, c.n, c.accion); err == nil || err.Error() != "acción no válida" {
			t.Errorf("%+v: %v", c, err)
		}
	}
}
