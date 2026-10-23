package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Paralelización automática: antes de enviar una petición a Claude Code, pgocode decide (sin tokens de pago: reglas
// y, si hay, los embeddings de la IA local) si conviene que el agente use subagentes, y en ese caso añade al final
// de ese mensaje una frase corta. Un subagente trabaja con su propio contexto y solo devuelve su resumen: la
// exploración amplia (leer muchos ficheros, buscar) no llena la conversación principal, que es lo que más cuesta.
//
//	exploración: la petición pide recorrer o buscar por mucho código («busca dónde se usa…», «revisa todo…»)
//	partes: la petición lista tareas distintas e independientes (viñetas o números sobre cosas que no se parecen)
//
// Las peticiones cortas, las órdenes («/…») y las que piden una sola cosa concreta no llevan nada.

const (
	// parallelMinWords: menos palabras no dicen de qué va («sigue», «compila y prueba»): no se decide nada.
	parallelMinWords = 9
	// parallelExploreSim: parecido mínimo con las peticiones de exploración (embeddings) y margen sobre las
	// concretas, para decidir solo con ellos cuando las reglas no ven nada claro.
	parallelExploreSim    = 0.45
	parallelExploreMargin = 0.06
	// parallelIndependent: parecido medio entre las partes por debajo del cual se consideran independientes.
	parallelIndependent = 0.62
)

// Mensajes que pgocode añade (en español, como el resto de lo que le llega al agente de Luis; cortos: se quedan en su historial).
const (
	hintExplore  = "(Para esto, delega la exploración amplia en subagentes: que lean y busquen ellos y te devuelvan solo lo relevante, para no llenar tu contexto.)"
	hintParallel = "(Las %d partes de la lista son independientes: lánzalas en paralelo con subagentes y reúne sus resultados.)"
)

// ParallelAdvice: lo que se decide de una petición.
type ParallelAdvice struct {
	Kind  string // «exploración» o «partes»
	Parts int    // en «partes», cuántas
	Hint  string // la frase para el agente
	Why   string // para el chat: por qué
}

var (
	exploreRules = []string{
		"busca ", "buscar ", "encuentra ", "localiza ", "rastrea ", "dónde se ", "donde se ", "dónde está", "dónde estan", "en qué ficheros", "en que ficheros",
		"qué ficheros", "que ficheros", "revisa todo", "revisa el código", "revisa el codigo", "revisa todos", "analiza todo", "analiza el código", "analiza el codigo",
		"audita ", "auditoría", "investiga ", "recorre ", "inventario", "todos los ficheros", "todas las referencias", "todos los usos",
		"en todo el proyecto", "en todo el repositorio", "en todo el código", "en todo el codigo", "por todo el", "cada uno de los",
		"find all", "search for", "where is", "where are", "audit ", "across the codebase", "every file", "all usages",
	}
	listLine = regexp.MustCompile(`(?m)^\s*(?:\d{1,2}[.)]|[-*•])\s+\S`)
)

// ParallelHintFor decide si la petición lleva una sugerencia de subagentes (nil: no). embed puede ser nil (solo
// reglas).
func ParallelHintFor(ctx context.Context, text string, embed func(context.Context, string, string) ([]float32, error)) *ParallelAdvice {
	t := strings.TrimSpace(text)
	if t == "" || strings.HasPrefix(t, "/") || len(strings.Fields(t)) < parallelMinWords {
		return nil
	}
	low := strings.ToLower(t)
	// Partes: una lista de tres o más cosas que no se parecen entre sí.
	if items := listItems(t); len(items) >= 3 {
		if independent(ctx, items, embed) {
			return &ParallelAdvice{Kind: "partes", Parts: len(items), Hint: fmt.Sprintf(hintParallel, len(items)),
				Why: fmt.Sprintf("%d partes independientes", len(items))}
		}
		return nil // pasos de un mismo trabajo: en orden, no en paralelo
	}
	hits := 0
	for _, r := range exploreRules {
		if strings.Contains(low, r) {
			hits++
		}
	}
	if hits >= 2 || (hits == 1 && exploreByEmbedding(ctx, t, embed, true)) || (hits == 0 && embed != nil && exploreByEmbedding(ctx, t, embed, false)) {
		return &ParallelAdvice{Kind: "exploración", Hint: hintExplore, Why: "exploración amplia"}
	}
	return nil
}

// ParallelHint: con los embeddings de la IA local (los del hilo de limpieza).
func ParallelHint(ctx context.Context, text string) *ParallelAdvice {
	return ParallelHintFor(ctx, text, pruneEmbed)
}

// listItems: los elementos de una lista con viñetas o números.
func listItems(t string) []string {
	var out []string
	for _, l := range strings.Split(t, "\n") {
		if listLine.MatchString(l) {
			out = append(out, strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-*•0123456789.) ")))
		}
	}
	return out
}

// independent: las partes no se parecen entre sí (parecido medio bajo). Sin embeddings no se puede saber: se
// toman por independientes solo si hay cinco o más (una lista tan larga rara vez es una secuencia).
func independent(ctx context.Context, items []string, embed func(context.Context, string, string) ([]float32, error)) bool {
	if embed == nil {
		return len(items) >= 5
	}
	vs := make([][]float32, 0, len(items))
	for _, it := range items {
		v, err := embed(ctx, it, "SEMANTIC_SIMILARITY")
		if err != nil {
			return len(items) >= 5
		}
		vs = append(vs, v)
	}
	sum, n := 0.0, 0
	for i := range vs {
		for j := i + 1; j < len(vs); j++ {
			sum += cosine(vs[i], vs[j])
			n++
		}
	}
	return n > 0 && sum/float64(n) < parallelIndependent
}

// Peticiones de referencia (el parecido con ellas decide cuando las reglas no ven nada claro).
var (
	exploreRefs = []string{
		"busca en todo el proyecto dónde se usa esta función y qué módulos dependen de ella",
		"revisa todo el código de la carpeta e infórmame de cómo está organizado",
		"analiza todos los ficheros del repositorio y dime dónde se gestionan los permisos",
		"investiga cómo funciona la autenticación recorriendo el código y los tests",
		"encuentra todos los lugares donde se construye esta consulta SQL",
		"find every place where this configuration value is read across the codebase",
	}
	focusRefs = []string{
		"cambia el color del botón de guardar a azul en el componente de cabecera",
		"corrige el error de la línea 42 de utils.py: falta comprobar que la lista no esté vacía",
		"añade un campo de fecha al formulario de pedidos y guárdalo en la base de datos",
		"explícame qué hace la función calcula_total del fichero facturas.py",
		"renombra la variable cliente_id a customer_id en este fichero",
		"escribe una prueba para la función que valida el NIF",
	}
	refVecs struct {
		sync.Mutex
		ok             bool
		explore, focus [][]float32
	}
)

func loadRefs(ctx context.Context, embed func(context.Context, string, string) ([]float32, error)) ([][]float32, [][]float32, bool) {
	refVecs.Lock()
	defer refVecs.Unlock()
	if refVecs.ok {
		return refVecs.explore, refVecs.focus, true
	}
	conv := func(l []string) ([][]float32, bool) {
		out := make([][]float32, 0, len(l))
		for _, s := range l {
			v, err := embed(ctx, s, "RETRIEVAL_DOCUMENT")
			if err != nil {
				return nil, false
			}
			out = append(out, v)
		}
		return out, true
	}
	e, ok1 := conv(exploreRefs)
	f, ok2 := conv(focusRefs)
	if !ok1 || !ok2 {
		return nil, nil, false
	}
	refVecs.explore, refVecs.focus, refVecs.ok = e, f, true
	return e, f, true
}

// exploreScores: el parecido máximo con las peticiones de exploración y con las concretas.
func exploreScores(ctx context.Context, t string, embed func(context.Context, string, string) ([]float32, error)) (ex, fo float64, ok bool) {
	if embed == nil {
		return 0, 0, false
	}
	e, f, ok := loadRefs(ctx, embed)
	if !ok {
		return 0, 0, false
	}
	v, err := embed(ctx, t, "RETRIEVAL_QUERY")
	if err != nil {
		return 0, 0, false
	}
	for _, r := range e {
		ex = max(ex, cosine(v, r))
	}
	for _, r := range f {
		fo = max(fo, cosine(v, r))
	}
	return ex, fo, true
}

// exploreByEmbedding: se parece a las peticiones de exploración más que a las concretas. Con una pista de las
// reglas (conPista) basta con parecerse algo más; sin ella, hace falta un margen claro.
func exploreByEmbedding(ctx context.Context, t string, embed func(context.Context, string, string) ([]float32, error), conPista bool) bool {
	ex, fo, ok := exploreScores(ctx, t, embed)
	if !ok {
		return false
	}
	if conPista {
		return ex >= parallelExploreSim-0.1 && ex > fo
	}
	return ex >= parallelExploreSim && ex-fo >= parallelExploreMargin
}
