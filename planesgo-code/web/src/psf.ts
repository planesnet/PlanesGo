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

// Review: one reviewer's answer. findings: what blocks (to fix); improvements: what does not block; decisions: what is
// for the person (scope, design).
export interface Review { verdict?: Verdict; findings?: string; improvements?: string; decisions?: string; running?: boolean; action?: string; cost?: number }

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

// ---- What each agent is told ----
// The know-how of each role (how to write a spec, the Builder's rules, the reviewers' criteria) lives in pgocode's
// system skills (internal/sysskills: SKILL.md and chapters, read only, apart from any user or project skill). The
// server leaves the skill in the agent's folder and the prompt carries its essentials and the index of its chapters;
// the agent reads only the chapters its task needs. What is left here is the protocol: what the agent works on, what
// it may not touch and the format of its answer, which pgocode reads.

// SystemSkill: a system skill as the server gives it (POST /api/psf/skill), installed in the agent's folder.
export interface SystemSkill { name: string; description: string; body: string; chapters: { name: string; description: string; path: string }[] }

// ROLE_SKILL: the system skill of each role.
export const ROLE_SKILL = {
  spec: 'psf-spec-writer', builder: 'psf-builder', supervisor: 'psf-supervisor',
  correctness: 'psf-revisor-correccion', security: 'psf-revisor-seguridad', testing: 'psf-revisor-tests', architecture: 'psf-revisor-arquitectura',
} as const;

// skillBlock: the skill for the prompt: its essentials and, if it has chapters, where they are and when to read each.
export function skillBlock(sk: SystemSkill): string {
  const chapters = sk.chapters.length
    ? `\n\nCapítulos de esta skill (léelos con tu herramienta de lectura de ficheros, solo los que toquen a tu tarea; la línea dice cuándo):\n` +
      sk.chapters.map(c => `- ${c.path} — ${c.description}`).join('\n')
    : '';
  return `${sk.body.trim()}${chapters}`;
}

export const AMBIGUITY_MARK = 'BLOQUEADA POR AMBIGÜEDAD';

// guidanceBlock: the person's instructions, first in priority.
const guidanceBlock = (g?: string) => (g?.trim() ? `\n\nIndicaciones de la persona (tienen prioridad sobre todo lo demás):\n${g.trim()}` : '');

// Spec Writer (phase 2): read-only; the answer is the spec (saved as a pgocode plan).
export function specPrompt(i: IssueDetail, specPath: string, skill: SystemSkill, guidance?: string): string {
  return `[PSF · Spec Writer] Escribe la especificación técnica de esta issue. Analiza el repositorio sin modificar ningún ` +
    `fichero ni ejecutar nada que cambie el sistema (para leer, usa tus herramientas de lectura de ficheros y de búsqueda; si ` +
    `ejecutas órdenes, que sean solo de lectura, como ls o grep).\n\n${skillBlock(skill)}\n\n` +
    `Responde únicamente con la especificación en Markdown (se guardará en ${specPath}). Si la issue es contradictoria o le ` +
    `falta información vital que no se puede deducir del código, responde solo con una línea «${AMBIGUITY_MARK}» seguida de ` +
    `las preguntas concretas.${guidanceBlock(guidance)}\n\n${issueText(i)}`;
}

export const isAmbiguous = (spec: string) => spec.trim().toUpperCase().startsWith(AMBIGUITY_MARK);

// Builder (phase 3): TDD with the approved spec; in later rounds, the findings to fix (the skill and the spec are
// already in its conversation: they are not sent again).
export function builderPrompt(i: Pick<IssueDetail, 'number' | 'title'>, spec: string, iteration: number, skill: SystemSkill | null, findings = '', guidance?: string): string {
  const protocol = `No toques ${PSF_CONFIG.specsPath} ni ${SKILLS_DIR} y no hagas push: pgocode commitea lo pendiente y lo sube al PR al acabar el turno.`;
  if (iteration <= 1) {
    return `[PSF · Builder] Implementa la issue #${i.number} «${i.title}» según esta especificación, aprobada por una persona.${guidanceBlock(guidance)}\n\n` +
      `${skill ? skillBlock(skill) + '\n\n' : ''}${protocol}\n\nEspecificación aprobada:\n\n${spec.trim()}`;
  }
  const what = findings.trim()
    ? `El comité de revisión ha rechazado el cambio de la issue #${i.number}. Corrige exactamente estos hallazgos sin salirte de la especificación (ya la tienes de la vuelta anterior):\n\n${findings.trim()}`
    : `Vuelve sobre el cambio de la issue #${i.number} sin salirte de la especificación (ya la tienes de la vuelta anterior).`;
  return `[PSF · Builder · iteración ${iteration}] ${what}${guidanceBlock(guidance)}\n\n${protocol}`;
}

