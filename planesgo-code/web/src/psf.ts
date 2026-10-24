// Automatic Planes Software Factory (PSF 1.0) session for a GitHub issue: the state stored in the session, the
// strip steps, the prompt of each role and the Approver decisions. Pure functions so they can be tested:
// node --test src/psf.test.ts

// Phases: start (ingestion, branch and PR) → spec (Spec Writer) → approval (waits for a person) → builder → review
// (4 reviewers in parallel) → approver → ready (PR ready; a person confirms the merge) → merged. blocked: no
// agreement after the iterations, or an ambiguous issue; error: something failed (it can be resumed).
// supervisor: the most capable model steps in on a conflict, failing checks or a committee with no agreement, and
// the change goes to the committee again.
export type PsfPhase = 'start' | 'spec' | 'approval' | 'builder' | 'supervisor' | 'review' | 'approver' | 'ready' | 'merged' | 'discarded' | 'blocked' | 'error';
export type SupervisorTask = 'conflict' | 'checks' | 'convergence';
export type ReviewerRole = 'correctness' | 'security' | 'testing' | 'architecture';
export type Verdict = 'APROBADO' | 'RECHAZADO';

export interface Review { verdict?: Verdict; findings?: string; running?: boolean; action?: string; cost?: number }

export interface PsfState {
  issue: number;
  title: string;
  url?: string;
  repo?: string;
  base?: string;          // remote default branch (the PR target)
  branch: string;         // psf/issue-<N>
  specPath: string;       // .psf/specs/issue-<N>-spec.md
  pr?: number;
  prUrl?: string;
  phase: PsfPhase;
  iteration: number;      // Builder round (from 1)
  maxIterations: number;
  reviews?: Partial<Record<ReviewerRole, Review>>;
  rejectedBy?: ReviewerRole[]; // who rejected in the previous round
  planPath?: string;      // the spec, as a pgocode plan (plan mode: review, edit, approve)
  error?: string;
  failedPhase?: PsfPhase; // where it was when it failed (to resume)
  blockedReason?: string;
  startedAt: number;
  endedAt?: number;
  reviewCost?: number;    // what the reviewers spent (they run outside the conversation)
  autoStarted?: boolean;  // started by the project's automatic resolution (not by a person)
  autoMergeHeld?: boolean; // automatic merge gave up (failing checks, conflicts…): a person decides
  heldReason?: 'conflict' | 'checks' | 'other'; // why it gave up (a conflict suggests what to tell the Builder)
  guidance?: string;       // what the person wrote to unblock it: goes with the next Spec Writer or Builder turn
  supervisorRuns?: number; // times the Supervisor stepped in (one per session: then a person decides)
  supervisorTask?: SupervisorTask;
  supervisorDetail?: string; // what it has to solve (failing checks, the pending findings…)
}

export const PSF_CONFIG = { specsPath: '.psf/specs/', branchPrefix: 'psf/issue-', maxIterations: 3, diffForReviewers: 60_000 } as const;

export const REVIEWERS: { role: ReviewerRole; name: string }[] = [
  { role: 'correctness', name: 'Correctness' },
  { role: 'security', name: 'Security' },
  { role: 'testing', name: 'Testing' },
  { role: 'architecture', name: 'Architecture' },
];

export function newPsfState(issue: number, title: string, url?: string): PsfState {
  return { issue, title, url, branch: PSF_CONFIG.branchPrefix + issue, specPath: `${PSF_CONFIG.specsPath}issue-${issue}-spec.md`,
    phase: 'start', iteration: 0, maxIterations: PSF_CONFIG.maxIterations, startedAt: Date.now() };
}

// ---- The strip: steps and their status ----

export type PsfStep = 'issue' | 'spec' | 'approval' | 'builder' | 'review' | 'approver' | 'ready';
export type StepStatus = 'done' | 'running' | 'waiting' | 'rejected' | 'error' | 'pending';

export const STEPS: { step: PsfStep; label: string }[] = [
  { step: 'issue', label: 'Issue' },
  { step: 'spec', label: 'Spec' },
  { step: 'approval', label: 'Aprobada' },
  { step: 'builder', label: 'Builder' },
  { step: 'review', label: 'Revisión' },
  { step: 'approver', label: 'Approver' },
  { step: 'ready', label: 'Merge' },
];

