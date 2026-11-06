import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  callFailed,
  counterpart,
  directionLabel,
  duration,
  findCall,
  statusLabel,
  type T,
  transcriptStatusLabel,
} from './core.ts';

const t: T = (key) => key;

test('duration reads as minutes and seconds', () => {
  assert.equal(duration(0), '0:00');
  assert.equal(duration(65), '1:05');
  assert.equal(duration(4504), '75:04');
  assert.equal(duration(-3), '0:00');
  assert.equal(duration(null), '0:00');
  assert.equal(duration(12.9), '0:12');
});

test('a status or direction without a label is shown readable, never as a translation key', () => {
  assert.equal(statusLabel(t, 'no-answer'), 'calls.status.no-answer');
  assert.equal(statusLabel(t, 'voicemail_left'), 'voicemail left');
  assert.equal(directionLabel(t, 'internal'), 'calls.direction.internal');
  assert.equal(directionLabel(t, 'forwarded'), 'forwarded');
  assert.equal(transcriptStatusLabel(t, 'processing'), 'calls.transcript_status.processing');
  assert.equal(transcriptStatusLabel(t, 'rate.limited'), 'rate limited');
});

test('callFailed marks the calls nobody talked on', () => {
  assert.equal(callFailed('busy'), true);
  assert.equal(callFailed('no-answer'), true);
  assert.equal(callFailed('completed'), false);
  assert.equal(callFailed('ringing'), false);
});

test('counterpart is the caller of an inbound call and the callee otherwise', () => {
  const numbers = { from_number: '+905551112233', to_number: '+908502223344' };
  assert.equal(counterpart({ direction: 'inbound', ...numbers }), '+905551112233');
  assert.equal(counterpart({ direction: 'outbound', ...numbers }), '+908502223344');
  assert.equal(counterpart({ direction: 'inbound', from_number: '', to_number: '+908502223344' }), '+908502223344');
  assert.equal(counterpart({ direction: 'outbound', from_number: '', to_number: '' }), '—');
});

test('findCall looks through every loaded page', () => {
  const pages = [{ items: [{ uuid: 'a' }, { uuid: 'b' }] }, { items: [{ uuid: 'c' }] }];
  assert.deepEqual(findCall(pages, 'c'), { uuid: 'c' });
  assert.equal(findCall(pages, 'z'), undefined);
  assert.equal(findCall(undefined, 'a'), undefined);
});
