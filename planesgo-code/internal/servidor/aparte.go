package servidor

import (
	"context"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"planesgo-code/internal/llm"
	"planesgo-code/internal/selector"
	"planesgo-code/internal/sesiones"
)

// Preguntas aparte: lo que el usuario pregunta sin que entre en la conversación con el agente (una interrupción
// que una persona olvidaría). Ni cambian la sesión del agente ni su contexto: o las responde un modelo barato sin
// historial («suelta»), o una copia de la sesión del agente que después se tira («contexto»). Antigravity no
// permite copiar sus conversaciones: ahí responde el modelo barato con el resumen que manda la interfaz.
//
// La IA local como DESTINO (docs/enrutador.md): la pregunta suelta (cultura general, traducciones, qué significa
// algo) la responde primero el modelo de texto de la IA local del equipo (gemma3:4b: 0 tokens, menos de 1 s en
// caliente; medido: 9 de 10 respuestas correctas); si no hay modelo de texto, no responde a tiempo o da una
// respuesta vacía, el modelo barato de pago. La interfaz deja repetirla con el de pago (motor «pago»). Con contexto
// NO: un modelo de 4B con 12k caracteres de resumen de la conversación responde peor que la copia de la sesión.

const sistemaAparte = "El usuario te hace una pregunta aparte mientras trabaja con un agente de programación. " +
	"Responde breve y claro. No tienes herramientas: si no lo sabes con lo que tienes, dilo."

// bifurcables: los agentes que pueden copiar su sesión para una pregunta aparte.
var bifurcables = map[string]bool{"claude-code": true, "opencode": true}

