package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"planesgo-code/internal/llm"
)

func banco(t *testing.T) Banco {
	t.Helper()
	var b Banco
	bs, err := os.ReadFile("../../consumo/pruebas.json")
	if err != nil || json.Unmarshal(bs, &b) != nil {
		t.Fatalf("pruebas.json: %v", err)
	}
	return b
}

// Las expresiones de las pruebas aceptan respuestas buenas y rechazan las malas.
func TestTextosEsperados(t *testing.T) {
	b := banco(t)
	casos := map[int]struct{ bien, mal []string }{
		1: {[]string{"OK", "OK.", "**OK**"}, []string{"Vale, OK", "No"}},
		2: {[]string{"8473", "El puerto es 8473."}, []string{"84730", "8069"}},
		3: {[]string{"facturacion/impuestos/calculo.go:24", "`facturacion/impuestos/calculo.go:20`", "facturacion/impuestos/calculo.go, línea 24"},
			[]string{"facturacion/impuestos/calculo.go:16", "facturacion/lineas.go:24"}},
	}
	for _, p := range b.Pruebas {
		c, ok := casos[p.ID]
		if !ok {
			continue
		}
		for _, x := range c.bien {
			if f := comprueba(p.Turnos[0], x, t.TempDir()); f != "" {
				t.Errorf("prueba %d, %q: %s", p.ID, x, f)
			}
		}
		for _, x := range c.mal {
			if comprueba(p.Turnos[0], x, t.TempDir()) == "" {
				t.Errorf("prueba %d acepta %q", p.ID, x)
			}
		}
	}
}

// benchProject copies a bench project (consumo/<name>) into a temporary git repository, as each run does, and
// returns its folder with helpers to change it like an agent would: edit replaces a text that has to be there,
// appendTo adds to the end of a file.
func benchProject(t *testing.T, name string) (dir string, edit func(file, old, new string), appendTo func(file, text string)) {
	t.Helper()
	dir = filepath.Join(t.TempDir(), "p")
	if err := copia(filepath.Join("../../consumo", name), dir); err != nil {
		t.Fatal(err)
	}
	edit = func(file, old, new string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil || !strings.Contains(string(b), old) {
			t.Fatalf("%s: no está %q", file, old)
		}
		os.WriteFile(filepath.Join(dir, file), []byte(strings.Replace(string(b), old, new, 1)), 0o644)
	}
	appendTo = func(file, text string) {
		t.Helper()
		f, err := os.OpenFile(filepath.Join(dir, file), os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		f.WriteString(text)
	}
	return dir, edit, appendTo
}

// El proyecto de prueba: con el fallo, las pruebas fallan; con el arreglo, pasan, y tocar las pruebas no vale.
func TestProyectoDePrueba(t *testing.T) {
	b := banco(t)
	arreglo := b.Pruebas[3].Turnos[0]
	d, edit, _ := benchProject(t, "proyecto")
	if comprueba(arreglo, "", d) == "" {
		t.Fatal("sin arreglar, las pruebas pasan")
	}
	edit("facturacion/lineas.go", "Redondea(l.Bruto()-l.Descuento, 2)", "Redondea(l.Bruto()*(1-l.Descuento/100), 2)")
	if r := comprueba(arreglo, "", d); r != "" {
		t.Fatalf("arreglado: %s", r)
	}
	os.WriteFile(filepath.Join(d, "tests", "lineas_test.go"), []byte("package tests\n"), 0o644)
	if comprueba(arreglo, "", d) == "" {
		t.Error("tocar las pruebas tendría que fallar")
	}
}

// A check is copied into the work copy only while it runs: the agent never sees it and it leaves no changes.
func TestRunCheck(t *testing.T) {
	d, _, _ := benchProject(t, "proyecto")
	for _, bad := range []string{"../pruebas.json", "p6_round_mode.go", "no_existe_test.go"} {
		if runCheck(bad, d) == "" {
			t.Errorf("%q: tendría que fallar", bad)
		}
	}
	if runCheck("p6_round_mode_test.go", d) == "" {
		t.Error("sin el modo de redondeo, la comprobación pasa")
	}
	if out, _ := exec.Command("git", "-C", d, "status", "--porcelain").Output(); len(out) > 0 {
		t.Errorf("la comprobación deja cambios: %s", out)
	}
}

// Antigravity sin pgocode da el consumo acumulado de su conversación: se resta el del turno anterior.
func TestAcumuladoAntigravity(t *testing.T) {
	c := &recoge{agente: "antigravity", previo: &llm.Uso{Entrada: 1000, Salida: 10, CacheLeida: 500}}
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 1800, Salida: 25, CacheLeida: 900}})
	r := c.fin()
	if r.Entrada != 800 || r.Salida != 15 || r.CacheLeida != 400 {
		t.Errorf("%+v", r.Medida)
	}
}

