// Ventana de contexto y precio de los modelos de OpenCode que ofrece pgocode (los del catálogo de OpenCode,
// models.dev), para elegir con conocimiento de causa: el texto de cada opción del selector de modelo.
interface ModelInfo { window: number; input?: number; output?: number } // precios en $ por millón de tokens

const MODELS: Record<string, ModelInfo> = {
  'google-vertex/gemini-4-argon': { window: 1_048_576, input: 2, output: 10 },
  'google-vertex/gemini-3.8-flash': { window: 1_048_576, input: 0.75, output: 3.75 },
  'google-vertex/gemini-3.1-flash-lite': { window: 1_048_576, input: 0.25, output: 1.5 },
  'vertex-deepseek/deepseek-ai/deepseek-v3.2-maas': { window: 163_840 },
};

const num = (x: number) => String(x).replace('.', ',');
const win = (n: number) => n >= 1_000_000 ? Math.round(n / 1_048_576) + ' M' : Math.round(n / 1000) + ' k';

// modelLabel: «gemini-3.8-flash · 1 M · 0,75/3,75 $» (entrada/salida por millón); el id tal cual si no se conoce.
export function modelLabel(id: string): string {
  const i = MODELS[id];
  if (!i) return id;
  const precio = i.input !== undefined && i.output !== undefined ? ` · ${num(i.input)}/${num(i.output)} $` : '';
  return `${id} · ${win(i.window)}${precio}`;
}

// modelInputPrice: el precio de entrada ($ por millón) de un modelo de OpenCode, si se conoce.
export function modelInputPrice(id: string): number | undefined {
  return MODELS[id]?.input;
}

// MODEL_LABEL_HELP: la explicación del texto de modelLabel (para el title del selector).
export const MODEL_LABEL_HELP = 'Ventana de contexto y precio por millón de tokens (entrada/salida)';
