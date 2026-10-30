import assert from 'node:assert/strict';
import { test } from 'node:test';

import { collapseEvents, relativeDay, slashQuery, timelineMarks } from './timeline.ts';

const day = (iso: string) => iso.slice(0, 10);

const msg = (created_at: string, over: Partial<Parameters<typeof timelineMarks>[0][number]> = {}) => ({
  created_at,
  direction: 'in',
  kind: 'text',
  private: false,
  sender: { uuid: 'c1', name: 'Ayşe' },
  ...over,
});

test('a burst from one sender is one run: only the newest bubble ends it', () => {
  // Newest first, like the inverted list.
  const marks = timelineMarks(
    [msg('2026-10-28T10:02:00Z'), msg('2026-10-28T10:01:00Z'), msg('2026-10-28T10:00:00Z')],
    day,
  );
  assert.deepEqual(
    marks.map((m) => [m.runStart, m.runEnd]),
    [
      [false, true],
      [false, false],
      [true, false],
    ],
  );
});

test('a new sender, a long pause or a note starts a new run', () => {
  const marks = timelineMarks(
    [
      msg('2026-10-28T10:30:00Z', { direction: 'internal', private: true, sender: { uuid: 'a1' } }),
      msg('2026-10-28T10:29:00Z', { direction: 'out', sender: { uuid: 'a1' } }),
      msg('2026-10-28T10:10:00Z'),
      msg('2026-10-28T10:00:00Z'),
    ],
    day,
  );
  assert.deepEqual(
    marks.map((m) => m.runEnd),
    [true, true, true, true],
  );
});

test('the oldest message of each day starts the day', () => {
  const marks = timelineMarks(
    [msg('2026-10-28T09:00:00Z'), msg('2026-10-27T18:00:00Z'), msg('2026-10-27T17:59:00Z')],
    day,
  );
  assert.deepEqual(
    marks.map((m) => m.dayStart),
    [true, false, true],
  );
});

test('events never join a run', () => {
  const marks = timelineMarks([msg('2026-10-28T10:01:00Z'), msg('2026-10-28T10:00:00Z', { kind: 'event' })], day);
  assert.equal(marks[0].runStart, true);
});

test('relativeDay names today and yesterday', () => {
  assert.equal(relativeDay('2026-10-28', '2026-10-28', '2026-10-27'), 'today');
  assert.equal(relativeDay('2026-10-27', '2026-10-28', '2026-10-27'), 'yesterday');
  assert.equal(relativeDay('2026-10-20', '2026-10-28', '2026-10-27'), null);
});

test('slashQuery opens canned responses for a lone /word', () => {
  assert.equal(slashQuery('/'), '');
  assert.equal(slashQuery('/iade'), 'iade');
  assert.equal(slashQuery('/kargo_takip'), 'kargo_takip');
  assert.equal(slashQuery('merhaba /iade'), null);
  assert.equal(slashQuery('/iade bilgisi'), null);
});

const event = (at: string, attributes: Record<string, unknown>) => ({ ...msg(at, { kind: 'event' }), attributes });

test('five adjacent merge sources display once without changing stored messages', () => {
  const items = Array.from({ length: 5 }, (_, i) =>
    event(`2026-10-28T10:01:0${5 - i}Z`, {
      event: 'merged_from',
      to: `#${i}`,
      actor_uuid: 'agent',
      messages: i + 1,
    }),
  );
  const before = JSON.stringify(items);
  const rows = collapseEvents(items, day);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].count, 5);
  assert.equal(rows[0].message, items[0]);
  assert.equal(JSON.stringify(items), before);
});

test('event collapse respects messages, actor, payload, day and long gaps', () => {
  const a = { event: 'tag_added', to: 'vip', actor_uuid: 'a' };
  const items = [
    event('2026-10-29T10:01:00Z', a),
    event('2026-10-29T10:00:00Z', { actor_uuid: 'a', to: 'vip', event: 'tag_added' }),
    event('2026-10-29T09:59:00Z', { ...a, to: 'urgent' }),
    event('2026-10-29T09:58:00Z', { ...a, actor_uuid: 'b' }),
    event('2026-10-28T23:59:00Z', a),
    { ...msg('2026-10-28T23:58:00Z'), attributes: null },
    event('2026-10-28T23:57:00Z', a),
    event('2026-10-28T23:40:00Z', a),
  ];
  const rows = collapseEvents(items, day);
  assert.deepEqual(
    rows.map((r) => r.count),
    [2, 1, 1, 1, 1, 1, 1],
  );
  const marks = timelineMarks(
    rows.map((r) => r.message),
    day,
  );
  assert.equal(marks[2].dayStart, true);
  assert.equal(marks[6].dayStart, true);
  assert.deepEqual(collapseEvents([], day), []);
});

test('merged events never collapse across calendar days even a second apart', () => {
  const rows = collapseEvents(
    [
      event('2026-10-29T00:00:00Z', { event: 'merged_from', to: '#2' }),
      event('2026-10-28T23:59:59Z', { event: 'merged_from', to: '#1' }),
    ],
    day,
  );
  assert.equal(rows.length, 2);
});
