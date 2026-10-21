// Propiedades de cada proyecto (por usuario). De momento en la app; más adelante, en Odoo.
export type Detalle = 'conciso' | 'normal' | 'detallado';

// Alojamiento (hosting) del proyecto: dónde se ejecuta y cómo se despliega.
export type TipoHosting = 'local' | 'odoo.sh' | 'autopyme.com' | 'gcloud' | 'servidor' | 'sage';
export interface Hosting { tipo: TipoHosting; campos: Record<string, string> }

export interface Campo { id: string; nombre: string; ayuda?: string; ejemplo?: string }
export const TIPOS_HOSTING: { id: TipoHosting; nombre: string; descripcion: string; campos: Campo[] }[] = [
  { id: 'local', nombre: 'Local', descripcion: 'Se ejecuta y prueba en este equipo.', campos: [] },
  { id: 'odoo.sh', nombre: 'Odoo.sh', descripcion: 'Odoo en Odoo.sh: cada rama se despliega sola al hacer push.', campos: [
    { id: 'url_pruebas', nombre: 'URL de pruebas', ejemplo: 'https://…-test-….dev.odoo.com/' }] },
  { id: 'autopyme.com', nombre: 'AutoPyme', descripcion: 'Odoo en AutoPyme.', campos: [
    { id: 'url_pruebas', nombre: 'URL de pruebas' }] },
  { id: 'gcloud', nombre: 'Google Cloud', descripcion: 'Máquina virtual de Google Cloud con despliegue automático desde las ramas master (producción) y test (pruebas).', campos: [
    { id: 'proyecto_gcp', nombre: 'Proyecto de Google Cloud', ejemplo: 'planes-dev' },
    { id: 'zona', nombre: 'Zona', ejemplo: 'europe-southwest1-a' },
    { id: 'vm_produccion', nombre: 'Máquina virtual de producción (rama master)', ejemplo: 'mi-app-prod' },
    { id: 'vm_pruebas', nombre: 'Máquina virtual de pruebas (rama test)', ejemplo: 'mi-app-test' },
    { id: 'ruta', nombre: 'Carpeta del programa en la máquina', ejemplo: '/opt/mi-app' },
    { id: 'url_produccion', nombre: 'URL de producción' },
    { id: 'url_pruebas', nombre: 'URL de pruebas' }] },
  { id: 'servidor', nombre: 'Servidor propio', descripcion: 'El programa o servicio se ejecuta en un servidor propio, con despliegue automatizado.', campos: [
    { id: 'host', nombre: 'Servidor', ejemplo: 'servidor.planesnet.com' },
    { id: 'usuario', nombre: 'Usuario SSH', ejemplo: 'deploy' },
    { id: 'ruta', nombre: 'Carpeta del programa', ejemplo: '/srv/mi-servicio' },
    { id: 'rama_produccion', nombre: 'Rama de producción', ejemplo: 'master' },
    { id: 'rama_pruebas', nombre: 'Rama de pruebas', ejemplo: 'test' },
    { id: 'reinicio', nombre: 'Orden para reiniciar el servicio', ejemplo: 'sudo systemctl restart mi-servicio' },
    { id: 'url_produccion', nombre: 'URL de producción' },
    { id: 'url_pruebas', nombre: 'URL de pruebas' }] },
  { id: 'sage', nombre: 'Servidor Sage', descripcion: 'Desarrollo sobre Sage: no se ejecuta en local. Los cambios se prueban en un servidor de pruebas de Sage; un servidor de ficheros hace de repositorio con todo lo necesario para programar.', campos: [
    { id: 'servidor_pruebas', nombre: 'Servidor de pruebas de Sage', ejemplo: 'sage-test.planesnet.local' },
    { id: 'acceso_pruebas', nombre: 'Acceso al servidor de pruebas', ayuda: 'Cómo se llega: escritorio remoto, SSH, carpeta compartida…', ejemplo: 'RDP / \\\\sage-test\\proyecto' },
    { id: 'repositorio', nombre: 'Servidor de ficheros (repositorio)', ayuda: 'Dónde se guardan los ficheros e información para programar en Sage.', ejemplo: '\\\\ficheros\\sage o ssh://ficheros/srv/sage' },
    { id: 'aplicar', nombre: 'Cómo se aplican los cambios en pruebas', ayuda: 'Copiar ficheros, importar un paquete, ejecutar un script…' },
    { id: 'notas', nombre: 'Notas' }] },
];

// Cómo se elige la skill de cada petición: el agente solo (como siempre), pgocode la sugiere (sin tocar lo que se
// envía) o pgocode la elige con embeddings y se la indica al agente.
export type SeleccionSkills = 'agente' | 'sugerir' | 'elegir';

export interface AjustesProyecto { detalle: Detalle; hosting?: Hosting; skills: SeleccionSkills; directorio?: string }

export const AJUSTES_DEFECTO: AjustesProyecto = { detalle: 'normal', skills: 'sugerir' };

const clave = (login: string, proyecto: number) => `pgo:ajustes:${login.toLowerCase()}:${proyecto}`;

export function leeAjustes(login: string, proyecto: number): AjustesProyecto {
  try { return { ...AJUSTES_DEFECTO, ...JSON.parse(localStorage.getItem(clave(login, proyecto)) || '{}') }; } catch { return AJUSTES_DEFECTO; }
}

export function guardaAjustes(login: string, proyecto: number, a: AjustesProyecto) {
  try { localStorage.setItem(clave(login, proyecto), JSON.stringify(a)); } catch { /* sin almacenamiento */ }
}
