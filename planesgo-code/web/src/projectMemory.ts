// Memoria del proyecto (internal/memory): lo que muestra su editor en las propiedades de la sesión. Funciones puras
// para poder probarlas: node --test src/projectMemory.test.ts
export interface MemoryView { texto: string; tokens: number; max: number; ruta: string }

// estimateMemoryTokens: lo que ocupará en el contexto del agente (como memory.Tokens: sin comentarios ni el título,
// 3,5 caracteres por token).
export function estimateMemoryTokens(text: string): number {
  const body = text.replace(/<!--[\s\S]*?-->/g, '').replace(/^\s*# .*$/m, '').trim();
  return Math.ceil([...body].length * 2 / 7);
}

// memoryState: la línea bajo el editor, si pasa del tope y si hay cambios sin guardar.
export function memoryState(view: MemoryView | null, text: string): { tokens: number; over: boolean; dirty: boolean; line: string } {
  const max = view?.max || 1500;
  const tokens = estimateMemoryTokens(text);
  const over = tokens > max;
  const dirty = Boolean(view) && text !== view!.texto;
  const line = !text.trim() ? 'Vacía: el agente no recibe nada.'
    : `~${tokens} de ${max} tokens${over ? ': pasa del tope, acórtala' : ''}${dirty ? ' · sin guardar' : ''}`;
  return { tokens, over, dirty, line };
}
