// Verifies the ClickFix app context token (EdDSA JWT) without dependencies.
//
// The panel posts the token to the iframe in a `clickfix.context` message.
// Your server must check: signature against the JWKS published at
// GET {api}/v1/app-context/jwks, alg=EdDSA, iss=clickfix:sidebar-apps,
// aud=your app's id, and the nbf/exp window. Only then trust the claims.
import { createPublicKey, verify } from 'node:crypto';

export const ISSUER = 'clickfix:sidebar-apps';
const SKEW_SECONDS = 30;

function b64url(part) {
  return Buffer.from(part, 'base64url');
}

/** Turns the JWKS envelope (`{success, data: {keys}}` or `{keys}`) into kid → KeyObject. */
export function keysFromJWKS(body) {
  const keys = body?.data?.keys ?? body?.keys ?? [];
  const out = new Map();
  for (const jwk of keys) {
    if (jwk.kty !== 'OKP' || jwk.crv !== 'Ed25519') continue;
    out.set(jwk.kid, createPublicKey({ key: { kty: 'OKP', crv: 'Ed25519', x: jwk.x }, format: 'jwk' }));
  }
  return out;
}

/**
 * Returns the verified claims or throws. `keys` comes from keysFromJWKS,
 * `appId` is the app's id shown in ClickFix (the token audience).
 */
export function verifyContextToken(token, { keys, appId, now = Date.now() }) {
  if (typeof token !== 'string') throw new Error('token missing');
  const parts = token.split('.');
  if (parts.length !== 3) throw new Error('malformed token');
  const [h, p, s] = parts;
  const header = JSON.parse(b64url(h).toString('utf8'));
  if (header.alg !== 'EdDSA') throw new Error('unexpected alg');
  const key = keys.get(header.kid) ?? (keys.size === 1 ? [...keys.values()][0] : undefined);
  if (!key) throw new Error('unknown key');
  if (!verify(null, Buffer.from(`${h}.${p}`), key, b64url(s))) throw new Error('bad signature');

  const claims = JSON.parse(b64url(p).toString('utf8'));
  const t = Math.floor(now / 1000);
  const aud = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (claims.iss !== ISSUER) throw new Error('unexpected issuer');
  if (!appId || !aud.includes(appId)) throw new Error('unexpected audience');
  if (typeof claims.exp !== 'number' || claims.exp + SKEW_SECONDS < t) throw new Error('token expired');
  if (typeof claims.nbf === 'number' && claims.nbf - SKEW_SECONDS > t) throw new Error('token not yet valid');
  return claims;
}
