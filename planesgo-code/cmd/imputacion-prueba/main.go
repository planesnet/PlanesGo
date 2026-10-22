// imputacion-prueba: ejecuta los escenarios de imputación (internal/imputacion/escenarios.go) contra el Odoo de
// verdad, en el proyecto PRUEBAS PLANESGO (lo crea si no existe), y comprueba que las líneas de parte que quedan
// son las esperadas. Son los mismos escenarios que «go test ./internal/imputacion» prueba con un Odoo en memoria.
//
// Usa la sesión de Odoo de pgocode (la de su fichero local de imputación) o la de -sesion. El reloj es simulado:
// cada escenario vive sus horas en segundos, el día de hoy (y el anterior, los de medianoche). Al acabar borra las
// líneas de prueba (no cuentan en el parte real); -conservar las deja para mirarlas en Odoo.
//
//	go run ./cmd/imputacion-prueba                  # todos
//	go run ./cmd/imputacion-prueba -escenarios E13,E19 -conservar
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"planesgo-code/internal/imputacion"
	"planesgo-code/internal/odoo"
)

const nombreProyecto = "PRUEBAS PLANESGO"

var (
	urlF       = flag.String("odoo", "https://planesnet.autopyme.com", "URL de Odoo")
	dbF        = flag.String("db", "ap113", "base de datos de Odoo")
	sesionF    = flag.String("sesion", "", "sesión de Odoo (vacío: la del fichero local de imputación de pgocode)")
	escF       = flag.String("escenarios", "", "escenarios (E01,E13…); vacío: todos")
	conservarF = flag.Bool("conservar", false, "no borra las líneas de prueba al acabar")
)

// sesionPgocode: la sesión de Odoo y el usuario con los que imputa pgocode (su fichero local).
func sesionPgocode() (string, int, error) {
	casa, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(casa, ".local", "share", "pgocode", "imputacion-dia.json"))
	if err != nil {
		return "", 0, err
	}
	var e struct {
		Dias []struct {
			UID        int    `json:"uid"`
			OdooSesion string `json:"odoo_sesion"`
			EscritoEn  string `json:"escrito_en"`
		}
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return "", 0, err
	}
	ses, uid, ultima := "", 0, ""
	for _, d := range e.Dias {
		if d.OdooSesion != "" && d.EscritoEn >= ultima {
			ses, uid, ultima = d.OdooSesion, d.UID, d.EscritoEn
		}
	}
	if ses == "" {
		return "", 0, fmt.Errorf("no hay sesión de Odoo en el fichero de imputación de pgocode")
	}
	return ses, uid, nil
}

