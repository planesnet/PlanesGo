package servidor

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/sesiones"
)

// Preguntas aparte: lo que el usuario pregunta sin que entre en la conversación con el agente (una interrupción
// que una persona olvidaría). Ni cambian la sesión del agente ni su contexto: o las responde un modelo barato sin
// historial («suelta»), o una copia de la sesión del agente que después se tira («contexto»). Antigravity no
// permite copiar sus conversaciones: ahí responde el modelo barato con el resumen que manda la interfaz.

const sistemaAparte = "El usuario te hace una pregunta aparte mientras trabaja con un agente de programación. " +
	"Responde breve y claro. No tienes herramientas: si no lo sabes con lo que tienes, dilo."

// bifurcables: los agentes que pueden copiar su sesión para una pregunta aparte.
var bifurcables = map[string]bool{"claude-code": true, "opencode": true}

func (s *Servidor) aparte(turno string, r *http.Request, tipo, contexto string, prov llm.Proveedor, info llm.Info, pet llm.Peticion, sesionImp, proveedor string, envia func(any)) {
	inicio := time.Now()
	pregunta := pet.Mensajes[0].Texto
	via := "modelo"
	var err error
	if tipo == "contexto" && pet.SesionAgente != "" && bifurcables[prov.ID()] {
		via = "copia"
		err = prov.Chat(r.Context(), pet, func(e llm.Evento) {
			if e.Uso != nil && s.Imputacion != nil {
				s.Imputacion.SumaUso(sesionImp, e.Uso.Entrada, e.Uso.Salida, e.Uso.Coste)
			}
			// Solo la respuesta: ni la sesión de la copia, ni programas, ni procesos vivos.
			if e.Texto != "" || e.Pensamiento != "" || e.Herramienta != nil || e.Uso != nil {
				envia(llm.Evento{Texto: e.Texto, Pensamiento: e.Pensamiento, Herramienta: e.Herramienta, Uso: e.Uso})
			}
		})
	} else {
		texto := pregunta
		if tipo == "contexto" && strings.TrimSpace(contexto) != "" {
			texto = "Contexto de la conversación con el agente (resumen de pgocode):\n" + recortaTexto(contexto, 12000) + "\n\nPregunta:\n" + pregunta
		}
		var resp string
		var u llm.Uso
		resp, u, err = llm.Gemini(r.Context(), llm.ModeloAparte, sistemaAparte+" "+pet.Sistema, texto, 2048, 512)
		if err == nil {
			if s.Imputacion != nil {
				s.Imputacion.SumaUso(sesionImp, u.Entrada, u.Salida, u.Coste)
			}
			envia(llm.Evento{Texto: resp})
			envia(llm.Evento{Uso: &u})
		}
	}
	if s.Imputacion != nil {
		c := map[string]any{"turno": turno, "tipo": tipo, "via": via, "proveedor": proveedor, "turno_s": time.Since(inicio).Seconds()}
		if err != nil {
			c["error"] = err.Error()
		}
		s.Imputacion.AnotaAgente(sesionImp, "aparte", c)
	}
	if err != nil && r.Context().Err() == nil {
		envia(map[string]string{"error": err.Error()})
	}
	envia(map[string]bool{"fin": true})
}

// clasifica: ¿el mensaje va a la conversación o es una pregunta aparte? Solo para lo que las reglas de la
// interfaz no tienen claro (y con el agente ocupado): un modelo barato con el mensaje y lo último del agente.
func (s *Servidor) clasifica(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Texto  string `json:"texto"`
		Ultimo string `json:"ultimo"` // lo último que ha dicho el agente
		Sesion string `json:"sesion"`
		// Ramas por tema: el tema de la sesión en 2 o 3 líneas (de la memoria de trabajo). Con él, el clasificador
		// también puede decir «tema» (otro asunto: mejor en una sesión hija). SoloTema: con el agente libre, solo
		// interesa si es otro tema (lo demás va a la conversación).
		Tema     string `json:"tema"`
		SoloTema bool   `json:"solo_tema"`
	}
	if err := lee(r, &p); err != nil || strings.TrimSpace(p.Texto) == "" {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	inicio := time.Now()
	tipo, u, err := clasificaMensaje(r.Context(), p.Texto, p.Ultimo, p.Tema, p.SoloTema)
	if s.Imputacion != nil && p.Sesion != "" {
		c := map[string]any{"tipo": tipo, "modelo": llm.ModeloClasifica, "ms": time.Since(inicio).Milliseconds(), "entrada": u.Entrada, "salida": u.Salida, "coste": u.Coste}
		if err != nil {
			c["error"] = err.Error()
		}
		s.Imputacion.AnotaAgente(p.Sesion, "clasifica", c)
	}
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, map[string]any{"tipo": tipo, "uso": u})
}

