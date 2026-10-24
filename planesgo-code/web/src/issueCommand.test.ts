// Tests of the «/issue» command: npm test (node --test, no dependencies).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseIssueCommand } from './issueCommand.ts';
import * as R from './router.ts';

test('/issue con y sin título', () => {
  assert.equal(parseIssueCommand('/issue'), '');
  assert.equal(parseIssueCommand('  /ISSUE  '), '');
  assert.equal(parseIssueCommand('/issue Falla el login'), 'Falla el login');
  assert.equal(parseIssueCommand('/issues'), null);
  assert.equal(parseIssueCommand('crea una /issue'), null);
});

test('el enrutador la hace pgocode (sin agente)', () => {
  const d = R.route({ text: '/issue Falla el login', isAgent: true, busy: false } as R.RouteInput);
  assert.equal(d.route, 'command');
  assert.equal(d.detail, 'issue');
  assert.equal(d.executor, 'pgocode');
  assert.equal(d.text, 'Falla el login');
  assert.match(R.tagText(d, false), /issue/i);
});
