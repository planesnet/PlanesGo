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
  pgo salida <nombre> [n]          últimas n líneas de su salida (sin n, las que quepan de las 100 últimas;
                                   la salida completa, en el fichero que indica)
  pgo abrir <url>                  abre la URL en el navegador integrado de pgocode

Google Drive, Docs, Sheets y Slides (solo lectura, con la sesión de Google del navegador de pgocode):
  pgo drive buscar <texto> [--tipo doc|hoja|presentacion|pdf|carpeta|imagen] [--carpeta <id|url>] [--max N]
  pgo drive listar [<carpeta id|url>]       sin carpeta: «Mi unidad»
  pgo drive leer <id|url> [--hoja <nombre|n>] [--rango A1:D20] [--max N]
                                   Docs en Markdown, Sheets en CSV; recorta a 40000 caracteres (--max)

Gmail (solo lectura, con la misma sesión: no envía, borra ni archiva nada):
  pgo gmail buscar <consulta de Gmail> [--max N]   p. ej. from:ana newer_than:7d, subject:factura has:attachment
  pgo gmail listar [--max N]                       lo más reciente de la bandeja de entrada
  pgo gmail leer <id de hilo> [--max N]            el hilo (sin lo citado); recorta a 20000 caracteres
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
		// Google se lee cargando páginas en el navegador de pgocode: puede tardar más.
		espera := 60 * time.Second
		if strings.HasPrefix(orden, "drive-") || strings.HasPrefix(orden, "gmail-") {
			espera = 4 * time.Minute
		}
		res, err := (&http.Client{Timeout: espera}).Do(req)
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
						return 0
					}
					fmt.Println(finishedLine(x.Nombre, x.Estado, x.Codigo))
					// Lo último de su salida, ya: así no hace falta otra llamada (con todo el contexto) para verla.
					if raw, err := llama("GET", "salida", nil, url.Values{"nombre": {x.Nombre}, "n": {strconv.Itoa(finishedTail)}}); err == nil {
						var s salidaProceso
						json.Unmarshal(raw, &s)
						fmt.Print(trimmedOutput(s.Lineas, s.Total, s.Registro, outputTotalMax))
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
		n, presupuesto := "100", outputTotalMax
		if len(args) > 2 {
			n, presupuesto = args[2], 0 // las que pide (para filtrarlas, p. ej. con grep)
		}
		raw, err := llama("GET", "salida", nil, url.Values{"nombre": {args[1]}, "n": {n}})
		if err != nil {
			return falla(err)
		}
		var s salidaProceso
		json.Unmarshal(raw, &s)
		fmt.Print(trimmedOutput(s.Lineas, s.Total, s.Registro, presupuesto))
	case "abrir":
		if len(args) < 2 {
			return 2
		}
		if _, err := llama("POST", "abrir", map[string]any{"url": args[1]}, nil); err != nil {
			return falla(err)
		}
		fmt.Println("abierto en el navegador de pgocode")
	case "drive":
		orden, q, err := driveQuery(args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgo drive:", err)
			fmt.Fprint(os.Stderr, ayudaPgo)
			return 2
		}
		raw, err := llama("GET", "drive-"+orden, nil, q)
		if err != nil {
			return falla(err)
		}
		var r struct{ Texto string }
		json.Unmarshal(raw, &r)
		fmt.Print(r.Texto)
	case "gmail":
		orden, q, err := gmailQuery(args[1:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "pgo gmail:", err)
			fmt.Fprint(os.Stderr, ayudaPgo)
			return 2
		}
		raw, err := llama("GET", "gmail-"+orden, nil, q)
		if err != nil {
			return falla(err)
		}
		var r struct{ Texto string }
		json.Unmarshal(raw, &r)
		fmt.Print(r.Texto)
	default:
		fmt.Fprint(os.Stderr, ayudaPgo)
		return 2
	}
	return 0
}

