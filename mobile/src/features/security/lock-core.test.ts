import assert from 'node:assert/strict';
import { test } from 'node:test';

import { parseLockSettings, shouldLock } from './lock-core.ts';

test('parseLockSettings keeps known values only', () => {
  assert.deepEqual(parseLockSettings(null), { enabled: false, timeoutMin: 1 });
  assert.deepEqual(parseLockSettings({ enabled: true, timeoutMin: 5 }), { enabled: true, timeoutMin: 5 });
  assert.deepEqual(parseLockSettings({ enabled: 'yes', timeoutMin: 7 }), { enabled: false, timeoutMin: 1 });
  assert.deepEqual(parseLockSettings({ enabled: true, timeoutMin: 0 }), { enabled: true, timeoutMin: 0 });
});

test('shouldLock after the timeout, at once with 0, always on a cold start', () => {
  const on = { enabled: true, timeoutMin: 5 as const };
  assert.equal(shouldLock(on, 1_000, 1_000 + 4 * 60_000), false);
  assert.equal(shouldLock(on, 1_000, 1_000 + 5 * 60_000), true);
  assert.equal(shouldLock({ enabled: true, timeoutMin: 0 }, 1_000, 1_001), true);
  assert.equal(shouldLock(on, null, 0), true);
  assert.equal(shouldLock({ enabled: false, timeoutMin: 0 }, null, 0), false);
});
