// La caché del prompt de la sesión en la ventana del hilo de limpieza: cuándo caduca o, si ya caducó, cuántos tokens
// irán sin ella en el próximo mensaje (cacheStatus.ts). Solo tokens.
import { useEffect, useState } from 'react';
import type { Mensaje } from '../api';
import * as CS from '../cacheStatus';

export default function CacheLine({ messages, agent }: { messages: Mensaje[]; agent: string }) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 15_000);
    return () => clearInterval(t);
  }, []);
  const st = CS.cacheState(messages, agent, now);
  const line = CS.cacheLine(st, agent, now);
  if (!line) return null;
  return <div className={'pequeno ' + (st?.warm ? 'suave' : 'aviso-txt')}>{line}</div>;
}
