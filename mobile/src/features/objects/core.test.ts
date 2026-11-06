import assert from 'node:assert/strict';
import { test } from 'node:test';

import { activeDefinitions, isEmptyValue, plainText, safeUrl, valueView } from './core.ts';

const def = (type: string, extra: object = {}) => ({ type, options: [], config: {}, ...extra });

test('activeDefinitions drops archived fields and sorts by position', () => {
  const list = activeDefinitions([
    { key: 'b', label: 'B', type: 'text', position: 2, archived: false },
    { key: 'x', label: 'X', type: 'text', position: 0, archived: true },
    { key: 'a', label: 'A', type: 'text', position: 1, archived: false },
  ]);
  assert.deepEqual(
    list.map((d) => d.key),
    ['a', 'b'],
  );
});

test('isEmptyValue', () => {
  for (const v of [null, undefined, '', '  ', []]) assert.equal(isEmptyValue(v), true);
  for (const v of [0, false, 'a', ['a'], {}]) assert.equal(isEmptyValue(v), false);
});

test('plainText never prints [object Object]', () => {
  assert.equal(plainText({ amount_minor: 1250, currency: 'TRY' }), 'amount_minor: 1250, currency: TRY');
  assert.equal(plainText(['a', 2, null]), 'a, 2');
  assert.equal(plainText(false), 'false');
});

test('select and multiselect resolve labels and mark missing options archived', () => {
  const options = [
    { value: 'gold', label: 'Altın' },
    { value: 'old', label: 'Eski', archived: true },
  ];
  assert.deepEqual(valueView(def('select', { options }), 'gold'), {
    kind: 'options',
    options: [{ label: 'Altın', archived: false }],
  });
  assert.deepEqual(valueView(def('multiselect', { options }), ['old', 'gone']), {
    kind: 'options',
    options: [
      { label: 'Eski', archived: true },
      { label: 'gone', archived: true },
    ],
  });
});

test('typed values', () => {
  assert.deepEqual(valueView(def('boolean'), false), { kind: 'boolean', value: false });
  assert.deepEqual(valueView(def('money'), { amount_minor: 125050, currency: 'TRY' }), {
    kind: 'money',
    amount: 1250.5,
    currency: 'TRY',
  });
  assert.deepEqual(valueView(def('number', { config: { precision: 2 } }), '3.14159'), {
    kind: 'number',
    value: 3.14159,
    precision: 2,
  });
  assert.deepEqual(valueView(def('date'), '2026-11-06'), { kind: 'date', iso: '2026-11-06' });
  assert.deepEqual(valueView(def('user'), 'u-1'), { kind: 'user', uuid: 'u-1' });
  assert.deepEqual(valueView(def('textarea'), 'a\nb'), { kind: 'text', text: 'a\nb', multiline: true });
  assert.deepEqual(valueView(def('text'), ''), { kind: 'empty' });
});

test('a value that does not fit its type, or an unknown type, falls back to text', () => {
  assert.deepEqual(valueView(def('money'), 12), { kind: 'text', text: '12' });
  assert.deepEqual(valueView(def('number'), 'çok'), { kind: 'text', text: 'çok' });
  assert.deepEqual(valueView(def('date'), 'yarın'), { kind: 'text', text: 'yarın' });
  assert.deepEqual(valueView(def('boolean'), 'yes'), { kind: 'text', text: 'yes' });
  assert.deepEqual(valueView(def('rating'), 4), { kind: 'text', text: '4' });
  assert.deepEqual(valueView(def('location'), { lat: 41, lng: 29 }), { kind: 'text', text: 'lat: 41, lng: 29' });
});

test('links open only safe schemes', () => {
  assert.deepEqual(valueView(def('url'), 'https://ornek.com/a?b=1'), {
    kind: 'link',
    text: 'ornek.com/a?b=1',
    href: 'https://ornek.com/a?b=1',
  });
  assert.deepEqual(valueView(def('url'), 'javascript:alert(1)'), { kind: 'text', text: 'javascript:alert(1)' });
  assert.equal(safeUrl('file:///etc/passwd'), null);
  assert.equal(safeUrl('https://a.b c'), null);
  assert.deepEqual(valueView(def('email'), 'a@b.co'), { kind: 'link', text: 'a@b.co', href: 'mailto:a@b.co' });
  assert.deepEqual(valueView(def('email'), 'a@b.co?cc=x y'), { kind: 'text', text: 'a@b.co?cc=x y' });
  assert.deepEqual(valueView(def('phone'), '+90 (555) 111-22-33'), {
    kind: 'link',
    text: '+90 (555) 111-22-33',
    href: 'tel:+905551112233',
  });
  assert.deepEqual(valueView(def('phone'), 'ara beni'), { kind: 'text', text: 'ara beni' });
});
