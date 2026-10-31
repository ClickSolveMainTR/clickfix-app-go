import { useMutation } from '@tanstack/react-query';
import { router } from 'expo-router';
import * as WebBrowser from 'expo-web-browser';
import { type ComponentRef, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TextInput, View } from 'react-native';
import { KeyboardAwareScrollView } from 'react-native-keyboard-controller';
import Animated from 'react-native-reanimated';
import { SafeAreaView } from 'react-native-safe-area-context';
import { withUniwind } from 'uniwind';

import { isApiError } from '@/core/api/errors';
import { login, requestEmailCode, signInWithApple } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { WEB_URL } from '@/core/config';
import { errorMessage } from '@/core/i18n';
import { openLegalPage } from '@/core/legal';
import { BrandLogo } from '@/ui/brand';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { brandIn, riseIn } from '@/ui/motion';
import { Text } from '@/ui/text';

import { type AppleCredential, AppleSignInButton } from './apple-sign-in-button';

const AwareScrollView = withUniwind(KeyboardAwareScrollView);

/**
 * E-mail + password against `/v1/auth/login` (ported from takwim-app-go
 * `mobile/src/features/auth/sign-in-screen.tsx`). When the account has 2FA
 * the API answers `MFA_REQUIRED`; the same form then asks for the code and
 * sends all three again. "E-mail me a code" starts the passwordless sign-in
 * (`/v1/auth/email-code/request`) and opens the code step. On iOS, Sign in
 * with Apple posts the native `id_token` to `/v1/auth/oauth/apple/native`
 * when the panel has it enabled; its `MFA_REQUIRED` uses the same code field.
 */