const PHASE_STEP: Record<PsfPhase, number> = { start: 0, spec: 1, approval: 2, builder: 3, supervisor: 3, review: 4, approver: 5, ready: 6, merged: 7, discarded: 6, blocked: 5, error: 0 };

// stepOf: the strip step the session is at (blocked: the Approver; error: where it failed).
export function stepOf(s: PsfState): number {
  if (s.phase === 'error') return PHASE_STEP[s.failedPhase || 'start'];
  if (s.phase === 'blocked' && s.blockedReason === 'ambiguity') return PHASE_STEP.spec;
  return PHASE_STEP[s.phase];
}

export function stepStatus(s: PsfState, step: PsfStep): StepStatus {
  const i = STEPS.findIndex(x => x.step === step);
  const cur = stepOf(s);
  if (i < cur) return 'done';
  if (i > cur) return 'pending';
  switch (s.phase) {
    case 'approval': case 'ready': return 'waiting';
    case 'blocked': case 'discarded': return 'rejected';
    case 'error': return 'error';
    default: return 'running';
  }
}

// statusText: where the session is, in words (the strip's tooltip: the strip itself stays visual).
export function statusText(s: PsfState, busy: boolean): string {
  const approved = REVIEWERS.filter(r => s.reviews?.[r.role]?.verdict === 'APROBADO').length;
  const idle = !busy && isWorkingPhase(s.phase) ? ' · parada: «Reanudar» sigue desde aquí' : '';
  switch (s.phase) {
    case 'start': return 'Preparando la rama y el PR' + idle;
    case 'spec': return 'El Spec Writer escribe la especificación' + idle;
    case 'approval': return 'Espera tu aprobación de la spec: «Ejecutar» en la tarjeta del plan';
    case 'builder': return `El Builder implementa la spec (iteración ${Math.max(1, s.iteration)}/${s.maxIterations})` + idle;
    case 'supervisor': return 'El Supervisor resuelve el bloqueo (' + ({ conflict: 'conflictos', checks: 'checks fallando', convergence: 'comité sin acuerdo' } as const)[s.supervisorTask || 'conflict'] + ')' + idle;
    case 'review': return `Comité de revisión: ${approved} de 4 aprobados` + idle;
    case 'approver': return 'El Approver decide' + idle;
    case 'ready':
      if (!s.autoMergeHeld) return 'PR listo: falta el merge';
      return s.heldReason === 'conflict' ? 'Merge parado: el PR tiene conflictos con ' + (s.base || 'la rama base')
        : s.heldReason === 'checks' ? 'Merge parado: hay checks de GitHub fallando' : 'Merge parado: decide cómo seguir';
    case 'merged': return `Fusionado en ${s.base || 'la rama base'}`;
    case 'discarded': return 'Descartada: la issue se cerró como «no se hará» y el PR, sin fusionar';
    case 'blocked': return s.blockedReason === 'ambiguity' ? 'Bloqueada: la issue es ambigua (las preguntas están en el PR)'
      : `Bloqueada: el comité no se pone de acuerdo tras ${s.iteration} iteraciones`;
    case 'error': return 'Parada: ' + (s.error || 'ha fallado algo');
  }
}

// decisionLabel: the button that applies the person's decision.
export const decisionLabel = (s: Pick<PsfState, 'phase' | 'blockedReason'>) =>
  s.phase === 'error' ? 'Reanudar' : s.phase === 'blocked' && s.blockedReason === 'ambiguity' ? 'Rehacer la spec' : 'Otra iteración';

// The session works on its own in this phase (if nothing is running, it was left halfway: it can be resumed).
export const isWorkingPhase = (p: PsfPhase) => p === 'start' || p === 'spec' || p === 'builder' || p === 'supervisor' || p === 'review' || p === 'approver';
// Waits for a person (amber badge).
export const isWaitingPhase = (p: PsfPhase) => p === 'approval' || p === 'ready' || p === 'blocked';

