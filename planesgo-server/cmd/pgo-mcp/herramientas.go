package main

// Las dos herramientas MCP (pgo_project y pgo_time) y la respuesta a los nombres antiguos.

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// herramientasAntiguas ya no se ofrecen; si un agente las llama, se le redirige sin error.
var herramientasAntiguas = map[string]bool{
	"planesgo_check": true, "planesgo_beat": true, "planesgo_stop": true, "planesgo_status": true,
	"planesgo_list_tasks": true, "planesgo_close_ticket": true, "planesgo_add_hours": true,
	"planesgo_search_projects": true, "planesgo_set_project": true, "planesgo_studio_emisor": true,
	"planesgo_url": true,
}

func textoHerramientaAntigua(nombre string) string {
	return fmt.Sprintf("La herramienta «%s» ya no existe: la imputación de tiempo es automática. "+
		"Si el usuario lo pide, usa pgo_project (buscar o vincular el proyecto) o pgo_time (añadir minutos).", nombre)
}

const descRuta = "Raíz del proyecto de la sesión"

func getToolsDefinition() []map[string]interface{} {
	texto := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	return []map[string]interface{}{
		{
			"name":        "pgo_project",
			"description": "Proyecto de Odoo de la sesión. Con query busca y lista candidatos sin vincular: pregunta al usuario cuál es y vuelve a llamar con project_id, que lo vincula.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query":        texto("Texto a buscar"),
					"project_id":   map[string]interface{}{"type": "integer", "description": "ID del proyecto a vincular"},
					"project_path": texto(descRuta),
				},
			},
		},
		{
			"name":        "pgo_time",
			"description": "Añade minutos al proyecto vinculado, en la tarea indicada (se crea si no existe) o en la de por defecto.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"minutes":      map[string]interface{}{"type": "number", "exclusiveMinimum": 0, "description": "Minutos"},
					"task":         texto("Tarea"),
					"description":  texto("Trabajo hecho, en español"),
					"project_path": texto(descRuta),
				},
				"required": []string{"minutes"},
			},
		},
	}
}

// executeToolCall ejecuta una herramienta MCP.
func executeToolCall(name string, args map[string]interface{}) ToolCallResult {
	if args == nil {
		args = map[string]interface{}{}
	}
	switch {
	case name == "pgo_project":
		if argEntero(args, "project_id") > 0 {
			return vinculaProyecto(args)
		}
		if argTexto(args, "query") == "" {
			return resultadoError("❌ Indica query (para buscar) o project_id (para vincular).")
		}
		res, n := buscaProyectos(args)
		if n > 0 {
			res.Content[0].Text += "\n\nNo se ha vinculado nada. Pregunta al usuario cuál es el proyecto definitivo " +
				"y vuelve a llamar a pgo_project con su project_id."
		}
		return res
	case name == "pgo_time":
		return anadeTiempo(args)
	case herramientasAntiguas[name]:
		return resultado(textoHerramientaAntigua(name))
	}
	return resultadoError(fmt.Sprintf("Herramienta '%s' no reconocida", name))
}

// clienteSinProyecto da un cliente aunque la sesión aún no tenga proyecto (para buscar o vincular).
func clienteSinProyecto(args map[string]interface{}) (*PlanesGoClient, string, string, error) {
	sesion := resolveSessionID(args)
	ruta := argTexto(args, "project_path")
	if ruta == "" {
		ruta = argTexto(args, "path")
	}
	cliente, _, err := newClientWithSession(sesion, ruta)
	if cliente == nil {
		return nil, sesion, ruta, err
	}
	return cliente, sesion, ruta, nil
}

// buscaProyectos lista los proyectos que coinciden con query y dice cuántos son. Nunca vincula,
// aunque solo haya uno: el usuario tiene que confirmar cuál es.
func buscaProyectos(args map[string]interface{}) (ToolCallResult, int) {
	query := argTexto(args, "query")
	cliente, _, _, err := clienteSinProyecto(args)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error de autenticación: %v", err)), 0
	}
	proyectos, err := cliente.SearchProjects(query)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al buscar proyectos: %v", err)), 0
	}
	if len(proyectos) == 0 {
		return resultado(fmt.Sprintf("📋 Sin proyectos que coincidan con '%s'", query)), 0
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 Proyectos que coinciden con '%s':", query)
	for i, p := range proyectos {
		id, _ := p["id"].(float64)
		nombre, _ := p["name"].(string)
		fmt.Fprintf(&sb, "\n%d. [%d] %s", i+1, int(id), nombre)
		if c := clienteDeProyecto(p); c != "" {
			fmt.Fprintf(&sb, " — %s", c)
		}
	}
	return resultado(sb.String()), len(proyectos)
}