// gmailQuery: la orden (buscar | listar | leer) y los parámetros de «pgo gmail …». La consulta va tal cual a Gmail
// (sus operadores: from:, to:, subject:, newer_than:, has:attachment, is:unread…).
func gmailQuery(args []string) (string, url.Values, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("falta la orden (buscar, listar o leer)")
	}
	orden, rest := args[0], args[1:]
	q := url.Values{}
	var pos []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, val, hasVal := strings.Cut(a, "=")
		switch {
		case name == "--max":
			if !hasVal {
				if i+1 >= len(rest) {
					return "", nil, fmt.Errorf("falta el valor de --max")
				}
				i++
				val = rest[i]
			}
			if n, err := strconv.Atoi(val); err != nil || n <= 0 {
				return "", nil, fmt.Errorf("--max tiene que ser un número")
			}
			q.Set("max", val)
		case strings.HasPrefix(a, "--"):
			return "", nil, fmt.Errorf("opción desconocida %s", a)
		default:
			pos = append(pos, a)
		}
	}
	switch orden {
	case "buscar":
		if len(pos) == 0 {
			return "", nil, fmt.Errorf("di qué buscar (con la sintaxis de Gmail: from:ana newer_than:7d…)")
		}
		q.Set("q", strings.Join(pos, " "))
	case "listar":
		if len(pos) > 0 {
			return "", nil, fmt.Errorf("listar no lleva consulta (para buscar: pgo gmail buscar <consulta>)")
		}
	case "leer":
		if len(pos) != 1 {
			return "", nil, fmt.Errorf("di qué leer: el ID de un hilo (el que da pgo gmail buscar)")
		}
		q.Set("ref", pos[0])
	default:
		return "", nil, fmt.Errorf("orden desconocida %q", orden)
	}
	return orden, q, nil
}

// driveQuery: la orden (buscar | listar | leer) y los parámetros de «pgo drive …».
func driveQuery(args []string) (string, url.Values, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("falta la orden (buscar, listar o leer)")
	}
	orden, rest := args[0], args[1:]
	opts := map[string]string{"--tipo": "tipo", "--carpeta": "carpeta", "--max": "max", "--hoja": "hoja", "--rango": "rango"}
	q := url.Values{}
	var pos []string
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, val, hasVal := strings.Cut(a, "=")
		if k, ok := opts[name]; ok {
			if !hasVal {
				if i+1 >= len(rest) {
					return "", nil, fmt.Errorf("falta el valor de %s", name)
				}
				i++
				val = rest[i]
			}
			q.Set(k, val)
			continue
		}
		if strings.HasPrefix(a, "--") {
			return "", nil, fmt.Errorf("opción desconocida %s", a)
		}
		pos = append(pos, a)
	}
	switch orden {
	case "buscar":
		if len(pos) == 0 && q.Get("tipo") == "" && q.Get("carpeta") == "" {
			return "", nil, fmt.Errorf("di qué buscar")
		}
		q.Set("q", strings.Join(pos, " "))
	case "listar":
		if len(pos) > 1 {
			return "", nil, fmt.Errorf("una sola carpeta")
		}
		if len(pos) == 1 {
			q.Set("carpeta", pos[0])
		}
	case "leer":
		if len(pos) != 1 {
			return "", nil, fmt.Errorf("di qué leer: un ID o una URL")
		}
		q.Set("ref", pos[0])
	default:
		return "", nil, fmt.Errorf("orden desconocida %q", orden)
	}
	return orden, q, nil
}

// salidaProceso: la respuesta de /api/pgo/salida.
type salidaProceso struct {
	Lineas   []string
	Total    int
	Registro string
}

const (
	// finishedTail: las líneas que enseña «pgo ejecutar» de un programa que ya ha acabado.
	finishedTail = 20
	// Lo que llega al agente de «pgo salida» como mucho: cada línea y en total (caracteres). Lo demás está en el
	// registro, que lee por partes si lo necesita.
	outputLineMax  = 1000
	outputTotalMax = 8000
)

// trimmedOutput: las últimas líneas de un programa recortadas (cada una a outputLineMax y, con presupuesto, entre
// todas a esos caracteres, quedándose con las últimas) y, si falta algo, delante, cuántas líneas faltan y dónde
// está la salida completa.
func trimmedOutput(lineas []string, total int, registro string, presupuesto int) string {
	var sel []string
	n := 0
	for i := len(lineas) - 1; i >= 0; i-- {
		l := lineas[i]
		if r := []rune(l); len(r) > outputLineMax {
			l = string(r[:outputLineMax]) + fmt.Sprintf("… [%d caracteres más]", len(r)-outputLineMax)
		}
		if presupuesto > 0 && n+len(l)+1 > presupuesto && len(sel) > 0 {
			break
		}
		sel = append(sel, l)
		n += len(l) + 1
	}
	for i, j := 0, len(sel)-1; i < j; i, j = i+1, j-1 {
		sel[i], sel[j] = sel[j], sel[i]
	}
	var b strings.Builder
	if faltan := max(total, len(lineas)) - len(sel); faltan > 0 {
		if registro != "" {
			fmt.Fprintf(&b, "[… %d líneas antes. Salida completa: %s (léela por partes con offset/limit o búscala con grep)]\n", faltan, registro)
		} else {
			fmt.Fprintf(&b, "[… %d líneas antes]\n", faltan)
		}
	}
	for _, l := range sel {
		b.WriteString(l + "\n")
	}
	return b.String()
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
