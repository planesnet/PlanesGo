// Historial del cuadro de texto con las flechas, como en una terminal: ↑ y ↓ recorren los mensajes del usuario de
// la sesión, del más reciente al más antiguo: primero los de la cola (que se editan en su sitio) y después los ya
// enviados (que se cargan tal cual, para mandarlos otra vez). Al volver abajo del todo, o con Esc, vuelve lo que
// se estaba escribiendo. Funciones puras: node --test src/inputHistory.test.ts

export type HistoryItem =
  | { source: 'queue'; queueIndex: number; text: string }
  | { source: 'sent'; text: string };

// HistoryState: dónde está el usuario (pos -1: en su borrador) y el borrador que había al empezar a recorrerlo.
export interface HistoryState { pos: number; draft: string }
export const HISTORY_START: HistoryState = { pos: -1, draft: '' };

// historyItems: la cola (la última, primero) y los enviados (el último, primero), sin vacíos ni repetidos seguidos.
export function historyItems(sent: readonly string[], queued: readonly string[]): HistoryItem[] {
  const items: HistoryItem[] = [];
  for (let i = queued.length - 1; i >= 0; i--) items.push({ source: 'queue', queueIndex: i, text: queued[i] });
  let previous = '';
  for (let i = sent.length - 1; i >= 0; i--) {
    const t = sent[i].trim();
    if (!t || t === previous) continue;
    previous = t;
    items.push({ source: 'sent', text: t });
  }
  return items;
}

// onFirstLine / onLastLine: ↑ solo actúa con el cursor en la primera línea y ↓ en la última (para no estorbar al
// editar un texto de varias líneas).
export const onFirstLine = (value: string, caret: number) => !value.slice(0, caret).includes('\n');
export const onLastLine = (value: string, caret: number) => !value.slice(caret).includes('\n');

// isCaretOnVisualBoundary: comprueba si el cursor está en la primera línea visual (up) o en la última (down) del
// textarea, teniendo en cuenta tanto los saltos de línea (\n) como el ajuste automático de línea (soft wrap).
export function isCaretOnVisualBoundary(el: HTMLTextAreaElement, dir: 'up' | 'down'): boolean {
  const value = el.value;
  const caret = el.selectionStart ?? 0;
  if (dir === 'up') {
    if (!onFirstLine(value, caret)) return false;
    if (caret <= 0) return true;
  } else {
    if (!onLastLine(value, caret)) return false;
    if (caret >= value.length) return true;
  }
  if (typeof document === 'undefined' || typeof window === 'undefined' || !el.clientWidth) return true;
  const style = window.getComputedStyle(el);
  const mirror = document.createElement('div');
  mirror.style.position = 'fixed';
  mirror.style.visibility = 'hidden';
  mirror.style.pointerEvents = 'none';
  mirror.style.top = '-9999px';
  mirror.style.left = '-9999px';
  mirror.style.boxSizing = 'border-box';
  mirror.style.border = '0';
  mirror.style.width = `${el.clientWidth}px`;
  mirror.style.padding = style.padding;
  mirror.style.font = style.font;
  mirror.style.fontFamily = style.fontFamily;
  mirror.style.fontSize = style.fontSize;
  mirror.style.fontWeight = style.fontWeight;
  mirror.style.lineHeight = style.lineHeight;
  mirror.style.letterSpacing = style.letterSpacing;
  mirror.style.whiteSpace = 'pre-wrap';
  mirror.style.overflowWrap = style.overflowWrap || 'anywhere';
  mirror.style.wordBreak = style.wordBreak;

  const startMarker = document.createElement('span');
  startMarker.textContent = '\u200b';
  const caretMarker = document.createElement('span');
  const ch = value[caret];
  if (ch && ch !== '\n') {
    caretMarker.textContent = ch;
  } else {
    caretMarker.textContent = '\u200b';
  }
  const endMarker = document.createElement('span');
  endMarker.textContent = '\u200b';

  mirror.append(
    startMarker,
    document.createTextNode(value.slice(0, caret)),
    caretMarker,
    document.createTextNode(ch && ch !== '\n' ? value.slice(caret + 1) : value.slice(caret)),
    endMarker,
  );
  document.body.appendChild(mirror);
  const startTop = startMarker.offsetTop;
  const caretTop = caretMarker.offsetTop;
  const endTop = endMarker.offsetTop;
  document.body.removeChild(mirror);

  return dir === 'up' ? caretTop <= startTop + 2 : caretTop >= endTop - 2;
}

// moveHistory: el paso con ↑ («up») o ↓ («down») desde «state», con «current» en el cuadro. null: no hay a dónde ir
// (la tecla hace lo de siempre). item: la entrada cargada (sin él, ha vuelto el borrador).
export function moveHistory(items: readonly HistoryItem[], state: HistoryState, dir: 'up' | 'down', current: string): { state: HistoryState; text: string; item?: HistoryItem } | null {
  if (dir === 'up') {
    const pos = state.pos + 1;
    if (pos >= items.length) return null;
    return { state: { pos, draft: state.pos === -1 ? current : state.draft }, text: items[pos].text, item: items[pos] };
  }
  if (state.pos < 0) return null;
  const pos = Math.min(state.pos, items.length) - 1;
  if (pos < 0) return { state: HISTORY_START, text: state.draft };
  return { state: { pos, draft: state.draft }, text: items[pos].text, item: items[pos] };
}

// queueNotice: el aviso mientras se edita uno de la cola.
export const queueNotice = (queueIndex: number) => `Editando el mensaje ${queueIndex + 1} de la cola · Intro lo guarda en su sitio · Esc vuelve a lo que escribías`;
