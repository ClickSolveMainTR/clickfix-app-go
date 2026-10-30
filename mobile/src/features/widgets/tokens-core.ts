/**
 * Token rules for the OS surfaces (CF-152): when the scoped widget token is
 * renewed, and which Live Activity push tokens still have to reach the API.
 * Free of app imports so they run under `node --test`.
 */

/** The widget token lives 72 h on the server; the app renews it after 12 h. */
export const WIDGET_TOKEN_RENEW_AFTER_MS = 12 * 60 * 60 * 1000;

export type StoredWidgetToken = {
  /** Organization slug the token is bound to. */
  org: string;
  /** When the app received it (epoch ms). */
  issuedAt: number;
  /** Server expiry (ISO 8601). */
  expiresAt: string;
};

/** True when there is no token for this organization, it is old or about to expire. */
export function shouldRenewWidgetToken(stored: StoredWidgetToken | null, org: string, now: number): boolean {
  if (!stored || stored.org !== org) return true;
  const expires = Date.parse(stored.expiresAt);
  if (Number.isNaN(expires) || expires - now < WIDGET_TOKEN_RENEW_AFTER_MS) return true;
  return now - stored.issuedAt >= WIDGET_TOKEN_RENEW_AFTER_MS;
}

export type LiveActivityKind = 'push_to_start' | 'update';
export type LiveActivityType = 'call' | 'urgent_conversation';
export type ApnsEnvironment = 'production' | 'sandbox';

export type LiveActivityTokenBody = {
  device_id: string;
  kind: LiveActivityKind;
  activity_type: LiveActivityType;
  activity_id?: string;
  ref?: string;
  token: string;
  environment: ApnsEnvironment;
};

const HEX = /^[0-9a-f]{32,255}$/i;

/** The API body for a token, or null when the token is not a hex APNs token. */
export function liveActivityTokenBody(input: {
  deviceId: string;
  kind: LiveActivityKind;
  type: LiveActivityType;
  token: string;
  environment: ApnsEnvironment;
  activityId?: string;
  ref?: string;
}): LiveActivityTokenBody | null {
  const token = input.token.trim().toLowerCase();
  if (!HEX.test(token) || !input.deviceId) return null;
  const body: LiveActivityTokenBody = {
    device_id: input.deviceId,
    kind: input.kind,
    activity_type: input.type,
    token,
    environment: input.environment,
  };
  if (input.kind === 'update') {
    if (input.activityId) body.activity_id = input.activityId.slice(0, 128);
    if (input.ref) body.ref = input.ref.slice(0, 64);
  }
  return body;
}

/** Development builds talk to the APNs sandbox, store / TestFlight builds to production. */
export function apnsEnvironment(isDev: boolean): ApnsEnvironment {
  return isDev ? 'sandbox' : 'production';
}

/**
 * Remembers which token bodies the API already has, so a token that the OS
 * hands out again (every launch for push-to-start) is not posted twice.
 */
export class SentTokens {
  private sent = new Set<string>();

  private key(b: LiveActivityTokenBody) {
    return [b.kind, b.activity_type, b.token, b.activity_id ?? '', b.ref ?? '', b.environment].join('|');
  }

  /** True when `body` still has to be sent; marks it as sent. */
  claim(body: LiveActivityTokenBody): boolean {
    const k = this.key(body);
    if (this.sent.has(k)) return false;
    this.sent.add(k);
    return true;
  }

  /** After a failed request, so the next event tries again. */
  release(body: LiveActivityTokenBody) {
    this.sent.delete(this.key(body));
  }

  clear() {
    this.sent.clear();
  }
}

/** `uuid` v4-shaped device id; `crypto.randomUUID` is missing on older Hermes. */
export function newDeviceId(random: () => number = Math.random): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.floor(random() * 16);
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}
