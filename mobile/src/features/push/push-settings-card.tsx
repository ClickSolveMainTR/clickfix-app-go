import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Linking } from 'react-native';

import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { Text } from '@/ui/text';

import { registerPush, usePushPermission } from './push';

/** Whether this phone gets pushes, and the way to turn them on. */
export function PushSettingsCard() {
  const { t } = useTranslation('settings');
  const status = usePushPermission((s) => s.status);
  return (
    <Card title={t('push')}>
      {status === 'unavailable' ? null : (
        <Text tone="muted">
          {status === 'granted' ? t('push_on') : status === 'denied' ? t('push_denied') : t('push_ask_hint')}
        </Text>
      )}
      {status === 'denied' ? (
        <Button
          title={t('push_open_settings')}
          icon="settings"
          variant="secondary"
          onPress={() => void Linking.openSettings()}
        />
      ) : status === 'undetermined' ? (
        <Button title={t('push_allow')} icon="bell" variant="secondary" onPress={() => void registerPush()} />
      ) : null}
      <Button
        title={t('push_prefs_open')}
        icon="settings"
        variant="secondary"
        onPress={() => router.push('/more/notifications')}
      />
    </Card>
  );
}
