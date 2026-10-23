package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOffloadReal: las salidas enormes a fichero con un agente de verdad (gasta tokens: céntimos con los modelos
// baratos). Una conversación de cinco turnos con una orden de 400 líneas, la lectura de un módulo de 600 líneas,
// un registro de 25 KB leído con cat, una pregunta sobre lo de antes y otra que obliga a releer un trozo del
// módulo; cada turno, con y sin las salidas a fichero, y el consumo de cada uno.
//
//	PGO_PRUEBA_SALIDAS=opencode|claude-code PGO_PRUEBA_SALIDAS_DIR=<carpeta en ~/.cache> go test -run TestOffloadReal -v -timeout 60m ./internal/llm
//
// Con OpenCode, HOME tiene que apuntar a una carpeta de prueba (sus sesiones van a HOME/.local/share/pgocode) y
// GOOGLE_APPLICATION_CREDENTIALS a las credenciales de Google.
func TestOffloadReal(t *testing.T) {
	clave := os.Getenv("PGO_PRUEBA_SALIDAS")
	base := os.Getenv("PGO_PRUEBA_SALIDAS_DIR")
	if clave == "" || base == "" {
		t.Skip()
	}
	var a *Agente
	for _, p := range Agentes() {
		if x, ok := p.(*Agente); ok && x.Clave == clave {
			a = x
		}
	}
	modelo := map[string]string{"opencode": "google-vertex/gemini-3.1-flash-lite", "claude-code": "haiku"}[clave]
	if m := os.Getenv("PGO_PRUEBA_SALIDAS_MODELO"); m != "" {
		modelo = m
	}
	pruneDirFunc = func() string { return filepath.Join(base, "poda") }
	turnos := []string{
		"Ejecuta `bash pruebas.sh` y dime qué prueba falla y por qué. Una línea.",
		"Lee inventario.py entero (todas sus líneas, con tu herramienta de lectura) y dime qué tipo de IVA aplica precio_con_iva. Una línea.",
		"Muestra registro.txt entero con `cat registro.txt` (no uses grep) y dime en qué puerto falla la conexión. Una línea.",
		"Sin ejecutar nada: ¿qué prueba fallaba al principio y qué puerto salía en el error del registro? Una línea.",
		"En inventario.py, ¿qué porcentaje de descuento da descuento_volumen a partir de 500 unidades? Una línea.",
	}
	comprueba := []string{"descuento", "21", "5433", "5433", "12"}
	if os.Getenv("PGO_PRUEBA_SALIDAS_ESCENARIO") == "largo" {
		// Turnos largos: tras cada salida grande, muchos pasos pequeños (lo normal en una tarea: la salida grande
		// se arrastra en todos los pasos que siguen).
		pasos := "Después, una por una (cada orden en una llamada distinta a tu herramienta, sin juntarlas), ejecuta: `wc -l inventario.py`, `ls -la`, `date`, `whoami`, `pwd`, `git log --oneline`, `python3 --version`, `head -3 registro.txt`, `tail -1 registro.txt`, `uname -r`. "
		turnos = []string{
			"Ejecuta `bash pruebas.sh`. " + pasos + "Al final, dime en una línea qué prueba falla y por qué.",
			"Lee inventario.py entero (todas sus líneas, con tu herramienta de lectura). " + pasos + "Al final, dime en una línea qué tipo de IVA aplica precio_con_iva.",
			"Muestra registro.txt entero con `cat registro.txt` (no uses grep). " + pasos + "Al final, dime en una línea en qué puerto falla la conexión.",
			"Sin ejecutar nada: ¿qué prueba fallaba al principio y qué puerto salía en el error del registro? Una línea.",
			"En inventario.py, ¿qué porcentaje de descuento da descuento_volumen a partir de 500 unidades? Una línea.",
		}
	}
	modos := []string{"con", "sin"}
	if m := os.Getenv("PGO_PRUEBA_SALIDAS_MODOS"); m != "" {
		modos = strings.Split(m, ",")
	}
	for _, modo := range modos {
		offloadOnWrite, offloadInSweep = modo == "con", modo == "con"
		carpeta := filepath.Join(base, clave+"-"+modo+fmt.Sprint(time.Now().Unix()%100000))
		preparaSalidas(t, carpeta)
		ctx, cancela := context.WithTimeout(context.Background(), 20*time.Minute)
		ses := ""
		var tot Uso
		for i, msg := range turnos {
			var texto strings.Builder
			var u Uso
			var rep *PruneReport
			err := a.Chat(ctx, Peticion{Carpeta: carpeta, SesionAgente: ses, Permisos: "auto", Prune: PruneRules, Idioma: "español",
				Modelo: modelo, Mensajes: []Mensaje{{Rol: "user", Texto: msg}}}, func(e Evento) {
				texto.WriteString(e.Texto)
				if e.SesionAgente != "" {
					ses = e.SesionAgente
				}
				if x := e.Uso; x != nil { // uno por turno (Claude Code) o por paso (OpenCode): se suman
					u.Entrada += x.Entrada
					u.CacheLeida += x.CacheLeida
					u.CacheEscrita += x.CacheEscrita
					u.Coste += x.Coste
					u.Llamadas += max(x.Llamadas, 1)
					u.Contexto = x.Contexto
				}
				if e.Prune != nil {
					rep = e.Prune
				}
			})
			if err != nil {
				t.Fatalf("%s turno %d: %v", modo, i+1, err)
			}
			ret := 0
			if rep != nil {
				ret = rep.Removed
			}
			ok := strings.Contains(texto.String(), comprueba[i])
			t.Logf("%s T%d: llamadas %d · entrada %d (caché leída %d, escrita %d) · contexto final %d · coste %.4f $ · podado %d · ok %v · %.120q",
				modo, i+1, u.Llamadas, u.Entrada, u.CacheLeida, u.CacheEscrita, u.Contexto, u.Coste, ret, ok, strings.TrimSpace(texto.String()))
			tot.Entrada += u.Entrada
			tot.CacheLeida += u.CacheLeida
			tot.CacheEscrita += u.CacheEscrita
			tot.Coste += u.Coste
			tot.Llamadas += u.Llamadas
			time.Sleep(4 * time.Second) // el marcado del turno, en segundo plano
		}
		cancela()
		sal, _ := filepath.Glob(filepath.Join(carpeta, ".pgocode", "salidas", "*"))
		t.Logf("%s TOTAL: llamadas %d · entrada %d (caché leída %d, escrita %d, sin caché %d) · coste %.4f $ · ficheros de salidas %d",
			modo, tot.Llamadas, tot.Entrada, tot.CacheLeida, tot.CacheEscrita, tot.Entrada-tot.CacheLeida-tot.CacheEscrita, tot.Coste, len(sal))
	}
}

// preparaSalidas: el proyecto de la prueba (repositorio git con .planesgo.json).
func preparaSalidas(t *testing.T, carpeta string) {
	t.Helper()
	os.MkdirAll(carpeta, 0o755)
	var b strings.Builder
	b.WriteString("#!/bin/bash\n# Simula una batería de pruebas larga.\nfor i in $(seq 1 299); do echo \"test_caso_$i ... ok (0.0${i}s) comprobando el módulo de inventario y sus existencias\"; done\n")
	b.WriteString("echo 'FAILED test_descuento_volumen: esperado 88.0, obtenido 100.0 (no se aplica el descuento por volumen)'\nexit 1\n")
	os.WriteFile(filepath.Join(carpeta, "pruebas.sh"), []byte(b.String()), 0o755)
	b.Reset()
	b.WriteString("\"\"\"Inventario del almacén (módulo de prueba).\"\"\"\n\n")
	for i := 0; i < 290; i++ {
		fmt.Fprintf(&b, "def existencias_%03d(almacen):\n    \"\"\"Existencias del artículo %d en el almacén.\"\"\"\n    return almacen.get('art_%03d', 0)\n\n", i, i, i)
		if i == 140 {
			b.WriteString("def precio_con_iva(precio):\n    \"\"\"Precio con el IVA general (21 %).\"\"\"\n    return round(precio * 1.21, 2)\n\n")
		}
		if i == 230 {
			b.WriteString("def descuento_volumen(unidades):\n    \"\"\"Descuento por volumen.\"\"\"\n    if unidades >= 500:\n        return 0.12\n    if unidades >= 100:\n        return 0.05\n    return 0.0\n\n")
		}
	}
	os.WriteFile(filepath.Join(carpeta, "inventario.py"), []byte(b.String()), 0o644)
	b.Reset()
	for i := 0; i < 300; i++ {
		if i == 161 {
			b.WriteString("2026-10-23 10:03:31 ERROR conexión rechazada en 127.0.0.1:5433 (base de datos de existencias)\n")
		}
		fmt.Fprintf(&b, "2026-10-23 10:%02d:%02d INFO sincronizado el lote %d del inventario sin incidencias\n", i/60, i%60, i)
	}
	b.WriteString("Resumen: 300 lotes, 1 error\n")
	os.WriteFile(filepath.Join(carpeta, "registro.txt"), []byte(b.String()), 0o644)
	os.WriteFile(filepath.Join(carpeta, ".planesgo.json"), []byte(`{"odoo_project_id": 999, "odoo_project_name": "PRUEBA PGOCODE"}`), 0o644)
	for _, c := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.email=p@p", "-c", "user.name=p", "commit", "-qm", "inicio"}} {
		if err := exec.Command("git", append([]string{"-C", carpeta}, c...)...).Run(); err != nil {
			t.Fatal(err)
		}
	}
}