// El contexto de un turno es el de su última llamada al modelo (no la suma).
func TestRecogeContexto(t *testing.T) {
	c := &recoge{agente: "claude-code"}
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 100, Contexto: 30}})
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 200, Contexto: 70}})
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 5}})
	if r := c.fin(); r.Entrada != 305 || r.Contexto != 70 {
		t.Errorf("%+v", r.Medida)
	}
	var m Medida
	m.suma(Medida{Entrada: 10, Contexto: 30})
	m.suma(Medida{Entrada: 20, Contexto: 50})
	if m.Entrada != 30 || m.Contexto != 50 {
		t.Errorf("suma: %+v", m)
	}
}

func TestEstrategias(t *testing.T) {
	if _, ok := strategies["ninguna"]; !ok {
		t.Fatal("falta la estrategia «ninguna»")
	}
	c := &conversation{Session: "s1", Message: "hola"}
	if err := strategies["ninguna"](0).before(context.Background(), c); err != nil || c.Session != "s1" || c.Message != "hola" {
		t.Errorf("«ninguna» no ha de cambiar nada: %+v %v", c, err)
	}
}

// El informe de una prueba de varios turnos lleva la evolución turno a turno, con cada modo.
func TestEvolucionPorTurno(t *testing.T) {
	p := Prueba{ID: 9, Nombre: "X", Turnos: []Turno{{Mensaje: "uno"}, {Mensaje: "dos"}}}
	tm := func(ctx, ent int) turnMeasure {
		return turnMeasure{Medida: Medida{Contexto: ctx, Entrada: ent}, Acierto: true}
	}
	todas := []Ejecucion{
		{Modo: "directo", Agente: "claude-code", Prueba: 9, Acierto: true, Turnos: []turnMeasure{tm(20000, 40000), tm(30000, 90000)}},
		{Modo: "pgocode", Agente: "claude-code", Prueba: 9, Acierto: false, Turnos: []turnMeasure{tm(12000, 24000)}},
	}
	inf := informe(Banco{Agentes: map[string]ConfAgente{"claude-code": {Modelo: "opus"}}}, []string{"claude-code"},
		[]Prueba{p}, []string{"directo", "pgocode"}, todas, t.TempDir(), 1, "ninguna")
	for _, x := range []string{"turno a turno", "Contexto sin pgocode", "Contexto pgocode",
		"| 1 | uno | 20.0k | 40.0k | 0.000 $ | 1/1 | 12.0k | 24.0k |", "| 2 | dos | 30.0k | 90.0k | 0.000 $ | 1/1 | — | — | — | — |", "Estrategia: `ninguna`"} {
		if !strings.Contains(inf, x) {
			t.Errorf("falta %q en:\n%s", x, inf)
		}
	}
}