export const PHASE_LABEL: Record<PsfPhase, string> = {
  start: 'preparando', spec: 'spec', approval: 'aprobar', builder: 'builder', supervisor: 'supervisor', review: 'revisión', approver: 'approver',
  ready: 'merge', merged: 'fusionado', discarded: 'descartada', blocked: 'bloqueada', error: 'error',
};

// ---- Prompts ----

export interface IssueComment { author: string; body: string }
export interface IssueDetail { number: number; title: string; body: string; url: string; labels: string[]; comments: IssueComment[] }

const cut = (t: string, n: number) => (t.length > n ? t.slice(0, n) + '\n… (recortado)' : t);

export function issueText(i: IssueDetail): string {
  const c = i.comments.length ? '\n\nComentarios:\n' + i.comments.map(x => `- @${x.author}: ${cut(x.body.trim(), 2000)}`).join('\n') : '';
  return `Issue #${i.number}: ${i.title}${i.labels.length ? `\nEtiquetas: ${i.labels.join(', ')}` : ''}\n${i.url}\n\n${cut(i.body.trim() || '(sin descripción)', 12_000)}${cut(c, 8000)}`;
}

export const AMBIGUITY_MARK = 'BLOQUEADA POR AMBIGÜEDAD';

// Spec Writer (phase 2): read-only; the answer is the spec (saved as a pgocode plan).
// guidanceBlock: the person's instructions, first in priority.
const guidanceBlock = (g?: string) => (g?.trim() ? `\n\nIndicaciones de la persona (tienen prioridad sobre todo lo demás):\n${g.trim()}` : '');

export function specPrompt(i: IssueDetail, specPath: string, guidance?: string): string {
  return `[PSF · Spec Writer] Eres el arquitecto de especificaciones técnicas de Planes Software Factory. Analiza el repositorio ` +
    `(estructura, módulos, patrones, configuración, modelos de datos y tests) sin modificar ningún fichero ni ejecutar nada que ` +
    `cambie el sistema (para leer, usa tus herramientas de lectura de ficheros y de búsqueda; si ejecutas órdenes, que sean solo ` +
    `de lectura, como ls o grep). La fuente de verdad es el propio repositorio: no inventes convenciones y reutiliza lo que ya existe.\n\n` +
    `Responde únicamente con la especificación en Markdown (se guardará en ${specPath}), con un título «#» y estas secciones:\n` +
    `1. Objetivo y alcance (y lo que queda fuera)\n2. Contexto extraído del repositorio (patrones y helpers que se reutilizan)\n` +
    `3. Archivos afectados (rutas exactas, a crear o modificar)\n4. Casos borde\n5. Plan de pruebas (TDD, con el framework que ya usa el repositorio)\n` +
    `6. Checklist de criterios de aceptación (verificables uno a uno)\n\n` +
    `Si la issue es contradictoria o le falta información vital que no se puede deducir del código, responde solo con una ` +
    `línea «${AMBIGUITY_MARK}» seguida de las preguntas concretas.${guidanceBlock(guidance)}\n\n${issueText(i)}`;
}

export const isAmbiguous = (spec: string) => spec.trim().toUpperCase().startsWith(AMBIGUITY_MARK);

// Builder (phase 3): TDD with the approved spec; in later rounds, the findings to fix.
export function builderPrompt(i: Pick<IssueDetail, 'number' | 'title'>, spec: string, iteration: number, findings = '', guidance?: string): string {
  const reglas = `Reglas: el código tiene que ser indistinguible del resto del repositorio (estilo, nombres, errores, arquitectura). ` +
    `Ciclo TDD: primero los tests de la especificación (que fallen por la razón esperada), luego la implementación mínima y el refactor. ` +
    `Ejecuta el linter, el chequeo de tipos y la suite de tests del repositorio: no termines con nada en rojo. Si el cambio altera ` +
    `el comportamiento público o la configuración, actualiza la documentación del repositorio. No toques ${PSF_CONFIG.specsPath} ` +
    `y no hagas push: pgocode commitea lo pendiente y lo sube al PR al acabar el turno. Termina con un resumen breve de lo hecho y del resultado de las pruebas.`;
  if (iteration <= 1) {
    return `[PSF · Builder] Implementa la issue #${i.number} «${i.title}» según esta especificación, aprobada por una persona.${guidanceBlock(guidance)}\n\n${reglas}\n\n` +
      `Especificación aprobada:\n\n${spec.trim()}`;
  }
  const what = findings.trim()
    ? `El comité de revisión ha rechazado el cambio de la issue #${i.number}. Corrige exactamente estos hallazgos sin salirte de la especificación (ya la tienes de la vuelta anterior):\n\n${findings.trim()}`
    : `Vuelve sobre el cambio de la issue #${i.number} sin salirte de la especificación (ya la tienes de la vuelta anterior).`;
  return `[PSF · Builder · iteración ${iteration}] ${what}${guidanceBlock(guidance)}\n\n${reglas}`;
}

