import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  agentTimes,
  bucketLabel,
  coverage,
  forecastRows,
  forecastTotals,
  forecastWindow,
  HORIZONS,
  timesheetWindow,
} from './core.ts';

test('forecastWindow starts at the current UTC hour and ends on a whole hour', () => {
  const now = new Date('2026-11-06T10:42:17.500Z');
  const w = forecastWindow(7, now);
  assert.equal(w.since, '2026-11-06T10:00:00.000Z');
  const until = new Date(w.until);
  assert.equal(until.getUTCMinutes(), 0);
  assert.equal(until.getUTCSeconds(), 0);
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  const days = (until.getTime() - midnight.getTime()) / 86_400_000;
  assert.ok(days > 6.9 && days <= 7, `7 local days, got ${days}`);
  assert.ok(until > new Date(w.since));
});

test('horizons stay inside the API limit of 31 days', () => {
  assert.ok([...HORIZONS.day, ...HORIZONS.hour].every((d) => d >= 1 && d <= 31));
});

const hours = [
  {
    start: '2026-11-06T10:00:00Z',
    volume: 4,
    required_agents: 3,
    scheduled_agents: 1,
    gap: 2,
    capacity_limited: false,
  },
  {
    start: '2026-11-06T11:00:00Z',
    volume: 2.5,
    required_agents: 2,
    scheduled_agents: 2,
    gap: 0,
    capacity_limited: false,
  },
  { start: '2026-11-06T12:00:00Z', volume: 1, required_agents: 1, scheduled_agents: 3, gap: 0, capacity_limited: true },
];

test('forecastRows reads hourly and daily buckets by their shape', () => {
  const rows = forecastRows(hours);
  assert.deepEqual(rows[0], {
    key: '2026-11-06T10:00:00Z',
    at: '2026-11-06T10:00:00Z',
    volume: 4,
    required: 3,
    scheduled: 1,
    gap: 2,
    capacityLimited: false,
  });
  const days = forecastRows([
    {
      date: '2026-11-06',
      volume: 40,
      required_agent_hours: 30,
      scheduled_agent_hours: 24,
      gap_agent_hours: 6,
      capacity_limited: false,
    },
  ]);
  assert.deepEqual(days[0], {
    key: '2026-11-06',
    at: '2026-11-06',
    volume: 40,
    required: 30,
    scheduled: 24,
    gap: 6,
    capacityLimited: false,
  });
});

test('forecastTotals and coverage', () => {
  const rows = forecastRows(hours);
  assert.deepEqual(forecastTotals(rows), {
    volume: 7.5,
    peakRequired: 3,
    gapBuckets: 1,
    gap: 2,
    capacityLimited: true,
  });
  assert.deepEqual(rows.map(coverage), ['gap', 'covered', 'surplus']);
  assert.deepEqual(forecastTotals([]), { volume: 0, peakRequired: 0, gapBuckets: 0, gap: 0, capacityLimited: false });
});

test('bucketLabel uses the organization zone and never throws', () => {
  const [row] = forecastRows(hours);
  assert.match(bucketLabel(row!, 'hour', 'Europe/Istanbul', 'en-GB'), /13:00/);
  assert.match(bucketLabel(row!, 'hour', 'UTC', 'en-GB'), /10:00/);
  assert.equal(bucketLabel(row!, 'hour', 'Not/AZone', 'en-GB'), row!.at);
  const day = { ...row!, at: '2026-11-06', key: '2026-11-06' };
  assert.match(bucketLabel(day, 'day', 'America/Los_Angeles', 'en-GB'), /6 Nov/);
});

test('timesheetWindow covers whole local days, today included', () => {
  const now = new Date('2026-11-06T10:42:00Z');
  const w = timesheetWindow(31, now);
  const span = (new Date(w.until).getTime() - new Date(w.since).getTime()) / 86_400_000;
  assert.ok(Math.abs(span - 31) < 0.1, `31 days, got ${span}`);
  assert.ok(new Date(w.until) > now && new Date(w.since) < now);
});

test('agentTimes groups by agent, most working time first', () => {
  const list = agentTimes([
    { agent_uuid: 'a', state: 'active', seconds: 600, conversation_uuid: null },
    { agent_uuid: 'a', state: 'handling', seconds: 120, conversation_uuid: 'c1' },
    { agent_uuid: 'a', state: 'handling', seconds: 300, conversation_uuid: null },
    { agent_uuid: 'a', state: 'away', seconds: 60 },
    { agent_uuid: 'b', state: 'active', seconds: 3600 },
    { agent_uuid: 'b', state: 'training', seconds: 9999 },
  ]);
  assert.deepEqual(
    list.map((x) => x.agent),
    ['b', 'a'],
  );
  assert.deepEqual(list[1], {
    agent: 'a',
    active: 600,
    away: 60,
    handling: 420,
    conversations: [
      { uuid: null, seconds: 300 },
      { uuid: 'c1', seconds: 120 },
    ],
  });
  // An unknown state is not counted as working or away time.
  assert.deepEqual([list[0]!.active, list[0]!.away, list[0]!.handling], [3600, 0, 0]);
});