// Prueba 6 («Ida y vuelta»): con una solución de referencia, cada turno cumple su comprobación y, antes de
// hacerlo, no (salvo las preguntas, que se comprueban con una respuesta mala).
func TestIdaYVuelta(t *testing.T) {
	b := banco(t)
	i := slices.IndexFunc(b.Pruebas, func(p Prueba) bool { return p.ID == 6 })
	if i < 0 {
		t.Fatal("falta la prueba 6")
	}
	p := b.Pruebas[i]
	if len(p.Turnos) != 10 {
		t.Fatalf("%d turnos", len(p.Turnos))
	}
	d, edit, appendTo := benchProject(t, "proyecto")
	rename := func() {
		filepath.WalkDir(d, func(f string, e os.DirEntry, _ error) error {
			if strings.HasSuffix(f, ".go") {
				bs, _ := os.ReadFile(f)
				os.WriteFile(f, []byte(strings.ReplaceAll(string(bs), "Subtotal(", "BaseImponible(")), 0o644)
			}
			return nil
		})
	}
	lineTest := func(name, body string) {
		appendTo("tests/lineas_test.go", "\nfunc "+name+"(t *testing.T) {\n\t"+body+"\n}\n")
	}
	steps := []struct {
		do        func()
		good, bad string
	}{
		{do: func() {
			edit("facturacion/lineas.go", "Redondea(l.Bruto()-l.Descuento, 2)", "Redondea(l.Bruto()*(1-l.Descuento/100), 2)")
		}},
		{do: func() {
			lineTest("TestDescuentoTotal", "if got := (facturacion.Linea{Producto: tornillo, Cantidad: 4, Descuento: 100}).Subtotal(); got != 0 {\n\t\tt.Error(got)\n\t}")
		}},
		{do: func() {
			edit("facturacion/redondeo.go", "decimales int) float64 {", "decimales int, modo ...string) float64 {")
			edit("facturacion/redondeo.go", "\tif new(big.Int)", "\tif (len(modo) == 0 || modo[0] != \"abajo\") && new(big.Int)")
		}},
		{do: func() {
			edit("facturacion/impuestos/calculo.go", "Redondea(base*porcentaje/100, 2)", "Redondea(base*porcentaje/100, 2, \"abajo\")")
		}},
		{do: rename},
		{do: func() {
			edit("facturacion/lineas.go", "\tDescuento float64 // porcentaje (0-100)\n", "\tDescuento float64 // porcentaje (0-100)\n\tDescuentoFijo float64 // euros\n")
			edit("facturacion/lineas.go", "Redondea(l.Bruto()*(1-l.Descuento/100), 2)", "Redondea(l.Bruto()*(1-l.Descuento/100)-l.DescuentoFijo, 2)")
		}},
		{good: "Arreglé el descuento (porcentaje), renombré Subtotal a `BaseImponible` y añadí `DescuentoFijo`.", bad: "Arreglé el descuento."},
		{do: func() {
			edit("facturacion/lineas.go", "Redondea(l.Bruto()*(1-l.Descuento/100)-l.DescuentoFijo, 2)", "Redondea(max(0, l.Bruto()*(1-l.Descuento/100)-l.DescuentoFijo), 2)")
		}},
		{do: func() {
			lineTest("TestDescuentoFijoMinimo", "if got := (facturacion.Linea{Producto: tornillo, Cantidad: 1, DescuentoFijo: 50}).BaseImponible(); got != 0 {\n\t\tt.Error(got)\n\t}")
		}},
		{good: "Antes, mitad hacia arriba; ahora, hacia abajo (trunca).", bad: "Igual que antes."},
	}
	for n, s := range steps {
		turn := p.Turnos[n]
		if s.do != nil {
			if comprueba(turn, s.good, d) == "" {
				t.Errorf("turno %d: se cumple sin hacerlo", n+1)
			}
			s.do()
		} else if comprueba(turn, s.bad, d) == "" {
			t.Errorf("turno %d acepta %q", n+1, s.bad)
		}
		if f := comprueba(turn, s.good, d); f != "" {
			t.Fatalf("turno %d: %s", n+1, f)
		}
	}
}

// Antigravity sin pgocode da el acumulado de la conversación: se resta el anterior; si baja (otra sesión), se toma
// entero. Claude Code no: el lector de internal/llm ya da lo del turno.
func TestAcumulado(t *testing.T) {
	if cumulativeUsage("claude-code", "directo") || cumulativeUsage("claude-code", "pgocode") || cumulativeUsage("antigravity", "pgocode") ||
		!cumulativeUsage("antigravity", "directo") {
		t.Fatal("cumulativeUsage")
	}
	previo := &llm.Uso{Entrada: 100000, Salida: 1000, CacheLeida: 85000, CacheEscrita: 14000, Coste: 0.15}
	c := &recoge{agente: "antigravity", previo: previo}
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 155000, Salida: 1500, CacheLeida: 136000, CacheEscrita: 19000, Coste: 0.20, Llamadas: 2, Contexto: 28000}})
	r := c.fin()
	if r.Entrada != 55000 || r.Salida != 500 || r.CacheLeida != 51000 || r.CacheEscrita != 5000 || r.Llamadas != 2 || r.Contexto != 28000 ||
		r.Coste < 0.049 || r.Coste > 0.051 || r.bruto.Entrada != 155000 {
		t.Errorf("%+v", r.Medida)
	}
	c = &recoge{agente: "antigravity", previo: previo}
	c.evento(llm.Evento{Uso: &llm.Uso{Entrada: 30000, Coste: 0.05}})
	if r := c.fin(); r.Entrada != 30000 || r.Coste != 0.05 {
		t.Errorf("otra sesión: %+v", r.Medida)
	}
}

