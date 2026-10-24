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

Google (solo lectura y solo por enlace: descarga el fichero, como «Archivo → Descargar», con la sesión de Google
del navegador de pgocode; nunca lee páginas, no busca ni lista):
  pgo drive leer <enlace o id> [--hoja <nombre|n>] [--rango A1:D20] [--max N]
                                   Docs en Markdown, Sheets en CSV, Slides y ficheros de texto/Word/Excel en texto;
                                   PDF y demás se guardan en .pgocode/salidas (te da la ruta); recorta a 40000 (--max)
  pgo gmail leer <enlace del correo> [--max N]
                                   el mensaje original (.eml): texto sin lo citado y adjuntos (nombre, tipo, tamaño)

SQL Server de la sesión (la conexión la define la persona en las propiedades de la sesión; tú no ves la contraseña):
  pgo sql "<orden T-SQL>" [--conexion NOMBRE] [--filas N] [--escribir]
  pgo sql -f <fichero.sql> [--conexion NOMBRE] [--filas N] [--escribir]
                                   resultados en texto separado por tabuladores (200 filas por resultado; --filas
                                   hasta 5000). Solo lectura: lo que cambie la orden se deshace al acabar. --escribir
                                   solo vale si la persona ha permitido los cambios en esa conexión. Una sesión puede
                                   tener varias conexiones con nombre; sin --conexion, la principal

PowerShell remoto (WinRM) de la sesión (la conexión con el equipo Windows la define la persona en las propiedades
de la sesión; tú no ves la contraseña):
  pgo ps "<orden PowerShell>" [--escribir]
  pgo ps -f <fichero.ps1> [--escribir]
                                   salida, errores y código de salida (hasta 1 MiB). Solo lectura: solo órdenes de
                                   consulta (Get-*, Test-*, Select-*, Where-*, Measure-*, Format-*…), sin
                                   asignar propiedades ni escribir ficheros; nada se puede deshacer, así que lo que
                                   cambia algo necesita --escribir y que la persona haya permitido los cambios
  pgo ps baja <ruta remota> <fichero local>
  pgo ps sube <fichero local> <ruta remota> --escribir
                                   copia un fichero (hasta 4 MiB); subir solo con los cambios permitidos
