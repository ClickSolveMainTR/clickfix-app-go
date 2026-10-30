import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  ASSIGN_ACTION,
  notificationOf,
  OPEN_ACTION,
  PUSH_CATEGORIES,
  READ_ACTION,
  REPLY_ACTION,
  runsInBackground,
} from './actions-core.ts';

test('every category the backend sends is registered', () => {
  // notifications/usecase/expo.go PushCategory* constants.
  for (const id of ['conversation', 'conversation_assign', 'generic']) assert.ok(PUSH_CATEGORIES[id], id);
});

test('open is last (a tap opens anyway; Android shows three buttons)', () => {
  for (const actions of Object.values(PUSH_CATEGORIES)) {
    assert.equal(actions[actions.length - 1], OPEN_ACTION);
    assert.ok(actions.includes(READ_ACTION));
  }
  assert.deepEqual(PUSH_CATEGORIES.conversation_assign.slice(0, 3), [ASSIGN_ACTION, REPLY_ACTION, READ_ACTION]);
});

test('only open brings the app to the foreground', () => {
  assert.equal(runsInBackground(OPEN_ACTION), false);
  for (const a of [REPLY_ACTION, READ_ACTION, ASSIGN_ACTION]) assert.equal(runsInBackground(a), true);
});

test('notification id must be a uuid', () => {
  assert.equal(
    notificationOf({ notification_uuid: '0b6f3c1e-8a52-4f0e-9a51-6d3f5c2b1a90' }),
    '0b6f3c1e-8a52-4f0e-9a51-6d3f5c2b1a90',
  );
  assert.equal(notificationOf({ notification_uuid: '../x' }), null);
  assert.equal(notificationOf(null), null);
});
