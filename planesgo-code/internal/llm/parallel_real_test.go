package llm

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// Peticiones reales (de Luis y de proyectos como INTRADEMA) con lo que debería decidir pgocode.
var parallelCases = []struct {
	text string
	want string // «», «exploración» o «partes»
}{
	{"busca en todo el proyecto dónde se calcula el descuento de pruebas y dime qué ficheros intervienen", "exploración"},
	{"revisa todo el código de las plantillas y dime si hay algún lugar donde se imprime el precio sin IVA", "exploración"},
	{"quiero entender cómo funciona la importación de clientes: dónde empieza, qué módulos toca y dónde se guardan los errores", "exploración"},
	{"analiza todos los ficheros de la carpeta inventario y hazme un resumen de qué hace cada uno", "exploración"},
	{"find every place where the session timeout is read across the codebase and tell me which ones differ", "exploración"},
	{"hazme un inventario de todas las llamadas a la API de Odoo que hay en el proyecto", "exploración"},
	{"cambia el texto del botón Guardar a Aceptar en el formulario de proyectos y comprueba que se ve bien", ""},
	{"corrige el error de la línea 42 de inventivo.py: falta comprobar que la lista de piezas no esté vacía", ""},
	{"añade una columna de fecha de entrega a la tabla de pedidos y que se pueda ordenar por ella", ""},
	{"explícame qué hace exactamente la función calcula_total del fichero facturas.py", ""},
	{"compila, ejecuta las pruebas y si todo va bien sube la versión y publícala", ""},
	{"el puerto del servidor está en config.json: cámbialo al 8081 y reinicia el servicio de pruebas", ""},
	{"- arregla el cálculo del descuento en pedidos.py\n- añade el campo observaciones al formulario de clientes\n- cambia el logo de la cabecera por el nuevo\n- actualiza el README con los nuevos puertos", "partes"},
	{"Haz estas tareas:\n1. traduce los mensajes de error del módulo de facturas\n2. optimiza la consulta del listado de clientes\n3. documenta la API de pedidos\n4. añade pruebas del validador de NIF", "partes"},
	{"Pasos:\n1. crea la tabla de etiquetas en la base de datos\n2. añade el modelo Etiqueta que use esa tabla\n3. crea el endpoint que lista las etiquetas del modelo\n4. muestra las etiquetas en el Kanban con ese endpoint", ""},
}

// PGO_PRUEBA_PARALELO=1 go test -run TestParallelReal -v ./internal/llm: con la IA local de verdad, lo que decide y
// los parecidos (para calibrar parallelExploreSim, parallelExploreMargin y parallelIndependent).
func TestParallelReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_PARALELO") == "" {
		t.Skip()
	}
	bien := 0
	for _, c := range parallelCases {
		ex, fo, _ := exploreScores(context.Background(), c.text, pruneEmbed)
		got := ""
		if a := ParallelHintFor(context.Background(), c.text, pruneEmbed); a != nil {
			got = a.Kind
		}
		marca := "✓"
		if got != c.want {
			marca = "✗"
		} else {
			bien++
		}
		sim := ""
		if items := listItems(c.text); len(items) >= 3 {
			s, n := 0.0, 0
			for i := range items {
				for j := i + 1; j < len(items); j++ {
					a, _ := pruneEmbed(context.Background(), items[i], "SEMANTIC_SIMILARITY")
					b, _ := pruneEmbed(context.Background(), items[j], "SEMANTIC_SIMILARITY")
					s += cosine(a, b)
					n++
				}
			}
			sim = " entre partes " + fmtF(s/float64(n))
		}
		t.Logf("%s esperado %-11q decide %-11q expl %.2f conc %.2f%s · %.50s", marca, c.want, got, ex, fo, sim, c.text)
	}
	t.Logf("%d de %d", bien, len(parallelCases))
}

func fmtF(f float64) string { return fmt.Sprintf("%.2f", f) }