// «-estrategia nombre[:N]»: N en miles de tokens; por defecto, 20k.
func TestParseStrategy(t *testing.T) {
	for _, x := range []struct {
		in   string
		name string
		n    int
		ok   bool
	}{{"ninguna", "ninguna", 0, true}, {"compactar", "compactar", 0, true}, {"compactar:30", "compactar", 30, true},
		{"relevo:25", "relevo", 25, true}, {"relevo:0", "", 0, false}, {"relevo:x", "", 0, false}, {"ninguna:5", "", 0, false}, {"otra", "", 0, false}} {
		name, n, err := parseStrategy(x.in)
		if (err == nil) != x.ok || name != x.name || n != x.n {
			t.Errorf("%q: %q %d %v", x.in, name, n, err)
		}
	}
	if s := strategies["compactar"](0).(compactStrategy); s.threshold != 20000 {
		t.Errorf("compactar por defecto: %d", s.threshold)
	}
	if s := strategies["relevo"](30).(relayStrategy); s.threshold != 30000 || !strings.HasSuffix(s.script, "relay.mts") {
		t.Errorf("relevo:30: %+v", s)
	}
	if !pgocodeOnly["compactar"] || !pgocodeOnly["relevo"] || pgocodeOnly["ninguna"] {
		t.Error("pgocodeOnly")
	}
}

// «compactar»: cada turno lleva el umbral y el contexto de la última llamada del turno anterior.
func TestCompactStrategy(t *testing.T) {
	s := compactStrategy{threshold: 20000}
	c := &conversation{Session: "s1", Message: "hola"}
	if s.before(context.Background(), c); c.CompactAt != 20000 || c.LastContext != 0 || c.Session != "s1" || c.Message != "hola" {
		t.Errorf("primer turno: %+v", c)
	}
	c.History = []exchange{{Usage: Medida{Contexto: 18000}}, {Usage: Medida{Contexto: 23000}}}
	if s.before(context.Background(), c); c.LastContext != 23000 || c.Session != "s1" {
		t.Errorf("tercer turno: %+v", c)
	}
}

// Las partes de la respuesta se juntan como en la interfaz (texto seguido; herramienta por su id) y las tareas
// se fusionan por su id.
func TestUIMessages(t *testing.T) {
	c := &recoge{agente: "claude-code"}
	c.evento(llm.Evento{Texto: "Voy "})
	c.evento(llm.Evento{Texto: "a arreglarlo."})
	c.evento(llm.Evento{Herramienta: &llm.Herramienta{ID: "1", Nombre: "Edit", Resumen: "facturacion/lineas.go", Entrada: "{}", Estado: "curso"}})
	c.evento(llm.Evento{Herramienta: &llm.Herramienta{ID: "1", Estado: "ok", Salida: "hecho"}})
	c.evento(llm.Evento{Tarea: &llm.Tarea{ID: "l", Tipo: "lista", Elementos: []llm.Elemento{{Texto: "Arreglar", Estado: "curso"}}}})
	c.evento(llm.Evento{Tarea: &llm.Tarea{ID: "l", Elementos: []llm.Elemento{{Texto: "Arreglar", Estado: "hecho"}}}})
	c.evento(llm.Evento{Texto: "Hecho."})
	r := c.fin()
	if len(r.parts) != 3 || r.parts[0].Texto != "Voy a arreglarlo." || r.parts[1].H.Nombre != "Edit" || r.parts[1].H.Resumen != "facturacion/lineas.go" ||
		r.parts[1].H.Estado != "ok" || r.parts[2].Texto != "Hecho." || r.Herramientas != 1 {
		t.Fatalf("%+v", r.parts)
	}
	if len(r.tasks) != 1 || r.tasks[0].Tipo != "lista" || r.tasks[0].Elementos[0].Estado != "hecho" {
		t.Errorf("tareas: %+v", r.tasks)
	}
	ms := uiMessages([]exchange{{Message: "Arregla", Reply: r.texto, Parts: r.parts, Tasks: r.tasks}})
	b, _ := json.Marshal(ms)
	for _, x := range []string{`"rol":"user","texto":"Arregla"`, `"tipo":"herramienta","h":{"id":"1","nombre":"Edit","resumen":"facturacion/lineas.go"`, `"tareas":[{"id":"l","tipo":"lista"`} {
		if !strings.Contains(string(b), x) {
			t.Errorf("falta %s en %s", x, b)
		}
	}
}

