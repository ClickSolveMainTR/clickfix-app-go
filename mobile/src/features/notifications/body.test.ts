import assert from 'node:assert/strict';
import { test } from 'node:test';

import { notificationBody, notificationBodyRoute } from './body.ts';

test('existing billing notifications expose a readable label and mobile target', () => {
  const body = 'Aboneliğiniz etkin. Faturalama: /billing';
  assert.equal(notificationBody(body, 'Faturalama sayfası'), 'Aboneliğiniz etkin. Faturalama: Faturalama sayfası');
  assert.equal(notificationBodyRoute(body), '/more/billing');
  assert.equal(notificationBody('Billing: /billing?tab=plan', 'Billing page'), 'Billing: Billing page');
});

test('plain notification copy drops raw internal paths without damaging normal text', () => {
  assert.equal(notificationBody('Go /settings/channels', 'Billing page'), 'Go');
  assert.equal(notificationBody('SLA 7/24', 'Billing page'), 'SLA 7/24');
  assert.equal(notificationBody('See https://example.com/help', 'Billing page'), 'See https://example.com/help');
  assert.equal(notificationBody(null, 'Billing page'), '');
});
