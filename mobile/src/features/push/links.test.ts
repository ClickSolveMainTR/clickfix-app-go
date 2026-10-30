import assert from 'node:assert/strict';
import { test } from 'node:test';

import { appRoute, pushTarget } from './links.ts';

const id = '0b6f3c1e-8a52-4f0e-9a51-6d3f5c2b1a90';
const session = 'AbC_123-xyz9AbC_123-xyz9AbC_123-xyz9AbC_123'.slice(0, 43);

test('a conversation push opens the conversation', () => {
  assert.equal(appRoute(`/inbox?c=${id}`), `/conversation/${id}`);
  assert.equal(appRoute(`https://app.clickfix.io/inbox?c=${id}`), `/conversation/${id}`);
});

test('the inbox without a valid conversation opens the inbox', () => {
  assert.equal(appRoute('/inbox'), '/');
  assert.equal(appRoute('/inbox?c=nope'), '/');
});

test('a QR sign-in link opens the approval screen', () => {
  assert.equal(appRoute(`https://app.clickfix.io/login/qr/${session}`), `/qr-login/${session}`);
  assert.equal(appRoute('/login/qr/short'), null);
  assert.equal(appRoute(`/login/qr/${session}/x`), null);
});

test('video links open the guest join or the member room screen', () => {
  const code = 'AQIDBAUGBwgJCgsMDQ4PEAERERERERERERERERERERERERERERE'.slice(0, 51);
  assert.equal(appRoute(`https://getclickfix.com/r/${code}`), `/r/${code}`);
  assert.equal(appRoute('/r/short'), null);
  assert.equal(appRoute(`/video/${id}`), `/video/${id}`);
  assert.equal(appRoute('/video/nope'), null);
});

test('anything else opens nothing', () => {
  assert.equal(appRoute(undefined), null);
  assert.equal(appRoute(''), null);
  assert.equal(appRoute('/platform/exports/x'), null);
});

test('pushTarget reads the organization and conversation of a push', () => {
  const c = '0b7c9f2e-1a2b-4c3d-8e9f-001122334455';
  assert.deepEqual(pushTarget({ url: `/inbox?c=${c}`, organization: 'acme-ltd', conversation_uuid: c }), {
    route: `/conversation/${c}`,
    organization: 'acme-ltd',
    conversation: c,
  });
  // Older pushes without the organization still open the conversation.
  assert.deepEqual(pushTarget({ url: `/inbox?c=${c}` }), {
    route: `/conversation/${c}`,
    organization: null,
    conversation: c,
  });
  // Malformed values are dropped.
  assert.deepEqual(pushTarget({ url: '/platform/exports/1', organization: '../x', conversation_uuid: 'nope' }), {
    route: null,
    organization: null,
    conversation: null,
  });
  assert.deepEqual(pushTarget(null), { route: null, organization: null, conversation: null });
});

test('panel links to records open their mobile screens', () => {
  const c = '0b7c9f2e-1a2b-4c3d-8e9f-001122334455';
  assert.equal(appRoute(`/inbox?status=all&c=${c}`), `/conversation/${c}`);
  assert.equal(appRoute(`/inbox?status=all&contact=${c}`), `/contact/${c}`);
  assert.equal(appRoute(`/inbox?status=all&company=${c}`), `/company/${c}`);
  assert.equal(appRoute(`/companies/${c}`), `/company/${c}`);
  assert.equal(appRoute(`/contacts/${c}`), `/contact/${c}`);
  assert.equal(appRoute(`/settings/knowledge/${c}`), `/article/${c}`);
  assert.equal(appRoute('/billing'), '/more/billing');
  assert.equal(appRoute('/channels/connect'), '/more/channels');
  assert.equal(appRoute('/companies/not-a-uuid'), null);
});
