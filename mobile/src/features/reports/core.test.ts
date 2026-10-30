import assert from 'node:assert/strict';
import { test } from 'node:test';

import { delta, periodRange } from './core.ts';

test('periodRange covers the last N days up to the end of today, day buckets up to a month', () => {
  const now = new Date(2026, 9, 29, 15, 30);
  const r = periodRange(7, now);
  assert.equal(new Date(r.to).getTime(), new Date(2026, 9, 30).getTime());
  assert.equal(new Date(r.from).getTime(), new Date(2026, 9, 23).getTime());
  assert.equal(r.bucket, 'day');
  assert.equal(periodRange(90, now).bucket, 'week');
});

test('delta is the relative change, null without a base', () => {
  assert.equal(delta(120, 100), 0.2);
  assert.equal(delta(80, 100), -0.2);
  assert.equal(delta(5, 0), null);
  assert.equal(delta(null, 3), null);
});

test('report deltas distinguish neutral, growth metrics and lower-is-better metrics', async () => {
  const { changeTone } = await import('./core.ts');
  for (const lower of [false, true]) {
    assert.equal(changeTone(0, lower), 'neutral');
    assert.equal(changeTone(null, lower), 'neutral');
    assert.equal(changeTone(0.004, lower), 'neutral');
    assert.equal(changeTone(-0.004, lower), 'neutral');
    assert.equal(changeTone(Number.NaN, lower), 'neutral');
  }
  assert.equal(changeTone(0.2), 'good');
  assert.equal(changeTone(-0.2), 'bad');
  assert.equal(changeTone(0.2, true), 'bad');
  assert.equal(changeTone(-0.2, true), 'good');
});
