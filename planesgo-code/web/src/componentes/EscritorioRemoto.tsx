// Escritorio remoto (pgocode --rdp): la ventana de pgocode para abrir el escritorio remoto de Windows de una conexión.
// Usa la sesión de pgocode (la misma app y el mismo backend); si no hay, el acceso de siempre. El proyecto es
// obligatorio (el buscador de la barra de proyectos): el tiempo se imputa en él con el servicio de pgocode. La
// ventana de Windows la abre el backend (FreeRDP); la contraseña va a FreeRDP y no se guarda. Siempre van el
// portapapeles y la carpeta compartida; la pantalla completa, cuando se quiera (Ctrl+Alt+Intro). Las conexiones
// (sin contraseña), en este navegador.
import { useEffect, useState, type FormEvent } from 'react';
import { escuchaAvisos, get, post, type Proyecto, type Usuario } from '../api';
import Acceso from './Acceso';
import { BuscaProyecto } from './Proyectos';

interface Opciones { sonido: boolean; impresoras: boolean }
interface Conexion { nombre: string; equipo: string; usuario: string; proyecto: number; opciones: Opciones }
interface Abierta { id: string; proyecto: number; nombre: string; equipo: string; usuario: string; estado: 'conectando' | 'conectado' | 'terminado'; error?: string }

const OPCIONES: [keyof Opciones, string][] = [['sonido', 'Sonido y micrófono'], ['impresoras', 'Impresoras de este equipo']];
const nueva = (): Conexion => ({ nombre: '', equipo: '', usuario: '', proyecto: 0, opciones: { sonido: true, impresoras: false } });

