package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const ayudaPgo = `pgo: programas del proyecto en local, desde el agente (el proyecto va implícito).

  pgo ejecutar <nombre> <orden…>   arranca el programa en la carpeta actual (sigue al acabar el turno)
  pgo parar <nombre>               lo detiene
  pgo procesos                     lista los programas del proyecto
  pgo salida <nombre> [n]          últimas n líneas de su salida (100 por defecto)
  pgo abrir <url>                  abre la URL en el navegador integrado de pgocode
`

// pgo es la orden que usan los agentes dentro de pgocode (el ejecutable enlazado como «pgo»).
func pgo(args []string) int {
	api, token := os.Getenv("PGO_API"), os.Getenv("PGO_TOKEN")
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "ayuda" {
		fmt.Print(ayudaPgo)
		return 0
	}
	if api == "" || token == "" {
		fmt.Fprintln(os.Stderr, "pgo: solo funciona dentro de pgocode (faltan PGO_API y PGO_TOKEN)")
		return 2
	}
	proyecto, _ := strconv.Atoi(os.Getenv("PGO_PROYECTO"))
	llama := func(metodo, orden string, cuerpo map[string]any, q url.Values) (json.RawMessage, error) {
		var b io.Reader
		if cuerpo != nil {
			cuerpo["proyecto"] = proyecto
			j, _ := json.Marshal(cuerpo)
			b = bytes.NewReader(j)
		}
		u := api + "/api/pgo/" + orden
		if q != nil {
			q.Set("proyecto", strconv.Itoa(proyecto))
			u += "?" + q.Encode()
		}
		req, _ := http.NewRequest(metodo, u, b)
		req.Header.Set("X-PGO-Agente", token)
		req.Header.Set("X-PGO", "1")
		req.Header.Set("Content-Type", "application/json")
		res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != http.StatusOK {
			var e struct{ Error string }
			json.Unmarshal(raw, &e)
			return nil, fmt.Errorf("%s", strings.TrimSpace(e.Error+" "+res.Status))
		}
		return raw, nil
	}
	falla := func(err error) int { fmt.Fprintln(os.Stderr, "pgo:", err); return 1 }
	switch args[0] {
	case "ejecutar":
		if len(args) < 3 {
			fmt.Fprint(os.Stderr, ayudaPgo)
			return 2
		}
		dir, _ := os.Getwd()
		raw, err := llama("POST", "ejecutar", map[string]any{"nombre": args[1], "carpeta": dir, "orden": strings.Join(args[2:], " ")}, nil)
		if err != nil {
			return falla(err)
		}
		var p struct{ Nombre, Estado string }
		json.Unmarshal(raw, &p)
		fmt.Printf("«%s» arrancado. Mira su salida con: pgo salida %s\n", p.Nombre, p.Nombre)
		// Unos segundos para que diga en qué URL escucha (pgocode la abre sola en su navegador).
		for i := 0; i < 8; i++ {
			time.Sleep(time.Second)
			raw, err := llama("GET", "procesos", nil, url.Values{})
			if err != nil {
				break
			}
			var l []struct {
				Nombre, Estado, URL string
				Codigo              int
			}
			json.Unmarshal(raw, &l)
			for _, x := range l {
				if x.Nombre == args[1] && (x.URL != "" || x.Estado != "curso") {
					if x.URL != "" {
						fmt.Printf("Escucha en %s (abierto en el navegador de pgocode).\n", x.URL)
					} else {
						fmt.Println(finishedLine(x.Nombre, x.Estado, x.Codigo))
					}
					return 0
				}
			}
		}
		return 0
	case "parar":
		if len(args) < 2 {
			return 2
		}
		if _, err := llama("POST", "parar", map[string]any{"nombre": args[1]}, nil); err != nil {
			return falla(err)
		}
		fmt.Println("parado")
	case "procesos":
		raw, err := llama("GET", "procesos", nil, url.Values{})
		if err != nil {
			return falla(err)
		}
		var l []struct{ Nombre, Estado, URL, Orden string }
		json.Unmarshal(raw, &l)
		if len(l) == 0 {
			fmt.Println("no hay programas")
		}
		for _, x := range l {
			fmt.Printf("%-16s %-10s %-28s %s\n", x.Nombre, x.Estado, x.URL, x.Orden)
		}
	case "salida":
		if len(args) < 2 {
			return 2
		}
		n := "100"
		if len(args) > 2 {
			n = args[2]
		}
		raw, err := llama("GET", "salida", nil, url.Values{"nombre": {args[1]}, "n": {n}})
		if err != nil {
			return falla(err)
		}
		var s struct{ Lineas []string }
		json.Unmarshal(raw, &s)
		fmt.Println(strings.Join(s.Lineas, "\n"))
	case "abrir":
		if len(args) < 2 {
			return 2
		}
		if _, err := llama("POST", "abrir", map[string]any{"url": args[1]}, nil); err != nil {
			return falla(err)
		}
		fmt.Println("abierto en el navegador de pgocode")
	default:
		fmt.Fprint(os.Stderr, ayudaPgo)
		return 2
	}
	return 0
}

// finishedLine: lo que dice «pgo ejecutar» de un programa que ya ha acabado: cómo (con su código de salida) y
// cómo ver su salida.
func finishedLine(nombre, estado string, codigo int) string {
	var t string
	switch {
	case estado == "parado":
		t = "Se ha parado"
	case codigo < 0:
		t = "Ha fallado (sin código de salida: no arrancó o lo ha matado una señal)"
	case codigo != 0 || estado == "error":
		t = fmt.Sprintf("Ha fallado (código de salida %d)", codigo)
	default:
		t = "Ha terminado (código de salida 0)"
	}
	return t + ". Mira su salida con: pgo salida " + nombre
}

// aleatorio: un secreto de 32 bytes en hexadecimal.
func aleatorio() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
