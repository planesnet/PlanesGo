// Relevo del banco de consumo (estrategia «relevo»): la misma memoria de trabajo y la misma decisión que la
// interfaz (web/src/workingMemory.ts), sin duplicar la lógica. Lee por stdin
//   {"agent", "threshold", "context", "messages": Mensaje[] (como web/src/api.ts), "message"}
// y escribe {"reason": por qué hay relevo ("": no lo hay), "message": lo que se envía, "memory_chars"}.
//
//	node --experimental-strip-types cmd/consumo/relay.mts < entrada.json
import * as W from '../../web/src/workingMemory.ts';

const chunks: Buffer[] = [];
for await (const c of process.stdin) chunks.push(c as Buffer);
const input = JSON.parse(Buffer.concat(chunks).toString('utf8'));
const messages = (input.messages || []).filter((m: { error?: boolean }) => !m.error);
const memory = W.buildWorkingMemory(messages);
const reason = W.shouldRelay({
  agent: input.agent, setting: input.threshold > 0 ? input.threshold : 'auto', context: input.context, idleMinutes: 0,
  hasAgentSession: true, attachments: 0, memoryChars: memory.length,
});
process.stdout.write(JSON.stringify({
  reason: reason || '', message: reason ? W.relayMessage(memory, input.message) : input.message, memory_chars: memory.length,
}));
