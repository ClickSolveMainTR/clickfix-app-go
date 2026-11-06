import assert from 'node:assert/strict';
import { test } from 'node:test';

import { canEdit, cleanShortcut, EMPTY_FORM, formOf, sameForm, validForm } from './core.ts';

test('cleanShortcut keeps only what the API accepts', () => {
  assert.equal(cleanShortcut('/İade Süreci!'), 'iadesreci');
  assert.equal(cleanShortcut('Kargo_takip-2'), 'kargo_takip-2');
  assert.equal(cleanShortcut('--_iade'), 'iade');
  assert.equal(cleanShortcut('a'.repeat(60)).length, 40);
});

test('validForm mirrors the API limits', () => {
  const ok = { ...EMPTY_FORM, title: 'İade', shortcut: 'iade', content: 'Merhaba {{contact.first_name}}' };
  assert.equal(validForm(ok), true);
  assert.equal(validForm({ ...ok, title: '  ' }), false);
  assert.equal(validForm({ ...ok, title: 'a'.repeat(121) }), false);
  assert.equal(validForm({ ...ok, shortcut: '' }), false);
  assert.equal(validForm({ ...ok, shortcut: '-iade' }), false);
  assert.equal(validForm({ ...ok, content: '\n ' }), false);
  assert.equal(validForm({ ...ok, content: 'a'.repeat(10001) }), false);
  assert.equal(validForm({ ...ok, category: 'a'.repeat(61) }), false);
  assert.equal(validForm({ ...ok, category: 'Kargo' }), true);
});

test('formOf and sameForm track unsaved changes', () => {
  const item = { title: 'İade', shortcut: 'iade', category: '', content: 'Metin', scope: 'organization' };
  const form = formOf(item);
  assert.equal(form.scope, 'organization');
  assert.equal(sameForm(form, formOf(item)), true);
  assert.equal(sameForm(form, { ...form, content: 'Metin 2' }), false);
  assert.equal(formOf({ ...item, scope: 'team' }).scope, 'personal');
});

test('canEdit: own personal replies, or shared ones with the write permission', () => {
  assert.equal(canEdit({ mine: true }, false), true);
  assert.equal(canEdit({ mine: false }, false), false);
  assert.equal(canEdit({ mine: false }, true), true);
});