const sistemaClasifica = `Un usuario escribe a un agente de programación que está trabajando. Decide adónde va su mensaje y responde SOLO una palabra:
conversacion: es una instrucción, un cambio, una respuesta al agente o algo que el agente debe saber o hacer.
contexto: es una pregunta sobre el trabajo o la conversación que no le pide nada al agente (curiosidad, aclaración).
suelta: es una pregunta que no tiene que ver con el trabajo (cultura general, otro tema).
Si dudas, conversacion.`

// Otro tema: una petición de trabajo sobre otro asunto, que va mejor en una sesión hija (con lo mínimo) que en una
// conversación que ya ha crecido. Se añade a lo anterior solo si la interfaz manda el tema de la sesión.
const lineaTema = "tema: SOLO si pide TRABAJO NUEVO (hacer, añadir, cambiar algo) sobre OTRA funcionalidad u otra parte del proyecto, distinta del tema de la sesión, y se entiende sin lo hablado. Dar un dato, pedir que recuerde algo, preguntar, opinar, continuar, corregir o depender de lo que se está haciendo NO es tema.\n" +
	"Ejemplos (sesión sobre el diseño de la lista de proyectos): «Añade las etiquetas de Odoo a la lista de tareas» → tema; «Recuerda esta palabra clave: AZUL» → conversacion; «Pon el botón más a la derecha» → conversacion; «¿Qué colores has usado?» → conversacion.\n"

const sistemaSoloTema = `Un usuario escribe a un agente de programación en una sesión que ya trata de un tema. Decide si su mensaje sigue en esa conversación o es otro tema y responde SOLO una palabra:
conversacion: sigue con el tema de la sesión, lo continúa, lo corrige, responde al agente o depende de lo hablado.
` + lineaTema + `Si dudas, conversacion.`

var tiposClasifica = []string{"conversacion", "contexto", "suelta", "tema"}

var clasificaGemini = llm.Gemini

func clasificaMensaje(ctx context.Context, texto, ultimo, tema string, soloTema bool) (string, llm.Uso, error) {
	q := "Mensaje del usuario:\n" + recortaTexto(texto, 1500)
	if strings.TrimSpace(ultimo) != "" {
		q = "Lo último que dijo el agente:\n" + recortaTexto(ultimo, 600) + "\n\n" + q
	}
	sistema := sistemaClasifica
	tema = strings.TrimSpace(tema)
	if tema != "" {
		q = "Tema de la sesión:\n" + recortaTexto(tema, 800) + "\n\n" + q
		if soloTema {
			sistema = sistemaSoloTema
		} else {
			sistema = strings.Replace(sistemaClasifica, "Si dudas", lineaTema+"Si dudas", 1)
		}
	}
	resp, u, err := clasificaGemini(ctx, llm.ModeloClasifica, sistema, q, 5, 0)
	if err != nil {
		return "conversacion", u, err
	}
	t := tipoClasifica(resp)
	// Sin tema no hay «tema»; y con el agente libre, lo que no es otro tema va a la conversación.
	if (t == "tema" && tema == "") || (soloTema && t != "tema") {
		t = "conversacion"
	}
	return t, u, nil
}

// tipoClasifica: la palabra de la respuesta (sin tildes ni signos); cualquier otra cosa, a la conversación.
func tipoClasifica(resp string) string {
	r := strings.ToLower(strings.Trim(strings.TrimSpace(resp), ".:*`\"' \n"))
	r = strings.ReplaceAll(r, "ó", "o")
	for _, t := range tiposClasifica {
		if r == t || (utf8.RuneCountInString(r) > len(t) && strings.HasPrefix(r, t)) {
			return t
		}
	}
	return "conversacion"
}