// SKILLS_DIR: where the server leaves the system skills in the agent's folder (internal/sysskills.Dir).
export const SKILLS_DIR = '.pgocode/skills-sistema/';

// projectCriteria: where a repository keeps what its reviewers have to know on top of the system skills (its threat
// model, its own rules). They are the project's documents, not skills.
export const PSF_CRITERIA = '.psf/criterios/';

// Reviewer (phase 4): isolated and read-only; gets the issue, the spec and the diff, and answers with a verdict.
// Only real, likely problems block (severity alta or media, confidence 8 or more); the rest are improvements that do
// not cost a round, and what is a decision for the person (scope, design) is said apart.
export function reviewerPrompt(role: ReviewerRole, i: Pick<IssueDetail, 'number' | 'title'>, skill: SystemSkill, spec: string, diff: string, base: string, branch: string): string {
  const name = REVIEWERS.find(r => r.role === role)!.name;
  return `[PSF · Reviewer-${name}] Eres uno de los 4 revisores independientes de Planes Software Factory. Revisa en solo lectura ` +
    `(no modifiques nada) el cambio de la issue #${i.number} «${i.title}»: la diff ${base}...${branch} de abajo frente a la ` +
    `especificación aprobada. Puedes leer el repositorio completo para comprobar el contexto.\n\nTus criterios:\n\n${skillBlock(skill)}\n\n` +
    `Antes de revisar, lee los criterios propios del proyecto si los hay (${PSF_CRITERIA}, y CLAUDE.md o AGENTS.md en la raíz y en ` +
    `las carpetas de los ficheros cambiados): mandan sobre los generales.\n\n` +
    `No ejecutes tests, compilaciones ni git: el Builder ya los ha pasado en verde y la diff la tienes abajo; para el ` +
    `contexto, lee los ficheros que necesites. Comprueba cada hallazgo leyendo el código antes de darlo, y puntúa: gravedad ` +
    `(alta: explotable o rompe algo de verdad; media: necesita condiciones concretas pero el efecto es serio; baja: el resto) y ` +
    `confianza de 1 a 10 en que es un problema real. RECHAZA solo si hay algún hallazgo de tu misión de gravedad alta o media con ` +
    `confianza 8 o más; lo demás no bloquea: ponlo en MEJORAS. Lo que no es un fallo sino una decisión de alcance o de diseño ` +
    `para la persona va en DECISIONES. No rechaces por gustos. Responde exactamente con este formato:\n` +
    `VEREDICTO: APROBADO o VEREDICTO: RECHAZADO\nHALLAZGOS:\n- [alta|media] (confianza N) fichero:línea — problema, cómo se produce y cómo corregirlo (o «ninguno»)\n` +
    `MEJORAS:\n- fichero:línea — mejora que no bloquea (o «ninguna»)\nDECISIONES:\n- decisión para la persona (o «ninguna»)\n\n` +
    `Especificación aprobada:\n\n${spec.trim()}\n\nDiff:\n\n\`\`\`diff\n${diff.trim() || '(vacía)'}\n\`\`\``;
}

// section: the lines of a reviewer's answer under one heading (until the next one), without «ninguno/ninguna».
function section(text: string, name: string): string[] {
  const m = new RegExp(`^\\W*${name}\\W*:?[ \\t]*$`, 'im').exec(text) || new RegExp(`${name}\\s*:`, 'i').exec(text);
  if (!m) return [];
  const rest = text.slice(m.index + m[0].length);
  const next = /^\W*(?:VEREDICTO|HALLAZGOS|MEJORAS|DECISIONES)\b/im.exec(rest);
  return (next ? rest.slice(0, next.index) : rest).split('\n').map(l => l.trimEnd())
    .filter(l => l.trim() && !/^[-*\s]*(ningun[oa]s?|none)\.?$/i.test(l.trim()));
}

