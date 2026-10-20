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

export default {
  id: "planesgo",
  async setup(ctx: any) {
    const workDir: string =
      ctx?.location?.directory || ctx?.location?.worktree || process.cwd();
    const regKey = Symbol.for("planesgo.opencode.registered_dirs");
    const regDirs: Set<string> = ((globalThis as any)[regKey] ||= new Set<string>());
    if (regDirs.has(workDir)) {
      return;
    }
    regDirs.add(workDir);
    const startedSessions = new Set<string>();

    // 1. Inyección de estado de vinculación PSF en el prompt de sistema de la sesión
    await ctx.session.hook("context", async (input: any) => {
      const sid = String(input?.sessionID || "default");
      const mode = startedSessions.has(sid) ? "prompt" : "session-start";
      startedSessions.add(sid);

      const res = runHook(workDir, mode, { session_id: sid });
      const ctxText = res?.hookSpecificOutput?.additionalContext;
      if (ctxText && Array.isArray(input?.system)) {
        input.system.push(`<planesgo-context>\n${ctxText}\n</planesgo-context>`);
      }
    });

    // 2. Registro de la primera petición del usuario y cancelación de consolidación pendiente
    await ctx.session.hook("prompt", async (input: any) => {
      const sid = String(input?.sessionID || "default");
      const text =
        typeof input?.prompt?.text === "string" ? input.prompt.text : "";
      if (text) {
        runHook(workDir, "prompt", { session_id: sid, prompt: text });
      }
    });

    // 3. Guardián PreToolUse + Inyección de project_path y session_id en MCP planesgo
    await ctx.tool.hook("execute.before", async (call: any) => {
      const sid = String(call?.sessionID || "default");
      const toolName = String(call?.tool || "");
      const args = (call?.input || {}) as Record<string, any>;

      // Herramientas MCP de PlanesGo (en OpenCode se exponen como planesgo_planesgo_*)
      if (
        toolName.startsWith("planesgo_") ||
        toolName.startsWith("mcp__planesgo__")
      ) {
        const res = runHook(workDir, "mcp", {
          session_id: sid,
          tool_name: toolName,
          tool_input: args,
        });
        const updated = res?.hookSpecificOutput?.updatedInput;
        if (updated && call.input) {
          Object.assign(call.input, updated);
        }
        if (call.input && !toolName.endsWith("planesgo_search_projects")) {
          if (!call.input.project_path && workDir) {
            call.input.project_path = workDir;
          }
          if (!call.input.session_id && sid) {
            call.input.session_id = sid;
          }
        }
        return;
      }

      // Herramientas de edición y consola (edit, write, patch, multiedit, shell, bash)
      if (TRACKED_TOOLS.has(toolName)) {
        const res = runHook(workDir, "guard", {
          session_id: sid,
          tool_name: toolName,
          tool_input: args,
        });
        const hso = res?.hookSpecificOutput;
        if (hso?.permissionDecision === "deny") {
          throw new Error(
            hso.permissionDecisionReason ||
              "Bloqueo PSF: El proyecto no está vinculado a Odoo (.planesgo.json)."
          );
        }
        if (hso?.updatedInput && call.input) {
          Object.assign(call.input, hso.updatedInput);
        }
      }
    });

    // 4. Latidos PostToolUse (apertura y actualización de cronómetro en Odoo)
    await ctx.tool.hook("execute.after", async (call: any) => {
      if (call?.status === "error") return;
      const sid = String(call?.sessionID || "default");
      const toolName = String(call?.tool || "");
      if (TRACKED_TOOLS.has(toolName)) {
        runHook(workDir, "track", {
          session_id: sid,
          tool_name: toolName,
          tool_input: call?.input || {},
        });
      }
    });

    // 5. Consolidación al quedar la sesión inactiva (session.idle) o cerrarse
    if (ctx?.event?.subscribe) {
      (async () => {
        try {
          for await (const ev of ctx.event.subscribe()) {
            const evType = String(ev?.type || "");
            const sid = String(
              ev?.properties?.sessionID || ev?.sessionID || "default"
            );
            if (evType === "session.idle") {
              runHook(workDir, "turn-end", { session_id: sid });
            } else if (
              evType === "session.deleted" ||
              evType === "location.shutdown" ||
              evType === "global.disposed"
            ) {
              runHook(workDir, "session-end", { session_id: sid });
            }
          }
        } catch {
          // Cierre normal del stream de eventos
        }
      })();
    }
  },
};
