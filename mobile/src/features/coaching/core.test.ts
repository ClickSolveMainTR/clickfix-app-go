import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  criteriaRows,
  groupItems,
  itemState,
  kindKey,
  leaderboardWindow,
  metricKey,
  otherYear,
  readable,
  shortSeconds,
  thresholdParts,
} from './core.ts';

test('readable never returns a key-looking value', () => {
  assert.equal(readable('needs_review'), 'Needs review');
  assert.equal(readable(undefined), '—');
});

test('leaderboardWindow covers whole local days, today included', () => {
  const now = new Date('2026-11-06T10:42:00Z');
  const w = leaderboardWindow(30, now);
  const span = (new Date(w.until).getTime() - new Date(w.since).getTime()) / 86_400_000;
  assert.ok(Math.abs(span - 30) < 0.1, `30 days, got ${span}`);
  assert.ok(new Date(w.until) > now && new Date(w.since) < now);
});

test('metric and kind lookups fall back for unknown values', () => {
  assert.equal(metricKey('csat'), 'csat');
  assert.equal(metricKey('nps'), null);
  assert.equal(kindKey('task'), 'task');
  assert.equal(kindKey('review'), null);
});

test('thresholdParts: only first response time is lower-wins', () => {
  assert.deepEqual(thresholdParts('frt', 120), { sign: '≤', seconds: true, scale: null });
  assert.deepEqual(thresholdParts('csat', 4.5), { sign: '≥', seconds: false, scale: 5 });
  assert.deepEqual(thresholdParts('qa', 80), { sign: '≥', seconds: false, scale: 100 });
  assert.deepEqual(thresholdParts('resolutions', 50), { sign: '≥', seconds: false, scale: null });
  assert.deepEqual(thresholdParts('nps', 1), { sign: '≥', seconds: false, scale: null });
});

const item = (
  uuid: string,
  kind: string,
  extra: Partial<{ parent_uuid: string; due_at: string; completed_at: string }> = {},
) => ({
  uuid,
  kind,
  parent_uuid: extra.parent_uuid ?? null,
  due_at: extra.due_at ?? null,
  completed_at: extra.completed_at ?? null,
});

test('groupItems keeps every row somewhere', () => {
  const items = [
    item('g1', 'goal'),
    item('t1', 'task', { parent_uuid: 'g1' }),
    item('t2', 'task', { parent_uuid: 'gone' }),
    item('n1', 'note'),
    item('x1', 'review'),
  ];
  const g = groupItems(items);
  assert.deepEqual(
    g.goals.map((i) => i.uuid),
    ['g1'],
  );
  assert.deepEqual(
    g.tasks.get('g1')?.map((i) => i.uuid),
    ['t1'],
  );
  assert.deepEqual(
    g.notes.map((i) => i.uuid),
    ['n1'],
  );
  assert.deepEqual(
    g.other.map((i) => i.uuid),
    ['t2', 'x1'],
  );
  const shown = g.goals.length + [...g.tasks.values()].flat().length + g.notes.length + g.other.length;
  assert.equal(shown, items.length);
});

test('itemState', () => {
  const now = new Date('2026-11-06T12:00:00Z').getTime();
  assert.equal(itemState(item('a', 'task', { due_at: '2026-11-05T00:00:00Z' }), now), 'overdue');
  assert.equal(itemState(item('a', 'task', { due_at: '2026-11-07T00:00:00Z' }), now), 'due');
  assert.equal(itemState(item('a', 'goal'), now), 'none');
  assert.equal(
    itemState(item('a', 'task', { due_at: '2026-11-05T00:00:00Z', completed_at: '2026-11-06T00:00:00Z' }), now),
    'completed',
  );
});

test('criteriaRows joins rubric and statistics and flags a high spread', () => {
  const rows = criteriaRows([{ key: 'tone', label: 'Üslup', weight: 40, critical: true }, { key: 'accuracy' }, {}], {
    tone: { count: 3, mean: 70, variance: 66.7, range: 20 },
    accuracy: { count: 3, mean: 90, variance: 4, range: 5 },
  });
  assert.deepEqual(
    rows.map((r) => [r.key, r.label, r.weight, r.critical, r.spread]),
    [
      ['tone', 'Üslup', 40, true, true],
      ['accuracy', 'Accuracy', 0, false, false],
      ['criterion-2', '—', 0, false, false],
    ],
  );
  // An open session has no statistics.
  assert.equal(criteriaRows([{ key: 'tone' }], undefined)[0]!.stats, null);
  // Not fooled by inherited object keys.
  assert.equal(criteriaRows([{ key: 'toString' }], {})[0]!.stats, null);
});

test('shortSeconds keeps sub-minute thresholds in seconds', () => {
  assert.equal(shortSeconds(20), 20);
  assert.equal(shortSeconds(45.4), 45);
  assert.equal(shortSeconds(0), 0);
  assert.equal(shortSeconds(60), null);
  assert.equal(shortSeconds(300), null);
});

test('otherYear asks for the year only outside the current one', () => {
  const now = new Date('2026-11-06T12:00:00Z');
  assert.equal(otherYear('2026-03-01T12:00:00Z', now), false);
  assert.equal(otherYear('2025-06-15T12:00:00Z', now), true);
  assert.equal(otherYear('2027-06-15T12:00:00Z', now), true);
  assert.equal(otherYear(null, now), false);
  assert.equal(otherYear('soon', now), false);
});