` + desktopHelp

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
		// Google se lee descargando en el navegador de pgocode: puede tardar más.
		espera := 60 * time.Second
		if strings.HasPrefix(orden, "drive-") || strings.HasPrefix(orden, "gmail-") {
			espera = 4 * time.Minute
		}
		if strings.HasPrefix(orden, "ps") { // a remote script: up to 5 minutes in the backend
			espera = 6 * time.Minute
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
	case "sql":
		body, err := sqlArgs(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "pgo sql: %v\n", err)
			return 2
		}
		raw, err := llama("POST", "sql", body, nil)
		if err != nil {
			return falla(err)
		}
		var r sqlResult
		json.Unmarshal(raw, &r)
		fmt.Print(sqlText(r))
	case "ps":
		req, err := psArgs(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "pgo ps: %v\n", err)
			return 2
		}
		raw, err := llama("POST", req.endpoint, req.body, nil)
		if err != nil {
			return falla(err)
		}
		return psOutput(req, raw)
	case "escritorio":
		dir, _ := os.Getwd() // las capturas van a su .pgocode/salidas
		body, err := desktopArgs(args[1:], dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pgo escritorio: %v\n", err)
			fmt.Fprint(os.Stderr, desktopHelp)
			return 2
		}
		raw, err := llama("POST", "escritorio", body, nil)
		if err != nil {
			return falla(err)
		}
		var r struct{ Text string }
		json.Unmarshal(raw, &r)
		fmt.Println(strings.TrimRight(r.Text, "\n"))
	case "drive", "gmail":
		q, err := googleQuery(args[0], args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "pgo %s: %v\n", args[0], err)
			fmt.Fprint(os.Stderr, ayudaPgo)
			return 2
		}
		if args[0] == "drive" {
			dir, _ := os.Getwd() // lo que no es texto se guarda en su .pgocode/salidas
			q.Set("carpeta", dir)
		}
		raw, err := llama("GET", args[0]+"-leer", nil, q)
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

// googleQuery: los parámetros de «pgo drive leer <enlace o id> [--hoja X] [--rango A1:D20] [--max N]» y de
// «pgo gmail leer <enlace> [--max N]». Solo hay «leer»: buscar y listar leerían páginas de Google.
func googleQuery(service string, args []string) (url.Values, error) {
	if len(args) == 0 || args[0] != "leer" {
		return nil, fmt.Errorf("solo hay «pgo %s leer <enlace>» (no se busca ni se lista)", service)
	}
	opts := map[string]string{"--max": "max"}
	if service == "drive" {
		opts["--hoja"], opts["--rango"] = "hoja", "rango"
	}
	q := url.Values{}
	var pos []string
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		name, val, hasVal := strings.Cut(a, "=")
		if k, ok := opts[name]; ok {
			if !hasVal {
				if i+1 >= len(rest) {
					return nil, fmt.Errorf("falta el valor de %s", name)
				}
				i++
				val = rest[i]
			}
			if k == "max" {
				if n, err := strconv.Atoi(val); err != nil || n <= 0 {
					return nil, fmt.Errorf("--max tiene que ser un número")
				}
			}
			q.Set(k, val)
			continue
		}
		if strings.HasPrefix(a, "--") {
			return nil, fmt.Errorf("opción desconocida %s", a)
		}
		pos = append(pos, a)
	}
	if len(pos) != 1 {
		if service == "gmail" {
			return nil, fmt.Errorf("di qué leer: el enlace del correo (ábrelo en Gmail y copia la dirección)")
		}
		return nil, fmt.Errorf("di qué leer: un enlace o un ID de Google Drive/Docs/Sheets/Slides")
	}
	q.Set("ref", pos[0])
	return q, nil
}

// sqlArgs: el cuerpo de «pgo sql "<orden>" | -f <fichero> [--conexion NOMBRE] [--filas N] [--escribir]».
func sqlArgs(args []string) (map[string]any, error) {
	body := map[string]any{}
	var batch string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--escribir":
			body["escribir"] = true
		case a == "--conexion":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("falta el nombre de la conexión")
			}
			i++
			body["conexion"] = args[i]
		case a == "--filas" || a == "-f":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("falta el valor de %s", a)
			}
			i++
			if a == "-f" {
				b, err := os.ReadFile(args[i])
				if err != nil {
					return nil, err
				}
				batch = string(b)
				continue
			}
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("--filas tiene que ser un número")
			}
			body["n"] = n
		case strings.HasPrefix(a, "--"):
			return nil, fmt.Errorf("opción desconocida %s", a)
		default:
			if batch != "" {
				return nil, fmt.Errorf("una sola orden (entre comillas) o -f <fichero>")
			}
			batch = a
		}
	}
	if strings.TrimSpace(batch) == "" {
		return nil, fmt.Errorf("falta la orden: pgo sql \"SELECT …\" o pgo sql -f <fichero.sql>")
	}
	body["sql"] = batch
	return body, nil
}

// sqlResult: la respuesta de /api/pgo/sql.
type sqlResult struct {
	Sets []struct {
		Columns   []string
		Rows      [][]any
		Truncated bool
	}
	Affected   int64
	RolledBack bool `json:"rolled_back"`
	Ms         int64
}

// sqlText: los resultados en texto (columnas separadas por tabuladores; cada valor en una línea), lo justo para el
// agente.
func sqlText(r sqlResult) string {
	var b strings.Builder
	cell := func(v any) string {
		if v == nil {
			return "NULL"
		}
		t := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(fmt.Sprint(v))
		if rs := []rune(t); len(rs) > outputLineMax {
			t = string(rs[:outputLineMax]) + "…"
		}
		return t
	}
	for i, s := range r.Sets {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.Join(s.Columns, "\t") + "\n")
		for _, row := range s.Rows {
			c := make([]string, len(row))
			for j, v := range row {
				c[j] = cell(v)
			}
			b.WriteString(strings.Join(c, "\t") + "\n")
		}
		fmt.Fprintf(&b, "(%d filas", len(s.Rows))
		if s.Truncated {
			b.WriteString("; hay más: usa --filas o filtra la consulta")
		}
		b.WriteString(")\n")
	}
	// With result sets, @@ROWCOUNT is the rows read by the last SELECT: only said for batches without results.
	switch {
	case len(r.Sets) > 0:
	case r.Affected > 0 && r.RolledBack:
		fmt.Fprintf(&b, "%d filas afectadas, deshecho (solo lectura)\n", r.Affected)
	case r.Affected > 0:
		fmt.Fprintf(&b, "%d filas afectadas\n", r.Affected)
	default:
		b.WriteString("hecho, sin resultados\n")
	}
	return b.String()
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
