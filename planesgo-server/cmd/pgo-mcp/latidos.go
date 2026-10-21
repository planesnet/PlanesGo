package main

// Imputación automática por línea de órdenes (--check, --beat, --stop), la que usan los hooks de los agentes.

import (
	"fmt"
	"strings"
)

// ToolContent y ToolCallResult son el resultado de una herramienta MCP (y de una orden de la línea de órdenes).
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError"`
}

func resultado(texto string) ToolCallResult {
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: texto}}}
}

func resultadoError(texto string) ToolCallResult {
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: texto}}, IsError: true}
}

const textoNoVinculado = "❌ Proyecto no vinculado (.planesgo.json requerido)"

// contexto es lo que necesita una imputación: cliente, configuración y proyecto resuelto.
type contexto struct {
	cliente  *PlanesGoClient
	cfg      *Config
	sesion   string
	ruta     string
	projID   int
	projName string
}

// resuelveContexto obtiene el proyecto de la sesión (o de .planesgo.json), con prioridad para un
// project_id explícito. Si resolverNombre, pide a PlanesGo el nombre canónico del project_id explícito.
func resuelveContexto(args map[string]interface{}, resolverNombre bool) (*contexto, *ToolCallResult) {
	ctx := &contexto{sesion: resolveSessionID(args)}
	for _, clave := range []string{"project_path", "path", "cwd"} {
		if p := argTexto(args, clave); p != "" {
			ctx.ruta = p
			break
		}
	}
	idExplicito := argEntero(args, "project_id")

	cliente, cfg, err := newClientWithSession(ctx.sesion, ctx.ruta)
	if err != nil {
		// Con un project_id explícito basta con tener ruta y token, aunque aún no exista .planesgo.json
		if err == errMissingProjectPath || cliente == nil || idExplicito <= 0 {
			r := resultadoError(fmt.Sprintf("❌ Proyecto no vinculado (%v)", err))
			return nil, &r
		}
	}
	ctx.cliente, ctx.cfg = cliente, cfg
	if cfg != nil {
		ctx.projID, ctx.projName = cfg.OdooProjectID, cfg.OdooProjectName
	}

	if idExplicito > 0 {
		if ctx.projID != idExplicito {
			// Otro proyecto: no se arrastran nombre, tarea ni ticket del vinculado
			ctx.projName = ""
			if cfg != nil {
				cfg.OdooTaskID, cfg.OdooTicketID, cfg.OdooTicketRef = 0, 0, ""
			}
		}
		ctx.projID = idExplicito
		if resolverNombre && ctx.projName == "" {
			if res, cErr := cliente.CheckStatus(ctx.projID, ""); cErr == nil {
				if pName, ok := res["project_name"].(string); ok && pName != "" {
					ctx.projName = pName
				}
			}
		}
	}
	return ctx, nil
}

// tareaVinculada devuelve el task_id de los argumentos o, si no viene, el de la configuración.
func (ctx *contexto) tareaVinculada(args map[string]interface{}) int {
	if id := argEntero(args, "task_id"); id > 0 {
		return id
	}
	if ctx.cfg != nil {
		return ctx.cfg.OdooTaskID
	}
	return 0
}

// ticketVinculado es el ticket de un .planesgo.json antiguo (ya no se pueden vincular tickets nuevos).
func (ctx *contexto) ticketVinculado() string {
	if ctx.cfg != nil {
		return ctx.cfg.OdooTicketRef
	}
	return ""
}

func (ctx *contexto) textoProyecto() string {
	switch {
	case ctx.projID > 0 && ctx.projName != "":
		return fmt.Sprintf("%s (ID: %d)", ctx.projName, ctx.projID)
	case ctx.projID > 0:
		return fmt.Sprintf("ID: %d", ctx.projID)
	case ctx.projName != "":
		return ctx.projName
	}
	return "No detectado"
}

// telemetria son los tokens y el modelo de IA de un latido o cierre.
type telemetria struct {
	total, entrada, salida int
	modelo                 string
	coste                  float64
	sesionIA               string
}

func leeTelemetria(args map[string]interface{}, sesion string) telemetria {
	t := telemetria{
		total:    argEntero(args, "tokens_total"),
		entrada:  argEntero(args, "tokens_input"),
		salida:   argEntero(args, "tokens_output"),
		modelo:   argTexto(args, "ai_model"),
		coste:    argNumero(args, "ai_cost"),
		sesionIA: sesion,
	}
	if _, ok := args["tokens_total"]; !ok {
		t.total = argEntero(args, "tokens")
	}
	if t.modelo == "" {
		t.modelo = argTexto(args, "model")
	}
	if s := argTexto(args, "ai_session_id"); s != "" {
		t.sesionIA = s
	}
	if t.total == 0 && (t.entrada > 0 || t.salida > 0) {
		t.total = t.entrada + t.salida
	}
	return t
}

func (t telemetria) texto(unidad string) string {
	if t.total <= 0 {
		return ""
	}
	if t.modelo != "" {
		return fmt.Sprintf(" | 🪙 %s %s (%s)", formatTokens(t.total), unidad, t.modelo)
	}
	return fmt.Sprintf(" | 🪙 %s %s", formatTokens(t.total), unidad)
}