// «relevo»: con node, la memoria y la decisión de web/src/workingMemory.ts. Por debajo del umbral, nada; por
// encima, sesión nueva con la memoria (las peticiones, los ficheros editados) delante del mensaje.
func TestRelayStrategy(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("sin node")
	}
	s := relayStrategy{threshold: 20000, script: "relay.mts"}
	h := []exchange{
		{Message: "Las pruebas fallan. Arregla el fallo del descuento sin tocar las pruebas.", Reply: "Arreglado: el descuento es un porcentaje.",
			Parts: []uiPart{{Tipo: "herramienta", H: &llm.Herramienta{ID: "1", Nombre: "Edit", Resumen: "facturacion/lineas.go", Estado: "ok"}},
				{Tipo: "herramienta", H: &llm.Herramienta{ID: "2", Nombre: "Bash", Resumen: "go test ./...", Estado: "ok", Salida: "OK"}},
				{Tipo: "texto", Texto: "Arreglado: el descuento es un porcentaje."}},
			Usage: Medida{Contexto: 17000}},
	}
	c := &conversation{Agent: "claude-code", Session: "s1", Message: "Renombra subtotal a base_imponible.", History: h}
	if err := s.before(context.Background(), c); err != nil || c.Session != "s1" || c.Message != "Renombra subtotal a base_imponible." || c.Note != "" {
		t.Fatalf("por debajo del umbral: %+v %v", c, err)
	}
	c.History[0].Usage.Contexto = 21000
	if err := s.before(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.Session != "" || !strings.HasPrefix(c.Note, "relevo") {
		t.Fatalf("por encima del umbral: %+v", c)
	}
	for _, x := range []string{"[Memoria de trabajo", "Arregla el fallo del descuento", "facturacion/lineas.go",
		"ok: `go test ./...`", "[Fin de la memoria de trabajo]\n\nRenombra subtotal a base_imponible."} {
		if !strings.Contains(c.Message, x) {
			t.Errorf("falta %q en:\n%s", x, c.Message)
		}
	}
	// Sin sesión del agente (el primer turno), nunca.
	c = &conversation{Agent: "claude-code", Message: "hola", History: h}
	if err := s.before(context.Background(), c); err != nil || c.Message != "hola" {
		t.Errorf("sin sesión: %+v %v", c, err)
	}
}

