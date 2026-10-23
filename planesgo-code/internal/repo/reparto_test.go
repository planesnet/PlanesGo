package repo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// madreConFichero: un repositorio con un fichero versionado y la rama «main».
func madreConFichero(t *testing.T) (string, func(dir string, args ...string)) {
	t.Helper()
	r, run := repoPrueba(t)
	os.WriteFile(filepath.Join(r, "base.txt"), []byte("base\n"), 0o644)
	run(r, "add", "-A")
	run(r, "commit", "-q", "-m", "base")
	return r, run
}

func escribe(t *testing.T, dir, nombre, texto string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, nombre), []byte(texto), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hija crea la copia de la hija n y deja su trabajo (commiteado o no).
func hija(t *testing.T, r string, n int) (CopiaCreada, string) {
	t.Helper()
	rama, err := RamaHija("sesion1", n)
	if err != nil {
		t.Fatal(err)
	}
	c, ramaMadre, err := CopiaHija(context.Background(), r, rama, filepath.Join(filepath.Dir(r), "hijas", "s1-"+string(rune('0'+n))))
	if err != nil {
		t.Fatal(err)
	}
	return c, ramaMadre
}

func TestRamaHija(t *testing.T) {
	if r, err := RamaHija("abc_1-x", 3); err != nil || r != "pgo/abc_1-x-3" {
		t.Fatal(r, err)
	}
	for _, m := range []string{"", "a/b", "a b", "..", "-x"[:0], strings.Repeat("a", 70)} {
		if _, err := RamaHija(m, 1); err == nil {
			t.Errorf("madre %q aceptada", m)
		}
	}
	if _, err := RamaHija("a", 0); err == nil {
		t.Error("n=0 aceptado")
	}
}

func TestCopiaHijaParteDeLaRamaDeLaMadre(t *testing.T) {
	r, run := madreConFichero(t)
	run(r, "checkout", "-q", "-b", "trabajo")
	escribe(t, r, "previo.txt", "previo\n")
	run(r, "add", "-A")
	run(r, "commit", "-q", "-m", "previo")
	c, ramaMadre := hija(t, r, 1)
	if ramaMadre != "trabajo" || c.Rama != "pgo/sesion1-1" || !c.RamaNueva || !existe(filepath.Join(c.Carpeta, "previo.txt")) {
		t.Fatalf("%+v %s", c, ramaMadre)
	}
	if RamaActual(context.Background(), r) != "trabajo" {
		t.Error("la madre ha cambiado de rama")
	}
	// Otra vez la misma: ni la carpeta ni la rama se reutilizan.
	if _, _, err := CopiaHija(context.Background(), r, c.Rama, c.Carpeta); err == nil {
		t.Error("se ha repetido una copia")
	}
	if _, _, err := CopiaHija(context.Background(), r, c.Rama, c.Carpeta+"b"); err == nil {
		t.Error("se ha repetido una rama")
	}
	if _, _, err := CopiaHija(context.Background(), r, "main", c.Carpeta+"c"); err == nil {
		t.Error("rama sin el prefijo de reparto")
	}
}

func TestCopiaHijaSinRamaNoSeReparte(t *testing.T) {
	r, run := madreConFichero(t)
	run(r, "checkout", "-q", "--detach")
	if _, _, err := CopiaHija(context.Background(), r, "pgo/x-1", filepath.Join(filepath.Dir(r), "h")); err == nil {
		t.Fatal("repartido desde un commit suelto")
	}
}

func TestUnionLimpiaYSeQuitaLaCopia(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	var hs []CopiaCreada
	for n := 1; n <= 3; n++ {
		c, _ := hija(t, r, n)
		escribe(t, c.Carpeta, "parte"+string(rune('0'+n))+".txt", "hecho\n")
		if n != 2 { // la 2 se deja sin commitear: lo guarda pgocode
			run(c.Carpeta, "add", "-A")
			run(c.Carpeta, "commit", "-q", "-m", "parte")
		}
		hs = append(hs, c)
	}
	for i, c := range hs {
		u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", i+1, []string{"", "Añade el cupón VERANO.\nCon pruebas", ""}[i])
		if err != nil || u.Estado != "unida" || !u.Quitada || !u.Rama || (i == 1) != u.Commit {
			t.Fatalf("hija %d: %+v %v", i+1, u, err)
		}
		if existe(c.Carpeta) {
			t.Errorf("la copia %d sigue", i+1)
		}
	}
	for n := 1; n <= 3; n++ {
		if !existe(filepath.Join(r, "parte"+string(rune('0'+n))+".txt")) {
			t.Errorf("falta parte%d en la madre", n)
		}
	}
	if ok, _ := Limpia(ctx, r); !ok {
		t.Error("la madre no queda limpia")
	}
	if l, _ := git(ctx, r, "branch", "--list", "pgo/*"); l != "" {
		t.Errorf("quedan ramas: %s", l)
	}
	if m, _ := git(ctx, r, "log", "--merges", "--format=%s"); strings.Count(m, "pgocode: une la parte") != 3 {
		t.Errorf("merges: %q", m)
	}
	if m, _ := git(ctx, r, "log", "--all", "--format=%s"); !strings.Contains(m, "pgocode: parte 2 — Añade el cupón VERANO\n") ||
		!strings.Contains(m, "pgocode: une la parte 2 (pgo/") || !strings.Contains(m, ") — Añade el cupón VERANO") {
		t.Errorf("sin el commit automático: %q", m)
	}
	if l := ListaCopias(ctx, r); len(l) != 0 {
		t.Errorf("copias: %v", l)
	}
}