func main() {
	log.SetFlags(log.Ltime)
	log.SetPrefix("imputación ")
	flag.Parse()
	ctx := context.Background()
	c := odoo.Nuevo(*urlF, *dbF)
	ses, uid, err := sesionPgocode()
	if *sesionF != "" {
		ses = *sesionF
	}
	if err != nil && ses == "" {
		log.Fatal(err)
	}
	// El usuario de la sesión (y que sigue valiendo).
	var info struct {
		UID  int    `json:"uid"`
		Name string `json:"name"`
	}
	var yo []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if uid > 0 {
		if err := c.CallKW(ctx, ses, "res.users", "read", []any{[]int{uid}, []string{"name"}}, nil, &yo); err != nil || len(yo) == 0 {
			log.Fatalf("la sesión de Odoo no vale (entra en pgocode y vuelve a probar): %v", err)
		}
		info.UID, info.Name = uid, yo[0].Name
	}
	// El proyecto de pruebas.
	var proy []struct {
		ID int `json:"id"`
	}
	if err := c.CallKW(ctx, ses, "project.project", "search_read", []any{[]any{[]any{"name", "=", nombreProyecto}}},
		map[string]any{"fields": []string{"id"}, "limit": 1}, &proy); err != nil {
		log.Fatalf("buscar el proyecto: %v", err)
	}
	pid := 0
	if len(proy) > 0 {
		pid = proy[0].ID
	} else {
		if err := c.CallKW(ctx, ses, "project.project", "create", []any{map[string]any{"name": nombreProyecto}}, nil, &pid); err != nil {
			log.Fatalf("crear el proyecto %s: %v", nombreProyecto, err)
		}
		log.Printf("creado el proyecto %s (%d)", nombreProyecto, pid)
	}
	log.Printf("Odoo %s · usuario %s (%d) · proyecto %s (%d)", *urlF, info.Name, info.UID, nombreProyecto, pid)

	dir, _ := os.MkdirTemp("", "imputacion-prueba-")
	defer os.RemoveAll(dir)
	b := imputacion.Banco{Odoo: c, Sesion: ses, UID: uid, Proyecto: pid, NombreProy: nombreProyecto, Dia: time.Now(), Dir: dir}
	var elegidos []string
	if *escF != "" {
		elegidos = strings.Split(*escF, ",")
	}
	var informe strings.Builder
	fmt.Fprintf(&informe, "# Prueba de imputación en Odoo · %s\n\nOdoo %s, usuario %s (%d), proyecto %s (%d).\n\n| Escenario | Resultado | Líneas en Odoo |\n|---|---|---|\n",
		time.Now().Format("2006-01-02 15:04"), *urlF, info.Name, info.UID, nombreProyecto, pid)
	bien, mal := 0, 0
	var borrar []int
	for _, e := range imputacion.Escenarios() {
		if len(elegidos) > 0 && !slices.Contains(elegidos, e.Clave) {
			continue
		}
		if err := b.Limpia(e); err != nil {
			log.Printf("%s: no se pudieron borrar las líneas de una ejecución anterior: %v", e.Clave, err)
		}
		t0 := time.Now()
		lineas, fallos := b.Ejecuta(e)
		var ls []string
		for _, l := range lineas {
			ls = append(ls, fmt.Sprintf("%s %s%s %.2f h (%d)", l.Fecha[5:], l.Clase, map[bool]string{true: " " + l.Agente}[l.Agente != ""], l.Horas, l.ID))
			borrar = append(borrar, l.ID)
		}
		res := "✓"
		if len(fallos) > 0 {
			res = "✗ " + strings.Join(fallos, "; ")
			mal++
		} else {
			bien++
		}
		log.Printf("%s %s: %s (%.1f s) — %s", e.Clave, e.Nombre, res, time.Since(t0).Seconds(), strings.Join(ls, " · "))
		fmt.Fprintf(&informe, "| %s · %s | %s | %s |\n", e.Clave, e.Nombre, res, strings.Join(ls, "<br>"))
	}
	fmt.Fprintf(&informe, "\n**%d bien, %d mal.**\n", bien, mal)
	if !*conservarF && len(borrar) > 0 {
		// Una a una: alguna puede no existir ya (la borró un escenario, como el usuario en E20).
		slices.Sort(borrar)
		borrar = slices.Compact(borrar)
		n := 0
		for _, id := range borrar {
			var nada any
			if c.CallKW(ctx, ses, "account.analytic.line", "unlink", []any{[]int{id}}, nil, &nada) == nil {
				n++
			}
		}
		var quedan []any
		c.CallKW(ctx, ses, "account.analytic.line", "search_read", []any{[]any{[]any{"project_id", "=", pid}, []any{"name", "=like", "pgocode (%"}}},
			map[string]any{"fields": []string{"id"}}, &quedan)
		log.Printf("borradas %d líneas de prueba (no cuentan en el parte); quedan %d en el proyecto", n, len(quedan))
		informe.WriteString(fmt.Sprintf("\nSe borraron las %d líneas de prueba al acabar (quedan %d de pgocode en el proyecto).\n", n, len(quedan)))
	}
	casa, _ := os.UserHomeDir()
	sal := filepath.Join(casa, ".local", "share", "pgocode", "imputacion-pruebas")
	os.MkdirAll(sal, 0o700)
	f := filepath.Join(sal, time.Now().Format("2006-01-02-150405")+".md")
	os.WriteFile(f, []byte(informe.String()), 0o600)
	log.Printf("%d bien, %d mal · informe en %s", bien, mal, f)
	if mal > 0 {
		os.Exit(1)
	}
}
