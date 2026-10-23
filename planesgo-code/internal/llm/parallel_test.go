package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// embedFalso: vectores a mano: las partes «iguales» se parecen, las demás no.
func embedFalso(_ context.Context, t, _ string) ([]float32, error) {
	switch {
	case strings.Contains(t, "tabla") || strings.Contains(t, "modelo"):
		return []float32{1, 0.1, 0}, nil
	case strings.Contains(t, "logo"):
		return []float32{0, 1, 0}, nil
	case strings.Contains(t, "README"):
		return []float32{0, 0, 1}, nil
	}
	return []float32{0.3, 0.3, 0.3}, nil
}

func TestParallelHintRules(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct{ text, want string }{
		{"sigue", ""},
		{"/compact ahora mismo por favor", ""},
		{"busca en todo el proyecto dónde se usa esta función y revisa todo el código que dependa de ella", "exploración"},
		{"cambia el color del botón de guardar a azul en el componente de cabecera de la página", ""},
		{"- arregla el descuento\n- cambia el logo\n- actualiza el README", "partes"},
		{"Pasos:\n1. crea la tabla de etiquetas\n2. crea el modelo de etiquetas\n3. crea la tabla de usos con ese modelo", ""},
		// Ficheros distintos: independientes aunque se parezcan (aquí, todas al mismo vector); el mismo fichero: no.
		{"Haz esto:\n1. en precios.py añade el IVA\n2. en textos.py añade capitaliza\n3. en fechas.py añade bisiesto", "partes"},
		{"Haz esto:\n1. en precios.py añade el IVA\n2. en precios.py añade descuentos\n3. en fechas.py añade bisiesto", ""},
		// Las peticiones de plan de pgocode y las tareas pequeñas, sin sugerencia.
		{"[Modo plan de pgocode] Investiga lo necesario sin modificar ningún fichero, busca en todo el proyecto dónde se usa la función y revisa todo el código", ""},
		{"añade devolucion y merma al tipo de movimiento y busca dónde se muestra el tipo para traducirlo", ""},
		{"añade logs en todos los usos de la API de Odoo: busca dónde se llama en todo el proyecto", "exploración"},
	} {
		a := ParallelHintFor(ctx, c.text, embedFalso)
		got := ""
		if a != nil {
			got = a.Kind
		}
		if got != c.want {
			t.Errorf("%q: %q, se esperaba %q", c.text, got, c.want)
		}
	}
	// Sin IA local: la exploración por reglas sigue; una lista corta no se reparte, una de cinco sí.
	if a := ParallelHintFor(ctx, "busca en todo el proyecto dónde se usa esta función y revisa todo el código que dependa de ella", nil); a == nil || a.Kind != "exploración" {
		t.Errorf("sin IA local: %+v", a)
	}
	if ParallelHintFor(ctx, "- uno\n- dos\n- tres y cuatro cosas más a la vez ahora", nil) != nil {
		t.Error("tres partes sin IA local no se reparten")
	}
	cinco := "Haz esto:\n- a\n- b\n- c\n- d\n- e"
	if a := ParallelHintFor(ctx, cinco, func(context.Context, string, string) ([]float32, error) { return nil, errors.New("caído") }); a == nil || a.Parts != 5 {
		t.Errorf("cinco partes sin IA local: %+v", a)
	}
}

// Claude Code recibe la sugerencia solo en ese mensaje, con aviso en el chat; los demás agentes, las preguntas
// aparte y quien no lo pide, nada.
func TestParallelHintEnChat(t *testing.T) {
	viejo := pruneEmbed
	pruneEmbed = embedFalso
	defer func() { pruneEmbed = viejo }()
	texto := "busca en todo el proyecto dónde se usa esta función y revisa todo el código que dependa de ella"
	for _, c := range []struct {
		clave    string
		aparte   bool
		paralelo bool
		quiere   bool
	}{{"claude-code", false, true, true}, {"claude-code", true, true, false}, {"claude-code", false, false, false}, {"opencode", false, true, false}} {
		var visto string
		a := &Agente{Clave: c.clave, Nombre: c.clave, Programa: "sh", Lee: leeOpenCode,
			Args: func(msg string, p Peticion) []string {
				visto = msg
				return []string{"-c", `echo '{"type":"text","sessionID":"ses_abc123","part":{"text":"hecho"}}'`}
			}}
		var salida strings.Builder
		err := a.Chat(context.Background(), Peticion{Carpeta: t.TempDir(), Permisos: "lectura", Aparte: c.aparte, Paralelo: c.paralelo,
			Mensajes: []Mensaje{{Rol: "user", Texto: texto}}}, func(e Evento) { salida.WriteString(e.Texto) })
		if err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
		lleva := strings.Contains(visto, hintExplore)
		avisa := strings.Contains(salida.String(), "ha sugerido usar subagentes")
		if lleva != c.quiere || avisa != c.quiere || (lleva && !strings.HasPrefix(visto, texto)) {
			t.Errorf("%+v: mensaje con sugerencia %v, aviso %v; mensaje %q", c, lleva, avisa, visto)
		}
	}
}

func TestSplitRequest(t *testing.T) {
	r := SplitRequest("En el módulo de facturas:\n\n1. Corrige el redondeo del IVA\n   en las líneas con descuento\n2. Añade el campo «referencia» al pedido\n- Traduce los textos del informe\n\nAl acabar, ejecuta las pruebas.")
	if r.Intro != "En el módulo de facturas:" || r.Outro != "Al acabar, ejecuta las pruebas." {
		t.Errorf("intro %q, cierre %q", r.Intro, r.Outro)
	}
	if len(r.Items) != 3 || r.Items[0] != "Corrige el redondeo del IVA en las líneas con descuento" || r.Items[2] != "Traduce los textos del informe" {
		t.Errorf("partes %q", r.Items)
	}
	// Los mismos elementos que cuenta listItems.
	t2 := "uno\n- a\n- b\n  c\n* d"
	if n := len(SplitRequest(t2).Items); n != len(listItems(t2)) {
		t.Errorf("%d partes, listItems %d", n, len(listItems(t2)))
	}
}
