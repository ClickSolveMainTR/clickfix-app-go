import { fetchUpload, type UploadProgress } from './upload-progress';
import createClient from 'openapi-fetch';
import { Platform } from 'react-native';

import { fromApi, getTokens, setTokens } from '@/core/auth/tokens';
import { API_URL, APP_VERSION } from '@/core/config';
import { i18n } from '@/core/i18n';
import { reportApiReachable } from '@/core/network';

import { classifyTransportError } from './endpoints';
import { ApiError, toApiError } from './errors';
import type { paths } from './schema';

/** Sign-in and refresh carry no Bearer: a 401 there is a wrong password or a dead chain. */
const AUTH_PATHS = ['/v1/auth/login', '/v1/auth/refresh', '/v1/auth/email-code/request', '/v1/auth/email-code/verify'];
/** A 401 here means a wrong password or code, not an expired token: never refresh and resend it. */
const NO_RETRY_PATHS = ['/v1/auth/step-up/'];
/** Refresh this long before the access token expires, so requests never race it. */
const REFRESH_MARGIN_MS = 30_000;
/**
 * A request with no answer by then is given up (and the send queue retries
 * it). Without a limit one stalled socket kept the send queue "sending"
 * forever and every later reply waited behind it. Uploads get longer.
 */
const TIMEOUT_MS = 30_000;
const UPLOAD_TIMEOUT_MS = 180_000;

/** Thrown by `authFetch` when it aborted a request itself. */
class RequestTimeout extends Error {
  constructor() {
    super('Request timed out');
    this.name = 'RequestTimeout';
  }
}

/**
 * fetch with a deadline, never sending cookies. The app authenticates with
 * a Bearer token only; React Native's fetch would otherwise attach whatever
 * the OS cookie store holds for the host (an edge / CDN cookie set on
 * getclickfix.com), and the panel's `/api/mobile` passthrough used to refuse
 * every such request with 400 COOKIE_NOT_ALLOWED (CF-79).
 */
async function timedFetch(
  input: Request | string,
  init: RequestInit,
  timeoutMs: number,
  upload?: UploadProgress,
): Promise<Response> {
  const controller = new AbortController();
  let timedOut = false;
  const timer = setTimeout(() => {
    timedOut = true;
    controller.abort();
  }, timeoutMs);
  try {
    const res =
      upload && typeof input !== 'string'
        ? await fetchUpload(input, controller.signal, upload)
        : await fetch(input, { ...init, credentials: 'omit', signal: controller.signal });
    reportApiReachable(true);
    return res;
  } catch (e) {
    reportApiReachable(false);
    throw timedOut ? new RequestTimeout() : e;
  } finally {
    clearTimeout(timer);
  }
}

let onSessionExpired: () => void = () => {};
/** The session store registers how to sign out when the refresh chain is gone. */
export function setSessionExpiredHandler(fn: () => void) {
  onSessionExpired = fn;
}

let activeOrganization: string | null = null;
/** Slug sent as `X-Organization` on every `/v1/tenant/*` call (set by the session store). */
export function setActiveOrganization(slug: string | null) {
  activeOrganization = slug;
}

/**
 * Typed header param for `/v1/tenant/*` calls. The middleware sets the same
 * header; the generated types still require it on each call.
 */
export function orgHeader(): { 'X-Organization': string } {
  return { 'X-Organization': activeOrganization ?? '' };
}

function baseHeaders(): Record<string, string> {
  return {
    Accept: 'application/json',
    'X-App-Version': APP_VERSION,
    'Accept-Language': i18n.language,
    'User-Agent': `ClickFixMobile/${APP_VERSION} (${Platform.OS})`,
  };
}

let refreshing: Promise<boolean> | null = null;

/** Rotates the refresh token once, however many requests ask at the same time. */
export function refreshTokens(): Promise<boolean> {
  refreshing ??= (async () => {
    const t = getTokens();
    if (!t) return false;
    try {
      const res = await timedFetch(
        `${API_URL}/v1/auth/refresh`,
        {
          method: 'POST',
          headers: { ...baseHeaders(), 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: t.refresh }),
        },
        TIMEOUT_MS,
      );
      if (!res.ok) {
        // 401: expired, revoked or reused refresh token; the chain is gone.
        if (res.status === 401) onSessionExpired();
        return false;
      }
      const body = await res.json();
      await setTokens(fromApi(body.data));
      return true;
    } catch {
      return false;
    }
  })().finally(() => {
    refreshing = null;
  });
  return refreshing;
}

