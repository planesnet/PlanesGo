import * as DR from '../direct';

// DirectNote: lo de una orden que ha hecho pgocode sin el agente, bajo su nota: la salida recortada (abierta si ha
// fallado), «0 tokens» y, si ha fallado, «Pásaselo al agente» (con la orden y solo los errores relevantes).
export default function DirectNote({ d, alPasar }: { d: DR.DirectNote; alPasar: (texto: string) => void }) {
  if (d.curso) return null;
  const fallo = d.ok === false && Boolean(d.errores || d.salida);
  return (
    <div className="direct-note">
      {d.salida && (
        <details className="direct-output" open={d.ok === false}>
          <summary>Salida{d.ok === false ? ' (recortada: errores y final)' : ''}</summary>
          <pre>{d.salida}</pre>
        </details>
      )}
      <div className="direct-footer">
        <span title="pgocode lo ha resuelto sin enviar nada al agente: no se ha reenviado su contexto">⚡ {DR.FOOTER}</span>
        {fallo && (
          <button className="enlace" onClick={() => alPasar(DR.agentMessage(d))}
            title="Enviar al agente un mensaje corto con la orden y solo los errores, para que lo arregle (a la cola si está trabajando)">
            Pásaselo al agente
          </button>
        )}
      </div>
    </div>
  );
}
