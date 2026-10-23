// Google Workspace (Propiedades → Google Workspace): tipos de /api/google y su estado en palabras. Funciones puras:
// node --test src/googleWorkspace.test.ts

export interface GoogleStatus {
  conectado: boolean; email?: string; ambitos?: string[]; fuente?: 'oauth' | 'adc' | 'gcloud'; solo_lectura?: boolean;
  desde?: string; cliente: boolean; cliente_origen?: 'fichero' | 'servidor'; cliente_proyecto?: string;
  en_curso?: boolean; problema?: string;
}

const DRIVE_RO = 'https://www.googleapis.com/auth/drive.readonly';
const DRIVE = 'https://www.googleapis.com/auth/drive';

// googleStatusText: el estado en una línea.
export function googleStatusText(s: GoogleStatus | null): string {
  if (!s) return '—';
  if (s.en_curso) return 'Esperando a que aceptes en la página de Google…';
  if (s.conectado) {
    const access = s.solo_lectura ? 'solo lectura de Drive' : 'Drive completo (pgocode solo lee)';
    if (s.fuente !== 'oauth') return `Con las credenciales de Google de este equipo${s.email ? ` (${s.email})` : ''} · ${access}`;
    return `Conectado como ${s.email || 'tu cuenta de Google'} · ${access}`;
  }
  if (!s.cliente) return 'Falta registrar la aplicación de Google: carga el fichero del cliente';
  return 'Sin conectar';
}

// googleActions: qué botones enseña la sección.
export function googleActions(s: GoogleStatus | null): { connect: boolean; disconnect: boolean; loadClient: boolean } {
  if (!s) return { connect: false, disconnect: false, loadClient: false };
  const own = s.conectado && s.fuente === 'oauth';
  return { connect: s.cliente && !own && !s.en_curso, disconnect: own, loadClient: !s.cliente || s.cliente_origen === 'fichero' };
}

// scopeNames: los ámbitos del token en palabras cortas.
export function scopeNames(ambitos?: string[]): string[] {
  return (ambitos || []).map(a => {
    if (a === DRIVE_RO) return 'Drive (solo lectura)';
    if (a === DRIVE) return 'Drive';
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
