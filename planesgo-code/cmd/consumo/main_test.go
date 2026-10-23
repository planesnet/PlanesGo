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
		3: {[]string{"facturacion/impuestos/calculo.py:21", "`facturacion/impuestos/calculo.py:17`", "facturacion/impuestos/calculo.py, línea 21"},
			[]string{"facturacion/impuestos/calculo.py:12", "facturacion/lineas.py:21"}},
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

// El proyecto de prueba: con el fallo, las pruebas fallan; con el arreglo, pasan, y tocar las pruebas no vale.
func TestProyectoDePrueba(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("sin python3")
	}
	b := banco(t)
	arreglo := b.Pruebas[3].Turnos[0]
	d := filepath.Join(t.TempDir(), "p")
	if err := copia("../../consumo/proyecto", d); err != nil {
		t.Fatal(err)
	}
	if comprueba(arreglo, "", d) == "" {
		t.Fatal("sin arreglar, las pruebas pasan")
	}
	f := filepath.Join(d, "facturacion", "lineas.py")
	bs, _ := os.ReadFile(f)
	os.WriteFile(f, []byte(strings.Replace(string(bs), "self.bruto() - self.descuento", "self.bruto() * (1 - self.descuento / 100)", 1)), 0o644)
	if r := comprueba(arreglo, "", d); r != "" {
		t.Fatalf("arreglado: %s", r)
	}
	os.WriteFile(filepath.Join(d, "tests", "test_lineas.py"), []byte("\n"), 0o644)
	if comprueba(arreglo, "", d) == "" {
		t.Error("tocar las pruebas tendría que fallar")
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
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("sin python3")
	}
	b := banco(t)
	i := slices.IndexFunc(b.Pruebas, func(p Prueba) bool { return p.ID == 6 })
	if i < 0 {
		t.Fatal("falta la prueba 6")
	}
	p := b.Pruebas[i]
	if len(p.Turnos) != 10 {
		t.Fatalf("%d turnos", len(p.Turnos))
	}
	d := filepath.Join(t.TempDir(), "p")
	if err := copia("../../consumo/proyecto", d); err != nil {
		t.Fatal(err)
	}
	cambia := func(f, de, a string) {
		t.Helper()
		bs, err := os.ReadFile(filepath.Join(d, f))
		if err != nil || !strings.Contains(string(bs), de) {
			t.Fatalf("%s: no está %q", f, de)
		}
		os.WriteFile(filepath.Join(d, f), []byte(strings.Replace(string(bs), de, a, 1)), 0o644)
	}
	renombra := func() {
		filepath.WalkDir(d, func(f string, e os.DirEntry, _ error) error {
			if strings.HasSuffix(f, ".py") {
				bs, _ := os.ReadFile(f)
				os.WriteFile(f, []byte(strings.ReplaceAll(string(bs), "subtotal(", "base_imponible(")), 0o644)
			}
			return nil
		})
	}
	pruebaLineas := func(nombre, cuerpo string) {
		cambia("tests/test_lineas.py", "\n\nif __name__", "\n\n    def "+nombre+"(self):\n        "+cuerpo+"\n\n\nif __name__")
	}
	pasos := []struct {
		hace      func()
		bien, mal string
	}{
		{hace: func() {
			cambia("facturacion/lineas.py", "self.bruto() - self.descuento", "self.bruto() * (1 - self.descuento / 100)")
		}},
		{hace: func() {
			pruebaLineas("test_descuento_total", "self.assertEqual(Linea(TORNILLO, 4, descuento=100).subtotal(), 0.0)")
		}},
		{hace: func() {
			cambia("facturacion/redondeo.py", "decimales=2):", "decimales=2, modo=\"mitad\"):")
			cambia("facturacion/redondeo.py", "rounding=ROUND_HALF_UP", "rounding=ROUND_DOWN if modo == \"abajo\" else ROUND_HALF_UP")
			cambia("facturacion/redondeo.py", "import Decimal, ROUND_HALF_UP", "import Decimal, ROUND_DOWN, ROUND_HALF_UP")
		}},
		{hace: func() {
			cambia("facturacion/impuestos/calculo.py", "redondea(base * porcentaje / 100)", "redondea(base * porcentaje / 100, modo=\"abajo\")")
		}},
		{hace: renombra},
		{hace: func() {
			cambia("facturacion/lineas.py", "descuento: float = 0.0  # porcentaje (0-100)", "descuento: float = 0.0  # porcentaje (0-100)\n    descuento_fijo: float = 0.0  # euros")
			cambia("facturacion/lineas.py", "self.bruto() * (1 - self.descuento / 100))", "self.bruto() * (1 - self.descuento / 100) - self.descuento_fijo)")
		}},
		{bien: "Arreglé el descuento (porcentaje), renombré subtotal a `base_imponible` y añadí `descuento_fijo`.", mal: "Arreglé el descuento."},
		{hace: func() {
			cambia("facturacion/lineas.py", "redondea(self.bruto()", "redondea(max(0, self.bruto()")
			cambia("facturacion/lineas.py", "- self.descuento_fijo)", "- self.descuento_fijo))")
		}},
		{hace: func() {
			pruebaLineas("test_descuento_fijo_minimo", "self.assertEqual(Linea(TORNILLO, 1, descuento_fijo=50).base_imponible(), 0.0)")
		}},
		{bien: "Antes, mitad hacia arriba (ROUND_HALF_UP); ahora, hacia abajo (trunca).", mal: "Igual que antes."},
	}
	for n, x := range pasos {
		tu := p.Turnos[n]
		if x.hace != nil {
			if f := comprueba(tu, x.bien, d); f == "" {
				t.Errorf("turno %d: se cumple sin hacerlo", n+1)
			}
			x.hace()
		} else if comprueba(tu, x.mal, d) == "" {
			t.Errorf("turno %d acepta %q", n+1, x.mal)
		}
		if f := comprueba(tu, x.bien, d); f != "" {
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
	c.evento(llm.Evento{Herramienta: &llm.Herramienta{ID: "1", Nombre: "Edit", Resumen: "facturacion/lineas.py", Entrada: "{}", Estado: "curso"}})
	c.evento(llm.Evento{Herramienta: &llm.Herramienta{ID: "1", Estado: "ok", Salida: "hecho"}})
	c.evento(llm.Evento{Tarea: &llm.Tarea{ID: "l", Tipo: "lista", Elementos: []llm.Elemento{{Texto: "Arreglar", Estado: "curso"}}}})
	c.evento(llm.Evento{Tarea: &llm.Tarea{ID: "l", Elementos: []llm.Elemento{{Texto: "Arreglar", Estado: "hecho"}}}})
	c.evento(llm.Evento{Texto: "Hecho."})
	r := c.fin()
	if len(r.parts) != 3 || r.parts[0].Texto != "Voy a arreglarlo." || r.parts[1].H.Nombre != "Edit" || r.parts[1].H.Resumen != "facturacion/lineas.py" ||
		r.parts[1].H.Estado != "ok" || r.parts[2].Texto != "Hecho." || r.Herramientas != 1 {
		t.Fatalf("%+v", r.parts)
	}
	if len(r.tasks) != 1 || r.tasks[0].Tipo != "lista" || r.tasks[0].Elementos[0].Estado != "hecho" {
		t.Errorf("tareas: %+v", r.tasks)
	}
	ms := uiMessages([]exchange{{Message: "Arregla", Reply: r.texto, Parts: r.parts, Tasks: r.tasks}})
	b, _ := json.Marshal(ms)
	for _, x := range []string{`"rol":"user","texto":"Arregla"`, `"tipo":"herramienta","h":{"id":"1","nombre":"Edit","resumen":"facturacion/lineas.py"`, `"tareas":[{"id":"l","tipo":"lista"`} {
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
			Parts: []uiPart{{Tipo: "herramienta", H: &llm.Herramienta{ID: "1", Nombre: "Edit", Resumen: "facturacion/lineas.py", Estado: "ok"}},
				{Tipo: "herramienta", H: &llm.Herramienta{ID: "2", Nombre: "Bash", Resumen: "python3 -m unittest discover -s tests", Estado: "ok", Salida: "OK"}},
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
	for _, x := range []string{"[Memoria de trabajo", "Arregla el fallo del descuento", "facturacion/lineas.py",
		"ok: `python3 -m unittest discover -s tests`", "[Fin de la memoria de trabajo]\n\nRenombra subtotal a base_imponible."} {
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
// cumple las comprobaciones una tras otra, y las de memoria aceptan las respuestas buenas y no las malas.
func TestConversacionLarga(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("sin python3")
	}
	b := banco(t)
	i := slices.IndexFunc(b.Pruebas, func(p Prueba) bool { return p.ID == 7 })
	if i < 0 {
		t.Fatal("falta la prueba 7")
	}
	p := b.Pruebas[i]
	d := filepath.Join(t.TempDir(), "p")
	if err := copia(filepath.Join("../../consumo", p.Proyecto), d); err != nil {
		t.Fatal(err)
	}
	cambia := func(f, viejo, nuevo string) {
		t.Helper()
		bs, err := os.ReadFile(filepath.Join(d, f))
		if err != nil || !strings.Contains(string(bs), viejo) {
			t.Fatalf("%s: no está %q", f, viejo)
		}
		os.WriteFile(filepath.Join(d, f), []byte(strings.Replace(string(bs), viejo, nuevo, 1)), 0o644)
	}
	if comprueba(p.Turnos[1], "", d) == "" {
		t.Fatal("sin arreglar, las pruebas pasan")
	}
	pasos := []func(){
		func() { cambia("almacen/precios.py", "if importe > minimo:", "if importe >= minimo:") },
		func() {
			cambia("almacen/envios.py", "def coste_envio(peso, zona):\n", "def coste_envio(peso, zona, urgente=False):\n"+
				"    return round(_base(peso, zona) + (RECARGO_URGENTE[zona] if urgente else 0), 2)\n\n\ndef _base(peso, zona):\n")
		},
		func() {
			cambia("almacen/informes.py", "ANCHO = 78\n", "from .envios import coste_envio, zona_de\nfrom .precios import aplica_descuento\n\n"+
				"ANCHO = 78\n\n\ndef total_pedido(pedido, cp):\n"+
				"    return round(aplica_descuento(pedido.importe()) + coste_envio(pedido.peso(), zona_de(cp), pedido.urgente), 2)\n")
		},
		func() {
			cambia("almacen/pedidos.py", "        self.historial.append((self.estado, nuevo))\n",
				"        if nuevo == \"EN_PREPARACION\" and inventario is not None:\n"+
					"            hechas = []\n            try:\n                for l in self.lineas:\n"+
					"                    inventario.reserva_stock(l.sku, l.cantidad)\n                    hechas.append(l)\n"+
					"            except SinStock:\n                for l in hechas:\n                    inventario.libera(l.sku, l.cantidad)\n"+
					"                raise\n        self.historial.append((self.estado, nuevo))\n")
			cambia("tests/test_pedidos.py", "\n\nif __name__", "\n    def test_reserva(self):\n        pass\n\n\nif __name__")
		},
		func() {
			cambia("almacen/devoluciones.py", "    return round(importe_productos + coste_envio, 2)",
				"    if motivo == \"desistimiento\":\n        coste_envio = 0\n    return round(importe_productos + coste_envio, 2)")
		},
	}
	for n, f := range pasos {
		f()
		if r := comprueba(p.Turnos[n+1], "", d); r != "" {
			t.Fatalf("turno %d: %s", n+2, r)
		}
	}
	textos := map[int]struct{ bien, mal []string }{
		0: {[]string{"Lo hace `Inventario.reserva_stock`: deja siempre 7 unidades (MARGEN_SEGURIDAD)."}, []string{"Lo hace reserva_stock, sin margen."}},
		6: {[]string{"Usaba `importe > minimo` y ahora `importe >= minimo`.", "Comparaba con > en vez de ≥."}, []string{"Comparaba con > y ahora también."}},
		7: {[]string{"MARGEN_SEGURIDAD = 7 unidades; el recargo urgente de la zona 3 es 5,25 €.", "5.25 € en la zona 3 y 7 unidades de margen (MARGEN_SEGURIDAD)."},
			[]string{"MARGEN_SEGURIDAD = 3; zona 3: 5,25 €.", "MARGEN_SEGURIDAD = 7; zona 3: 4 €."}},
	}
	for n, c := range textos {
		for _, x := range c.bien {
			if f := comprueba(p.Turnos[n], x, d); f != "" {
				t.Errorf("turno %d, %q: %s", n+1, x, f)
			}
		}
		for _, x := range c.mal {
			if comprueba(p.Turnos[n], x, d) == "" {
				t.Errorf("turno %d acepta %q", n+1, x)
			}
		}
	}
}
