/**
 * Pure URL rules for the API and the realtime socket (CF-79). Kept free of
 * app imports so they run under `node --test`.
 */

/**
 * The API origin from `EXPO_PUBLIC_API_URL`: trimmed (EAS / CI values often
 * carry a trailing newline or space), without trailing slashes and without a
 * trailing `/v1` (the client adds `/v1/...` itself, so `.../api/mobile/v1`
 * would call `/v1/v1/...`). Empty or missing → `fallback`.
 */
export function normalizeApiUrl(raw: string | undefined | null, fallback: string): string {
  let url = (raw ?? '').trim();
  if (!url) url = fallback;
  url = url.replace(/\/+$/, '');
  url = url.replace(/\/v1$/i, '');
  return url.replace(/\/+$/, '');
}

/** `scheme://host[:port]` of a URL, or '' when it is not absolute. */
export function originOf(url: string): string {
  const m = /^([a-z][a-z\d+.-]*:\/\/[^/?#]+)/i.exec(url.trim());
  return m ? m[1].toLowerCase() : '';
}

function hostOf(url: string): string {
  const o = originOf(url).replace(/^[a-z][a-z\d+.-]*:\/\//i, '');
  // Drop user info and the port; keep IPv6 brackets.
  const host = o.replace(/^[^@]*@/, '');
  if (host.startsWith('[')) return host.slice(0, host.indexOf(']') + 1);
  return host.replace(/:\d+$/, '');
}

/** Hosts a phone can never reach on its own: they name the phone itself. */
export function isLoopbackHost(host: string): boolean {
  const h = host.toLowerCase();
  return h === 'localhost' || h.endsWith('.localhost') || h.startsWith('127.') || h === '0.0.0.0' || h === '[::1]';
}

/**
 * The Centrifugo WebSocket URL the app connects to.
 *
 * - `override` (`EXPO_PUBLIC_REALTIME_URL`) wins when set.
 * - Otherwise the `ws_url` the API returned with the connection token, except
 *   when it names a loopback host while the API itself is remote (a server
 *   left on the `ws://127.0.0.1:8000` default): the phone would dial itself,
 *   so realtime is skipped (`null`) and the screens poll instead.
 * - A plain `ws://` URL is upgraded to `wss://` when the API is `https://`
 *   (iOS refuses clear-text sockets, and the edge only terminates TLS).
 */
export function resolveRealtimeUrl(
  wsUrl: string | undefined | null,
  apiUrl: string,
  override?: string | null,
): string | null {
  const forced = (override ?? '').trim();
  if (forced) return forced;
  const url = (wsUrl ?? '').trim();
  if (!url || !/^wss?:\/\//i.test(url)) return null;
  if (isLoopbackHost(hostOf(url)) && !isLoopbackHost(hostOf(apiUrl))) return null;
  if (/^https:\/\//i.test(apiUrl) && /^ws:\/\//i.test(url)) return `wss://${url.slice('ws://'.length)}`;
  return url;
}

/** Why a request never got an answer. */
export type TransportFailure = 'timeout' | 'network' | 'invalid_response';

/**
 * Sorts an error thrown while calling the API: an abort we started is a
 * timeout, a JSON parse error means something other than the API answered
 * (a captive portal, an HTML error page), everything else is the network.
 */
export function classifyTransportError(e: unknown, timedOut: boolean): TransportFailure {
  if (timedOut) return 'timeout';
  const name = typeof e === 'object' && e && 'name' in e ? String((e as { name: unknown }).name) : '';
  if (name === 'SyntaxError') return 'invalid_response';
  return 'network';
}