// El aviso de compactación de pgocode (compactionNotice) se reconoce en la respuesta.
func TestCompactionNote(t *testing.T) {
	for in, want := range map[string]string{
		"Hecho.\n\n_(Claude Code ha compactado su historial: de 21k a 6k tokens.)_\n\n": "ha compactado su historial: de 21k a 6k tokens",
		"_(Antigravity ha compactado su historial.)_":                                   "ha compactado su historial",
		"Nada que compactar.": "",
	} {
		if got := compactionNote.FindString(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

// La prueba 7 (Conversación larga): su proyecto falla al empezar; con un arreglo de referencia de cada turno
// cumple las comprobaciones una tras otra (y, antes de cada arreglo, no), y las de memoria aceptan las respuestas
// buenas y no las malas.
func TestConversacionLarga(t *testing.T) {
	b := banco(t)
	i := slices.IndexFunc(b.Pruebas, func(p Prueba) bool { return p.ID == 7 })
	if i < 0 {
		t.Fatal("falta la prueba 7")
	}
	p := b.Pruebas[i]
	d, edit, appendTo := benchProject(t, p.Proyecto)
	steps := []func(){
		func() { edit("almacen/precios.go", "if importe > t.Minimo {", "if importe >= t.Minimo {") },
		func() {
			edit("almacen/envios.go", "func CosteEnvio(peso float64, zona int) float64 {\n",
				"func CosteEnvio(peso float64, zona int, urgente bool) float64 {\n\tc := costeBase(peso, zona)\n"+
					"\tif urgente {\n\t\tc += RecargoUrgente[zona]\n\t}\n\treturn redondea2(c)\n}\n\n"+
					"func costeBase(peso float64, zona int) float64 {\n")
			edit("tests/envios_test.go", "almacen.CosteEnvio(0.5, 1)", "almacen.CosteEnvio(0.5, 1, false)")
		},
		func() {
			appendTo("almacen/informes.go", "\nfunc TotalPedido(p *Pedido, cp string) (float64, error) {\n"+
				"\tz, err := ZonaDe(cp)\n\tif err != nil {\n\t\treturn 0, err\n\t}\n"+
				"\treturn redondea2(AplicaDescuento(p.Importe()) + CosteEnvio(p.Peso(), z, p.Urgente)), nil\n}\n")
		},
		func() {
			edit("almacen/pedidos.go", "\tp.Historial = append(p.Historial, [2]string{p.Estado, nuevo})\n",
				"\tif nuevo == \"EN_PREPARACION\" && inv != nil {\n\t\tfor i, l := range p.Lineas {\n"+
					"\t\t\tif err := inv.ReservaStock(l.SKU, l.Cantidad); err != nil {\n"+
					"\t\t\t\tfor _, hecha := range p.Lineas[:i] {\n\t\t\t\t\tinv.Libera(hecha.SKU, hecha.Cantidad)\n\t\t\t\t}\n"+
					"\t\t\t\treturn err\n\t\t\t}\n\t\t}\n\t}\n"+
					"\tp.Historial = append(p.Historial, [2]string{p.Estado, nuevo})\n")
			appendTo("tests/pedidos_test.go", "\nfunc TestReservaAlPreparar(t *testing.T) {}\n")
		},
		func() {
			edit("almacen/devoluciones.go", "\treturn redondea2(importeProductos + costeEnvio), nil",
				"\tif motivo == \"desistimiento\" {\n\t\tcosteEnvio = 0\n\t}\n\treturn redondea2(importeProductos + costeEnvio), nil")
		},
	}
	for n, step := range steps {
		turn := p.Turnos[n+1]
		if comprueba(turn, "", d) == "" {
			t.Errorf("turno %d: se cumple sin hacerlo", n+2)
		}
		step()
		if r := comprueba(turn, "", d); r != "" {
			t.Fatalf("turno %d: %s", n+2, r)
		}
	}
	texts := map[int]struct{ good, bad []string }{
		0: {[]string{"Lo hace `Inventario.ReservaStock`: deja siempre 7 unidades (MargenSeguridad)."}, []string{"Lo hace ReservaStock, sin margen."}},
		6: {[]string{"Usaba `importe > t.Minimo` y ahora `importe >= t.Minimo`.", "Comparaba con > en vez de ≥."}, []string{"Comparaba con > y ahora también."}},
		7: {[]string{"MargenSeguridad = 7 unidades; el recargo urgente de la zona 3 es 5,25 €.", "5.25 € en la zona 3 y 7 unidades de margen (MargenSeguridad)."},
			[]string{"MargenSeguridad = 3; zona 3: 5,25 €.", "MargenSeguridad = 7; zona 3: 4 €."}},
	}
	for n, c := range texts {
		for _, x := range c.good {
			if f := comprueba(p.Turnos[n], x, d); f != "" {
				t.Errorf("turno %d, %q: %s", n+1, x, f)
			}
		}
		for _, x := range c.bad {
			if comprueba(p.Turnos[n], x, d) == "" {
				t.Errorf("turno %d acepta %q", n+1, x)
			}
		}
	}
}
