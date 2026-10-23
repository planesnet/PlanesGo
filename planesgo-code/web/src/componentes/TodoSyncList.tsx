import type { SyncItem, SyncView } from '../todoSync';

// La lista de tareas del agente, sincronizada con lo que hace (todoSync.ts): arriba la que se hace ahora (▶ la que marca
// el agente; ▷ la que hace según sus acciones), debajo solo lo que falta y, plegadas, las hechas y las canceladas. La
// usan el chat (la lista de cada turno) y Actividad.

const MARK: Record<SyncItem['vista'], string> = { curso: '▶', inferida: '▷', pendiente: '○', 'sin-hacer': '○', hecha: '✓', probable: '≈', cancelada: '✕' };

function Item({ e }: { e: SyncItem }) {
  const nota = e.vista === 'probable' ? 'probablemente hecha'
    : e.vista === 'sin-hacer' ? (e.estado === 'curso' ? 'sin terminar al acabar el turno' : 'sin hacer al acabar el turno') : '';
  return (
    <li className={'todo-item ' + e.vista} title={e.motivo}>
      <span className="todo-marca" aria-hidden>{MARK[e.vista]}</span>
      <span className="todo-texto">{e.texto}{nota && <span className="todo-nota"> · {nota}</span>}</span>
    </li>
  );
}

export default function TodoSyncList({ v, trabajando }: { v: SyncView; trabajando: boolean }) {
  const a = v.ahora;
  return (
    <div className="todo-sync">
      {a ? (
        <div className={'todo-ahora' + (a.vista === 'inferida' ? ' inferida' : '')}
          title={a.vista === 'inferida' ? (a.motivo || '') + '. El agente aún no lo ha marcado en su lista.' : 'La que el agente marca en curso'}>
          {a.vista === 'inferida' ? '▷ En curso (según lo que hace): ' : '▶ Ahora: '}{a.texto}
        </div>
      ) : trabajando && v.siguiente ? <div className="todo-ahora siguiente">Siguiente: {v.siguiente.texto}</div>
        : v.finished ? <div className="todo-ahora hecha">✓ Todas hechas{v.probables ? ` (${v.probables} probablemente)` : ''}</div> : null}
      {v.pendientes.length > 0 && (
        <>
          <div className="todo-grupo">Pendientes ({v.pendientes.length})</div>
          <ul>{v.pendientes.map(e => <Item key={e.i} e={e} />)}</ul>
        </>
      )}
      {v.hechas.length > 0 && (
        <details className="todo-plegadas">
          <summary>✓ Hechas ({v.hechas.length}){v.probables ? <span className="todo-nota"> · {v.probables} probablemente</span> : null}</summary>
          <ul>{v.hechas.map(e => <Item key={e.i} e={e} />)}</ul>
        </details>
      )}
      {v.canceladas.length > 0 && (
        <details className="todo-plegadas todo-canceladas">
          <summary>Canceladas ({v.canceladas.length})</summary>
          <ul>{v.canceladas.map(e => <Item key={e.i} e={e} />)}</ul>
        </details>
      )}
    </div>
  );
}

// todoCount: «3/5» (o «≈4/5» si alguna está hecha solo según lo que ha hecho el agente).
export const todoCount = (v: SyncView) => (v.probables ? '≈' : '') + (v.done + v.probables) + '/' + v.total;
