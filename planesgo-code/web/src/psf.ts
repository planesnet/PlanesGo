// Automatic Planes Software Factory (PSF 1.0) session for a GitHub issue: the state stored in the session, the
// strip steps, the prompt of each role and the Approver decisions. Pure functions so they can be tested:
// node --test src/psf.test.ts

// Phases: start (ingestion, branch and PR) → spec (Spec Writer) → approval (waits for a person) → builder → review
// (4 reviewers in parallel) → approver → ready (PR ready; a person confirms the merge) → merged. blocked: no
// agreement after the iterations, or an ambiguous issue; error: something failed (it can be resumed).
export type PsfPhase = 'start' | 'spec' | 'approval' | 'builder' | 'review' | 'approver' | 'ready' | 'merged' | 'blocked' | 'error';
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
  { step: 'ready', label: 'Ready' },
];

const PHASE_STEP: Record<PsfPhase, number> = { start: 0, spec: 1, approval: 2, builder: 3, review: 4, approver: 5, ready: 6, merged: 7, blocked: 5, error: 0 };

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
    case 'blocked': return 'rejected';
    case 'error': return 'error';
    default: return 'running';
  }
}

// The session works on its own in this phase (if nothing is running, it was left halfway: it can be resumed).
export const isWorkingPhase = (p: PsfPhase) => p === 'start' || p === 'spec' || p === 'builder' || p === 'review' || p === 'approver';
// Waits for a person (amber badge).
export const isWaitingPhase = (p: PsfPhase) => p === 'approval' || p === 'ready' || p === 'blocked';

export const PHASE_LABEL: Record<PsfPhase, string> = {
  start: 'preparando', spec: 'spec', approval: 'aprobar spec', builder: 'builder', review: 'revisión', approver: 'approver',
  ready: 'listo para merge', merged: 'fusionado', blocked: 'bloqueada', error: 'error',
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
export function specPrompt(i: IssueDetail, specPath: string): string {
  return `[PSF · Spec Writer] Eres el arquitecto de especificaciones técnicas de Planes Software Factory. Analiza el repositorio ` +
    `(estructura, módulos, patrones, configuración, modelos de datos y tests) sin modificar ningún fichero ni ejecutar nada que ` +
    `cambie el sistema. La fuente de verdad es el propio repositorio: no inventes convenciones y reutiliza lo que ya existe.\n\n` +
    `Responde únicamente con la especificación en Markdown (se guardará en ${specPath}), con un título «#» y estas secciones:\n` +
    `1. Objetivo y alcance (y lo que queda fuera)\n2. Contexto extraído del repositorio (patrones y helpers que se reutilizan)\n` +
    `3. Archivos afectados (rutas exactas, a crear o modificar)\n4. Casos borde\n5. Plan de pruebas (TDD, con el framework que ya usa el repositorio)\n` +
    `6. Checklist de criterios de aceptación (verificables uno a uno)\n\n` +
    `Si la issue es contradictoria o le falta información vital que no se puede deducir del código, responde solo con una ` +
    `línea «${AMBIGUITY_MARK}» seguida de las preguntas concretas.\n\n${issueText(i)}`;
}

export const isAmbiguous = (spec: string) => spec.trim().toUpperCase().startsWith(AMBIGUITY_MARK);

// Builder (phase 3): TDD with the approved spec; in later rounds, the findings to fix.
export function builderPrompt(i: Pick<IssueDetail, 'number' | 'title'>, spec: string, iteration: number, findings = ''): string {
  const reglas = `Reglas: el código tiene que ser indistinguible del resto del repositorio (estilo, nombres, errores, arquitectura). ` +
    `Ciclo TDD: primero los tests de la especificación (que fallen por la razón esperada), luego la implementación mínima y el refactor. ` +
    `Ejecuta el linter, el chequeo de tipos y la suite de tests del repositorio: no termines con nada en rojo. Si el cambio altera ` +
    `el comportamiento público o la configuración, actualiza la documentación del repositorio. No toques ${PSF_CONFIG.specsPath} ` +
    `y no hagas push: pgocode commitea lo pendiente y lo sube al PR al acabar el turno. Termina con un resumen breve de lo hecho y del resultado de las pruebas.`;
  if (iteration <= 1) {
    return `[PSF · Builder] Implementa la issue #${i.number} «${i.title}» según esta especificación, aprobada por una persona.\n\n${reglas}\n\n` +
      `Especificación aprobada:\n\n${spec.trim()}`;
  }
  return `[PSF · Builder · iteración ${iteration}] El comité de revisión ha rechazado el cambio de la issue #${i.number}. ` +
    `Corrige exactamente estos hallazgos sin salirte de la especificación (ya la tienes de la vuelta anterior):\n\n${findings.trim()}\n\n${reglas}`;
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

// ---- Automatic resolution of new issues (project «Issues» settings) ----

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