const MISSION: Record<ReviewerRole, string> = {
  correctness: 'Correctitud y especificación. Audita que la diff cumpla el 100% de la especificación aprobada. Rechaza si hay requisitos omitidos o a medias (TODO), bugs lógicos, condiciones de carrera o casos borde de la spec sin manejar.',
  security: 'Seguridad y permisos. Rechaza si hay inyecciones (SQL, XSS, órdenes), entradas sin validar, secretos o tokens en el código, datos sensibles en logs, escalada de privilegios o se rompe la autenticación/autorización existente.',
  testing: 'Calidad de tests. Rechaza si los tests solo prueban mocks y no el comportamiento real, faltan tests de los caminos nuevos o de error, o se han borrado o desactivado (skip) tests existentes.',
  architecture: 'Arquitectura y coherencia. Rechaza si duplica funciones o componentes que ya existen en el repositorio, hay sobreingeniería no pedida, se rompe la estructura o los patrones del repositorio o se añaden dependencias sin justificar.',
};

// Reviewer (phase 4): isolated and read-only; gets the issue, the spec and the diff, and answers with a verdict.
export function reviewerPrompt(role: ReviewerRole, i: Pick<IssueDetail, 'number' | 'title'>, spec: string, diff: string, base: string, branch: string): string {
  const name = REVIEWERS.find(r => r.role === role)!.name;
  return `[PSF · Reviewer-${name}] Eres uno de los 4 revisores independientes de Planes Software Factory. Revisa en solo lectura ` +
    `(no modifiques nada) el cambio de la issue #${i.number} «${i.title}»: la diff ${base}...${branch} de abajo frente a la ` +
    `especificación aprobada. Puedes leer el repositorio completo para comprobar el contexto.\n\nTu misión: ${MISSION[role]}\n\n` +
    `No ejecutes tests, compilaciones ni git: el Builder ya los ha pasado en verde y la diff la tienes abajo; para el ` +
    `contexto, lee los ficheros que necesites. Rechaza solo por problemas reales de tu misión, no por gustos. ` +
    `Responde exactamente con este formato:\n` +
    `VEREDICTO: APROBADO o VEREDICTO: RECHAZADO\nHALLAZGOS:\n- fichero:línea — problema y cómo corregirlo (o «ninguno»)\n\n` +
    `Especificación aprobada:\n\n${spec.trim()}\n\nDiff:\n\n\`\`\`diff\n${diff.trim() || '(vacía)'}\n\`\`\``;
}

