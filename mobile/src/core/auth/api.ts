import { api, unwrap } from '@/core/api/client';
import type { components } from '@/core/api/schema';

import { getTokens } from './tokens';

export type Me = components['schemas']['Me'];
export type MeOrganization = components['schemas']['MeOrganization'];
export type ApiTokens = components['schemas']['Tokens'];

/** E-mail + password; `MFA_REQUIRED` asks for the authenticator code on a second try. */
export function login(input: { email: string; password: string; totpCode?: string }) {
  return unwrap(
    api.POST('/v1/auth/login', {
      body: { email: input.email.trim(), password: input.password, totp_code: input.totpCode || undefined },
    }),
  );
}

/**
 * Passwordless sign-in: e-mails a 6-digit code. The answer is the same
 * whether or not the address has an account, so it never tells who signed up.
 */
export function requestEmailCode(email: string) {
  return unwrap(api.POST('/v1/auth/email-code/request', { body: { email: email.trim() } }));
}

/** Trades the e-mailed code for a token pair; `MFA_REQUIRED` works as on `login`. */
export function verifyEmailCode(input: { email: string; code: string; totpCode?: string }) {
  return unwrap(
    api.POST('/v1/auth/email-code/verify', {
      body: { email: input.email.trim(), code: input.code, totp_code: input.totpCode || undefined },
    }),
  );
}

export function fetchMe() {
  return unwrap(api.GET('/v1/auth/me'));
}

/** Revokes this device's refresh token (the access token expires on its own). */
export function logout() {
  const refresh = getTokens()?.refresh;
  if (!refresh) return Promise.resolve(undefined);
  return unwrap(api.POST('/v1/auth/logout', { body: { refresh_token: refresh } }));
}
