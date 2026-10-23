// Google Workspace (Propiedades → Google Workspace): tipos de /api/google y su estado en palabras. Funciones puras:
// node --test src/googleWorkspace.test.ts

export interface GoogleStatus {
  conectado: boolean; email?: string; ambitos?: string[]; fuente?: 'adc' | 'gcloud'; solo_lectura?: boolean;
  gcloud: boolean; problema?: string; orden: string; orden_alternativa: string;
  proyecto_cuota?: string; cuota_en_uso?: boolean; orden_servicios?: string;
}

const DRIVE_RO = 'https://www.googleapis.com/auth/drive.readonly';
const DRIVE = 'https://www.googleapis.com/auth/drive';

// googleStatusText: el estado en una línea.
export function googleStatusText(s: GoogleStatus | null): string {
  if (!s) return '—';
  if (!s.gcloud) return 'Falta la herramienta gcloud de Google en este equipo';
  if (s.conectado) return `Conectado como ${s.email || 'tu cuenta de Google'} · ${s.solo_lectura ? 'solo lectura de Drive' : 'Drive completo (pgocode solo lee)'}`;
  if (s.email) return `${s.email}: sin acceso a Drive`;
  return 'Sin acceso a Drive';
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

// sourceText: de dónde sale el token.
export const sourceText = (f?: string) => f === 'adc' ? 'credenciales por defecto de gcloud (ADC)' : f === 'gcloud' ? 'sesión de gcloud' : '—';
