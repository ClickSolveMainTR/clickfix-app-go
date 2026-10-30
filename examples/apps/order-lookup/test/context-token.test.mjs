import { test } from 'node:test';
import assert from 'node:assert/strict';
import { generateKeyPairSync, sign } from 'node:crypto';

import { ISSUER, keysFromJWKS, verifyContextToken } from '../lib/context-token.mjs';

const { publicKey, privateKey } = generateKeyPairSync('ed25519');
const jwks = { success: true, data: { keys: [{ ...publicKey.export({ format: 'jwk' }), kid: 'k1', alg: 'EdDSA', use: 'sig' }] } };
const keys = keysFromJWKS(jwks);
const now = Date.UTC(2026, 9, 29, 12);
const appId = '7d8a1d5e-0000-4000-8000-000000000001';

function token(claims, header = { alg: 'EdDSA', typ: 'JWT', kid: 'k1' }) {
  const enc = (o) => Buffer.from(JSON.stringify(o)).toString('base64url');
  const body = `${enc(header)}.${enc(claims)}`;
  return `${body}.${sign(null, Buffer.from(body), privateKey).toString('base64url')}`;
}
const base = () => ({ iss: ISSUER, aud: [appId], sub: 'user', nbf: now / 1000 - 1, exp: now / 1000 + 120, conversation_uuid: 'c1', scopes: ['conversation.read'] });

test('accepts a valid token', () => {
  assert.equal(verifyContextToken(token(base()), { keys, appId, now }).conversation_uuid, 'c1');
});

test('rejects wrong audience, issuer, expiry and tampering', () => {
  assert.throws(() => verifyContextToken(token(base()), { keys, appId: 'other', now }), /audience/);
  assert.throws(() => verifyContextToken(token({ ...base(), iss: 'x' }), { keys, appId, now }), /issuer/);
  assert.throws(() => verifyContextToken(token(base()), { keys, appId, now: now + 10 * 60_000 }), /expired/);
  const t = token(base()).split('.');
  t[1] = Buffer.from(JSON.stringify({ ...base(), scopes: ['conversation.reply'] })).toString('base64url');
  assert.throws(() => verifyContextToken(t.join('.'), { keys, appId, now }), /signature/);
  assert.throws(() => verifyContextToken(token(base(), { alg: 'none', kid: 'k1' }), { keys, appId, now }), /alg/);
});
