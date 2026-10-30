import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  apnsEnvironment,
  liveActivityTokenBody,
  newDeviceId,
  SentTokens,
  shouldRenewWidgetToken,
  WIDGET_TOKEN_RENEW_AFTER_MS,
} from './tokens-core.ts';

const now = Date.parse('2026-10-30T09:00:00Z');
const hour = 60 * 60 * 1000;
const token = 'a'.repeat(64);

test('a widget token is renewed when missing, for another organization, old or close to expiry', () => {
  const fresh = { org: 'acme', issuedAt: now - hour, expiresAt: new Date(now + 71 * hour).toISOString() };
  assert.equal(shouldRenewWidgetToken(null, 'acme', now), true);
  assert.equal(shouldRenewWidgetToken(fresh, 'acme', now), false);
  assert.equal(shouldRenewWidgetToken(fresh, 'other', now), true);
  assert.equal(shouldRenewWidgetToken({ ...fresh, issuedAt: now - WIDGET_TOKEN_RENEW_AFTER_MS }, 'acme', now), true);
  assert.equal(
    shouldRenewWidgetToken({ ...fresh, expiresAt: new Date(now + 2 * hour).toISOString() }, 'acme', now),
    true,
  );
  assert.equal(shouldRenewWidgetToken({ ...fresh, expiresAt: 'garbage' }, 'acme', now), true);
});

test('live activity token bodies are validated and normalized', () => {
  assert.deepEqual(
    liveActivityTokenBody({
      deviceId: 'd1',
      kind: 'push_to_start',
      type: 'urgent_conversation',
      token: ` ${token.toUpperCase()} `,
      environment: 'production',
      activityId: 'ignored',
    }),
    { device_id: 'd1', kind: 'push_to_start', activity_type: 'urgent_conversation', token, environment: 'production' },
  );
  assert.deepEqual(
    liveActivityTokenBody({
      deviceId: 'd1',
      kind: 'update',
      type: 'call',
      token,
      environment: 'sandbox',
      activityId: 'act-1',
      ref: 'room-1',
    }),
    {
      device_id: 'd1',
      kind: 'update',
      activity_type: 'call',
      token,
      environment: 'sandbox',
      activity_id: 'act-1',
      ref: 'room-1',
    },
  );
  assert.equal(
    liveActivityTokenBody({ deviceId: 'd1', kind: 'update', type: 'call', token: 'xyz', environment: 'sandbox' }),
    null,
  );
  assert.equal(
    liveActivityTokenBody({ deviceId: '', kind: 'update', type: 'call', token, environment: 'sandbox' }),
    null,
  );
});

test('SentTokens posts each token once until released', () => {
  const sent = new SentTokens();
  const body = liveActivityTokenBody({
    deviceId: 'd1',
    kind: 'push_to_start',
    type: 'call',
    token,
    environment: 'production',
  });
  assert.ok(body);
  assert.equal(sent.claim(body), true);
  assert.equal(sent.claim(body), false);
  assert.equal(sent.claim({ ...body, activity_type: 'urgent_conversation' }), true);
  sent.release(body);
  assert.equal(sent.claim(body), true);
  sent.clear();
  assert.equal(sent.claim(body), true);
});

test('apnsEnvironment and newDeviceId', () => {
  assert.equal(apnsEnvironment(true), 'sandbox');
  assert.equal(apnsEnvironment(false), 'production');
  assert.match(newDeviceId(), /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
});
