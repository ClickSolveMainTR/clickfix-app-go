import assert from 'node:assert/strict';
import { test } from 'node:test';

import { backoffMs, dueItems, enqueue, itemsFor, type OutboxItem, outcomeOf, removeItem } from './outbox-core.ts';

const item = (client_id: string, over: Partial<OutboxItem> = {}): OutboxItem => ({
  org: 'acme',
  conversation: 'c1',
  client_id,
  content: client_id,
  private: false,
  attachments: [],
  sender: { uuid: null, name: 'Ayşe' },
  created_at: `2026-10-28T10:00:0${client_id.length}Z`,
  attempts: 0,
  next_at: 0,
  ...over,
});

test('enqueue replaces an item with the same client_id instead of sending it twice', () => {
  const list = enqueue(enqueue([], item('a')), item('b'));
  const again = enqueue(list, item('a', { content: 'edited' }));
  assert.equal(again.length, 2);
  assert.equal(again.find((i) => i.client_id === 'a')?.content, 'edited');
  assert.deepEqual(
    removeItem(again, 'a').map((i) => i.client_id),
    ['b'],
  );
});

test('itemsFor keeps one conversation of one organization, oldest first', () => {
  const list = [
    item('bb', { created_at: '2026-10-28T10:00:02Z' }),
    item('a', { created_at: '2026-10-28T10:00:01Z' }),
    item('x', { org: 'other' }),
    item('y', { conversation: 'c2' }),
  ];
  assert.deepEqual(
    itemsFor(list, 'acme', 'c1').map((i) => i.client_id),
    ['a', 'bb'],
  );
});

test('network failures, timeouts, rate limits and 5xx retry; refusals drop', () => {
  for (const s of [0, 408, 429, 500, 502, 503]) assert.equal(outcomeOf(s), 'retry', String(s));
  for (const s of [400, 401, 403, 404, 409, 422]) assert.equal(outcomeOf(s), 'drop', String(s));
});

test('backoff doubles from 2 s and stops at 2 minutes', () => {
  assert.equal(backoffMs(1), 2000);
  assert.equal(backoffMs(2), 4000);
  assert.equal(backoffMs(3), 8000);
  assert.equal(backoffMs(30), 120_000);
});

test('dueItems skips items still backing off', () => {
  const list = [item('a', { next_at: 5000 }), item('b', { next_at: 0 })];
  assert.deepEqual(
    dueItems(list, 1000).map((i) => i.client_id),
    ['b'],
  );
  assert.deepEqual(
    dueItems(list, 6000).map((i) => i.client_id),
    ['a', 'b'],
  );
});

test('a refused item stays for the agent but is never sent again on its own', () => {
  const list = [item('a', { failed: 'CONFLICT' }), item('bb')];
  assert.deepEqual(
    dueItems(list, Date.now()).map((i) => i.client_id),
    ['bb'],
  );
});
