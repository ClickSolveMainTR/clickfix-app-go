import assert from 'node:assert/strict';
import { test } from 'node:test';

import { markdownToText, parseInline, parseMarkdown } from './markdown-core.ts';

test('inline bold, italic, code and links', () => {
  assert.deepEqual(parseInline('Bir **önemli** not, `kod` ve [link](https://x.y) _eğik_'), [
    { kind: 'text', text: 'Bir ' },
    { kind: 'bold', text: 'önemli' },
    { kind: 'text', text: ' not, ' },
    { kind: 'code', text: 'kod' },
    { kind: 'text', text: ' ve ' },
    { kind: 'link', text: 'link', href: 'https://x.y' },
    { kind: 'text', text: ' ' },
    { kind: 'italic', text: 'eğik' },
  ]);
});

test('blocks: heading, paragraph joined across lines, lists, quote, code', () => {
  const blocks = parseMarkdown('# İade\n\nİlk satır\nikinci satır\n\n- bir\n- iki\n1. üç\n> alıntı\n```\nx = 1\n```');
  assert.deepEqual(
    blocks.map((b) => b.kind),
    ['heading', 'paragraph', 'list', 'list', 'quote', 'code'],
  );
  const para = blocks[1];
  assert.equal(para.kind === 'paragraph' && para.inline[0].text, 'İlk satır ikinci satır');
  const bullets = blocks[2];
  assert.equal(bullets.kind === 'list' && bullets.items.length, 2);
  assert.equal(blocks[5].kind === 'code' && blocks[5].text, 'x = 1');
});

test('markdownToText drops the markup', () => {
  assert.equal(markdownToText('## Başlık\n\n**Kalın** metin\n\n- a\n- b'), 'Başlık\n\nKalın metin\n\n• a\n• b');
});

test('tables keep inline formatting, escaped pipes and incomplete rows', () => {
  const blocks = parseMarkdown('| Name | Value |\n| :--- | ---: |\n| **Plan** | A\\|B |\n| Empty |');
  assert.equal(blocks[0].kind, 'table');
  if (blocks[0].kind !== 'table') throw new Error('Expected table');
  assert.deepEqual(blocks[0].rows[0][0], [{ kind: 'bold', text: 'Plan' }]);
  assert.deepEqual(blocks[0].rows[0][1], [{ kind: 'text', text: 'A|B' }]);
  assert.deepEqual(blocks[0].rows[1][1], []);
  assert.equal(markdownToText('| A | B |\n| --- | --- |\n| 1 | 2 |'), 'A | B\n1 | 2');
});

test('table syntax inside fenced code remains code', () => {
  assert.deepEqual(parseMarkdown('```\n| A | B |\n| --- | --- |\n```'), [
    { kind: 'code', text: '| A | B |\n| --- | --- |' },
  ]);
});
