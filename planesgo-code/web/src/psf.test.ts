// Tests of the automatic PSF session: npm test (node --test, no dependencies).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import * as P from './psf.ts';

const issue: P.IssueDetail = { number: 7, title: 'Arreglar login', body: 'No entra', url: 'https://github.com/a/b/issues/7', labels: ['bug'], comments: [{ author: 'luis', body: 'Pasa en Firefox' }] };

test('estado inicial y pasos de la franja', () => {
  const s = P.newPsfState(7, 'Arreglar login');
  assert.equal(s.branch, 'psf/issue-7');
  assert.equal(s.specPath, '.psf/specs/issue-7-spec.md');
  assert.equal(P.stepStatus(s, 'issue'), 'running');
  assert.equal(P.stepStatus(s, 'spec'), 'pending');
  const r = { ...s, phase: 'review' as const };
  assert.equal(P.stepStatus(r, 'builder'), 'done');
  assert.equal(P.stepStatus(r, 'review'), 'running');
  assert.equal(P.stepStatus({ ...s, phase: 'approval' }, 'approval'), 'waiting');
  assert.equal(P.stepStatus({ ...s, phase: 'ready' }, 'ready'), 'waiting');
  assert.equal(P.stepStatus({ ...s, phase: 'merged' }, 'ready'), 'done');
  assert.equal(P.stepStatus({ ...s, phase: 'blocked' }, 'approver'), 'rejected');
  assert.equal(P.stepStatus({ ...s, phase: 'blocked', blockedReason: 'ambiguity' }, 'spec'), 'rejected');
  assert.equal(P.stepStatus({ ...s, phase: 'error', failedPhase: 'builder' }, 'builder'), 'error');
  assert.equal(P.stepStatus({ ...s, phase: 'error', failedPhase: 'builder' }, 'spec'), 'done');
});

// sk: a system skill as the server gives it (its essentials and where its chapters are).
const sk: P.SystemSkill = { name: 'psf-x', description: 'd', body: 'Checklist de criterios de aceptación; TDD; árbitro',
  chapters: [{ name: '1-a', description: 'Siempre.', path: '.pgocode/skills-sistema/psf-x/capitulos/1-a.md' }] };