function prepare(request: Request) {
  for (const [k, v] of Object.entries(baseHeaders())) request.headers.set(k, v);
  const path = new URL(request.url).pathname;
  // A call made for another organization (the send queue, a push about another
  // organization) names it itself; everything else goes to the active one.
  if (activeOrganization && path.includes('/v1/tenant/') && !request.headers.get('X-Organization'))
    request.headers.set('X-Organization', activeOrganization);
  const t = getTokens();
  if (t && !AUTH_PATHS.some((p) => path.endsWith(p))) request.headers.set('Authorization', `Bearer ${t.access}`);
  return request;
}

async function authenticatedFetch(request: Request, upload?: UploadProgress): Promise<Response> {
  const path = new URL(request.url).pathname;
  const retry = request.clone();
  const t = getTokens();
  if (t && t.accessExpiresAt - Date.now() < REFRESH_MARGIN_MS) await refreshTokens();

  const timeout = path.endsWith('/v1/tenant/attachments') ? UPLOAD_TIMEOUT_MS : TIMEOUT_MS;
  let res = await timedFetch(prepare(request), {}, timeout, upload);
  const noRetry = [...AUTH_PATHS, ...NO_RETRY_PATHS].some((p) => path.includes(p));
  if (res.status === 401 && getTokens() && !noRetry) {
    if (await refreshTokens()) res = await timedFetch(prepare(retry), {}, timeout, upload);
  }
  return res;
}

/** Composer uploads share the normal session refresh and transport deadline. */
export function attachmentFetch(request: Request, upload: UploadProgress) {
  return authenticatedFetch(request, upload);
}

export const api = createClient<paths>({
  baseUrl: API_URL,
  fetch: (request) => authenticatedFetch(request),
  credentials: 'omit',
});

/**
 * An error thrown before the API's answer could be read. A timeout and a
 * dropped connection are NETWORK_ERROR / TIMEOUT (status 0: the send queue
 * waits and retries); a body that is not JSON means something else answered
 * (captive portal, HTML error page) and is INVALID_RESPONSE (502, retried).
 * Before CF-79 a non-JSON body also read as "no connection".
 */
function transportError(e: unknown): ApiError {
  const message = e instanceof Error ? e.message : String(e);
  switch (classifyTransportError(e, e instanceof RequestTimeout)) {
    case 'timeout':
      return new ApiError(0, { code: 'TIMEOUT', message });
    case 'invalid_response':
      return new ApiError(502, { code: 'INVALID_RESPONSE', message });
    default:
      return new ApiError(0, { code: 'NETWORK_ERROR', message });
  }
}

type Result<T> = { data?: { data: T }; error?: unknown; response: Response };

/**
 * Turns an openapi-fetch call into the envelope's `data`, or throws ApiError
 * with the backend error code. Network failures become NETWORK_ERROR.
 */
export async function unwrap<T>(call: Promise<Result<T>>): Promise<T> {
  let r: Result<T>;
  try {
    r = await call;
  } catch (e) {
    throw transportError(e);
  }
  if (r.error !== undefined || !r.response.ok || !r.data) throw toApiError(r.response.status, r.error);
  return r.data.data;
}

/** Like `unwrap`, but keeps the list envelope's `meta` (pagination). */
export async function unwrapList<T, M>(
  call: Promise<{ data?: { data: T; meta: M }; error?: unknown; response: Response }>,
) {
  let r: Awaited<typeof call>;
  try {
    r = await call;
  } catch (e) {
    throw transportError(e);
  }
  if (r.error !== undefined || !r.response.ok || !r.data) throw toApiError(r.response.status, r.error);
  return { items: r.data.data, meta: r.data.meta };
}

/** For `204 No Content` answers: resolves when the call succeeded, throws ApiError otherwise. */
export async function unwrapVoid(call: Promise<{ error?: unknown; response: Response }>): Promise<void> {
  let r: Awaited<typeof call>;
  try {
    r = await call;
  } catch (e) {
    throw transportError(e);
  }
  if (r.error !== undefined || !r.response.ok) throw toApiError(r.response.status, r.error);
}

/**
 * An API-owned file path (`download_url`, `thumbnail_url`) as an image /
 * audio source: the URL under the API origin plus the Bearer token and the
 * organization, which native image and audio loaders send as headers.
 * Media URLs are never assembled from storage keys in the app.
 */
export function mediaSource(path: string, organization?: string): { uri: string; headers: Record<string, string> } {
  const headers: Record<string, string> = { 'X-App-Version': APP_VERSION };
  const t = getTokens();
  if (t) headers.Authorization = `Bearer ${t.access}`;
  const org = organization ?? activeOrganization;
  if (org) headers['X-Organization'] = org;
  return { uri: path.startsWith('http') ? path : `${API_URL}${path}`, headers };
}
