import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  readyQueueSessions,
  resolveTurnSession,
  sessionMessages,
  shouldClearDraft,
  updateRunningMap,
  type ProviderRef,
  type SessionRef,
} from './parallelSession.ts';

const sesProject1: SessionRef = {
  id: 's-proj-1',
  proyecto: 10,
  agente: 'claude-code',
  carpeta: '/repo/p1',
  rama: 'main',
  hosting: 'github',
};

const sesProject2: SessionRef = {
  id: 's-proj-2',
  proyecto: 20,
  agente: 'antigravity',
  carpeta: '/repo/p2',
  rama: 'dev',
  hosting: 'github',
};

const providers: ProviderRef[] = [
  { id: 'claude-code', disponible: true },
  { id: 'antigravity', disponible: true },
];

test('resolveTurnSession: interactive turns require matching active project, background turns keep their own project', () => {
  assert.equal(resolveTurnSession(sesProject1, undefined, 10)?.id, 's-proj-1');
  assert.equal(resolveTurnSession(sesProject1, undefined, 20), null);
  assert.equal(resolveTurnSession(sesProject2, sesProject1, 20)?.proyecto, 10);
  assert.equal(resolveTurnSession(null, sesProject1, null)?.carpeta, '/repo/p1');
});

test('sessionMessages: never leaks active session messages into a background session from another project', () => {
  const stored: Record<string, string[]> = {
    's-proj-1': ['msg-p1-stored'],
    's-proj-2': ['msg-p2-stored'],
  };
  const live: Record<string, string[] | undefined> = {};
  const activeMessages = ['msg-p2-screen'];

  assert.deepEqual(
    sessionMessages('s-proj-1', 's-proj-2', activeMessages, live, id => stored[id] || []),
    ['msg-p1-stored'],
  );
  assert.deepEqual(
    sessionMessages('s-proj-2', 's-proj-2', activeMessages, live, id => stored[id] || []),
    ['msg-p2-screen'],
  );
  live['s-proj-1'] = ['msg-p1-live'];
  assert.deepEqual(
    sessionMessages('s-proj-1', 's-proj-2', activeMessages, live, id => stored[id] || []),
    ['msg-p1-live'],
  );
});

test('shouldClearDraft: only clears input when the interactive send is still on the active session', () => {
  assert.equal(shouldClearDraft(false, false, 's-proj-1', 's-proj-1'), true);
  assert.equal(shouldClearDraft(false, false, 's-proj-1', 's-proj-2'), false);
  assert.equal(shouldClearDraft(false, true, 's-proj-1', 's-proj-1'), false);
  assert.equal(shouldClearDraft(true, false, 's-proj-1', 's-proj-1'), false);
});

test('readyQueueSessions: drains queues of background projects while another project is open', () => {
  const queues = { 's-proj-1': ['queued-1'], 's-proj-2': ['queued-2'] };
  const known = { 's-proj-1': sesProject1, 's-proj-2': sesProject2 };

  const whileP1Running = readyQueueSessions(queues, { 's-proj-1': {} }, sesProject2, known, providers);
  assert.deepEqual(whileP1Running.map(x => x.id), ['s-proj-2']);

  const afterP1Finishes = readyQueueSessions(queues, {}, sesProject2, known, providers);
  assert.deepEqual(afterP1Finishes.map(x => x.id), ['s-proj-1', 's-proj-2']);
});

test('updateRunningMap: synchronous start and guarded finish', () => {
  const c1 = { id: 1 };
  const c2 = { id: 2 };
  let map = updateRunningMap<{ id: number }>({}, 's-proj-1', c1);
  assert.equal(map['s-proj-1'], c1);
  map = updateRunningMap(map, 's-proj-1', null, c2);
  assert.equal(map['s-proj-1'], c1);
  map = updateRunningMap(map, 's-proj-1', null, c1);
  assert.equal('s-proj-1' in map, false);
});
