import { useEffect } from 'react';
import * as AY from '../ayuda';
import Icono from './Icono';
import './ayuda.css';

// Ayuda («/help»): las teclas y órdenes más habituales, encima del cuadro de texto. Se cierra con Esc o con ×.
export function Ayuda({ alTurno, inyecta, alCerrar }: { alTurno: boolean; inyecta: boolean; alCerrar: () => void }) {
  useEffect(() => {
    const tecla = (e: KeyboardEvent) => { if (e.key === 'Escape') alCerrar(); };
    document.addEventListener('keydown', tecla);
    return () => document.removeEventListener('keydown', tecla);
  }, [alCerrar]);
  return (
    <section className="ayuda-chat" aria-label="Ayuda">
      <header>
        <b>Ayuda</b>
        <button className="icono" onClick={alCerrar} title="Cerrar (Esc)" aria-label="Cerrar la ayuda"><Icono n="cerrar" t={13} /></button>
      </header>
      <div className="ayuda-secciones">
        {AY.secciones({ alTurno, inyecta }).map(s => (
          <div key={s.titulo}>
            <h3>{s.titulo}</h3>
            <dl>{s.entradas.map(e => <div key={e.clave}><dt><kbd>{e.clave}</kbd></dt><dd>{e.que}</dd></div>)}</dl>
          </div>
        ))}
      </div>
    </section>
  );
}
