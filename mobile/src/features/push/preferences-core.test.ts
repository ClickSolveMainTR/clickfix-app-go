import assert from 'node:assert/strict';
import { test } from 'node:test';

import { applyUpdate, channelOn } from './preferences-core.ts';

const prefs = {
  push_enabled: true,
  event_channels: { 'message.received': { email: true } },
  quiet_hours: { enabled: false, start: '22:00', end: '08:00', timezone: 'Europe/Istanbul' },
};
const ev = { key: 'message.received', defaults: { push: true, email: false }, locked: [] as string[] };

test('channelOn uses the stored cell, else the default; locked channels stay on', () => {
  assert.equal(channelOn(prefs, ev, 'email'), true);
  assert.equal(channelOn(prefs, ev, 'push'), true);
  assert.equal(channelOn(prefs, ev, 'inapp'), false);
  assert.equal(channelOn({ ...prefs, event_channels: { 'message.received': { push: false } } }, ev, 'push'), false);
  assert.equal(
    channelOn(
      { ...prefs, event_channels: { 'sla.breached': { push: false } } },
      { ...ev, key: 'sla.breached', locked: ['push'] },
      'push',
    ),
    true,
  );
});

test('applyUpdate merges event cells and quiet hours like the API', () => {
  const next = applyUpdate(prefs, { event_channels: { 'message.received': { push: false } }, push_enabled: false });
  assert.deepEqual(next.event_channels['message.received'], { email: true, push: false });
  assert.equal(next.push_enabled, false);
  assert.equal(next.quiet_hours.start, '22:00');
  const quiet = applyUpdate(prefs, { quiet_hours: { ...prefs.quiet_hours, enabled: true } });
  assert.equal(quiet.quiet_hours.enabled, true);
  assert.deepEqual(prefs.event_channels['message.received'], { email: true });
});
