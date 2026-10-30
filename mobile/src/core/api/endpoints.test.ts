import assert from 'node:assert/strict';
import { test } from 'node:test';

import { classifyTransportError, normalizeApiUrl, originOf, resolveRealtimeUrl } from './endpoints.ts';

const FALLBACK = 'http://localhost:3000/api/mobile';

test('normalizeApiUrl trims whitespace and trailing slashes from an EAS value', () => {
  assert.equal(
    normalizeApiUrl(' https://getclickfix.com/api/mobile/\n', FALLBACK),
    'https://getclickfix.com/api/mobile',
  );
});

test('normalizeApiUrl drops a trailing /v1 so calls never hit /v1/v1', () => {
  assert.equal(
    normalizeApiUrl('https://getclickfix.com/api/mobile/v1/', FALLBACK),
    'https://getclickfix.com/api/mobile',
  );
  assert.equal(normalizeApiUrl('http://192.168.1.5:8080/v1', FALLBACK), 'http://192.168.1.5:8080');
});

test('normalizeApiUrl falls back when the variable is missing or blank', () => {
  assert.equal(normalizeApiUrl(undefined, FALLBACK), FALLBACK);
  assert.equal(normalizeApiUrl('   ', FALLBACK), FALLBACK);
});

test('originOf keeps scheme, host and port only', () => {
  assert.equal(originOf('https://getclickfix.com/api/mobile'), 'https://getclickfix.com');
  assert.equal(originOf('http://localhost:3000'), 'http://localhost:3000');
  assert.equal(originOf('not a url'), '');
});

const API = 'https://getclickfix.com/api/mobile';
const WS = 'wss://rt.getclickfix.com/connection/websocket';

test('resolveRealtimeUrl uses the ws_url the API returned', () => {
  assert.equal(resolveRealtimeUrl(WS, API), WS);
});

test('resolveRealtimeUrl prefers the app override', () => {
  assert.equal(resolveRealtimeUrl('ws://127.0.0.1:8000/connection/websocket', API, ` ${WS} `), WS);
});

test('resolveRealtimeUrl skips a loopback socket when the API is remote', () => {
  assert.equal(resolveRealtimeUrl('ws://127.0.0.1:8000/connection/websocket', API), null);
  assert.equal(resolveRealtimeUrl('ws://localhost:8000/connection/websocket', API), null);
});

test('resolveRealtimeUrl keeps a loopback socket for a local API (simulator)', () => {
  const local = 'ws://127.0.0.1:8000/connection/websocket';
  assert.equal(resolveRealtimeUrl(local, 'http://localhost:3000/api/mobile'), local);
});

test('resolveRealtimeUrl upgrades ws:// to wss:// behind an https API', () => {
  assert.equal(resolveRealtimeUrl('ws://rt.getclickfix.com/connection/websocket', API), WS);
});

test('resolveRealtimeUrl ignores empty or non-websocket values', () => {
  assert.equal(resolveRealtimeUrl('', API), null);
  assert.equal(resolveRealtimeUrl(undefined, API), null);
  assert.equal(resolveRealtimeUrl('https://rt.getclickfix.com', API), null);
});

test('classifyTransportError tells timeouts, bad bodies and network failures apart', () => {
  assert.equal(classifyTransportError(new TypeError('Network request failed'), false), 'network');
  assert.equal(classifyTransportError({ name: 'AbortError' }, true), 'timeout');
  assert.equal(classifyTransportError(new SyntaxError('Unexpected token <'), false), 'invalid_response');
});