func TestUnionConConflictoSeConserva(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	a, _ := hija(t, r, 1)
	b, _ := hija(t, r, 2)
	escribe(t, a.Carpeta, "base.txt", "de la uno\n")
	escribe(t, b.Carpeta, "base.txt", "de la dos\n")
	escribe(t, b.Carpeta, "otra.txt", "otra\n")
	for _, c := range []CopiaCreada{a, b} {
		run(c.Carpeta, "add", "-A")
		run(c.Carpeta, "commit", "-q", "-m", "parte")
	}
	if u, err := UneHija(ctx, r, a.Carpeta, a.Rama, "main", 1, ""); err != nil || u.Estado != "unida" {
		t.Fatalf("%+v %v", u, err)
	}
	u, err := UneHija(ctx, r, b.Carpeta, b.Rama, "main", 2, "")
	if err != nil || u.Estado != "conflicto" || len(u.Ficheros) != 1 || u.Ficheros[0] != "base.txt" || u.Quitada {
		t.Fatalf("%+v %v", u, err)
	}
	if ok, _ := Limpia(ctx, r); !ok {
		t.Fatal("la madre no ha quedado limpia tras abortar")
	}
	if got, _ := os.ReadFile(filepath.Join(r, "base.txt")); string(got) != "de la uno\n" {
		t.Errorf("la madre ha cambiado: %q", got)
	}
	if !existe(b.Carpeta) || RamaActual(ctx, b.Carpeta) != b.Rama {
		t.Error("la copia de la hija en conflicto no se conserva")
	}
	if _, err := git(ctx, r, "show-ref", "--verify", "--quiet", "refs/heads/"+b.Rama); err != nil {
		t.Error("la rama de la hija se ha perdido")
	}
}

func TestUnionPendienteSiLaMadreEstaSucia(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	c, _ := hija(t, r, 1)
	escribe(t, c.Carpeta, "x.txt", "x\n")
	run(c.Carpeta, "add", "-A")
	run(c.Carpeta, "commit", "-q", "-m", "p")
	escribe(t, r, "suelto.txt", "sin seguimiento\n")
	u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", 1, "")
	if err != nil || u.Estado != "pendiente" || !strings.Contains(u.Motivo, "cambios sin guardar") || u.Quitada {
		t.Fatalf("%+v %v", u, err)
	}
	if existe(filepath.Join(r, "x.txt")) || !existe(c.Carpeta) {
		t.Fatal("ha tocado algo con la madre sucia")
	}
	// Limpia la madre: ahora se une.
	os.Remove(filepath.Join(r, "suelto.txt"))
	if u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", 1, ""); err != nil || u.Estado != "unida" || !u.Quitada {
		t.Fatalf("%+v %v", u, err)
	}
	// Cambios en un fichero versionado también cuentan.
	c2, _ := hija(t, r, 2)
	escribe(t, c2.Carpeta, "y.txt", "y\n")
	escribe(t, r, "base.txt", "modificado\n")
	if u, _ := UneHija(ctx, r, c2.Carpeta, c2.Rama, "main", 2, ""); u.Estado != "pendiente" {
		t.Fatalf("%+v", u)
	}
	// Con lo de la hija ya guardado en su rama (commit automático): no se pierde.
	if l, _ := git(ctx, c2.Carpeta, "log", "-1", "--format=%s"); l != "pgocode: parte 2" {
		t.Errorf("sin commit automático: %q", l)
	}
}

