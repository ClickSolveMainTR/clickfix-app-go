import { useMutation } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, View } from 'react-native';
import { toast } from '@/ui/toast';
import { SettingsSection, SettingsTextRow } from '@/ui/settings-rows';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { errorMessage } from '@/core/i18n';
import { List, ListRow } from '@/ui/list';
import { FormScreen } from '@/ui/screen';
import { Text } from '@/ui/text';

/**
 * In-app account deletion (App Store guideline 5.1.1(v)): a code sent to the
 * account's e-mail confirms `/v1/auth/account/deactivate`, which disables the
 * user and revokes every session; then this device signs out.
 */
export function DeleteAccountScreen() {
  const { t } = useTranslation('settings');
  const email = useSession((s) => s.me?.user.email ?? '');
  const [code, setCode] = useState('');

  const request = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/auth/account/deactivate/request')),
  });
  const confirm = useMutation({
    mutationFn: () => unwrap(api.POST('/v1/auth/account/deactivate', { body: { code: code.trim() } })),
    onSuccess: () => void useSession.getState().signOut({ remote: false }),
    onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
  });

  return (
    <FormScreen>
      <View className="gap-2 px-4 py-2">
        <Text variant="headline">{t('delete_title')}</Text>
        <Text tone="muted">{t('delete_body')}</Text>
      </View>
      {request.isSuccess ? (
        <>
          <SettingsSection footer={t('delete_code_hint', { email })}>
            <SettingsTextRow
              last
              label={t('delete_code')}
              value={code}
              onChangeText={setCode}
              keyboardType="number-pad"
              textContentType="oneTimeCode"
              autoComplete="one-time-code"
              autoFocus
            />
          </SettingsSection>
          <List>
            <ListRow
              icon="trash"
              title={t('delete_confirm')}
              tone="danger"
              disabled={code.trim().length < 6 || confirm.isPending}
              trailing={confirm.isPending ? <ActivityIndicator /> : <View />}
              onPress={() => confirm.mutate()}
              last
            />
          </List>
        </>
      ) : (
        <>
          {request.error ? (
            <Text tone="danger">
              {errorMessage(isApiError(request.error) ? request.error.code : undefined)}
            </Text>
          ) : null}
          <List>
            <ListRow
              icon="mail"
              title={t('delete_send_code')}
              disabled={request.isPending}
              trailing={request.isPending ? <ActivityIndicator /> : <View />}
              onPress={() => request.mutate()}
              last
            />
          </List>
        </>
      )}
    </FormScreen>
  );
}
