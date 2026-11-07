import assert from 'node:assert/strict';
import { test } from 'node:test';

import { appHost, groupApps, placementKey, readable, scopeKey } from './core.ts';

test('known and unknown placements and scopes', () => {
  assert.equal(placementKey('nav'), 'nav');
  assert.equal(placementKey('dashboard'), null);
  assert.equal(scopeKey('conversation.reply'), 'conversation_reply');
  assert.equal(scopeKey('conversation.export'), null);
  assert.equal(readable('conversation.export'), 'Conversation export');
});

test('groupApps lists enabled apps by placement, unknown placements last', () => {
  const app = (uuid: string, placement: string, enabled = true) => ({ uuid, name: uuid, placement, enabled });
  const groups = groupApps([
    app('n1', 'nav'),
    app('x1', 'dashboard'),
    app('c1', 'conversation_sidebar'),
    app('off', 'conversation_sidebar', false),
    app('p1', 'contact_panel'),
    app('c2', 'conversation_sidebar'),
  ]);
  assert.deepEqual(
    groups.map((g) => [g.placement, g.apps.map((a) => a.uuid)]),
    [
      ['conversation_sidebar', ['c1', 'c2']],
      ['contact_panel', ['p1']],
      ['nav', ['n1']],
      ['dashboard', ['x1']],
    ],
  );
});

test('appHost shows the host of an https URL only', () => {
  assert.equal(appHost('https://CRM.example.com/panel?x=1'), 'crm.example.com');
  assert.equal(appHost('http://crm.example.com'), null);
  assert.equal(appHost('https://user:pass@evil.example/'), null);
  assert.equal(appHost('javascript:alert(1)'), null);
});