// parseVerdict: a reviewer's verdict. For safety, anything without a clear «VEREDICTO: APROBADO» (or with any
// RECHAZADO) counts as a rejection.
export function parseVerdict(text: string): { verdict: Verdict; findings: string } {
  const re = /VEREDICTO\s*:?\s*[*_`[\s]*(APROBADO|RECHAZADO)\b(?!\s*\|)/gi;
  const all = [...text.matchAll(re)].map(m => m[1].toUpperCase());
  const h = text.search(/HALLAZGOS\s*:?/i);
  const findings = (h >= 0 ? text.slice(h).replace(/^HALLAZGOS\s*:?\s*/i, '') : text).trim();
  if (!all.length) return { verdict: 'RECHAZADO', findings: 'El revisor no ha dado un veredicto con el formato pedido.\n\n' + cut(text.trim(), 4000) };
  return { verdict: all.includes('RECHAZADO') ? 'RECHAZADO' : 'APROBADO', findings };
}

// conflictGuidance: what to tell the Builder when the PR conflicts with the base branch.
export const conflictGuidance = (base: string) =>
  `El PR tiene conflictos con ${base}. pgocode ya ha traído origin/${base} (no hagas git fetch: puede que no tengas red). ` +
  `Integra origin/${base} en la rama (git merge origin/${base}), resuelve los ` +
  `conflictos conservando lo que ya hay en ${base} y lo de esta issue, y deja tests y linter en verde. Haz tú el commit del merge.`;

// needsDecision: the session is stopped and a person has to say how to go on (the strip shows where to write it).
export const needsDecision = (s: Pick<PsfState, 'phase' | 'autoMergeHeld'>) => s.phase === 'blocked' || s.phase === 'error' || (s.phase === 'ready' && Boolean(s.autoMergeHeld));

// gaveUp: the reviewer's agent ended without an answer (e.g. its mode did not let it run a command): that is a
// failed run, not a verdict, so it must not cost a Builder round (it is resumed instead).
export const gaveUp = (text: string) => !/VEREDICTO/i.test(text) && (!text.trim() || /ha acabado sin responder/i.test(text));

// decide: what the Approver does with the 4 verdicts.
export function decide(reviews: Partial<Record<ReviewerRole, Review>>, iteration: number, max: number): 'ready' | 'retry' | 'blocked' {
  const all = REVIEWERS.map(r => reviews[r.role]?.verdict);
  if (all.every(v => v === 'APROBADO')) return 'ready';
  return iteration < max ? 'retry' : 'blocked';
}

export const rejected = (reviews: Partial<Record<ReviewerRole, Review>>) => REVIEWERS.filter(r => reviews[r.role]?.verdict !== 'APROBADO').map(r => r.role);

// findingsFor: the report for the Builder with what the rejecting reviewers found.
export function findingsFor(reviews: Partial<Record<ReviewerRole, Review>>): string {
  return rejected(reviews).map(r => `## Reviewer-${REVIEWERS.find(x => x.role === r)!.name}\n${(reviews[r]?.findings || '').trim() || '(sin detalle)'}`).join('\n\n');
}

// approverSummary: the audit summary (for the chat and the PR).
export function approverSummary(s: Pick<PsfState, 'iteration' | 'maxIterations' | 'reviews'>, outcome: 'ready' | 'retry' | 'blocked'): string {
  const r = s.reviews || {};
  const lineas = REVIEWERS.map(x => `- Reviewer-${x.name}: ${r[x.role]?.verdict === 'APROBADO' ? '✅ APROBADO' : '❌ RECHAZADO'}`).join('\n');
  const que = outcome === 'ready' ? 'Los 4 revisores aprueban: el PR pasa a «Ready». El merge lo confirma una persona.'
    : outcome === 'retry' ? `Hay rechazos: vuelve al Builder (iteración ${s.iteration + 1}/${s.maxIterations}).`
    : `Sin acuerdo tras ${s.maxIterations} iteraciones: se para sin merge y queda para revisión humana.`;
  const detalle = outcome === 'ready' ? '' : '\n\n' + findingsFor(r);
  return `### PSF · Approver (iteración ${s.iteration}/${s.maxIterations})\n\n${lineas}\n\n${que}${detalle}`;
}

// bestModel: the Supervisor's model: the most capable one of the agent (otherwise the first of its list, which
// starts with the best, or the default one).
export function bestModel(models: readonly string[] | null | undefined, fallback: string): string {
  const l = models || [];
  for (const k of ['opus', 'argon-high', 'pro-high', 'pro']) {
    const m = l.find(x => x.toLowerCase().includes(k));
    if (m) return m;
  }
  return l[0] || fallback;
}

// supervisorPrompt: the Supervisor steps in on what the usual flow cannot solve by itself.
export function supervisorPrompt(task: SupervisorTask, i: Pick<IssueDetail, 'number' | 'title'>, spec: string, base: string, detail: string): string {
  const reglas = `Reglas: respeta la especificación aprobada y el estilo del repositorio; ejecuta el linter, el chequeo de tipos y la ` +
    `suite de tests: no termines con nada en rojo. Haz tú los commits que necesites y no hagas push: pgocode lo sube al PR. ` +
    `Termina con un resumen breve de qué has hecho y por qué.`;
  const what = task === 'conflict'
    ? `El PR choca con ${base}. pgocode ya ha traído origin/${base} (no hagas git fetch: puede que no tengas red). ` +
      `Integra origin/${base} en la rama (git merge origin/${base}), resuelve los conflictos ` +
      `conservando lo que ya hay en ${base} y lo de esta issue, y haz el commit del merge.`
    : task === 'checks'
      ? `Los checks de GitHub del PR fallan (${detail.trim() || 'sin detalle'}). Reprodúcelos en local con las órdenes del repositorio, ` +
        `encuentra la causa y corrígela.`
      : `El comité de revisión no se pone de acuerdo tras varias iteraciones. Actúa como árbitro: separa los hallazgos válidos ` +
        `(según la especificación y el repositorio) de los que no lo son, corrige los válidos y explica en tu resumen por qué ` +
        `descartas el resto.\n\nHallazgos pendientes:\n\n${detail.trim()}`;
  return `[PSF · Supervisor] Eres el supervisor de Planes Software Factory para la issue #${i.number} «${i.title}»: intervienes ` +
    `cuando el flujo normal se atasca. ${what}\n\n${reglas}\n\nEspecificación aprobada:\n\n${spec.trim()}`;
}

// cheapModel: the reviewers' model: the cheapest in the agent's list (otherwise the default one).
export function cheapModel(models: readonly string[] | null | undefined, fallback: string): string {
  const l = models || [];
  for (const k of ['haiku', 'flash-lite', 'flash', 'mini', 'sonnet']) {
    const m = l.find(x => x.toLowerCase().includes(k));
    if (m) return m;
  }
  return fallback;
}

export const commitMessage = (issue: number, iteration: number) =>
  iteration <= 1 ? `feat(issue-${issue}): implement spec` : `fix(issue-${issue}): address review findings (iter ${iteration})`;

// elapsed: «23 min» since it started (until it ended, if it did).
export function elapsed(s: Pick<PsfState, 'startedAt' | 'endedAt'>, now = Date.now()): string {
  const min = Math.max(0, Math.round(((s.endedAt || now) - s.startedAt) / 60_000));
  return min < 60 ? `${min} min` : `${Math.floor(min / 60)} h ${min % 60} min`;
}

// SHOW_EVIDENCE: event that opens the side panel on the TV with the open session's pictures (the strip's «Fotos»).
export const SHOW_EVIDENCE = 'pgo:ver-fotos';

// ---- Automatic resolution of new issues (project «Issues» settings) ----

// PSF_CHECK: event that asks for an immediate check of new issues (after creating one with «/issue», or when the
// automatic resolution is turned on), instead of waiting for the next 5-minute round.
export const PSF_CHECK = 'pgo:psf-check';

export interface WatchedIssue { numero: number; titulo: string; url: string; estado: string; fase: string; creada: string; prs: unknown[] }
interface SessionLike { psf?: Pick<PsfState, 'issue' | 'phase'>; archivada?: boolean }

// issuesToResolve: the new issues to start on their own, oldest first: open, not started (no PR), created after
// the option was turned on and without an automatic session yet (archived ones count: they were already handled).
export function issuesToResolve(issues: readonly WatchedIssue[], sessions: readonly SessionLike[], since: number): WatchedIssue[] {
  const taken = new Set(sessions.filter(s => s.psf).map(s => s.psf!.issue));
  return issues
    .filter(i => i.estado === 'OPEN' && i.fase === 'por-empezar' && !i.prs.length && Date.parse(i.creada) >= since && !taken.has(i.numero))
    .sort((a, b) => Date.parse(a.creada) - Date.parse(b.creada));
}

// canStartAutomatic: one automatic session working at a time per project (no runaway agent processes).
export const canStartAutomatic = (sessions: readonly SessionLike[]) => !sessions.some(s => s.psf && !s.archivada && isWorkingPhase(s.psf.phase));
