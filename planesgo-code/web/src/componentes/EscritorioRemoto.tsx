// Escritorio remoto (pgocode --rdp): la ventana de pgocode para entrar por escritorio remoto en cualquier equipo
// Windows. Usa la sesión de pgocode (la misma app y el mismo backend); si no hay, el acceso de siempre. El proyecto
// es obligatorio: si la conexión ya lo tiene, se muestra; si no, el buscador de la barra de proyectos. El tiempo se
// imputa en él con el servicio de pgocode, y arriba se ven las horas como en la barra de pgocode. La ventana de
// Windows la abre el backend (FreeRDP) con todo permitido (portapapeles, carpeta «Intercambio», sonido, micrófono e
// impresoras) y se cierra desde ella misma. La contraseña va a FreeRDP y no se guarda; las conexiones, sin
// contraseña, en este navegador.
import { useEffect, useRef, useState, type FormEvent } from 'react';
import { escuchaAvisos, get, post, type Proyecto, type Usuario } from '../api';
import Acceso, { CampoClave } from './Acceso';
import Imputacion, { type SesionMarcador } from './Imputacion';
import { Dialogs, showMessage } from './Modal';
import { BuscaProyecto } from './Proyectos';

interface Conexion { equipo: string; usuario: string; proyecto: number }
interface Abierta { id: string; proyecto: number; estado: 'conectando' | 'conectado' | 'terminado'; error?: string }

const clave = (login: string) => `pgo:rdp:${login.toLowerCase()}`;
const leeConexiones = (login: string): Conexion[] => { try { return JSON.parse(localStorage.getItem(clave(login)) || '[]'); } catch { return []; } };
const guardaConexiones = (login: string, l: Conexion[]) => { try { localStorage.setItem(clave(login), JSON.stringify(l)); } catch { /* sin almacenamiento */ } };

export default function EscritorioRemoto() {
  const [usuario, setUsuario] = useState<Usuario | null | undefined>(undefined);
  useEffect(() => { get<Usuario>('/api/yo').then(setUsuario).catch(() => setUsuario(null)); }, []);
  if (usuario === undefined) return null;
  if (!usuario) return <Acceso aviso="" alEntrar={setUsuario} />;
  return <Formulario usuario={usuario} />;
}

function Formulario({ usuario }: { usuario: Usuario }) {
  const [proyectos, setProyectos] = useState<Proyecto[]>([]);
  const [conexiones, setConexiones] = useState<Conexion[]>(() => leeConexiones(usuario.login));
  const [c, setC] = useState<Conexion>(() => leeConexiones(usuario.login)[0] || { equipo: '', usuario: '', proyecto: 0 });
  const [cambiaProyecto, setCambiaProyecto] = useState(false);
  const [contrasena, setContrasena] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [version, setVersion] = useState(0); // horas de hoy: se recargan con el aviso «imputacion»
  const [activa, setActiva] = useState<SesionMarcador>();
  const mias = useRef(new Set<string>()); // las conexiones abiertas desde esta ventana (para avisar de sus errores)
  const [versionApp, setVersionApp] = useState('');

  // Es pgocode: su versión, en el título de la ventana (como la principal) y en la tarjeta.
  useEffect(() => {
    get<{ version: string }>('/api/version').then(v => { setVersionApp(v.version); document.title = 'Escritorio remoto · Planes Go Code ' + v.version; }).catch(() => {});
  }, []);

  useEffect(() => { get<Proyecto[]>('/api/proyectos').then(setProyectos).catch(() => {}); }, []);
  useEffect(() => {
    const carga = () => get<{ conexiones: Abierta[] }>('/api/rdp').then(r => {
      const viva = r.conexiones.find(a => mias.current.has(a.id) && a.estado !== 'terminado');
      setActiva(viva ? { id: 'rdp-' + viva.id, proyecto: viva.proyecto } : undefined);
      for (const a of r.conexiones) {
        if (a.estado === 'terminado' && mias.current.delete(a.id) && a.error) showMessage(a.error, { level: 'error', title: 'Escritorio remoto' });
      }
    }).catch(() => {});
    const fuera = escuchaAvisos(a => { if (a.tipo === 'rdp') carga(); else if (a.tipo === 'imputacion') setVersion(v => v + 1); });
    const t = setInterval(carga, 5000); // por si se pierde un aviso
    return () => { clearInterval(t); fuera(); };
  }, []);

  const proyecto = proyectos.find(p => p.id === c.proyecto);
  const conoceProyecto = Boolean(proyecto) && !cambiaProyecto;
  const cambiaEquipo = (equipo: string) => {
    const g = conexiones.find(x => x.equipo === equipo); // una conexión guardada: su usuario y su proyecto
    setC(g ? { ...g } : x => ({ ...x, equipo }));
  };

  async function conectar(e: FormEvent) {
    e.preventDefault();
    if (!c.proyecto) return;
    setEnviando(true);
    try {
      const r = await post<{ id: string }>('/api/rdp/conectar', { proyecto: c.proyecto, nombre: c.equipo, equipo: c.equipo, usuario: c.usuario, contrasena });
      mias.current.add(r.id);
      const l = [c, ...conexiones.filter(x => x.equipo !== c.equipo)];
      setConexiones(l);
      guardaConexiones(usuario.login, l);
      setCambiaProyecto(false);
    } catch (err) {
      showMessage((err as Error).message, { level: 'error', title: 'Escritorio remoto' });
    } finally {
      setEnviando(false);
      setContrasena('');
    }
  }

  return (
    <main className="acceso rdp">
      <form onSubmit={conectar} className="tarjeta-acceso">
        <header className="rdp-cabecera">
          <img className="logo" src="/logo.svg" alt="" width="40" height="40" />
          <h1>Escritorio <span>remoto</span>{versionApp && <small className="rdp-version">Planes Go Code {versionApp}</small>}</h1>
        </header>
        <div className="rdp-horas"><Imputacion version={version} sesion={activa} /></div>
        <div className="rdp-proyecto">
          <span className="rdp-etiqueta">Proyecto</span>
          {conoceProyecto
            ? <p className="rdp-elegido"><b>{proyecto!.nombre}</b><button type="button" className="enlace" onClick={() => setCambiaProyecto(true)}>Cambiar</button></p>
            : <BuscaProyecto login={usuario.login} lista={proyectos} elegido={c.proyecto} alElegir={p => { setC(x => ({ ...x, proyecto: p.id })); setCambiaProyecto(false); }} />}
        </div>
        <label>Equipo
          <input list="rdp-equipos" value={c.equipo} onChange={e => cambiaEquipo(e.target.value)} placeholder="nombre o IP[:puerto]" required />
          <datalist id="rdp-equipos">{conexiones.map(x => <option key={x.equipo} value={x.equipo}>{x.usuario}</option>)}</datalist>
        </label>
        <label>Usuario
          <input autoComplete="off" value={c.usuario} onChange={e => setC(x => ({ ...x, usuario: e.target.value }))} placeholder="DOMINIO\usuario o usuario" required />
        </label>
        <label>Contraseña
          <CampoClave value={contrasena} onChange={setContrasena} autoComplete="off" />
        </label>
        <button className="primario" disabled={enviando || !c.proyecto}>{enviando ? 'Conectando…' : 'Conectar'}</button>
      </form>
      <Dialogs />
    </main>
  );
}
