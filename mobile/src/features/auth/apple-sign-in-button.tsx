import { useQuery } from '@tanstack/react-query';
import * as AppleAuthentication from 'expo-apple-authentication';
import * as Crypto from 'expo-crypto';
import { Platform } from 'react-native';
import { useUniwind } from 'uniwind';

import { fetchAppConfig } from '@/core/auth/api';

export type AppleCredential = {
  idToken: string;
  nonce: string;
  authorizationCode: string | null;
  givenName: string | null;
  familyName: string | null;
};

const IOS = Platform.OS === 'ios';

/**
 * Apple's own button (App Store guideline 4.8 asks for it), shown on iOS only
 * when the device supports it and the panel has Apple sign-in enabled
 * (`/v1/app/config` → `auth_methods.apple.login`). A cancelled sheet is silent.
 */
export function AppleSignInButton({
  disabled,
  onCredential,
  onError,
}: {
  disabled?: boolean;
  onCredential: (credential: AppleCredential) => void;
  onError: (error: unknown) => void;
}) {
  const { theme } = useUniwind();
  const enabled = useQuery({
    queryKey: ['app-config'],
    queryFn: fetchAppConfig,
    enabled: IOS,
    staleTime: 5 * 60_000,
    select: (config) => config.auth_methods?.apple?.login === true,
  });
  const available = useQuery({
    queryKey: ['apple-auth-available'],
    queryFn: AppleAuthentication.isAvailableAsync,
    enabled: IOS,
    staleTime: Infinity,
  });
  if (!enabled.data || !available.data) return null;

  const press = async () => {
    try {
      // Apple gets the SHA-256; the API takes the raw nonce and accepts either form in the token.
      const nonce = Crypto.randomUUID();
      const hashed = await Crypto.digestStringAsync(Crypto.CryptoDigestAlgorithm.SHA256, nonce);
      const c = await AppleAuthentication.signInAsync({
        requestedScopes: [
          AppleAuthentication.AppleAuthenticationScope.FULL_NAME,
          AppleAuthentication.AppleAuthenticationScope.EMAIL,
        ],
        nonce: hashed,
      });
      if (!c.identityToken) throw new Error('Apple returned no identity token');
      onCredential({
        idToken: c.identityToken,
        nonce,
        authorizationCode: c.authorizationCode,
        givenName: c.fullName?.givenName ?? null,
        familyName: c.fullName?.familyName ?? null,
      });
    } catch (e) {
      if ((e as { code?: string }).code === 'ERR_REQUEST_CANCELED') return;
      onError(e);
    }
  };

  return (
    <AppleAuthentication.AppleAuthenticationButton
      buttonType={AppleAuthentication.AppleAuthenticationButtonType.SIGN_IN}
      buttonStyle={
        theme === 'dark'
          ? AppleAuthentication.AppleAuthenticationButtonStyle.WHITE
          : AppleAuthentication.AppleAuthenticationButtonStyle.BLACK
      }
      cornerRadius={12}
      style={{ height: 50, width: '100%', opacity: disabled ? 0.5 : 1 }}
      onPress={() => {
        if (!disabled) void press();
      }}
    />
  );
}
