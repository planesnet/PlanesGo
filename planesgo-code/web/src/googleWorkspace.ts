// Google Workspace (Propiedades → Google Workspace): tipos de /api/google y su estado en palabras. Funciones puras:
// node --test src/googleWorkspace.test.ts

export interface GoogleStatus {
  conectado: boolean; drive?: boolean; gmail?: boolean; email?: string; ambitos?: string[]; fuente?: 'oauth' | 'adc' | 'gcloud'; solo_lectura?: boolean;
  desde?: string; cliente: boolean; cliente_origen?: 'fichero' | 'servidor'; cliente_proyecto?: string;
  en_curso?: boolean; problema?: string;
}

const DRIVE_RO = 'https://www.googleapis.com/auth/drive.readonly';
const DRIVE = 'https://www.googleapis.com/auth/drive';
const GMAIL_RO = 'https://www.googleapis.com/auth/gmail.readonly';

// googleStatusText: el estado en una línea.
export function googleStatusText(s: GoogleStatus | null): string {
  if (!s) return '—';
  if (s.en_curso) return 'Esperando a que aceptes en la página de Google…';
  if (s.conectado) {
    const access = s.drive && !s.solo_lectura ? 'Drive completo (pgocode solo lee)' : 'solo lectura';
    if (s.fuente !== 'oauth') return `Con las credenciales de Google de este equipo${s.email ? ` (${s.email})` : ''} · ${access}`;
    return `Conectado como ${s.email || 'tu cuenta de Google'} · ${access}`;
  }
  if (!s.cliente) return 'Falta registrar la aplicación de Google: carga el fichero del cliente';
  return 'Sin conectar';
}

// googleActions: qué botones enseña la sección. reconnect: la cuenta está conectada pero le falta un permiso (p. ej.
// conectada antes de que pgocode leyera Gmail): el botón de conectar dice «Volver a conectar» (pide los dos).
export function googleActions(s: GoogleStatus | null): { connect: boolean; reconnect: boolean; disconnect: boolean; loadClient: boolean } {
  if (!s) return { connect: false, reconnect: false, disconnect: false, loadClient: false };
  const own = s.conectado && s.fuente === 'oauth';
  const reconnect = own && (!s.drive || !s.gmail);
  return { connect: s.cliente && (!own || reconnect) && !s.en_curso, reconnect, disconnect: own, loadClient: !s.cliente || s.cliente_origen === 'fichero' };
}

export interface GoogleService { name: string; ok: boolean; text: string }

// googleServices: Drive y Gmail con su estado (solo si hay acceso a Google).
export function googleServices(s: GoogleStatus | null): GoogleService[] {
  if (!s?.conectado) return [];
  const missing = s.fuente === 'oauth' ? 'sin autorizar: «Volver a conectar» y marca el permiso en la página de Google' : 'sin autorizar: conecta tu cuenta de Google';
  const row = (name: string, ok: boolean, what: string): GoogleService => ({ name, ok, text: ok ? `autorizado · ${what}` : missing });
  return [row('Drive, Docs y Sheets', Boolean(s.drive), 'pgo drive'), row('Gmail', Boolean(s.gmail), 'pgo gmail (solo leer: no envía ni marca nada)')];
}

// scopeNames: los ámbitos del token en palabras cortas.
export function scopeNames(ambitos?: string[]): string[] {
  return (ambitos || []).map(a => {
    if (a === DRIVE_RO) return 'Drive (solo lectura)';
    if (a === DRIVE) return 'Drive';
    if (a === GMAIL_RO) return 'Gmail (solo lectura)';
    if (a === 'openid') return 'identidad';
    if (a.endsWith('/userinfo.email') || a === 'email') return 'correo';
    if (a.endsWith('/cloud-platform')) return 'Google Cloud';
    return a.replace(/^https:\/\/www\.googleapis\.com\/auth\//, '');
  });
}

// clientText: de dónde sale la aplicación de Google (nunca su identificador ni su secreto).
export function clientText(s: GoogleStatus | null): string {
  if (!s?.cliente) return '—';
  const where = s.cliente_origen === 'servidor' ? 'la da el servidor de PlanesGo' : 'fichero cargado en este equipo';
  return `Registrada${s.cliente_proyecto ? ` (proyecto ${s.cliente_proyecto})` : ''} · ${where}`;
}

// clientFileProblem: por qué no se manda un fichero arrastrado o elegido ('' si se puede mandar).
export function clientFileProblem(name: string, size: number): string {
  if (!/\.json$/i.test(name.trim())) return 'Tiene que ser el fichero JSON del cliente que descarga la consola de Google';
  if (size > 64 * 1024) return 'El fichero es demasiado grande para ser el de un cliente OAuth';
  return '';
}