func TestUnionPendienteSiLaMadreCambiaDeRama(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	c, ramaMadre := hija(t, r, 1)
	escribe(t, c.Carpeta, "x.txt", "x\n")
	run(r, "checkout", "-q", "-b", "otra")
	u, err := UneHija(ctx, r, c.Carpeta, c.Rama, ramaMadre, 1, "")
	if err != nil || u.Estado != "pendiente" || !strings.Contains(u.Motivo, "ya no está en la rama") {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestUnionSinCambios(t *testing.T) {
	ctx := context.Background()
	r, _ := madreConFichero(t)
	c, _ := hija(t, r, 1)
	u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", 1, "")
	if err != nil || u.Estado != "sin_cambios" || !u.Quitada || !u.Rama || existe(c.Carpeta) {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestUnionConStashNoQuitaLaCopia(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	c, _ := hija(t, r, 1)
	escribe(t, c.Carpeta, "x.txt", "x\n")
	run(c.Carpeta, "add", "-A")
	run(c.Carpeta, "commit", "-q", "-m", "p")
	escribe(t, c.Carpeta, "guardado.txt", "g\n")
	run(c.Carpeta, "stash", "push", "-q", "-u")
	u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", 1, "")
	if err != nil || u.Estado != "unida" || u.Quitada || u.Aviso == "" || !existe(c.Carpeta) {
		t.Fatalf("%+v %v", u, err)
	}
}

func TestUnionRechazaLoQueNoEsDeReparto(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	d := filepath.Join(filepath.Dir(r), "dev")
	if err := PreparaRama(ctx, r, "dev", d); err != nil {
		t.Fatal(err)
	}
	if _, err := UneHija(ctx, r, d, "dev", "main", 1, ""); err == nil {
		t.Error("unió una rama sin el prefijo de reparto")
	}
	if _, err := UneHija(ctx, r, r, "pgo/x-1", "main", 1, ""); err == nil {
		t.Error("unió la carpeta principal")
	}
	// Una copia de otro repositorio.
	o, runO := madreConFichero(t)
	_ = runO
	c, _ := hija(t, o, 1)
	if _, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", 1, ""); err == nil {
		t.Error("unió la copia de otro repositorio")
	}
	// La madre no se toca.
	run(r, "status", "--porcelain")
}

func TestQuitaUnidaNoBorraLoQueNoEstaUnido(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	c, _ := hija(t, r, 1)
	escribe(t, c.Carpeta, "x.txt", "x\n")
	run(c.Carpeta, "add", "-A")
	run(c.Carpeta, "commit", "-q", "-m", "p")
	if q, rb, err := QuitaUnida(ctx, c.Carpeta, r); err == nil || q || rb || !existe(c.Carpeta) {
		t.Fatalf("quitó una rama sin unir: %v %v %v", q, rb, err)
	}
	escribe(t, c.Carpeta, "y.txt", "y\n")
	if _, _, err := QuitaUnida(ctx, c.Carpeta, r); err == nil || !existe(filepath.Join(c.Carpeta, "y.txt")) {
		t.Fatal("quitó una copia con cambios")
	}
}

// Cuatro hijas a la vez: sus copias se crean y se trabajan en paralelo; la unión, de una en una.
func TestCuatroHijasALaVez(t *testing.T) {
	ctx := context.Background()
	r, run := madreConFichero(t)
	var wg sync.WaitGroup
	hs := make([]CopiaCreada, 4)
	for n := 1; n <= 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rama, _ := RamaHija("par", n)
			c, _, err := CopiaHija(ctx, r, rama, filepath.Join(filepath.Dir(r), "h", rama[4:]))
			if err != nil {
				t.Error(err)
				return
			}
			escribe(t, c.Carpeta, "p"+string(rune('0'+n))+".txt", "x\n")
			hs[n-1] = c
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	if l := ListaCopias(ctx, r); len(l) != 4 {
		t.Fatalf("copias: %v", l)
	}
	var mu sync.Mutex
	for n, c := range hs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock() // la unión se serializa por repositorio (lo hace el servidor)
			defer mu.Unlock()
			if u, err := UneHija(ctx, r, c.Carpeta, c.Rama, "main", n+1, ""); err != nil || u.Estado != "unida" || !u.Quitada {
				t.Errorf("hija %d: %+v %v", n+1, u, err)
			}
		}()
	}
	wg.Wait()
	for n := 1; n <= 4; n++ {
		if !existe(filepath.Join(r, "p"+string(rune('0'+n))+".txt")) {
			t.Errorf("falta p%d", n)
		}
	}
	_ = run
}

func TestMensajeParte(t *testing.T) {
	if m := MensajeParte(3, ""); m != "pgocode: parte 3" {
		t.Error(m)
	}
	largo := strings.Repeat("palabra ", 20)
	if m := MensajeParte(1, "  añade   el cupón\nmás detalle"); m != "pgocode: parte 1 — añade el cupón" {
		t.Error(m)
	}
	if m := []rune(MensajeParte(1, largo)); len(m) > 100 || m[len(m)-1] != '…' {
		t.Error(string(m))
	}
}
