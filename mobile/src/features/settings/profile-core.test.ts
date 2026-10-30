import assert from 'node:assert/strict';
import test from 'node:test';
import { profileEditState } from './profile-core.ts';

test('profile save requires a meaningful name or surname change', () => {
  assert.equal(profileEditState({ name: 'Ada', surname: 'Kaya' }, { name: ' Ada ', surname: 'Kaya ' }).dirty, false);
  const changed = profileEditState({ name: 'Ada', surname: 'Kaya' }, { name: 'Ada', surname: '' });
  assert.equal(changed.dirty && changed.valid, true);
  assert.deepEqual(changed.payload, { name: 'Ada', surname: '' });
});
test('profile rejects an empty name and accepts a profile loaded without names', () => {
  assert.equal(profileEditState({ name: 'Ada' }, { name: '  ' }).valid, false);
  assert.deepEqual(profileEditState(undefined, { name: ' Ada ' }), {
    payload: { name: 'Ada', surname: '' }, dirty: true, valid: true,
  });
});