// rating: the severity and confidence of a finding («- [alta] (confianza 9) …»); null if it has none.
export function rating(line: string): { severity: 'alta' | 'media' | 'baja'; confidence: number } | null {
  const s = /\[?\b(alta|media|baja)\b\]?/i.exec(line), c = /confianza\D{0,3}(\d{1,2})/i.exec(line);
  if (!s || !c) return null;
  return { severity: s[1].toLowerCase() as 'alta' | 'media' | 'baja', confidence: Number(c[1]) };
}

// blocks: a rated finding that stops the change (severity alta or media, confidence 8 or more).
const blocks = (r: NonNullable<ReturnType<typeof rating>>) => r.severity !== 'baja' && r.confidence >= 8;

// parseVerdict: a reviewer's verdict. For safety, anything without a clear «VEREDICTO: APROBADO» (or with any
// RECHAZADO) counts as a rejection. When the findings are rated, the rule decides, not the word: a rejection whose
// findings are all low or uncertain is an approval (they go to the improvements), and an approval with a finding
// that blocks is a rejection.
export function parseVerdict(text: string): { verdict: Verdict; findings: string; improvements: string; decisions: string } {
  const re = /VEREDICTO\s*:?\s*[*_`[\s]*(APROBADO|RECHAZADO)\b(?!\s*\|)/gi;
  const all = [...text.matchAll(re)].map(m => m[1].toUpperCase());
  if (!all.length) return { verdict: 'RECHAZADO', findings: 'El revisor no ha dado un veredicto con el formato pedido.\n\n' + cut(text.trim(), 4000), improvements: '', decisions: '' };
  let verdict: Verdict = all.includes('RECHAZADO') ? 'RECHAZADO' : 'APROBADO';
  const found = section(text, 'HALLAZGOS');
  const improvements = section(text, 'MEJORAS');
  const decisions = section(text, 'DECISIONES').join('\n');
  // Each finding with its continuation lines (a finding can take several).
  const items = found.reduce<string[]>((l, line) => {
    if (/^\s*[-*]/.test(line) || !l.length) l.push(line); else l[l.length - 1] += '\n' + line;
    return l;
  }, []);
  const rated = items.map(rating);
  if (rated.length && rated.every(Boolean)) {
    const blocking = items.filter((_, i) => blocks(rated[i]!));
    improvements.unshift(...items.filter((_, i) => !blocks(rated[i]!)));
    verdict = blocking.length ? 'RECHAZADO' : 'APROBADO';
    return { verdict, findings: blocking.join('\n'), improvements: improvements.join('\n'), decisions };
  }
  // Not rated (an older format, or free text): the reviewer's word, with everything it found.
  const h = text.search(/HALLAZGOS\s*:?/i);
  const findings = found.length ? found.join('\n') : (h >= 0 ? text.slice(h).replace(/^HALLAZGOS\s*:?\s*/i, '') : text).trim();
  return { verdict, findings, improvements: improvements.join('\n'), decisions };
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

// findingsFor: the report for the Builder with what the rejecting reviewers found, and the improvements of all.
export function findingsFor(reviews: Partial<Record<ReviewerRole, Review>>): string {
  const blocking = rejected(reviews).map(r => `## Reviewer-${REVIEWERS.find(x => x.role === r)!.name}\n${(reviews[r]?.findings || '').trim() || '(sin detalle)'}`).join('\n\n');
  const better = listed(reviews, 'improvements');
  // The improvements ride along with a round that happens anyway (they never cost one of their own).
  return blocking + (better ? `\n\n## Mejoras que no bloquean (aplícalas solo si son sencillas y no salen de la especificación)\n${better}` : '');
}

// listed: what every reviewer put under one of its non-blocking sections, by reviewer.
export function listed(reviews: Partial<Record<ReviewerRole, Review>>, what: 'improvements' | 'decisions'): string {
  return REVIEWERS.map(x => ({ x, t: (reviews[x.role]?.[what] || '').trim() })).filter(e => e.t)
    .map(e => `- Reviewer-${e.x.name}:\n${e.t.split('\n').map(l => '  ' + l.trim()).join('\n')}`).join('\n');
}

// approverSummary: the audit summary (for the chat and the PR).
export function approverSummary(s: Pick<PsfState, 'iteration' | 'maxIterations' | 'reviews'>, outcome: 'ready' | 'retry' | 'blocked'): string {
  const r = s.reviews || {};
  const lineas = REVIEWERS.map(x => `- Reviewer-${x.name}: ${r[x.role]?.verdict === 'APROBADO' ? '✅ APROBADO' : '❌ RECHAZADO'}`).join('\n');
  const que = outcome === 'ready' ? 'Los 4 revisores aprueban: el PR pasa a «Ready». El merge lo confirma una persona.'
    : outcome === 'retry' ? `Hay rechazos: vuelve al Builder (iteración ${s.iteration + 1}/${s.maxIterations}).`
    : `Sin acuerdo tras ${s.maxIterations} iteraciones: se para sin merge y queda para revisión humana.`;
  const better = listed(r, 'improvements'), decisions = listed(r, 'decisions');
  const detalle = outcome === 'ready' ? (better ? `\n\n**Mejoras propuestas (no bloquean):**\n${better}` : '') : '\n\n' + findingsFor(r);
  const persona = decisions ? `\n\n**Decisiones para la persona:**\n${decisions}` : '';
  return `### PSF · Approver (iteración ${s.iteration}/${s.maxIterations})\n\n${lineas}\n\n${que}${detalle}${persona}`;
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
export function supervisorPrompt(task: SupervisorTask, i: Pick<IssueDetail, 'number' | 'title'>, skill: SystemSkill, spec: string, base: string, detail: string): string {
  const what = task === 'conflict'
    ? `El PR choca con ${base}. pgocode ya ha traído origin/${base} (no hagas git fetch: puede que no tengas red). ` +
      `Intégralo en la rama con git merge origin/${base}.`
    : task === 'checks'
      ? `Los checks de GitHub del PR fallan (${detail.trim() || 'sin detalle'}).`
      : `El comité de revisión no se pone de acuerdo tras varias iteraciones.\n\nHallazgos pendientes:\n\n${detail.trim()}`;
  return `[PSF · Supervisor] Intervienes en la issue #${i.number} «${i.title}». ${what}\n\n${skillBlock(skill)}\n\n` +
    `No toques ${PSF_CONFIG.specsPath} ni ${SKILLS_DIR} y no hagas push: pgocode lo sube al PR.\n\nEspecificación aprobada:\n\n${spec.trim()}`;
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

export interface WatchedIssue { numero: number; titulo: string; url: string; estado: string; fase: string; creada: string; prs: unknown[]; autor_confiable?: boolean }
interface SessionLike { psf?: Pick<PsfState, 'issue' | 'phase'>; archivada?: boolean }

// issuesToResolve: the new issues to start on their own, oldest first: open, not started (no PR), created after
// the option was turned on and without an automatic session yet (archived ones count: they were already handled).
// Only issues of people with a role in the repository: in a public one anyone can open an issue, and its text would
// go to an agent working on this computer (a stranger's one is started by hand, after reading it).
export function issuesToResolve(issues: readonly WatchedIssue[], sessions: readonly SessionLike[], since: number): WatchedIssue[] {
  const taken = new Set(sessions.filter(s => s.psf).map(s => s.psf!.issue));
  return issues
    .filter(i => i.autor_confiable && i.estado === 'OPEN' && i.fase === 'por-empezar' && !i.prs.length && Date.parse(i.creada) >= since && !taken.has(i.numero))
    .sort((a, b) => Date.parse(a.creada) - Date.parse(b.creada));
}

// canStartAutomatic: one automatic session working at a time per project (no runaway agent processes).
export const canStartAutomatic = (sessions: readonly SessionLike[]) => !sessions.some(s => s.psf && !s.archivada && isWorkingPhase(s.psf.phase));
