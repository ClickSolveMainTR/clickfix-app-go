import assert from 'node:assert/strict';
import { test } from 'node:test';
import { barFractions } from './chart-core.ts';
test('volume bars retain relative counts, gaps in data, and zero baselines', () => {
  assert.deepEqual(barFractions([10, 2, 5, 0, null, 8, 1]), [1, 0.2, 0.5, 0, 0, 0.8, 0.1]);
  assert.deepEqual(barFractions([0, null, -2, NaN, Infinity]), [0, 0, 0, 0, 0]);
  assert.deepEqual(barFractions([]), []);
});
