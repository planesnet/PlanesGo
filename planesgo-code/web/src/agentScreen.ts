// The person's view of the agent's desktop (session properties → Escritorio del agente → «Ver y usar la pantalla»;
// internal/agentdesktop/person.go): where a click falls on the remote screen and which key each key press is.

// screenPoint: a point of the image as shown (shownW x shownH) on the screen (w x h).
export function screenPoint(x: number, y: number, shownW: number, shownH: number, w: number, h: number): [number, number] {
  const sx = Math.min(w - 1, Math.max(0, Math.round((x * w) / Math.max(1, shownW))));
  const sy = Math.min(h - 1, Math.max(0, Math.round((y * h) / Math.max(1, shownH))));
  return [sx, sy];
}

export interface KeyLike { key: string; ctrlKey: boolean; altKey: boolean; metaKey: boolean; shiftKey: boolean }

const NAMES: Record<string, string> = {
  Enter: 'Return', Backspace: 'BackSpace', Tab: 'Tab', Escape: 'Escape', Delete: 'Delete', Insert: 'Insert',
  ArrowLeft: 'Left', ArrowRight: 'Right', ArrowUp: 'Up', ArrowDown: 'Down', Home: 'Home', End: 'End',
  PageUp: 'Prior', PageDown: 'Next', ' ': 'space',
};

// keyAction: what a key press sends: text (a character, typed as it is: accents, ñ, €) or a key for xdotool (with
// ctrl/alt/super, or a special one); null for a modifier pressed alone.
export function keyAction(e: KeyLike): { text: string } | { key: string } | null {
  if (['Shift', 'Control', 'Alt', 'Meta', 'AltGraph', 'CapsLock'].includes(e.key)) return null;
  const mods = [e.ctrlKey && 'ctrl', e.altKey && 'alt', e.metaKey && 'super'].filter(Boolean) as string[];
  if (e.key.length === 1 && !mods.length) return { text: e.key };
  let name = NAMES[e.key] || (/^F([1-9]|1[0-2])$/.test(e.key) ? e.key : '');
  if (!name && e.key.length === 1 && /^[a-z0-9]$/i.test(e.key)) name = e.key.toLowerCase();
  if (!name) return null;
  // Shift goes with a combination or a special key (shift+Tab); with a character alone it is already in the character.
  if (e.shiftKey && (mods.length || (name.length > 1 && name !== 'space'))) mods.push('shift');
  return { key: [...mods, name].join('+') };
}
