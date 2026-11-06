import assert from 'node:assert/strict';
import { test } from 'node:test';

import { countTiles, firstName, pendingItems, pendingView } from './core.ts';

test('countTiles keeps the order and alarms only on SLA breaches', () => {
  assert.deepEqual(countTiles({ open: 12, mine: 4, unassigned: 3, breached: 0 }), [
    { key: 'open', value: 12, alert: false },
    { key: 'mine', value: 4, alert: false },
    { key: 'unassigned', value: 3, alert: false },
    { key: 'breached', value: 0, alert: false },
  ]);
  assert.equal(countTiles({ open: 1, mine: 0, unassigned: 0, breached: 2 })[3].alert, true);
});

test('countTiles never shows a negative or broken number', () => {
  const tiles = countTiles({ open: -1, mine: Number.NaN, unassigned: 2.9, breached: 0 });
  assert.deepEqual(
    tiles.map((t) => t.value),
    [0, 0, 2, 0],
  );
});

test('pendingItems lists only what waits, incidents first', () => {
  assert.deepEqual(pendingItems({ incidents: 1, unread: 5, setup: { active: true, completed: 2, total: 6 } }), [
    { key: 'incidents', count: 1 },
    { key: 'notifications', count: 5 },
    { key: 'setup', count: 4 },
  ]);
  assert.deepEqual(pendingItems({ incidents: 0, unread: 0, setup: { active: true, completed: 6, total: 6 } }), []);
  assert.deepEqual(pendingItems({ incidents: null, unread: null, setup: null }), []);
});

test('a set-up checklist that is not active leaves no row', () => {
  assert.deepEqual(pendingItems({ incidents: null, unread: 0, setup: { active: false, completed: 1, total: 5 } }), []);
});

test('pendingView never says "nothing waits" while a source loads or after it failed', () => {
  const ok = { enabled: true, loaded: true, failed: false };
  const loading = { enabled: true, loaded: false, failed: false };
  const failed = { enabled: true, loaded: false, failed: true };
  const off = { enabled: false, loaded: false, failed: false };
  assert.equal(pendingView(0, [ok, loading]), 'loading');
  assert.equal(pendingView(0, [ok, failed]), 'error');
  assert.equal(pendingView(0, [ok, off]), 'empty');
  assert.equal(pendingView(2, [loading, failed]), 'list');
  // A background refetch that failed keeps the answer it already had.
  assert.equal(pendingView(0, [ok, { enabled: true, loaded: true, failed: true }]), 'empty');
});

test('firstName takes the first word', () => {
  assert.equal(firstName('  Ayşe Nur Yılmaz '), 'Ayşe');
  assert.equal(firstName(''), '');
  assert.equal(firstName(undefined), '');
});