test('prompts de cada rol', () => {
  const sp = P.specPrompt(issue, '.psf/specs/issue-7-spec.md', sk);
  assert.match(sp, /Spec Writer/);
  assert.match(sp, /sin modificar ningún fichero/);
  assert.match(sp, /Pasa en Firefox/);
  assert.match(sp, /Checklist de criterios de aceptación/);
  const b1 = P.builderPrompt(issue, '# Spec', 1, sk);
  assert.match(b1, /TDD/);
  assert.match(b1, /# Spec/);
  const b2 = P.builderPrompt(issue, '# Spec', 2, null, '- a.go:3 — falta test');
  assert.match(b2, /iteración 2/);
  assert.match(b2, /falta test/);
  assert.doesNotMatch(b2, /# Spec/); // it already has the spec from the previous round (not paid again)
  for (const r of P.REVIEWERS) {
    const t = P.reviewerPrompt(r.role, issue, sk, '# Spec', '+x', 'master', 'psf/issue-7');
    assert.match(t, new RegExp('Reviewer-' + r.name));
    assert.match(t, /VEREDICTO: APROBADO o VEREDICTO: RECHAZADO/);
    assert.match(t, /master\.\.\.psf\/issue-7/);
  }
});

test('veredictos: sin un APROBADO claro, rechazo', () => {
  assert.equal(P.parseVerdict('VEREDICTO: APROBADO\nHALLAZGOS:\n- ninguno').verdict, 'APROBADO');
  assert.equal(P.parseVerdict('**VEREDICTO:** **APROBADO**').verdict, 'APROBADO');
  assert.equal(P.parseVerdict('VEREDICTO: [APROBADO]').verdict, 'APROBADO');
  const r = P.parseVerdict('Bien.\nVEREDICTO: RECHAZADO\nHALLAZGOS:\n- a.go:3 — sin validar');
  assert.equal(r.verdict, 'RECHAZADO');
  assert.equal(r.findings, '- a.go:3 — sin validar');
  assert.equal(P.parseVerdict('Todo correcto, lo apruebo').verdict, 'RECHAZADO');
  assert.equal(P.parseVerdict('VEREDICTO: [APROBADO | RECHAZADO]').verdict, 'RECHAZADO'); // the template echoed back
  assert.equal(P.parseVerdict('VEREDICTO: APROBADO\n…\nVEREDICTO: RECHAZADO').verdict, 'RECHAZADO');
});

test('decisión del Approver', () => {
  const ok: P.Review = { verdict: 'APROBADO' }, ko: P.Review = { verdict: 'RECHAZADO', findings: '- falta test' };
  const todos = { correctness: ok, security: ok, testing: ok, architecture: ok };
  assert.equal(P.decide(todos, 1, 3), 'ready');
  assert.equal(P.decide({ ...todos, testing: ko }, 1, 3), 'retry');
  assert.equal(P.decide({ ...todos, testing: ko }, 2, 3), 'retry');
  assert.equal(P.decide({ ...todos, testing: ko }, 3, 3), 'blocked');
  assert.equal(P.decide({ correctness: ok, security: ok, testing: ok }, 1, 3), 'retry'); // one missing: not 4/4
  assert.deepEqual(P.rejected({ ...todos, testing: ko }), ['testing']);
  assert.match(P.findingsFor({ ...todos, testing: ko }), /Reviewer-Testing\n- falta test/);
  assert.match(P.approverSummary({ iteration: 1, maxIterations: 3, reviews: { ...todos, testing: ko } }, 'retry'), /iteración 2\/3/);
  assert.match(P.approverSummary({ iteration: 1, maxIterations: 3, reviews: todos }, 'ready'), /confirma una persona/);
});

test('ambigüedad, modelo barato, commits y tiempo', () => {
  assert.ok(P.isAmbiguous('BLOQUEADA POR AMBIGÜEDAD\n- ¿qué navegador?'));
  assert.ok(!P.isAmbiguous('# Spec\n…'));
  assert.equal(P.cheapModel(['opus', 'sonnet', 'haiku'], 'opus'), 'haiku');
  assert.equal(P.cheapModel(['gemini-2.5-pro', 'gemini-2.5-flash'], 'x'), 'gemini-2.5-flash');
  assert.equal(P.cheapModel([], 'x'), 'x');
  assert.equal(P.commitMessage(7, 1), 'feat(issue-7): implement spec');
  assert.equal(P.commitMessage(7, 2), 'fix(issue-7): address review findings (iter 2)');
  assert.equal(P.elapsed({ startedAt: 0, endedAt: 23 * 60_000 }), '23 min');
  assert.equal(P.elapsed({ startedAt: 0, endedAt: 83 * 60_000 }), '1 h 23 min');
});

test('resolución automática: solo issues nuevas, sin PR ni sesión, de una en una', () => {
  const i = (numero: number, creada: string, extra: Partial<P.WatchedIssue> = {}): P.WatchedIssue =>
    ({ numero, titulo: 'T' + numero, url: 'u', estado: 'OPEN', fase: 'por-empezar', creada, prs: [], autor_confiable: true, ...extra });
  const since = Date.parse('2026-10-24T10:00:00Z');
  const issues = [i(1, '2026-10-24T09:00:00Z'), i(3, '2026-10-24T12:00:00Z'), i(2, '2026-10-24T11:00:00Z'),
    i(4, '2026-10-24T11:30:00Z', { prs: [{}] }), i(5, '2026-10-24T11:40:00Z', { estado: 'CLOSED' }), i(6, '2026-10-24T11:50:00Z', { fase: 'desarrollo' }),
    i(7, '2026-10-24T11:55:00Z', { autor_confiable: false })]; // a stranger's issue never starts on its own
  assert.deepEqual(P.issuesToResolve(issues, [], since).map(x => x.numero), [2, 3]);
  assert.deepEqual(P.issuesToResolve(issues, [{ psf: { issue: 2, phase: 'merged' }, archivada: true }], since).map(x => x.numero), [3]);
  assert.ok(P.canStartAutomatic([{ psf: { issue: 2, phase: 'approval' } }, {}]));
  assert.ok(!P.canStartAutomatic([{ psf: { issue: 2, phase: 'builder' } }]));
  assert.ok(P.canStartAutomatic([{ psf: { issue: 2, phase: 'builder' }, archivada: true }]));
});

test('un revisor que no llega a responder es un fallo, no un rechazo', () => {
  assert.ok(P.gaveUp('_(Antigravity ha acabado sin responder: el modo de la sesión no le deja ejecutar órdenes (RunCommand).)_'));
  assert.ok(P.gaveUp('   '));
  assert.ok(!P.gaveUp('Todo correcto, lo apruebo')); // answered without the format: that one is a rejection
  assert.ok(!P.gaveUp('VEREDICTO: APROBADO\nHALLAZGOS:\n- ninguno'));
  assert.match(P.reviewerPrompt('testing', { number: 1, title: 'T' }, sk, 'S', 'D', 'master', 'psf/issue-1'), /No ejecutes tests, compilaciones ni git/);
});

test('indicaciones de la persona para desbloquear', () => {
  const b = P.builderPrompt({ number: 4, title: 'T' }, '# Spec', 2, null, '', 'Usa el paquete existente');
  assert.match(b, /Vuelve sobre el cambio/);
  assert.match(b, /Indicaciones de la persona[\s\S]*Usa el paquete existente/);
  assert.doesNotMatch(P.builderPrompt({ number: 4, title: 'T' }, '# Spec', 1, sk), /Indicaciones de la persona/);
  assert.match(P.specPrompt(issue, 'x.md', sk, 'Que sea solo para Linux'), /Que sea solo para Linux/);
  assert.match(P.conflictGuidance('master'), /git merge origin\/master/);
  assert.ok(P.needsDecision({ phase: 'blocked' }));
  assert.ok(P.needsDecision({ phase: 'ready', autoMergeHeld: true }));
  assert.ok(!P.needsDecision({ phase: 'ready' }));
});

test('línea de estado de la franja', () => {
  assert.equal(P.stepStatus({ ...P.newPsfState(1, 'T'), phase: 'discarded' }, 'ready'), 'rejected');
  assert.ok(!P.needsDecision({ phase: 'discarded' }) && !P.isWorkingPhase('discarded'));
  const s = P.newPsfState(4, 'T');
  assert.match(P.statusText({ ...s, phase: 'review', reviews: { security: { verdict: 'APROBADO' } } }, true), /1 de 4 aprobados/);
  assert.match(P.statusText({ ...s, phase: 'builder', iteration: 2 }, false), /iteración 2\/3\) · parada/);
  assert.match(P.statusText({ ...s, phase: 'ready', autoMergeHeld: true, heldReason: 'conflict', base: 'master' }, false), /conflictos con master/);
  assert.match(P.statusText({ ...s, phase: 'merged', base: 'master' }, false), /Fusionado en master/);
  assert.equal(P.decisionLabel({ phase: 'blocked', blockedReason: 'ambiguity' }), 'Rehacer la spec');
  assert.equal(P.decisionLabel({ phase: 'ready' }), 'Otra iteración');
  assert.equal(P.stepStatus({ ...s, phase: 'merged' }, 'ready'), 'done');
});

test('supervisor: modelo más capaz y su encargo', () => {
  assert.equal(P.bestModel(['opus', 'sonnet', 'haiku'], 'x'), 'opus');
  assert.equal(P.bestModel(['gemini-4-argon-high', 'gemini-3.8-flash-high', 'gemini-3.1-pro-high'], 'x'), 'gemini-4-argon-high');
  assert.equal(P.bestModel(['a', 'b'], 'x'), 'a');
  assert.equal(P.bestModel([], 'x'), 'x');
  assert.match(P.supervisorPrompt('conflict', { number: 4, title: 'T' }, sk, '# S', 'master', ''), /git merge origin\/master/);
  assert.match(P.supervisorPrompt('convergence', { number: 4, title: 'T' }, sk, '# S', 'master', '- a.go:1 falta test'), /falta test[\s\S]*árbitro/);
  assert.match(P.supervisorPrompt('checks', { number: 4, title: 'T' }, sk, '# S', 'master', 'test'), /checks de GitHub del PR fallan \(test\)/);
  const s = { ...P.newPsfState(4, 'T'), phase: 'supervisor' as const, supervisorTask: 'conflict' as const };
  assert.equal(P.stepStatus(s, 'builder'), 'running');
  assert.match(P.statusText(s, true), /Supervisor resuelve el bloqueo \(conflictos\)/);
});

test('veredictos puntuados: solo bloquea lo grave y probable', () => {
  // A rejection whose findings are all low or uncertain is an approval: they become improvements.
  const r1 = P.parseVerdict('VEREDICTO: RECHAZADO\nHALLAZGOS:\n- [baja] (confianza 9) a.go:3 — nombre poco claro\n- [media] (confianza 5) b.go:7 — quizá una carrera\nMEJORAS:\n- c.go:1 — usar el helper\nDECISIONES:\n- ninguna');
  assert.equal(r1.verdict, 'APROBADO');
  assert.equal(r1.findings, '');
  assert.match(r1.improvements, /a\.go:3/);
  assert.match(r1.improvements, /c\.go:1/);
  assert.equal(r1.decisions, '');
  // One that blocks is kept, the rest go to the improvements; an approval with a blocking finding is a rejection.
  const r2 = P.parseVerdict('**VEREDICTO:** APROBADO\n**HALLAZGOS:**\n- [alta] (confianza 9) s.go:10 — inyección SQL\n- [baja] (confianza 8) t.go:2 — log ruidoso\nDECISIONES:\n- ¿Exponer la API fuera de localhost?');
  assert.equal(r2.verdict, 'RECHAZADO');
  assert.equal(r2.findings, '- [alta] (confianza 9) s.go:10 — inyección SQL');
  assert.match(r2.improvements, /t\.go:2/);
  assert.match(r2.decisions, /localhost/);
  // The Builder gets the blocking findings and, along with them, the improvements; the summary shows the decisions.
  const reviews: Partial<Record<P.ReviewerRole, P.Review>> = { correctness: { verdict: 'APROBADO', improvements: '- c.go:1 — usar el helper' },
    security: { ...r2 }, testing: { verdict: 'APROBADO' }, architecture: { verdict: 'APROBADO' } };
  const f = P.findingsFor(reviews);
  assert.match(f, /Reviewer-Security\n- \[alta\]/);
  assert.match(f, /Mejoras que no bloquean[\s\S]*Reviewer-Correctness:\n  - c\.go:1/);
  assert.match(P.approverSummary({ iteration: 1, maxIterations: 3, reviews }, 'retry'), /Decisiones para la persona[\s\S]*localhost/);
  const ok = { ...reviews, security: { verdict: 'APROBADO' as const } };
  assert.match(P.approverSummary({ iteration: 1, maxIterations: 3, reviews: ok }, 'ready'), /Mejoras propuestas \(no bloquean\)/);
  // Each reviewer gets its skill (its essentials and the index of its chapters) and the project's own criteria.
  const rp = P.reviewerPrompt('security', { number: 1, title: 'T' }, sk, 'S', 'D', 'master', 'b');
  assert.match(rp, /Tus criterios:\n\nChecklist/);
  assert.match(rp, /\.pgocode\/skills-sistema\/psf-x\/capitulos\/1-a\.md — Siempre\./);
  assert.match(rp, /\.psf\/criterios\//);
  assert.match(P.builderPrompt(issue, '# Spec', 1, sk), /No toques \.psf\/specs\/ ni \.pgocode\/skills-sistema\//);
  assert.doesNotMatch(P.builderPrompt(issue, '# Spec', 2, null, '- x'), /Checklist/); // the skill is already in its conversation
});
