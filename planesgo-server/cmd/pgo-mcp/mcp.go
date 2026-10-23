package main

// Servidor MCP sobre stdio (JSON-RPC por líneas), modo dormido con pgocode y aviso de cambio de herramientas.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// mcpInstructions se envía al cliente MCP al inicializar.
const mcpInstructions = "La imputación de tiempo en Odoo es automática: usa pgo_project o pgo_time solo si el usuario lo pide. " +
	"Todo texto que se envíe a PlanesGo (tareas y descripciones) va en español."

func runMCPServer() {
	go vigilaGestor()
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var req JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if resp := atiendePeticion(req); resp != nil {
			sendResponse(*resp)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "[pgo-mcp] Error leyendo stdin: %v\n", err)
	}
}

// atiendePeticion responde a una petición JSON-RPC (nil si es una notificación sin respuesta).
func atiendePeticion(req JSONRPCRequest) *JSONRPCResponse {
	resp := &JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{"listChanged": true},
			},
			"serverInfo": map[string]interface{}{
				"name":    "pgo-mcp",
				"version": Version,
			},
			"instructions": instruccionesMCP(),
		}

	case "notifications/initialized":
		return nil

	case "ping":
		resp.Result = map[string]interface{}{}

	case "tools/list":
		// Con pgocode al mando, el agente no ve ninguna herramienta (ni paga sus descripciones).
		herramientas := getToolsDefinition()
		if gestorPgocode() {
			herramientas = []map[string]interface{}{}
		}
		resp.Result = map[string]interface{}{"tools": herramientas}

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: -32602, Message: fmt.Sprintf("Parámetros inválidos para tools/call: %v", err)}
			return resp
		}
		if gestorPgocode() {
			resp.Result = resultado(textoGestorPgocode)
		} else {
			resp.Result = executeToolCall(params.Name, params.Arguments)
		}

	default:
		if req.ID == nil {
			return nil
		}
		resp.Error = &RPCError{Code: -32601, Message: fmt.Sprintf("Método no implementado: %s", req.Method)}
	}
	return resp
}

var muSalida sync.Mutex

func sendResponse(resp JSONRPCResponse) {
	data, err := json.Marshal(resp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[pgo-mcp] Error serializando respuesta: %v\n", err)
		return
	}
	escribeMensaje(data)
}

// escribeMensaje escribe una línea JSON-RPC en la salida (respuestas y avisos pueden coincidir: con cerrojo).
func escribeMensaje(data []byte) {
	muSalida.Lock()
	defer muSalida.Unlock()
	os.Stdout.Write(append(data, '\n'))
}

// Interruptor «PlanesGo en los agentes»: si ~/.planesgo/gestor dice «pgocode», la imputación la lleva pgocode y
// este servidor no ofrece herramientas a los agentes. Se lee en cada petición: vale para las sesiones ya abiertas.
const textoGestorPgocode = "La imputación de tiempo la gestiona pgocode: no hace falta vincular el proyecto ni enviar latidos. Sigue con la tarea."

func ficheroGestor() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".planesgo", "gestor")
}

func gestorPgocode() bool {
	if dentroDePgocode() { // agente lanzado por pgocode (o descendiente): siempre, aunque cambie su entorno
		return true
	}
	b, err := os.ReadFile(ficheroGestor())
	return err == nil && strings.TrimSpace(string(b)) == "pgocode"
}

func instruccionesMCP() string {
	if gestorPgocode() {
		return textoGestorPgocode
	}
	return mcpInstructions
}

// vigilaGestor avisa al agente (notifications/tools/list_changed) cuando el interruptor cambia, para que las
// sesiones abiertas dejen de ver (o vuelvan a ver) las herramientas sin reiniciarse.
func vigilaGestor() {
	antes := gestorPgocode()
	for range time.Tick(3 * time.Second) {
		if ahora := gestorPgocode(); ahora != antes {
			antes = ahora
			escribeMensaje([]byte(`{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}`))
		}
	}
}
