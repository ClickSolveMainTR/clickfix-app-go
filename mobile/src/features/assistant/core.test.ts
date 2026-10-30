import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  appendMessages,
  type AssistantMessage,
  lastSuggestions,
  messagesLeft,
  paramLines,
  visibleMessages,
  withActionStatus,
} from './core.ts';

const msg = (uuid: string, role: AssistantMessage['role'], over: Partial<AssistantMessage> = {}): AssistantMessage => ({
  uuid,
  role,
  content: uuid,
  created_at: '2026-10-29T10:00:00Z',
  ...over,
});

test('tool messages are hidden', () => {
  assert.deepEqual(
    visibleMessages([msg('a', 'user'), msg('b', 'tool'), msg('c', 'assistant')]).map((m) => m.uuid),
    ['a', 'c'],
  );
});

test('suggestions come from the last answer only', () => {
  const answer = msg('b', 'assistant', { suggestions: [' Evet ', '', 'Hayır'] });
  assert.deepEqual(lastSuggestions([msg('a', 'user'), answer]), ['Evet', 'Hayır']);
  assert.deepEqual(lastSuggestions([answer, msg('c', 'user')]), []);
  assert.deepEqual(lastSuggestions([answer, msg('t', 'tool')]), ['Evet', 'Hayır']);
  assert.deepEqual(lastSuggestions([]), []);
});

test('appendMessages skips duplicates', () => {
  const list = appendMessages([msg('a', 'user')], [msg('a', 'user'), msg('b', 'assistant')]);
  assert.deepEqual(
    list.map((m) => m.uuid),
    ['a', 'b'],
  );
  assert.equal(appendMessages(undefined, [msg('x', 'user')]).length, 1);
});

test('withActionStatus updates the action in place', () => {
  const action = { id: 'x1', kind: 'enable_ai', title: 'AI', summary: '', params: {}, status: 'proposed' as const };
  const list = [msg('a', 'assistant', { actions: [action] }), msg('b', 'user')];
  const next = withActionStatus(list, 'x1', 'done');
  assert.equal(next[0].actions?.[0].status, 'done');
  assert.equal(list[0].actions?.[0].status, 'proposed');
  assert.equal(next[1], list[1]);
});

test('messagesLeft', () => {
  assert.equal(messagesLeft(undefined), null);
  assert.equal(messagesLeft({ enabled: true, daily_limit: 0, used_today: 3 }), null);
  assert.equal(messagesLeft({ enabled: true, daily_limit: 20, used_today: 5 }), 15);
  assert.equal(messagesLeft({ enabled: true, daily_limit: 20, used_today: 25 }), 0);
});

test('paramLines summarizes parameters', () => {
  assert.deepEqual(paramLines({ email: 'a@b.co', role_name: 'Agent', empty: '', days: [1, 2] }), [
    'email: a@b.co',
    'role name: Agent',
    'days: [1,2]',
  ]);
  assert.equal(paramLines({ a: 1, b: 2, c: 3 }, 2).length, 2);
  assert.ok(paramLines({ long: 'x'.repeat(100) })[0].endsWith('…'));
});
