package servidor

import (
	"context"
	"os"
	"testing"
	"time"

	"planesgo-code/internal/selector"
)

// PGO_PRUEBA_CLASIFICA=1 go test -run TestClasificaLocalReal -v ./internal/servidor: la clasificación con la IA
// local de verdad (gemma3) sobre mensajes etiquetados: aciertos y tiempo.
func TestClasificaLocalReal(t *testing.T) {
	if os.Getenv("PGO_PRUEBA_CLASIFICA") == "" {
		t.Skip()
	}
	casos := []struct{ texto, ultimo, quiere string }{
		{"para, eso no es así: usa la tabla de clientes nueva", "Estoy cambiando la consulta de clientes.", "conversacion"},
		{"añade también un test para el caso vacío", "He corregido la función.", "conversacion"},
		{"sí, adelante", "¿Quieres que borre los ficheros temporales?", "conversacion"},
		{"usa el puerto 8081 en vez del 8080", "Arranco el servidor.", "conversacion"},
		{"no toques el módulo de facturas", "Voy a revisar los módulos.", "conversacion"},
		{"¿por qué has elegido una lista en vez de un diccionario?", "He guardado los precios en una lista.", "contexto"},
		{"¿qué fichero estás leyendo ahora?", "Leyendo la configuración…", "contexto"},
		{"¿cuánto le queda?", "Ejecutando las pruebas.", "contexto"},
		{"¿qué significa el error que ha salido antes?", "Ha fallado una prueba con KeyError.", "contexto"},
		{"¿cuál es la capital de Australia?", "Compilando.", "suelta"},
		{"¿qué tiempo hace hoy en Madrid?", "Revisando el código.", "suelta"},
		{"¿cómo se dice ordenador en inglés?", "Editando el README.", "suelta"},
	}
	bien := 0
	for _, c := range casos {
		t0 := time.Now()
		q := "Lo último que dijo el agente:\n" + c.ultimo + "\n\nMensaje del usuario:\n" + c.texto
		resp, modelo, err := selector.LocalChat(context.Background(), sistemaClasificaLocal, q, 8)
		got, ok := tipoClasificaExacto(resp)
		marca := "✓"
		if err != nil || !ok || got != c.quiere {
			marca = "✗"
		} else {
			bien++
		}
		t.Logf("%s %-12s → %-12q (%s, %d ms) %v · %s", marca, c.quiere, resp, modelo, time.Since(t0).Milliseconds(), err, c.texto)
	}
	t.Logf("%d de %d", bien, len(casos))
}
