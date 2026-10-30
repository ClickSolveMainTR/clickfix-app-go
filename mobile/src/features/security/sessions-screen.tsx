import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { EmptyState } from '@/ui/empty-state';
import { Button } from '@/ui/button';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { QueryState } from '@/ui/query-state';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { deviceLabel } from './device-label';
import { type AuthSession, useRevokeOtherSessions, useRevokeSession, useSessions } from './sessions';

/**
 * Signed-in devices (CF-82): this phone first, then the others with their
 * device, address and sign-in time; sign one out or all others at once.
 */
export function SessionsScreen() {
  const { t } = useTranslation('security');
  const sessions = useSessions();
  const refresh = useManualRefresh(sessions);
  const revokeOthers = useRevokeOtherSessions();
  return (
    <>
      <Stack.Screen options={{ title: t('sessions') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={sessions}>
          {(data) => {
            const items = [...data.items].sort((a, b) => Number(b.current) - Number(a.current));
            const others = items.filter((s) => !s.current).length;
            return (
              <>
                {!items.length ? <EmptyState icon="device" title={t('common:no_results')} /> : null}
                {items.length ? <List>{items.map((s, index) => (
                  <SessionRow key={s.uuid} session={s} last={index === items.length - 1} />
                ))}</List> : null}
                {others > 0 ? (
                  <Button
                    title={t('revoke_others')}
                    icon="signOut"
                    variant="destructive"
                    loading={revokeOthers.isPending}
                    onPress={() =>
                      Alert.alert(t('revoke_others_title'), t('revoke_others_body', { count: others }), [
                        { text: t('common:cancel'), style: 'cancel' },
                        {
                          text: t('revoke_others'),
                          style: 'destructive',
                          onPress: () =>
                            revokeOthers.mutate(undefined, {
                              onSuccess: () => toast.success(t('revoked')),
                              onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                            }),
                        },
                      ])
                    }
                  />
                ) : null}
              </>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

function SessionRow({ session: s, last }: { session: AuthSession; last: boolean }) {
  const { t } = useTranslation('security');
  const revoke = useRevokeSession();
  const device = deviceLabel(s.user_agent);
  return (
    <ListRow
      icon="device"
      title={device.label || t('unknown_device')}
      titleLines={2}
      subtitle={[
        s.ip_address,
        t('signed_in_at', { time: formatShort(s.created_at) }),
        s.impersonated ? t('impersonated') : null,
      ].filter(Boolean).join(' · ')}
      last={last}
      trailing={s.current ? (
        <Text variant="footnote" tone="muted">{t('this_device')}</Text>
      ) : (
        <Button title={t('revoke')} variant="ghost" loading={revoke.isPending}
          onPress={() => revoke.mutate(s.uuid, {
            onSuccess: () => toast.success(t('revoked')),
            onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
          })} />
      )}
    />
  );
}