// clienteDeProyecto saca el cliente del resultado de búsqueda si el servidor lo envía
// (partner_name, client, o partner_id como [id, nombre] de Odoo o {"name": …}).
func clienteDeProyecto(p map[string]interface{}) string {
	for _, k := range []string{"partner_name", "client", "cliente"} {
		if s, ok := p[k].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	switch v := p["partner_id"].(type) {
	case []interface{}:
		if len(v) > 1 {
			if s, ok := v[1].(string); ok {
				return s
			}
		}
	case map[string]interface{}:
		if s, ok := v["name"].(string); ok {
			return s
		}
	}
	return ""
}

// vinculaProyecto vincula project_id a la sesión (si hay session_id) o al .planesgo.json de project_path.
func vinculaProyecto(args map[string]interface{}) ToolCallResult {
	idPedido := argEntero(args, "project_id")
	cliente, sesion, ruta, err := clienteSinProyecto(args)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error de autenticación: %v", err))
	}

	res, err := cliente.CheckStatus(idPedido, "")
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al validar el proyecto %d en Odoo: %v", idPedido, err))
	}
	nombre, _ := res["project_name"].(string)
	id := idPedido
	if idVal, ok := res["project_id"].(float64); ok && int(idVal) > 0 {
		id = int(idVal)
	}

	destino := ruta
	if destino == "" {
		cwd := sessionCwd()
		if cwd == "" || isBackgroundDir(cwd) {
			return resultadoError("❌ " + errMissingProjectPath.Error())
		}
		destino = cwd
	}
	destino, _ = filepath.Abs(destino)
	if home, _ := os.UserHomeDir(); destino == "/" || (home != "" && destino == filepath.Clean(home)) {
		return resultadoError("❌ No se puede vincular un proyecto en la raíz del usuario (~): indica la ruta del proyecto.")
	}

	nueva := Config{OdooProjectID: id, OdooProjectName: nombre, PlanesGoURL: cliente.BaseURL}
	fichero := filepath.Join(destino, ".planesgo.json")

	// Multisesión: con session_id, la vinculación va al fichero de la sesión para no pisar a otras
	// sesiones del mismo repositorio; .planesgo.json solo se escribe si el repositorio aún no tenía proyecto.
	if sesion != "" {
		if err := saveSessionConfig(sesion, destino, nueva); err != nil {
			return resultadoError(fmt.Sprintf("❌ Error al guardar configuración de sesión (%s): %v", sesion, err))
		}
		if datos, rErr := os.ReadFile(fichero); rErr != nil || !tieneProyecto(datos) {
			_ = saveConfigFile(nueva, destino)
		}
		return resultado(fmt.Sprintf("✅ Proyecto vinculado: %s (ID: %d), en esta sesión (%s)", nombre, id, sesion))
	}

	if err := saveConfigFile(nueva, destino); err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al guardar %s: %v", fichero, err))
	}
	return resultado(fmt.Sprintf("✅ Proyecto vinculado: %s (ID: %d)", nombre, id))
}

func tieneProyecto(datos []byte) bool {
	var cfg Config
	return json.Unmarshal(datos, &cfg) == nil && cfg.OdooProjectID > 0
}

// anadeTiempo suma minutos al proyecto vinculado: un latido con elapsed_seconds y un stop inmediato
// (POST /antigravity/update_tasks). Si la tarea no existe en el proyecto, la crea el servidor.
func anadeTiempo(args map[string]interface{}) ToolCallResult {
	minutos := argNumero(args, "minutes")
	if minutos <= 0 {
		return resultadoError("❌ 'minutes' es obligatorio y debe ser mayor que 0")
	}
	if minutos > 24*60 {
		return resultadoError("❌ 'minutes' no puede superar 1440 (24 h) en una sola llamada")
	}

	ctx, fallo := resuelveContexto(args, true)
	if fallo != nil {
		return *fallo
	}
	if ctx.projID <= 0 {
		return resultadoError("❌ Proyecto no vinculado: vincúlalo antes con pgo_project.")
	}

	tarea := argTexto(args, "task")
	tipo := argTexto(args, "task_type")
	if tarea == "" {
		tarea = TASK_NAME_DEFAULT
		if tipo == "" {
			tipo = TaskTypeDesarrollo
		}
	}
	textoMin := strconv.FormatFloat(minutos, 'f', -1, 64)
	desc := argTexto(args, "description")
	if desc == "" {
		desc = fmt.Sprintf("Tiempo añadido a mano (+%s min)", textoMin)
	}
	modelo := argTexto(args, "ai_model")
	if modelo == "" {
		modelo = "Claude Code"
	}
	tareaID := ctx.tareaVinculada(args)

	tipoCanonico, nombreTarea := NormalizeTaskType(tarea, desc, tipo)
	segundos := int(math.Round(minutos * 60))

	res, err := ctx.cliente.SendHeartbeatWithElapsed("heartbeat", nombreTarea, tareaID, ctx.projID, ctx.projName, desc, tipoCanonico, segundos, modelo)
	if err != nil {
		return resultadoError(fmt.Sprintf("❌ Error al añadir tiempo: %v", err))
	}
	// El servidor devuelve el cronómetro con la tarea y el proyecto que ha usado (o creado)
	proyecto, tareaUsada := ctx.projName, nombreTarea
	if timer, ok := res["timer"].(map[string]interface{}); ok {
		if s, _ := timer["task_name"].(string); s != "" {
			tareaUsada = s
		}
		if s, _ := timer["project_name"].(string); s != "" {
			proyecto = s
		}
	}
	if proyecto == "" {
		proyecto = fmt.Sprintf("proyecto %d", ctx.projID)
	}

	// Se cierra en el acto para dejar el registro con esos minutos, sin un cronómetro que siga sumando.
	aviso := ""
	if _, err := ctx.cliente.SendHeartbeatWithElapsed("stop", nombreTarea, tareaID, ctx.projID, ctx.projName, desc, tipoCanonico, 0, modelo); err != nil {
		aviso = fmt.Sprintf(" (aviso: no se pudo cerrar el cronómetro: %v)", err)
	}
	return resultado(fmt.Sprintf("Añadidos %s min a %s · %s%s", textoMin, proyecto, tareaUsada, aviso))
}
