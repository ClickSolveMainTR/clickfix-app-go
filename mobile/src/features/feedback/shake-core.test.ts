import assert from 'node:assert/strict';
import { test } from 'node:test';

import { createShakeDetector } from './shake-core.ts';

const rest = { x: 0, y: 0, z: -1 };
const jolt = { x: 2.4, y: 0.3, z: -1 };

test('a phone at rest or carried never shakes', () => {
  const d = createShakeDetector();
  for (let t = 0; t < 5000; t += 100) assert.equal(d(rest, t), false);
  assert.equal(d({ x: 0.6, y: 0.4, z: -1.2 }, 5100), false);
});

test('three jolts close together are a shake, then it rests', () => {
  const d = createShakeDetector();
  assert.equal(d(jolt, 0), false);
  assert.equal(d(jolt, 200), false);
  assert.equal(d(jolt, 400), true);
  // Cooldown: the next jolts right after do not count.
  assert.equal(d(jolt, 600), false);
  assert.equal(d(jolt, 800), false);
  assert.equal(d(jolt, 1000), false);
});

test('jolts far apart (a bump, then another) are not a shake', () => {
  const d = createShakeDetector();
  assert.equal(d(jolt, 0), false);
  assert.equal(d(jolt, 1500), false);
  assert.equal(d(jolt, 3000), false);
});

test('shakes again after the cooldown', () => {
  const d = createShakeDetector({ cooldownMs: 1000 });
  d(jolt, 0);
  d(jolt, 100);
  assert.equal(d(jolt, 200), true);
  d(jolt, 1300);
  d(jolt, 1400);
  assert.equal(d(jolt, 1500), true);
});
