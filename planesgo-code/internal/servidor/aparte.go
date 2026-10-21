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
	"Responde breve, claro y en español. No tienes herramientas: si no lo sabes con lo que tienes, dilo."

// bifurcables: los agentes que pueden copiar su sesión para una pregunta aparte.
var bifurcables = map[string]bool{"claude-code": true, "opencode": true}

func (s *Servidor) aparte(r *http.Request, tipo, contexto string, prov llm.Proveedor, info llm.Info, pet llm.Peticion, sesionImp, proveedor string, envia func(any)) {
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
		c := map[string]any{"tipo": tipo, "via": via, "proveedor": proveedor, "turno_s": time.Since(inicio).Seconds()}
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
	}
	if err := lee(r, &p); err != nil || strings.TrimSpace(p.Texto) == "" {
		jsonError(w, http.StatusBadRequest, "petición no válida")
		return
	}
	tipo, u, err := clasificaMensaje(r.Context(), p.Texto, p.Ultimo)
	if s.Imputacion != nil && p.Sesion != "" {
		c := map[string]any{"tipo": tipo, "entrada": u.Entrada, "salida": u.Salida, "coste": u.Coste}
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

var tiposClasifica = []string{"conversacion", "contexto", "suelta"}

var clasificaGemini = llm.Gemini

func clasificaMensaje(ctx context.Context, texto, ultimo string) (string, llm.Uso, error) {
	q := "Mensaje del usuario:\n" + recortaTexto(texto, 1500)
	if strings.TrimSpace(ultimo) != "" {
		q = "Lo último que dijo el agente:\n" + recortaTexto(ultimo, 600) + "\n\n" + q
	}
	resp, u, err := clasificaGemini(ctx, llm.ModeloClasifica, sistemaClasifica, q, 5, 0)
	if err != nil {
		return "conversacion", u, err
	}
	return tipoClasifica(resp), u, nil
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
