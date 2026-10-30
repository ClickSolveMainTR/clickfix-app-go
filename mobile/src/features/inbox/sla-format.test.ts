import assert from 'node:assert/strict';
import { test } from 'node:test';

import { formatSlaTime } from './helpers.ts';

test('overdue SLA clocks keep the largest unit and remaining clocks keep precision', () => {
  assert.equal(formatSlaTime(-(22 * 3600 + 10 * 60), 'tr'), '22 sa');
  assert.equal(formatSlaTime(-(11 * 86400 + 23 * 60), 'tr'), '11 g');
  assert.equal(formatSlaTime(3 * 3600 + 25 * 60, 'tr'), '3 sa 25 dk');
  assert.equal(formatSlaTime(-22 * 3600, 'en'), '22h');
  assert.equal(formatSlaTime(-11 * 86400, 'en'), '11d');
  assert.equal(formatSlaTime(3 * 3600 + 25 * 60, 'en'), '3h 25m');
  assert.equal(formatSlaTime(-60, 'tr'), '1 dk');
});
