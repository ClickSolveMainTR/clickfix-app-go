import { router } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, Switch, View } from 'react-native';

import { List, ListRow, SectionHeader } from '@/ui/list';
import { ActionMenu } from '@/ui/menu';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { biometricsAvailable, setLockEnabled, setLockTimeout, useAppLock } from './app-lock';
import { LOCK_TIMEOUTS, type LockTimeout } from './lock-core';

/** Settings → Security (CF-82): biometric app lock, its timeout, signed-in devices. */
export function SecurityCard() {
  const { t } = useTranslation('security');
  const enabled = useAppLock((s) => s.enabled);
  const timeout = useAppLock((s) => s.timeoutMin);
  const [available, setAvailable] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void biometricsAvailable().then(setAvailable);
  }, []);

  return (
    <View>
      <SectionHeader title={t('title')} />
      <List>
        <ListRow
          iconTile
          icon="lockClosed"
          title={t('lock')}
          trailing={
            <Switch
              accessibilityLabel={t('lock')}
              value={enabled}
              disabled={busy || available !== true}
              onValueChange={async (value) => {
                setBusy(true);
                try {
                  if (!(await setLockEnabled(value)) && value) toast.error(t('lock_failed'));
                } finally {
                  setBusy(false);
                }
              }}
            />
          }
        />
        {enabled ? (
          <ActionMenu
            title={t('lock_after')}
            fullWidth
            value={String(timeout)}
            options={LOCK_TIMEOUTS.map((m) => ({
              value: String(m),
              label: m === 0 ? t('lock_immediately') : t('lock_minutes', { count: m }),
            }))}
            onPick={(value) => setLockTimeout(Number(value) as LockTimeout)}
            trigger={
              <Pressable accessibilityRole="button">
                <ListRow
                  iconTile
                  icon="clock"
                  title={t('lock_after')}
                  trailing={
                    <Text tone="muted">
                      {timeout === 0 ? t('lock_immediately') : t('lock_minutes', { count: timeout })}
                    </Text>
                  }
                />
              </Pressable>
            }
          />
        ) : null}
        <ListRow
          iconTile
          icon="device"
          title={t('sessions')}
          onPress={() => router.push('/more/sessions')}
          last
        />
      </List>
      {available === false ? (
        <Text variant="footnote" tone="muted" className="px-4 pt-2">
          {t('lock_unavailable_short')}
        </Text>
      ) : null}
    </View>
  );
}