export function SignInScreen() {
  const { t } = useTranslation('auth');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [totp, setTotp] = useState('');
  const [needsTotp, setNeedsTotp] = useState(false);
  const totpRef = useRef<ComponentRef<typeof TextInput>>(null);
  const passwordRef = useRef<ComponentRef<typeof TextInput>>(null);

  const signIn = useMutation({
    mutationFn: async () => {
      const tokens = await login({ email, password, totpCode: needsTotp ? totp : undefined });
      await useSession.getState().signIn(tokens);
    },
    onError: (e) => {
      if (isApiError(e, 'MFA_REQUIRED')) setNeedsTotp(true);
    },
  });

  // Kept after `MFA_REQUIRED` so the code step resends the same Apple sign-in.
  const [appleCredential, setAppleCredential] = useState<AppleCredential | null>(null);
  // The native Apple sheet failing (not a cancel) before any API call.
  const [appleError, setAppleError] = useState<unknown>(null);
  const apple = useMutation({
    mutationFn: async ({ credential, totpCode }: { credential: AppleCredential; totpCode?: string }) => {
      const tokens = await signInWithApple({ ...credential, totpCode });
      await useSession.getState().signIn(tokens);
    },
    onError: (e, { credential }) => {
      if (isApiError(e, 'MFA_REQUIRED')) {
        setAppleCredential(credential);
        setNeedsTotp(true);
      }
    },
  });

  const code = useMutation({
    mutationFn: () => requestEmailCode(email),
    onSuccess: () => router.push({ pathname: '/code', params: { email: email.trim() } }),
  });

  const failed = code.error ?? signIn.error ?? apple.error ?? appleError;
  const error =
    failed && !isApiError(failed, 'MFA_REQUIRED')
      ? isApiError(failed, 'OAUTH_LINK_CHOICE_REQUIRED')
        ? t('apple_link_required')
        : errorMessage(isApiError(failed) ? failed.code : undefined)
      : undefined;
  const validEmail = /^\S+@\S+\.\S+$/.test(email.trim());
  const ready = appleCredential ? totp.trim() : validEmail && password && (!needsTotp || totp.trim());
  const submit = () => {
    if (!ready) return;
    code.reset();
    setAppleError(null);
    if (appleCredential) apple.mutate({ credential: appleCredential, totpCode: totp });
    else signIn.mutate();
  };
  const busy = signIn.isPending || apple.isPending;

  return (
    <SafeAreaView className="flex-1 bg-background">
      {/* The focused field and the sign-in button stay above the keyboard. */}
      <AwareScrollView
        mode="layout"
        keyboardDismissMode="interactive"
        bottomOffset={8}
        contentContainerClassName="flex-grow justify-center gap-6 px-6 py-10"
        keyboardShouldPersistTaps="handled">
        <View className="items-center gap-4">
          <Animated.View entering={brandIn}>
            <BrandLogo size={30} />
          </Animated.View>
          <Animated.View entering={riseIn(2)} className="items-center gap-2">
            <Text variant="largeTitle" className="text-center">
              {t('welcome_title')}
            </Text>
            <Text variant="callout" tone="muted" className="text-center">
              {t('welcome_body')}
            </Text>
          </Animated.View>
        </View>

        <Animated.View entering={riseIn(4)} className="gap-4">
          <Input
            label={t('email')}
            icon="mail"
            value={email}
            onChangeText={setEmail}
            autoCapitalize="none"
            autoComplete="email"
            keyboardType="email-address"
            textContentType="username"
            returnKeyType="next"
            onSubmitEditing={() => passwordRef.current?.focus()}
          />
          <Input
            ref={passwordRef}
            label={t('password')}
            icon="password"
            value={password}
            onChangeText={setPassword}
            secureTextEntry
            autoComplete="current-password"
            textContentType="password"
            returnKeyType={needsTotp ? 'next' : 'go'}
            submitBehavior={needsTotp ? 'submit' : 'blurAndSubmit'}
            onSubmitEditing={() => (needsTotp ? totpRef.current?.focus() : submit())}
          />
          {needsTotp ? (
            <Input
              ref={totpRef}
              label={t('totp_code')}
              icon="security"
              hint={t('totp_hint')}
              value={totp}
              onChangeText={setTotp}
              autoFocus
              autoCapitalize="none"
              keyboardType="number-pad"
              textContentType="oneTimeCode"
              autoComplete="one-time-code"
              returnKeyType="go"
              onSubmitEditing={submit}
            />
          ) : null}
          {error ? (
            <Text variant="footnote" tone="danger" accessibilityLiveRegion="polite">
              {error}
            </Text>
          ) : null}
          <Button
            icon="signIn"
            title={t('sign_in')}
            size="lg"
            loading={busy}
            disabled={!ready || code.isPending}
            onPress={submit}
          />
          <View className="flex-row items-center gap-3">
            <View className="h-px flex-1 bg-border" />
            <Text variant="footnote" tone="muted">
              {t('or')}
            </Text>
            <View className="h-px flex-1 bg-border" />
          </View>
          <AppleSignInButton
            disabled={busy || code.isPending}
            onCredential={(credential) => {
              signIn.reset();
              code.reset();
              setAppleError(null);
              setAppleCredential(null);
              setNeedsTotp(false);
              setTotp('');
              apple.mutate({ credential });
            }}
            onError={(e) => {
              apple.reset();
              setAppleError(e);
            }}
          />
          <Button
            icon="mail"
            title={t('email_code_cta')}
            variant="outline"
            size="lg"
            loading={code.isPending}
            disabled={!validEmail || busy}
            onPress={() => {
              signIn.reset();
              apple.reset();
              code.mutate();
            }}
          />
          {!validEmail ? (
            <Text variant="footnote" tone="muted" className="text-center">
              {t('email_code_hint')}
            </Text>
          ) : null}
          <Button
            title={t('forgot_password')}
            variant="ghost"
            onPress={() => void WebBrowser.openBrowserAsync(`${WEB_URL}/forgot-password`)}
          />
        </Animated.View>

        <Animated.View entering={riseIn(6)} className="flex-row justify-center gap-4">
          <Button title={t('settings:privacy')} variant="ghost" onPress={() => void openLegalPage('privacy')} />
          <Button title={t('settings:terms')} variant="ghost" onPress={() => void openLegalPage('terms')} />
        </Animated.View>
      </AwareScrollView>
    </SafeAreaView>
  );
}
