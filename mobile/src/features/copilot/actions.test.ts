import assert from 'node:assert/strict';
import { test } from 'node:test';

import { availableActions, needsDraft, resultTarget } from './actions.ts';

test('draft tools appear only with a draft', () => {
  assert.deepEqual(availableActions(''), ['suggest_reply', 'summarize']);
  assert.deepEqual(availableActions('  merhaba '), ['suggest_reply', 'summarize', 'translate', 'shorten', 'rewrite']);
  assert.equal(needsDraft('suggest_reply'), false);
  assert.equal(needsDraft('translate'), true);
});

test('a summary is read, everything else goes to the draft', () => {
  assert.equal(resultTarget('summarize'), 'read');
  assert.equal(resultTarget('suggest_reply'), 'draft');
  assert.equal(resultTarget('translate'), 'draft');
});