// motor «pago»: la suelta, con el modelo de pago aunque haya IA local (lo pide el usuario).
func (s *Servidor) aparte(turno string, r *http.Request, tipo, contexto, motor string, prov llm.Proveedor, info llm.Info, pet llm.Peticion, sesionImp, proveedor string, envia func(any)) {
	inicio := time.Now()
	pregunta := pet.Mensajes[0].Texto
	via := "modelo"
	var err error
	if tipo == "contexto" && pet.SesionAgente != "" && bifurcables[prov.ID()] {
		via = "copia"
		envia(map[string]string{"via": via})
		err = prov.Chat(r.Context(), pet, func(e llm.Evento) {
			if e.Uso != nil && s.Imputacion != nil {
				s.Imputacion.SumaUso(sesionImp, e.Uso.Entrada, e.Uso.Salida, e.Uso.Coste)
			}
			// Solo la respuesta: ni la sesión de la copia, ni programas, ni procesos vivos.
			if e.Texto != "" || e.Pensamiento != "" || e.Herramienta != nil || e.Uso != nil {
				envia(llm.Evento{Texto: e.Texto, Pensamiento: e.Pensamiento, Herramienta: e.Herramienta, Uso: e.Uso})
			}
		})
	} else if resp, modelo, ok := s.aparteLocal(r.Context(), tipo, motor, pet.Sistema, pregunta); ok {
		via = "local/" + modelo
		envia(map[string]string{"via": via})
		envia(llm.Evento{Texto: resp})
		envia(llm.Evento{Uso: &llm.Uso{}})
	} else {
		texto := pregunta
		if tipo == "contexto" && strings.TrimSpace(contexto) != "" {
			texto = "Contexto de la conversación con el agente (resumen de pgocode):\n" + recortaTexto(contexto, 12000) + "\n\nPregunta:\n" + pregunta
		}
		var resp string
		var u llm.Uso
		resp, u, err = aparteGemini(r.Context(), llm.ModeloAparte, sistemaAparte+" "+pet.Sistema, texto, 2048, 512)
		if err == nil {
			envia(map[string]string{"via": via})
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

// aparteGemini y aparteLocalChat: los modelos de las preguntas aparte (en las pruebas se sustituyen).
var (
	aparteGemini    = llm.Gemini
	aparteLocalChat = selector.LocalChat
)

// aparteLocalMax: lo que se espera a la IA local para una pregunta suelta (la primera vez carga el modelo).
const aparteLocalMax = 20 * time.Second

// aparteLocal: la pregunta suelta con la IA local del equipo (ok: hay respuesta). No con contexto ni si el usuario
// pide el modelo de pago.
func (s *Servidor) aparteLocal(ctx context.Context, tipo, motor, sistema, pregunta string) (string, string, bool) {
	if tipo != "suelta" || motor == "pago" {
		return "", "", false
	}
	c, cancela := context.WithTimeout(ctx, aparteLocalMax)
	defer cancela()
	resp, modelo, err := aparteLocalChat(c, sistemaAparte+" Si no lo sabes con seguridad, di que no lo sabes. "+sistema, pregunta, 600)
	resp = strings.TrimSpace(resp)
	if err != nil || resp == "" {
		return "", "", false
	}
	return resp, modelo, true
}

// clasifica: ¿el mensaje va a la conversación o es una pregunta aparte? Solo para lo que las reglas de la
// interfaz no tienen claro (y con el agente ocupado): un modelo barato con el mensaje y lo último del agente.
func (s *Servidor) clasifica(w http.ResponseWriter, r *http.Request, ses *sesiones.Sesion) {
	var p struct {
		Texto  string `json:"texto"`
		Ultimo string `json:"ultimo"` // lo último que ha dicho el agente
		Sesion string `json:"sesion"`
	}
	if err := lee(r, &p); err != nil || strings.TrimSpace(p.Texto) == "" {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	inicio := time.Now()
	tipo, modelo, u, err := clasificaMensaje(r.Context(), p.Texto, p.Ultimo)
	if s.Imputacion != nil && p.Sesion != "" {
		c := map[string]any{"tipo": tipo, "modelo": modelo, "ms": time.Since(inicio).Milliseconds(), "entrada": u.Entrada, "salida": u.Salida, "coste": u.Coste}
		if err != nil {
			c["error"] = err.Error()
		}
		s.Imputacion.AnotaAgente(p.Sesion, "clasifica", c)
	}
	if err != nil {
		jsonError(w, http.StatusBadGateway, err.Error())
		return
	}
	jsonOK(w, map[string]any{"tipo": tipo, "uso": u, "modelo": modelo})
}

const sistemaClasifica = `Un usuario escribe a un agente de programación que está trabajando. Decide adónde va su mensaje y responde SOLO una palabra:
conversacion: es una instrucción, un cambio, una respuesta al agente o algo que el agente debe saber o hacer.
contexto: es una pregunta sobre el trabajo o la conversación que no le pide nada al agente (curiosidad, aclaración).
suelta: es una pregunta que no tiene que ver con el trabajo (cultura general, otro tema).
Si dudas, conversacion.`

// sistemaClasificaLocal: lo mismo para el modelo pequeño de la IA local, con ejemplos (sin ellos lo manda casi todo
// a la conversación).
const sistemaClasificaLocal = `Clasifica el mensaje que un usuario escribe a un agente de programación mientras trabaja. Responde SOLO una palabra: conversacion, contexto o suelta.
conversacion: le pide al agente que haga, cambie, pare o tenga en cuenta algo, o contesta a una pregunta del agente (sí, no, adelante, vale).
contexto: es una PREGUNTA sobre lo que el agente hace o ha hecho (por qué, qué está haciendo, cuánto falta, qué significa algo de su trabajo). No le pide hacer nada.
suelta: es una PREGUNTA que no tiene nada que ver con el trabajo ni con el código (geografía, tiempo, traducciones, deportes, cultura general).
Ejemplos:
«cambia el color del botón a rojo» → conversacion
«vale, hazlo así» → conversacion
«no uses esa librería» → conversacion
«¿por qué has usado un bucle aquí?» → contexto
«¿en qué paso vas?» → contexto
«¿qué hace esa función que has creado?» → contexto
«¿quién ganó el mundial de 2010?» → suelta
«¿cuántos habitantes tiene Francia?» → suelta`

var tiposClasifica = []string{"conversacion", "contexto", "suelta"}

var clasificaGemini = llm.Gemini

// clasificaLocal: con el modelo de texto de la IA local (sin tokens de pago); en las pruebas se sustituye.
var clasificaLocal = selector.LocalChat

// clasificaLocalMax: lo que se espera a la IA local (la primera vez carga el modelo); después, Gemini.
const clasificaLocalMax = 6 * time.Second

// clasificaMensaje: primero la IA local; si no tiene modelo de texto, no responde a tiempo o dice otra cosa, el
// modelo barato de pago. Devuelve el tipo y el modelo que decidió.
func clasificaMensaje(ctx context.Context, texto, ultimo string) (string, string, llm.Uso, error) {
	q := "Mensaje del usuario:\n" + recortaTexto(texto, 1500)
	if strings.TrimSpace(ultimo) != "" {
		q = "Lo último que dijo el agente:\n" + recortaTexto(ultimo, 600) + "\n\n" + q
	}
	cl, cancela := context.WithTimeout(ctx, clasificaLocalMax)
	resp, modelo, err := clasificaLocal(cl, sistemaClasificaLocal, q, 8)
	cancela()
	if err == nil {
		if t, ok := tipoClasificaExacto(resp); ok {
			return t, "local/" + modelo, llm.Uso{}, nil
		}
	}
	resp, u, err := clasificaGemini(ctx, llm.ModeloClasifica, sistemaClasifica, q, 5, 0)
	if err != nil {
		return "conversacion", llm.ModeloClasifica, u, err
	}
	return tipoClasifica(resp), llm.ModeloClasifica, u, nil
}

// tipoClasificaExacto: la respuesta es una de las palabras (si no, no se sabe: decide el otro modelo).
func tipoClasificaExacto(resp string) (string, bool) {
	t := tipoClasifica(resp)
	r := strings.ToLower(strings.Trim(strings.TrimSpace(resp), ".:*`\"' \n"))
	return t, strings.HasPrefix(strings.ReplaceAll(r, "ó", "o"), t)
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
