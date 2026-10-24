import { useEffect, useState } from 'react';
import * as RP from '../reparto';

// Las tarjetas del reparto en sesiones paralelas (reparto.ts) que salen en el chat de la sesión madre.

// La propuesta: las partes y la cuenta atrás; si no se pulsa nada, se reparte.
export function TarjetaPropuesta({ prop, hasta, alRepartir, alEnEstaSesion }: { prop: RP.Propuesta; hasta: number; alRepartir: () => void; alEnEstaSesion: () => void }) {
  const [ahora, setAhora] = useState(Date.now());
  useEffect(() => { const t = setInterval(() => setAhora(Date.now()), 250); return () => clearInterval(t); }, []);
  const partes = prop.partes || [];
  const seg = RP.restante(hasta, ahora);
  return (
    <div className="mensaje assistant tarjeta-reparto" role="group" aria-label="Repartir en sesiones paralelas">
      <p><b>Esta petición tiene {partes.length} partes independientes.</b> Las reparto en {partes.length} sesiones en paralelo, cada una en su copia del repositorio.
        {partes.length > (prop.max || RP.MAX_HIJAS) && <span className="suave"> ({prop.max || RP.MAX_HIJAS} a la vez; el resto, en cola.)</span>}</p>
      <ol className="reparto-partes">{partes.map((p, i) => <li key={i} title={p}>{p}</li>)}</ol>
      {prop.madre_sucia && <p className="suave pequeno">Esta sesión tiene cambios sin guardar: las hijas parten de lo ya guardado y la unión quedará pendiente hasta que lo guardes.</p>}
      <div className="tarjeta-plan-acciones">
        <button className="boton-secundario" onClick={alEnEstaSesion} title="Cancela el reparto: la petición sigue en esta sesión, como siempre">Hacerlo en esta sesión</button>
        <button className="primario" onClick={alRepartir} title="Reparte ya, sin esperar a la cuenta atrás">Repartir{seg > 0 ? ` (${seg} s)` : '…'}</button>
        <span className="suave pequeno" aria-live="off">{seg > 0 ? `Se reparte solo en ${seg} s.` : 'Repartiendo…'}</span>
      </div>
    </div>
  );
}

const CLASE_HIJA: Record<RP.EstadoHija, string> = { espera: '', trabajando: 'curso', hecha: 'ok', parada: 'aviso', error: 'error' };

// El reparto en marcha o acabado: cada hija con su estado y su unión, y lo que se puede hacer con ella.
export function TarjetaReparto({ reparto, trabajando, alAbrir, alParar, alUnir, alResolver, alQuitar }: {
  reparto: RP.Reparto; trabajando: (h: RP.Hija) => boolean;
  alAbrir: (h: RP.Hija) => void; alParar: (h: RP.Hija) => void; alUnir: (h: RP.Hija) => void; alResolver: (h: RP.Hija) => void; alQuitar: () => void;
}) {
  const acabado = RP.acabado(reparto);
  return (
    <div className="mensaje assistant tarjeta-reparto" role="group" aria-label="Reparto en sesiones paralelas">
      <div className="tarjeta-plan-cabecera">
        <b>Reparto en {reparto.hijas.length} sesiones paralelas</b>
        <span className="suave pequeno recorta">desde «{reparto.ramaMadre || 'la rama de esta sesión'}»</span>
        <span className="hueco" />
        {acabado && <button className="icono" onClick={alQuitar} title="Quitar esta tarjeta (las ramas y subcarpetas pendientes se conservan)" aria-label="Quitar la tarjeta del reparto">×</button>}
      </div>
      <ul className="reparto-hijas">
        {reparto.hijas.map(h => (
          <li key={h.n} className="reparto-hija">
            <span className={'plan-estado reparto-estado ' + CLASE_HIJA[h.estado]}>{RP.NOMBRE_HIJA[h.estado]}</span>
            <span className="reparto-texto recorta" title={h.parte}>Parte {h.n}: {h.parte}</span>
            {h.union !== 'sin' && <span className={'plan-estado reparto-estado ' + (h.union === 'unida' || h.union === 'sin_cambios' ? 'ok' : h.union === 'pendiente' ? 'aviso' : 'error')}
              title={RP.fraseUnion(h)}>{RP.NOMBRE_UNION[h.union]}</span>}
            <span className="reparto-acciones">
              {(h.estado === 'trabajando' || h.estado === 'espera') && <button className="enlace" onClick={() => alParar(h)} disabled={h.estado === 'trabajando' && !trabajando(h)}>Parar</button>}
              {h.carpeta && !h.quitada && <button className="enlace" onClick={() => alAbrir(h)} title={`Abrir la sesión de la parte ${h.n}`}>Abrir</button>}
              {h.union === 'conflicto' && !h.resolving && h.rama && <button className="enlace" onClick={() => alResolver(h)}
                title="Pedir al agente de esta sesión que una la rama de esta parte y resuelva el conflicto (a la cola si está trabajando)">Resolver con el agente</button>}
              {h.union === 'conflicto' && h.resolving && <span className="suave pequeno" title="Cuando el agente acabe, pgocode comprueba la unión y quita la subcarpeta de la parte">pedido al agente</span>}
              {RP.seUne(h) && (h.union !== 'sin' || h.estado === 'parada') && <button className="enlace" onClick={() => alUnir(h)} title="Unir la rama de esta parte a la de esta sesión (si está limpia y sin agente trabajando)">Unir cambios</button>}
            </span>
            {(h.union === 'conflicto' || h.union === 'pendiente' || h.union === 'error' || h.aviso || h.estado === 'error') && (
              <span className="reparto-detalle suave pequeno">{h.estado === 'error' && h.union === 'sin' ? (h.motivo || 'ha terminado con error') + (h.rama ? `. Rama «${h.rama}» conservada.` : '') : RP.fraseUnion(h)}</span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
