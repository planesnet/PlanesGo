/**
 * Planes Software Factory (PSF) - Plugin nativo de PlanesGo para OpenCode Desktop / CLI (v2.1+)
 *
 * Conecta el ciclo de vida de OpenCode con el guardián y motor de imputación
 * de PlanesGo (~/.claude/hooks/planesgo_claude_hook.py y planesgo-mcp):
 *  - session.hook("context")       -> Verifica vinculación (.planesgo.json) e inyecta contexto PSF
 *  - session.hook("prompt")        -> Registra la petición inicial del turno para el resumen en español
 *  - tool.hook("execute.before")   -> Bloquea ediciones/shell en proyectos no vinculados e inyecta project_path/session_id en MCP
 *  - tool.hook("execute.after")    -> Envía latidos automáticos (beat) al modificar archivos o ejecutar comandos
 *  - event.subscribe (session.idle)-> Programa consolidación automática del parte en Odoo tras inactividad
 */
import { spawnSync } from "node:child_process";
import { existsSync } from "node:fs";
import { homedir } from "node:os";
import { join } from "node:path";

const HOME = homedir();
const GLOBAL_HOOK = join(HOME, ".claude", "hooks", "planesgo_claude_hook.py");

const TRACKED_TOOLS = new Set([
  "edit",
  "write",
  "patch",
  "multiedit",
  "notebookedit",
  "bash",
  "shell",
  "Edit",
  "Write",
  "MultiEdit",
  "NotebookEdit",
  "Bash",
]);

interface HookOutput {
  hookSpecificOutput?: {
    hookEventName?: string;
    additionalContext?: string;
    permissionDecision?: "allow" | "deny";
    permissionDecisionReason?: string;
    updatedInput?: Record<string, any>;
  };
}

function findHookScript(workDir: string): string | null {
  if (existsSync(GLOBAL_HOOK)) return GLOBAL_HOOK;
  const localHook = join(workDir, ".claude", "hooks", "planesgo_claude_hook.py");
  if (existsSync(localHook)) return localHook;
  return null;
}

function runHook(
  workDir: string,
  mode: string,
  payload: Record<string, any>
): HookOutput | null {
  const hookPath = findHookScript(workDir);
  if (!hookPath) return null;

  try {
    const res = spawnSync("python3", [hookPath, mode], {
      input: JSON.stringify({ cwd: workDir, ...payload }),
      encoding: "utf-8",
      timeout: 15000,
      env: {
        ...process.env,
        PLANESGO_AI_MODEL: "OpenCode",
      },
    });
    if (res.status === 0 && res.stdout && res.stdout.trim()) {
      return JSON.parse(res.stdout.trim()) as HookOutput;
    }
  } catch {
    // Contrato de no interferencia ante errores inesperados
  }
  return null;
}

import { readFileSync } from "node:fs";

// Interruptor «PlanesGo en los agentes»: si ~/.planesgo/gestor dice «pgocode», la imputación la lleva pgocode y
// el plugin no hace nada (ni lanza el hook). Se lee en cada evento: vale para las sesiones ya abiertas.
function gestorPgocode(): boolean {
  if (process.env.PLANESGO_GESTOR === "pgocode") return true; // agente lanzado por pgocode: siempre
  try {
    return readFileSync(join(HOME, ".planesgo", "gestor"), "utf-8").trim() === "pgocode";
  } catch {
    return false;
  }
}

// Formato de plugin de OpenCode 1.x: una función que devuelve los hooks (no un objeto {id, setup}).
export const PlanesGo = async ({ directory, worktree }: { directory?: string; worktree?: string }) => {
  const workDir: string = directory || worktree || process.cwd();
  const startedSessions = new Set<string>();

  return {
    // 1. Estado de vinculación PSF en el prompt de sistema de la sesión
    "experimental.chat.system.transform": async (input: any, output: any) => {
      if (gestorPgocode()) return;
      const sid = String(input?.sessionID || "default");
      const mode = startedSessions.has(sid) ? "prompt" : "session-start";
      startedSessions.add(sid);
      const ctxText = runHook(workDir, mode, { session_id: sid })?.hookSpecificOutput?.additionalContext;
      if (ctxText && Array.isArray(output?.system)) {
        output.system.push(`<planesgo-context>\n${ctxText}\n</planesgo-context>`);
      }
    },

    // 2. Registro de la petición del usuario
    "chat.message": async (input: any, output: any) => {
      if (gestorPgocode()) return;
      const sid = String(input?.sessionID || "default");
      const text = (output?.parts || [])
        .filter((p: any) => p?.type === "text" && typeof p.text === "string")
        .map((p: any) => p.text)
        .join("\n");
      if (text) runHook(workDir, "prompt", { session_id: sid, prompt: text });
    },

    // 3. Guardián antes de cada herramienta e inyección de project_path/session_id en el MCP planesgo
    "tool.execute.before": async (input: any, output: any) => {
      if (gestorPgocode()) return;
      const sid = String(input?.sessionID || "default");
      const toolName = String(input?.tool || "");
      const args = (output?.args || {}) as Record<string, any>;
      if (toolName.startsWith("pgo_") || toolName.startsWith("planesgo_") || toolName.startsWith("mcp__pgo__") || toolName.startsWith("mcp__planesgo__")) {
        const updated = runHook(workDir, "mcp", { session_id: sid, tool_name: toolName, tool_input: args })?.hookSpecificOutput?.updatedInput;
        if (updated) Object.assign(args, updated);
        // Buscar proyecto (pgo_project sin project_id) no lleva ruta ni sesión.
        if (!toolName.endsWith("planesgo_search_projects") && !(toolName.endsWith("pgo_project") && !args.project_id)) {
          if (!args.project_path && workDir) args.project_path = workDir;
          if (!args.session_id && sid) args.session_id = sid;
        }
        output.args = args;
        return;
      }
      if (TRACKED_TOOLS.has(toolName)) {
        const hso = runHook(workDir, "guard", { session_id: sid, tool_name: toolName, tool_input: args })?.hookSpecificOutput;
        if (hso?.permissionDecision === "deny") {
          throw new Error(hso.permissionDecisionReason || "Bloqueo PSF: El proyecto no está vinculado a Odoo (.planesgo.json).");
        }
        if (hso?.updatedInput) Object.assign(args, hso.updatedInput);
        output.args = args;
      }
    },

    // 4. Latidos tras editar o ejecutar
    "tool.execute.after": async (input: any, output: any) => {
      if (gestorPgocode()) return;
      const toolName = String(input?.tool || "");
      if (TRACKED_TOOLS.has(toolName)) {
        runHook(workDir, "track", { session_id: String(input?.sessionID || "default"), tool_name: toolName, tool_input: output?.metadata?.input || {} });
      }
    },

    // 5. Consolidación del parte al quedar la sesión inactiva o cerrarse
    event: async ({ event }: { event: any }) => {
      if (gestorPgocode()) return;
      const t = String(event?.type || "");
      const sid = String(event?.properties?.sessionID || "default");
      if (t === "session.idle") runHook(workDir, "turn-end", { session_id: sid });
      else if (t === "session.deleted") runHook(workDir, "session-end", { session_id: sid });
    },
  };
};
