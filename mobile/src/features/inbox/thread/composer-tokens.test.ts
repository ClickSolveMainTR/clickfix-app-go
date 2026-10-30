import assert from 'node:assert/strict';
import { test } from 'node:test';
import { insertToken, parseTrigger, serializeMentions, mentionParts } from './composer-tokens.ts';

test('triggers require a word boundary and stop at whitespace', () => {
  for (const text of ['@', '/', 'hi @ali', 'hi /not', '\n@Çağ']) assert.ok(parseTrigger(text), text);
  for (const text of ['a@b.com', 'https://example.com', 'word/not', '@Ali ', '/not ', 'plain'])
    assert.equal(parseTrigger(text), null, text);
  assert.equal(parseTrigger('hi @ali')?.query, 'ali');
  assert.equal(parseTrigger('hi /not')?.kind, '/');
  assert.equal(parseTrigger('@a end', 2)?.query, 'a');
});
test('insertion replaces only the active token, preserves surrounding text and caret', () => {
  assert.deepEqual(insertToken('Hi @al end', parseTrigger('Hi @al end', 6)!, '@Ali Can'), {
    text: 'Hi @Ali Can end',
    caret: 11,
  });
  assert.deepEqual(insertToken('/not', parseTrigger('/not')!, ''), { text: '', caret: 0 });
  assert.deepEqual(insertToken('/hi', parseTrigger('/hi')!, 'Hello'), { text: 'Hello ', caret: 6 });
});
test('mention serialization deduplicates, ignores deleted and partial names', () => {
  const mentions = [
    { uuid: '1', name: 'Ali Can' },
    { uuid: '1', name: 'Ali Can' },
    { uuid: '2', name: 'Ayşe' },
  ];
  assert.deepEqual(serializeMentions('@Ali Can, hello @Ayşe!', mentions), ['1', '2']);
  assert.deepEqual(serializeMentions('@Ali Caner mail@Ayşe', mentions), []);
  assert.deepEqual(serializeMentions('', mentions), []);
});

test('insertion replaces the full token when the caret is in its middle', () => {
  assert.deepEqual(insertToken('hi @alice end', parseTrigger('hi @alice end', 6)!, '@Ali'), {
    text: 'hi @Ali end',
    caret: 7,
  });
});
test('highlighting follows mention boundaries, including names with punctuation', () => {
  const members = [{ uuid: '1', name: 'Ali (TR)' }];
  assert.deepEqual(mentionParts('@Ali (TR), hello', members), [
    { text: '@Ali (TR)', mention: true },
    { text: ', hello', mention: false },
  ]);
  assert.deepEqual(mentionParts('@Ali (TR)x', members), [{ text: '@Ali (TR)x', mention: false }]);
});
