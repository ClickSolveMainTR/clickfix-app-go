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

/** The part of `/v1/app/config` the app reads; the OpenAPI schema only types it as a status envelope. */
export type AppConfig = {
  auth_methods?: Partial<Record<'apple' | 'google' | 'facebook' | 'github', { login?: boolean }>>;
};

/** Public sign-in options (`auth_methods`), so the app only shows enabled providers. */
export async function fetchAppConfig(): Promise<AppConfig> {
  return (await unwrap(api.GET('/v1/app/config'))) as unknown as AppConfig;
}

/**
 * Native Sign in with Apple: the iOS SDK's `id_token` plus the raw nonce given
 * to it. `authorization_code` lets the server store a refresh token (revoked
 * on account deletion); Apple sends the name only on the first sign-in.
 * `MFA_REQUIRED` works as on `login`.
 */
export function signInWithApple(input: {
  idToken: string;
  nonce: string;
  authorizationCode?: string | null;
  givenName?: string | null;
  familyName?: string | null;
  totpCode?: string;
}) {
  return unwrap(
    api.POST('/v1/auth/oauth/{provider}/native', {
      params: { path: { provider: 'apple' } },
      body: {
        id_token: input.idToken,
        nonce: input.nonce,
        authorization_code: input.authorizationCode || undefined,
        given_name: input.givenName || undefined,
        family_name: input.familyName || undefined,
        totp_code: input.totpCode || undefined,
      },
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
