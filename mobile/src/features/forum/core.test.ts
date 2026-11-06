import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  acceptedReply,
  categoryName,
  flagLabel,
  isReply,
  isRisky,
  needsConfirm,
  outcomeStatus,
  readModerationItem,
  type T,
} from './core.ts';

const t: T = (key, params) => (params ? `${key}(${Object.values(params).join(',')})` : key);

test('readModerationItem reads the open API object and never invents fields', () => {
  assert.deepEqual(
    readModerationItem({
      uuid: 'p-1',
      title: 'Kargo nerede?',
      body: 'Siparişim gelmedi',
      flag_reason: 'link_spam',
      topic_id: null,
      created_at: '2026-11-05T10:00:00Z',
      status: 'pending',
    }),
    {
      uuid: 'p-1',
      title: 'Kargo nerede?',
      body: 'Siparişim gelmedi',
      flag_reason: 'link_spam',
      topic_id: null,
      created_at: '2026-11-05T10:00:00Z',
    },
  );
  const sparse = readModerationItem({ uuid: 'p-2', topic_id: 14, title: 42 });
  assert.deepEqual(sparse, { uuid: 'p-2', title: '', body: '', flag_reason: '', topic_id: 14, created_at: '' });
  assert.equal(isReply(sparse!), true);
});

test('a queue row without an id is dropped', () => {
  assert.equal(readModerationItem({ title: 'x' }), null);
  assert.equal(readModerationItem(null), null);
});

test('flagLabel translates known reasons, quotes AI notes and shows the rest readable', () => {
  assert.equal(flagLabel(t, ''), null);
  assert.equal(flagLabel(t, 'link_spam'), 'forum.flag.link_spam');
  assert.equal(flagLabel(t, 'ai: küfür içeriyor'), 'forum.flag.ai(küfür içeriyor)');
  assert.equal(flagLabel(t, 'manual_hold'), 'manual hold');
});

test('isRisky marks the flags that warn the moderator', () => {
  assert.equal(isRisky('unsafe_instructions'), true);
  assert.equal(isRisky('ai: spam'), true);
  assert.equal(isRisky('awaiting_review'), false);
  assert.equal(isRisky(''), false);
});

test('reject and spam both hide the post and ask first; approve publishes at once', () => {
  assert.equal(outcomeStatus('approve'), 'published');
  assert.equal(outcomeStatus('reject'), 'rejected');
  assert.equal(outcomeStatus('spam'), 'rejected');
  assert.equal(needsConfirm('approve'), false);
  assert.equal(needsConfirm('reject'), true);
  assert.equal(needsConfirm('spam'), true);
});

test('categoryName falls back to a readable slug', () => {
  const categories = [{ slug: 'kargo', name: 'Kargo ve teslimat' }];
  assert.equal(categoryName(categories, 'kargo'), 'Kargo ve teslimat');
  assert.equal(categoryName(categories, 'iade-ve-degisim'), 'iade ve degisim');
  assert.equal(categoryName(undefined, 'genel'), 'genel');
});

test('acceptedReply prefers the loaded topic over the list row', () => {
  assert.equal(acceptedReply({ accepted_answer: { uuid: 'r-2' } }, { accepted_reply_uuid: 'r-1' }), 'r-2');
  assert.equal(acceptedReply({ accepted_answer: null }, { accepted_reply_uuid: 'r-1' }), null);
  assert.equal(acceptedReply(undefined, { accepted_reply_uuid: 'r-1' }), 'r-1');
  assert.equal(acceptedReply(undefined), null);
});
