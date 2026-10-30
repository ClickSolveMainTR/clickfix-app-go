import assert from 'node:assert/strict';
import { test } from 'node:test';

import { localClock, shiftToday, type WorkingDay } from './shift-core.ts';

const week = (open: boolean, start = '09:00', end = '18:00'): WorkingDay[] =>
  (['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'] as const).map((day) => ({
    day,
    open: open && day !== 'sat' && day !== 'sun',
    start,
    end,
  }));

// Wednesday 28 Oct 2026, 11:30 UTC = 14:30 in Istanbul.
const wed = new Date('2026-10-28T11:30:00Z');

test('localClock reads the weekday and time in the zone', () => {
  assert.deepEqual(localClock(wed, 'Europe/Istanbul'), { day: 'wed', time: '14:30' });
  assert.deepEqual(localClock(wed, 'America/New_York'), { day: 'wed', time: '07:30' });
});

test('shiftToday: org hours, day off, before / on / after, overnight', () => {
  assert.deepEqual(shiftToday(null, wed, 'Europe/Istanbul'), { state: 'org' });
  assert.equal(shiftToday(week(true), wed, 'Europe/Istanbul').state, 'on');
  assert.equal(shiftToday(week(true), wed, 'America/New_York').state, 'before');
  assert.equal(shiftToday(week(true, '08:00', '12:00'), wed, 'Europe/Istanbul').state, 'after');
  assert.equal(shiftToday(week(false), wed, 'Europe/Istanbul').state, 'closed');
  assert.equal(shiftToday(week(true, '22:00', '06:00'), new Date('2026-10-28T23:30:00Z'), 'UTC').state, 'on');
  assert.equal(shiftToday(week(true, '22:00', '06:00'), wed, 'Europe/Istanbul').state, 'before');
});