func textoTicket(ticket string) string {
	if ticket == "" {
		return ""
	}
	return " | Ticket: " + ticket
}

// ordenCheck comprueba el token y el proyecto vinculado, y dice sobre qué tarea se imputará (--check).
func ordenCheck(args map[string]interface{}) ToolCallResult {
	ctx, fallo := resuelveContexto(args, false)
	if fallo != nil {
		return *fallo
	}
	if ctx.projID <= 0 && ctx.projName == "" {
		return resultadoError(textoNoVinculado)
	}

	res, err := ctx.cliente.CheckStatus(ctx.projID, ctx.projName)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al verificar proyecto: %v", err))
	}
	if pName, _ := res["project_name"].(string); pName != "" {
		ctx.projName = pName
	}
	if idVal, ok := res["project_id"].(float64); ok && int(idVal) > 0 {
		ctx.projID = int(idVal)
	}

	// Sin tarea indicada, la del cronómetro activo en PlanesGo
	taskName := argTexto(args, "task_name")
	if taskName == "" {
		if timers, ok := res["active_timers"].([]interface{}); ok && len(timers) > 0 {
			if tMap, ok := timers[0].(map[string]interface{}); ok {
				taskName, _ = tMap["task_name"].(string)
				if ctx.projName == "" {
					ctx.projName, _ = tMap["project_name"].(string)
				}
				if ctx.projID == 0 {
					if timerPID, ok := tMap["project_id"].(float64); ok {
						ctx.projID = int(timerPID)
					}
				}
			}
		}
	}
	if taskName != "" && taskName != "Pendiente de asignar en latido" {
		_, taskName = NormalizeTaskType(taskName, argTexto(args, "description"), argTexto(args, "task_type"))
	}

	return resultado(fmt.Sprintf("✅ %s%s | Tarea: %s", ctx.textoProyecto(), textoTicket(ctx.ticketVinculado()), taskName))
}

// ordenBeat envía un latido de la tarea indicada (--beat).
func ordenBeat(args map[string]interface{}) ToolCallResult {
	ctx, fallo := resuelveContexto(args, true)
	if fallo != nil {
		return *fallo
	}
	if ctx.projID <= 0 {
		return resultadoError(textoNoVinculado)
	}
	taskName := argTexto(args, "task_name")
	if taskName == "" {
		return resultadoError("❌ Campo 'task_name' requerido")
	}
	desc, _ := args["description"].(string)
	tel := leeTelemetria(args, ctx.sesion)
	ticket := ctx.ticketVinculado()

	canonicalType, normalizedTaskName := NormalizeTaskType(taskName, desc, argTexto(args, "task_type"))
	_, err := ctx.cliente.SendTaskActionWithTokens("heartbeat", normalizedTaskName, ctx.tareaVinculada(args), ctx.projID, ctx.projName, desc, canonicalType, ticket, tel.total, tel.entrada, tel.salida, tel.modelo, tel.coste, tel.sesionIA)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al registrar latido: %v", err))
	}
	return resultado(fmt.Sprintf("⏱️ %s%s | %s%s", ctx.textoProyecto(), textoTicket(ticket), normalizedTaskName, tel.texto("tok")))
}

// ordenStop cierra la tarea y consolida la imputación (--stop).
func ordenStop(args map[string]interface{}) ToolCallResult {
	ctx, fallo := resuelveContexto(args, true)
	if fallo != nil {
		return *fallo
	}
	if ctx.projID <= 0 {
		return resultadoError(textoNoVinculado)
	}
	taskName, _ := args["task_name"].(string)
	desc, _ := args["description"].(string)
	tel := leeTelemetria(args, ctx.sesion)
	ticket := ctx.ticketVinculado()

	// Sin tokens explícitos se estiman de la sesión activa. El estimador lee transcripts de
	// Antigravity: no aplica a sesiones de Claude Code.
	if tel.total <= 0 && !strings.Contains(strings.ToLower(tel.modelo), "claude") {
		if estTotal, estIn, estOut, estModel := estimateTokensFromActiveSession(); estTotal > 0 {
			tel.total, tel.entrada, tel.salida = estTotal, estIn, estOut
			if tel.modelo == "" {
				tel.modelo = estModel
			}
		}
	}

	canonicalType, normalizedTaskName := NormalizeTaskType(taskName, desc, argTexto(args, "task_type"))
	finalDesc := strings.TrimSpace(desc)
	if finalDesc == "" || finalDesc == "Trabajo en curso" {
		finalDesc = cleanAntigravityTaskName(taskName)
	}

	_, err := ctx.cliente.SendTaskActionWithTokens("stop", normalizedTaskName, ctx.tareaVinculada(args), ctx.projID, ctx.projName, finalDesc, canonicalType, ticket, tel.total, tel.entrada, tel.salida, tel.modelo, tel.coste, tel.sesionIA)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al consolidar imputación: %v", err))
	}
	return resultado(fmt.Sprintf("⏹️ Imputado | %s%s | %s: %s%s", ctx.textoProyecto(), textoTicket(ticket), normalizedTaskName, finalDesc, tel.texto("tokens")))
}
