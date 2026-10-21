// Barra de prueba del chat: lanzar lo que se está desarrollando (una web en el navegador interno o en el del
// sistema, con el formato de dispositivo, o un programa del proyecto). Funciones puras para poder probarlas:
// node --test src/lanzador.test.ts

export type Destino = 'interno' | 'sistema' | 'programa';
export type Dispositivo = 'ordenador' | 'tablet' | 'movil';
export type Orientacion = 'horizontal' | 'vertical';

export interface Prueba {
  destino: Destino;
  url: string;
  programa?: string; // nombre del programa del proyecto (destino «programa»)
  dispositivo: Dispositivo;
  orientacion: Orientacion;
}

export const PRUEBA_DEFECTO: Prueba = { destino: 'interno', url: '', dispositivo: 'ordenador', orientacion: 'horizontal' };

// Tamaños CSS habituales (vertical); el ordenador no se emula: la página ocupa el panel.
const TAMANOS: Record<Exclude<Dispositivo, 'ordenador'>, { ancho: number; alto: number }> = {
  tablet: { ancho: 820, alto: 1180 },
  movil: { ancho: 390, alto: 844 },
};

// tamano: el tamaño de la página para un dispositivo y orientación (null: ordenador, sin emular).
export function tamano(d: Dispositivo, o: Orientacion): { ancho: number; alto: number } | null {
  if (d === 'ordenador') return null;
  const t = TAMANOS[d];
  return o === 'horizontal' ? { ancho: t.alto, alto: t.ancho } : t;
}

// escalaMarco: lo que hay que reducir la página emulada para que quepa entera en el hueco (nunca se amplía).
export function escalaMarco(t: { ancho: number; alto: number }, hueco: { ancho: number; alto: number }, margen = 16): number {
  const f = Math.min((hueco.ancho - margen) / t.ancho, (hueco.alto - margen) / t.alto, 1);
  return f > 0 ? f : 1;
}

// Agente de usuario para tablet y móvil: muchas webs deciden su versión por él, no solo por el ancho.
export function agenteUsuario(d: Dispositivo): string | undefined {
  if (d === 'movil') return 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36';
  if (d === 'tablet') return 'Mozilla/5.0 (Linux; Android 14; Pixel Tablet) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';
  return undefined;
}

// normalizaUrl: lo escrito en la barra → URL (sin esquema: http:// si es local o un puerto, https:// si no).
export function normalizaUrl(s: string): string {
  s = s.trim();
  if (!s) return '';
  if (/^:\d+/.test(s)) s = 'localhost' + s;
  if (/^\d+$/.test(s)) s = 'localhost:' + s;
  if (!/^https?:\/\//i.test(s)) s = (/^(localhost|\d{1,3}(\.\d{1,3}){3}|\[::1\]|[\w-]+\.(localhost|test))(:\d+)?(\/|$)/i.test(s) ? 'http://' : 'https://') + s;
  try { const u = new URL(s); return u.protocol === 'http:' || u.protocol === 'https:' ? u.toString() : ''; } catch { return ''; }
}

// sugerencias: direcciones a proponer, sin repetir: las de los programas en marcha primero, luego las guardadas
// en la configuración de programas y la última usada.
export function sugerencias(enMarcha: (string | undefined)[], configuradas: (string | undefined)[], ultima?: string): string[] {
  const l: string[] = [];
  for (const u of [...enMarcha, ...configuradas, ultima]) {
    const n = u ? normalizaUrl(u) : '';
    if (n && !l.includes(n)) l.push(n);
  }
  return l;
}

// Lo guardado de una vez para otra (por proyecto), tolerante con lo que haya en el almacenamiento.
export function leePrueba(crudo: string | null): Prueba {
  try {
    const p = JSON.parse(crudo || '{}') as Partial<Prueba>;
    return {
      destino: p.destino === 'sistema' || p.destino === 'programa' ? p.destino : 'interno',
      url: typeof p.url === 'string' ? p.url : '',
      ...(typeof p.programa === 'string' ? { programa: p.programa } : {}),
      dispositivo: p.dispositivo === 'tablet' || p.dispositivo === 'movil' ? p.dispositivo : 'ordenador',
      orientacion: p.orientacion === 'vertical' ? 'vertical' : 'horizontal',
    };
  } catch { return { ...PRUEBA_DEFECTO }; }
}

// La orientación por defecto al elegir un dispositivo: el ordenador en horizontal; tablet y móvil en vertical.
export const orientacionDe = (d: Dispositivo): Orientacion => (d === 'ordenador' ? 'horizontal' : 'vertical');

// direccionesEn: las direcciones locales (lo que se está desarrollando) que aparecen en un texto del agente, en
// orden y sin repetir; 0.0.0.0 (escuchar en todas las interfaces) se abre como localhost.
export function direccionesEn(texto: string): string[] {
  const l: string[] = [];
  const re = /https?:\/\/(?:localhost|127\.0\.0\.1|0\.0\.0\.0|\[::1\]|\d{1,3}(?:\.\d{1,3}){3}|[\w-]+\.(?:localhost|test))(?::\d{2,5})?(?:\/[^\s`'"<>)\]*]*)?/gi;
  for (const m of texto.matchAll(re)) {
    const u = normalizaUrl(m[0].replace(/[.,;:!?]+$/, '').replace('//0.0.0.0', '//localhost'));
    if (u && !l.includes(u)) l.push(u);
  }
  return l;
}
