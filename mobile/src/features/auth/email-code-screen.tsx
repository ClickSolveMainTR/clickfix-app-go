import { useMutation } from '@tanstack/react-query';
import { useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import { KeyboardAwareScrollView } from 'react-native-keyboard-controller';
import { withUniwind } from 'uniwind';

import { isApiError } from '@/core/api/errors';
import { requestEmailCode, verifyEmailCode } from '@/core/auth/api';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { Input } from '@/ui/input';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { CodeInput } from './code-input';

const AwareScrollView = withUniwind(KeyboardAwareScrollView);
/** The API allows a new code after a short wait; the button counts down so a tap never hits 429. */
const RESEND_AFTER_S = 60;

/**
 * Second step of the passwordless sign-in: the 6-digit code e-mailed by
 * `/v1/auth/email-code/request`, verified with `/v1/auth/email-code/verify`.
 * A filled code submits itself; with 2FA the API answers `MFA_REQUIRED` and
 * the authenticator code is asked for below.
 */
export function EmailCodeScreen() {
  const { t } = useTranslation('auth');
  const { email = '' } = useLocalSearchParams<{ email: string }>();
  const [code, setCode] = useState('');
  const [totp, setTotp] = useState('');
  const [needsTotp, setNeedsTotp] = useState(false);
  const [errorKey, setErrorKey] = useState(0);
  const [wait, setWait] = useState(RESEND_AFTER_S);

  useEffect(() => {
    if (wait <= 0) return;
    const id = setTimeout(() => setWait((w) => w - 1), 1000);
    return () => clearTimeout(id);
  }, [wait]);

  const verify = useMutation({
    mutationFn: async (input: { code: string; totp?: string }) => {
      const tokens = await verifyEmailCode({ email, code: input.code, totpCode: input.totp });
      await useSession.getState().signIn(tokens);
    },
    onError: (e) => {
      if (isApiError(e, 'MFA_REQUIRED')) {
        setNeedsTotp(true);
        return;
      }
      setErrorKey((k) => k + 1);
      if (!needsTotp) setCode('');
    },
  });

  const resend = useMutation({
    mutationFn: () => requestEmailCode(email),
    onSuccess: () => {
      setWait(RESEND_AFTER_S);
      setCode('');
      verify.reset();
      toast.success(t('email_code_resent'));
    },
    onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  const failed = verify.error && !isApiError(verify.error, 'MFA_REQUIRED') ? verify.error : null;

  return (
    <AwareScrollView
      mode="layout"
      keyboardDismissMode="interactive"
      bottomOffset={8}
      contentInsetAdjustmentBehavior="automatic"
      keyboardShouldPersistTaps="handled"
      className="flex-1 bg-background"
      contentContainerClassName="gap-7 px-6 pb-10 pt-4">
      <View className="gap-2">
        <Text variant="largeTitle">{t('email_code_title')}</Text>
        <Text variant="callout" tone="muted">
          {t('email_code_body', { email })}
        </Text>
      </View>
      <CodeInput
        value={code}
        onChange={(v) => {
          setCode(v);
          if (failed) verify.reset();
        }}
        onComplete={(v) => {
          if (!needsTotp) verify.mutate({ code: v });
        }}
        error={!!failed}
        errorKey={errorKey}
        disabled={verify.isPending}
        accessibilityLabel={t('email_code_title')}
      />
      {needsTotp ? (
        <Input
          label={t('totp_code')}
          icon="security"
          hint={t('totp_hint')}
          value={totp}
          onChangeText={setTotp}
          autoFocus
          autoCapitalize="none"
          keyboardType="number-pad"
          textContentType="oneTimeCode"
          returnKeyType="go"
          onSubmitEditing={() => verify.mutate({ code, totp })}
        />
      ) : null}
      {failed ? (
        <Text variant="footnote" tone="danger" accessibilityLiveRegion="polite">
          {errorMessage(isApiError(failed) ? failed.code : undefined)}
        </Text>
      ) : null}
      <View className="gap-2">
        <Button
          icon="signIn"
          title={t('sign_in')}
          size="lg"
          loading={verify.isPending}
          disabled={code.length < 6 || (needsTotp && !totp.trim())}
          onPress={() => verify.mutate({ code, totp: needsTotp ? totp : undefined })}
        />
        <Button
          title={wait > 0 ? t('email_code_resend_in', { seconds: String(wait) }) : t('email_code_resend')}
          variant="ghost"
          loading={resend.isPending}
          disabled={wait > 0}
          onPress={() => resend.mutate()}
        />
      </View>
    </AwareScrollView>
  );
}
