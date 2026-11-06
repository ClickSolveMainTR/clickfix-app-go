import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  channelKey,
  componentStatus,
  groupComponents,
  intervalParts,
  monitorState,
  readable,
  resultsSummary,
  stepTarget,
} from './oncall-core.ts';

test('readable turns an unknown API value into text, never a key', () => {
  assert.equal(readable('partial_outage'), 'Partial outage');
  assert.equal(readable('under-investigation'), 'Under investigation');
  assert.equal(readable(''), '—');
  assert.equal(readable(undefined), '—');
});

test('channelKey knows the four channels only', () => {
  assert.equal(channelKey('push'), 'push');
  assert.equal(channelKey('sms'), 'sms');
  assert.equal(channelKey('whatsapp'), null);
});

test('stepTarget prefers rota, then team, then agent', () => {
  assert.deepEqual(stepTarget({ rota: 'r', team: 't', agent: 'a' }), { kind: 'rota', uuid: 'r' });
  assert.deepEqual(stepTarget({ team: 't', agent: 'a' }), { kind: 'team', uuid: 't' });
  assert.deepEqual(stepTarget({ agent: 'a' }), { kind: 'agent', uuid: 'a' });
  assert.equal(stepTarget({}), null);
});

test('intervalParts picks the largest whole unit', () => {
  assert.deepEqual(intervalParts(7200), { unit: 'hours', count: 2 });
  assert.deepEqual(intervalParts(300), { unit: 'minutes', count: 5 });
  assert.deepEqual(intervalParts(90), { unit: 'seconds', count: 90 });
  assert.deepEqual(intervalParts(0), { unit: 'seconds', count: 0 });
});

test('monitorState mirrors the panel chip', () => {
  assert.deepEqual(monitorState({ enabled: false, last_up: false, last_status: 500 }), {
    key: 'paused',
    tone: 'neutral',
  });
  assert.deepEqual(monitorState({ enabled: true, last_up: null, last_status: 0 }), { key: 'pending', tone: 'neutral' });
  assert.deepEqual(monitorState({ enabled: true, last_up: true, last_status: 200 }), { key: 'up', tone: 'done' });
  assert.deepEqual(monitorState({ enabled: true, last_up: false, last_status: 503 }), {
    key: 'down_code',
    tone: 'problem',
    code: 503,
  });
  assert.deepEqual(monitorState({ enabled: true, last_up: false, last_status: 0 }), { key: 'down', tone: 'problem' });
});

test('resultsSummary counts successes and reads the newest latency', () => {
  assert.equal(resultsSummary([]), null);
  assert.deepEqual(
    resultsSummary([
      { up: true, latency_ms: 120, checked_at: '2026-11-06T10:02:00Z' },
      { up: false, latency_ms: 3000, checked_at: '2026-11-06T10:01:00Z' },
      { up: true, latency_ms: 90, checked_at: '2026-11-06T10:00:00Z' },
      { up: true, latency_ms: 95, checked_at: '2026-11-06T09:59:00Z' },
    ]),
    { up: 3, total: 4, ratio: 0.75, latency: 120 },
  );
});

test('componentStatus falls back for an unknown status', () => {
  assert.deepEqual(componentStatus('operational'), { key: 'operational', tone: 'done' });
  assert.deepEqual(componentStatus('major_outage'), { key: 'major_outage', tone: 'problem' });
  assert.deepEqual(componentStatus('maintenance'), { key: 'maintenance', tone: 'neutral' });
  assert.deepEqual(componentStatus('investigating'), { key: null, tone: 'neutral' });
  assert.deepEqual(componentStatus(undefined), { key: null, tone: 'neutral' });
  // Not fooled by inherited object keys.
  assert.deepEqual(componentStatus('toString').key, null);
});

test('groupComponents keeps the status page order', () => {
  const groups = groupComponents([
    { uuid: '1', name: 'API', group_name: 'Platform', sort_order: 2 },
    { uuid: '2', name: 'Panel', group_name: '', sort_order: 3 },
    { uuid: '3', name: 'Widget', group_name: 'Platform', sort_order: 1 },
    { uuid: '4', name: 'E-posta', sort_order: 3 },
  ]);
  assert.deepEqual(
    groups.map((g) => [g.group, g.items.map((i) => i.name)]),
    [
      ['', ['E-posta', 'Panel']],
      ['Platform', ['Widget', 'API']],
    ],
  );
});
