import assert from 'node:assert/strict';
import { test } from 'node:test';

import { formatPhone, identityLabel } from './display.ts';

test('Turkish phone variants display the same international grouping', () => {
  for (const phone of ['905550000032', '+90 555 000 00 32', '05550000032', '5550000032']) {
    assert.equal(formatPhone(phone), '+90 555 000 00 32');
  }
  assert.equal(formatPhone('+44 20 1234 5678'), '+44 20 1234 5678');
});

test('channel labels show human names and handles, never external ids', () => {
  const identity = { channel_type: 'instagram', display_name: 'Deniz', username: '@deniz', external_id: 'demo-ig-32' };
  assert.equal(identityLabel(identity, 'Contact'), 'Deniz · @deniz');
  assert.equal(identityLabel({ ...identity, display_name: '', username: '' }, 'Contact'), 'Contact');
});
