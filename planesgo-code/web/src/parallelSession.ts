export interface SessionRef {
  id: string;
  proyecto: number;
  agente: string;
  carpeta: string;
  rama: string;
  hosting?: string;
  directo?: boolean;
}

export interface ProviderRef {
  id: string;
  disponible: boolean;
}

export function resolveTurnSession<S extends SessionRef>(
  activeSession: S | null | undefined,
  requestedSession: S | null | undefined,
  activeProjectId: number | null | undefined,
): S | null {
  const target = requestedSession ?? activeSession ?? null;
  if (!target || !target.hosting) return null;
  if (!requestedSession && (!activeProjectId || target.proyecto !== activeProjectId)) return null;
  return target;
}

export function sessionMessages<M>(
  sessionId: string,
  activeSessionId: string | undefined,
  activeMessages: M[],
  liveBySession: Record<string, M[] | undefined>,
  readStored: (id: string) => M[],
  preferStored = false,
): M[] {
  const live = liveBySession[sessionId];
  if (live) return live;
  if (!preferStored && activeSessionId === sessionId) return activeMessages;
  return readStored(sessionId);
}

export function shouldClearDraft(
  isForced: boolean,
  hasExplicitSession: boolean,
  targetSessionId: string,
  activeSessionId: string | undefined,
): boolean {
  return !isForced && !hasExplicitSession && targetSessionId === activeSessionId;
}

export function readyQueueSessions<S extends SessionRef, P extends ProviderRef, Q>(
  queues: Record<string, Q>,
  runningById: Record<string, unknown>,
  activeSession: S | null | undefined,
  knownById: Record<string, S | undefined>,
  providers: P[],
): { id: string; session: S; provider: P; queue: Q }[] {
  const ready: { id: string; session: S; provider: P; queue: Q }[] = [];
  for (const [id, queue] of Object.entries(queues)) {
    if (runningById[id]) continue;
    const session = (activeSession?.id === id ? activeSession : knownById[id]) || null;
    if (!session) continue;
    const provider = providers.find(p => p.id === session.agente);
    if (!provider?.disponible) continue;
    ready.push({ id, session, provider, queue });
  }
  return ready;
}

export function updateRunningMap<T>(
  current: Record<string, T>,
  sessionId: string,
  controller: T | null,
  expectedCurrent?: T,
): Record<string, T> {
  if (controller === null) {
    if (expectedCurrent !== undefined && current[sessionId] !== expectedCurrent) return current;
    if (!(sessionId in current)) return current;
    const next = { ...current };
    delete next[sessionId];
    return next;
  }
  return { ...current, [sessionId]: controller };
}
