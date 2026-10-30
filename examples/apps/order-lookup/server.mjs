// ClickFix example app: order lookup for the conversation sidebar.
//
//   node server.mjs            (Node 22+, no dependencies)
//
// Environment:
//   CLICKFIX_APP_ID     App id from ClickFix (Uygulamalar → app → "Uygulama kimliği"). Required.
//   CLICKFIX_API_URL    ClickFix API origin. Default https://api.getclickfix.com
//   CLICKFIX_API_KEY    API key with conversations:read. Without it the app runs in demo mode.
//   PANEL_ORIGINS       Comma-separated origins allowed to frame the app. Default https://getclickfix.com
//   PORT                Default 8787
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { extname, join, normalize } from 'node:path';
import { fileURLToPath } from 'node:url';

import { keysFromJWKS, verifyContextToken } from './lib/context-token.mjs';

const root = fileURLToPath(new URL('.', import.meta.url));
const API = (process.env.CLICKFIX_API_URL || 'https://api.getclickfix.com').replace(/\/$/, '');
const APP_ID = process.env.CLICKFIX_APP_ID || '';
const API_KEY = process.env.CLICKFIX_API_KEY || '';
const PANEL_ORIGINS = (process.env.PANEL_ORIGINS || 'https://getclickfix.com').split(',').map((o) => o.trim()).filter(Boolean);
const PORT = Number(process.env.PORT || 8787);
const orders = JSON.parse(await readFile(join(root, 'data/orders.json'), 'utf8'));

const TYPES = { '.html': 'text/html; charset=utf-8', '.js': 'text/javascript; charset=utf-8', '.css': 'text/css; charset=utf-8', '.svg': 'image/svg+xml' };

let jwks = { keys: new Map(), at: 0 };
async function signingKeys() {
  if (Date.now() - jwks.at < 10 * 60_000 && jwks.keys.size) return jwks.keys;
  const res = await fetch(`${API}/v1/app-context/jwks`, { signal: AbortSignal.timeout(10_000) });
  if (!res.ok) throw new Error(`jwks ${res.status}`);
  jwks = { keys: keysFromJWKS(await res.json()), at: Date.now() };
  return jwks.keys;
}

/** The customer's email for the conversation, read with the public API. */
async function contactEmail(conversationUUID) {
  const res = await fetch(`${API}/v1/public-api/conversations/${encodeURIComponent(conversationUUID)}`, {
    headers: { Authorization: `Bearer ${API_KEY}` },
    signal: AbortSignal.timeout(10_000),
  });
  if (!res.ok) throw new Error(`conversation ${res.status}`);
  const body = await res.json();
  return body?.data?.contact?.email ?? null;
}

function send(res, status, body, headers = {}) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store', ...headers });
  res.end(JSON.stringify(body));
}

async function lookup(req, res) {
  const token = (req.headers.authorization || '').replace(/^Bearer /, '');
  let keys;
  try {
    keys = await signingKeys();
  } catch {
    return send(res, 503, { error: 'jwks_unavailable' });
  }
  let claims;
  try {
    claims = verifyContextToken(token, { keys, appId: APP_ID });
  } catch (err) {
    return send(res, 401, { error: 'invalid_context', detail: String(err.message) });
  }
  if (!claims.conversation_uuid) return send(res, 200, { email: null, orders: [], reason: 'no_conversation' });
  try {
    const email = API_KEY ? await contactEmail(claims.conversation_uuid) : 'demo@example.com';
    const found = email ? orders.filter((o) => o.email.toLowerCase() === email.toLowerCase()) : [];
    return send(res, 200, { email, demo: !API_KEY, orders: found });
  } catch {
    return send(res, 502, { error: 'lookup_failed' });
  }
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url, 'http://localhost');
  if (req.method === 'GET' && url.pathname === '/api/orders') return lookup(req, res);
  // The page posts `clickfix.ready` only to these origins and accepts messages only from them.
  if (req.method === 'GET' && url.pathname === '/api/config') return send(res, 200, { panel_origins: PANEL_ORIGINS });
  if (req.method !== 'GET') return send(res, 405, { error: 'method_not_allowed' });

  const rel = normalize(url.pathname === '/' ? '/index.html' : url.pathname).replace(/^(\.\.[/\\])+/, '');
  const file = join(root, 'public', rel);
  if (!file.startsWith(join(root, 'public'))) return send(res, 404, { error: 'not_found' });
  try {
    const body = await readFile(file);
    res.writeHead(200, {
      'Content-Type': TYPES[extname(file)] || 'application/octet-stream',
      // Only the ClickFix panel may frame this app.
      'Content-Security-Policy': `default-src 'self'; frame-ancestors ${PANEL_ORIGINS.join(' ')}`,
      'Referrer-Policy': 'no-referrer',
      'X-Content-Type-Options': 'nosniff',
    });
    res.end(body);
  } catch {
    send(res, 404, { error: 'not_found' });
  }
});

if (!APP_ID) console.warn('CLICKFIX_APP_ID is not set: every context token will be rejected.');
if (!API_KEY) console.warn('CLICKFIX_API_KEY is not set: demo mode, showing orders of demo@example.com.');
server.listen(PORT, () => console.log(`order-lookup listening on :${PORT}`));
