import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  acceptedReply,
  AlreadyHandled,
  categoryName,
  decisionProblem,
  flagLabel,
  isLongBody,
  isReply,
  isRisky,
  needsConfirm,
  outcomeStatus,
  readModerationItem,
  removedLine,
  stillPending,
  type T,
  visibilityCopy,
  visibilityProblem,
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

test('stillPending looks through every loaded page of the queue', () => {
  const pages = [{ items: [{ uuid: 'a' }] }, { items: [{ uuid: 'b' }] }];
  assert.equal(stillPending(pages, 'b'), true);
  assert.equal(stillPending(pages, 'z'), false);
  assert.equal(stillPending(undefined, 'a'), false);
});

test('decisionProblem explains a post that is gone and a reply whose topic waits', () => {
  assert.equal(decisionProblem(new AlreadyHandled(), null), 'forum.already_handled');
  assert.equal(decisionProblem(new Error('x'), 404), 'forum.already_handled');
  assert.equal(decisionProblem(new Error('x'), 409), 'forum.topic_first');
  assert.equal(decisionProblem(new Error('x'), 500), null);
  assert.equal(decisionProblem(new Error('x'), null), null);
});

test('isLongBody marks text the queue row would cut', () => {
  assert.equal(isLongBody('kısa'), false);
  assert.equal(isLongBody('a'.repeat(321)), true);
  assert.equal(isLongBody(Array.from({ length: 9 }, () => 'satır').join('\n')), true);
});

test('taking down and republishing ask first, naming the kind of post', () => {
  assert.deepEqual(visibilityCopy('unpublish', 'reply'), {
    title: 'forum.unpublish_reply_title',
    body: 'forum.unpublish_reply_body',
    action: 'forum.unpublish',
    destructive: true,
  });
  assert.equal(visibilityCopy('republish', 'topic').title, 'forum.republish_topic_title');
  assert.equal(visibilityCopy('republish', 'topic').destructive, false);
});

test('a refused takedown or republish tells the moderator what happened', () => {
  assert.equal(visibilityProblem(409, 'unpublish'), 'forum.unpublish_conflict');
  assert.equal(visibilityProblem(409, 'republish'), 'forum.republish_conflict');
  assert.equal(visibilityProblem(404, 'unpublish'), 'forum.already_handled');
  assert.equal(visibilityProblem(500, 'republish'), null);
});

test('the removed line names who, when and the reason only when given', () => {
  assert.equal(
    removedLine(t, { removed_by_name: 'Ayşe', removed_reason: '' }, '5 Kas'),
    'forum.removed_by(Ayşe,5 Kas)',
  );
  assert.equal(
    removedLine(t, { removed_by_name: null, removed_reason: 'spam' }, '5 Kas'),
    '5 Kas · forum.removed_reason(spam)',
  );
});