const clave = (login: string) => `pgo:rdp:${login.toLowerCase()}`;
const leeConexiones = (login: string): Conexion[] => { try { return JSON.parse(localStorage.getItem(clave(login)) || '[]'); } catch { return []; } };
const guardaConexiones = (login: string, l: Conexion[]) => { try { localStorage.setItem(clave(login), JSON.stringify(l)); } catch { /* sin almacenamiento */ } };
// Las impresoras no se recuerdan: ninguna hasta que se piden para esa conexión.
const deGuardada = (c?: Conexion): Conexion => c ? { ...c, opciones: { sonido: c.opciones?.sonido ?? true, impresoras: false } } : nueva();

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
  const [elegida, setElegida] = useState(0); // índice en conexiones; conexiones.length = nueva
  const [c, setC] = useState<Conexion>(() => deGuardada(leeConexiones(usuario.login)[0]));
  const [contrasena, setContrasena] = useState('');
  const [error, setError] = useState('');
  const [enviando, setEnviando] = useState(false);
  const [abiertas, setAbiertas] = useState<Abierta[]>([]);
  const [disponible, setDisponible] = useState(true);

  useEffect(() => { get<Proyecto[]>('/api/proyectos').then(setProyectos).catch(e => setError((e as Error).message)); }, []);
  useEffect(() => {
    const carga = () => get<{ disponible: boolean; conexiones: Abierta[] }>('/api/rdp')
      .then(r => { setDisponible(r.disponible); setAbiertas(r.conexiones); }).catch(() => {});
    carga();
    const t = setInterval(carga, 5000); // también llega el aviso «rdp»; esto cubre una conexión del backend perdida
    const fuera = escuchaAvisos(a => { if (a.tipo === 'rdp') carga(); });
    return () => { clearInterval(t); fuera(); };
  }, []);

  const elige = (i: number) => { setElegida(i); setC(deGuardada(conexiones[i])); setError(''); };
  const cambia = (cambio: Partial<Conexion>) => setC(x => ({ ...x, ...cambio }));
  const opcion = (k: keyof Opciones, v: boolean) => setC(x => ({ ...x, opciones: { ...x.opciones, [k]: v } }));

  async function conectar(e: FormEvent) {
    e.preventDefault();
    if (!c.proyecto) { setError('Elige el proyecto al que se imputa el tiempo.'); return; }
    setEnviando(true);
    setError('');
    const guardar = { ...c, nombre: c.nombre.trim() || c.equipo.trim() };
    try {
      await post('/api/rdp/conectar', { ...guardar, contrasena });
      const l = [guardar, ...conexiones.filter(x => x.nombre !== guardar.nombre)];
      setConexiones(l);
      guardaConexiones(usuario.login, l);
      setElegida(0);
      setC(deGuardada(guardar));
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setEnviando(false);
      setContrasena('');
    }
  }

  const proyecto = proyectos.find(p => p.id === c.proyecto);
  const nombreProyecto = (id: number) => proyectos.find(p => p.id === id)?.nombre || `Proyecto ${id}`;
  return (
    <main className="acceso rdp">
      <form onSubmit={conectar} className="tarjeta-acceso">
        <img className="logo" src="/logo.svg" alt="" width="56" height="56" />
        <h1>Escritorio <span>remoto</span></h1>
        <p className="suave">{usuario.nombre} · el tiempo se imputa en el proyecto que elijas.</p>
        {!disponible && <p className="aviso">Falta FreeRDP en este equipo: ejecuta <code>connectors/instala.sh</code>.</p>}
        <div className="rdp-proyecto">
          <span className="rdp-etiqueta">Proyecto{proyecto && <b> · {proyecto.nombre}</b>}</span>
          <BuscaProyecto login={usuario.login} lista={proyectos} elegido={c.proyecto} alElegir={p => cambia({ proyecto: p.id })} />
        </div>
        <label>Conexión
          <select value={elegida} onChange={e => elige(Number(e.target.value))}>
            {conexiones.map((x, i) => <option key={x.nombre} value={i}>{x.nombre} · {x.equipo}</option>)}
            <option value={conexiones.length}>Nueva conexión…</option>
          </select>
        </label>
        <div className="rdp-campos">
          <label>Nombre<input value={c.nombre} onChange={e => cambia({ nombre: e.target.value })} placeholder="Sage pruebas" /></label>
          <label>Equipo<input value={c.equipo} onChange={e => cambia({ equipo: e.target.value })} placeholder="nombre o IP[:puerto]" required /></label>
        </div>
        <label>Usuario de Windows
          <input autoComplete="off" value={c.usuario} onChange={e => cambia({ usuario: e.target.value })} placeholder="DOMINIO\usuario o usuario" required />
        </label>
        <label>Contraseña de Windows
          <input type="password" autoComplete="new-password" value={contrasena} onChange={e => setContrasena(e.target.value)} required />
        </label>
        <fieldset className="rdp-opciones">
          {OPCIONES.map(([k, texto]) => (
            <label key={k}><input type="checkbox" checked={c.opciones[k]} onChange={e => opcion(k, e.target.checked)} />{texto}</label>
          ))}
        </fieldset>
        <p className="suave rdp-nota">Siempre: portapapeles y carpeta compartida «Intercambio». Pantalla completa: Ctrl+Alt+Intro.</p>
        {error && <p className="error" role="alert">{error}</p>}
        <button className="primario" disabled={enviando || !disponible}>{enviando ? 'Conectando…' : 'Conectar'}</button>
        {abiertas.length > 0 && (
          <ul className="rdp-abiertas">
            {abiertas.map(a => (
              <li key={a.id}>
                <span className={`rdp-estado ${a.estado}`} aria-hidden="true" />
                <span><b>{a.nombre}</b> <span className="suave">{a.usuario}@{a.equipo} · {nombreProyecto(a.proyecto)}</span>
                  {a.error && <span className="error"> · {a.error}</span>}</span>
                {a.estado !== 'terminado' && <button type="button" className="boton-secundario peligro" onClick={() => post('/api/rdp/desconectar', { id: a.id }).catch(() => {})}>Desconectar</button>}
              </li>
            ))}
          </ul>
        )}
      </form>
    </main>
  );
}
