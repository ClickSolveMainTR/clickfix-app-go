import { useMutation, useQuery } from '@tanstack/react-query';
import * as Haptics from 'expo-haptics';
import { router, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { api, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { Screen } from '@/ui/screen';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

/**
 * Approve a web panel sign-in by its QR (ported from olexfilms-ecosystem
 * `mobile/src/features/qr-login/qr-login-screen.tsx`, on this repo's
 * `modules/qrlogin` API): shows the browser, IP and place asking, then
 * approves or rejects. The first signed-in viewer claims the session.
 */
export function QrLoginScreen() {
  const { t } = useTranslation('qr');
  const { id } = useLocalSearchParams<{ id: string }>();
  const me = useSession((s) => s.me);

  const session = useQuery({
    queryKey: ['qr-login', id],
    gcTime: 0,
    retry: false,
    queryFn: () => unwrap(api.GET('/v1/auth/qr/sessions/{id}', { params: { path: { id } } })),
  });

  const decide = useMutation({
    mutationFn: (approve: boolean) =>
      unwrap(
        approve
          ? api.POST('/v1/auth/qr/sessions/{id}/approve', { params: { path: { id } } })
          : api.POST('/v1/auth/qr/sessions/{id}/reject', { params: { path: { id } } }),
      ),
    onSuccess: (_, approve) => {
      void Haptics.notificationAsync(
        approve ? Haptics.NotificationFeedbackType.Success : Haptics.NotificationFeedbackType.Warning,
      );
      toast.success(approve ? t('approved') : t('rejected'));
      if (router.canGoBack()) router.back();
      else router.replace('/');
    },
  });

  const d = session.data;
  const failure = decide.error ?? session.error;
  const place = d?.location ? [d.location.city, d.location.country].filter(Boolean).join(', ') : '';

  return (
    <Screen className="pt-4">
      {session.isPending ? (
        <Skeleton className="h-44" />
      ) : d && d.status === 'pending' ? (
        <>
          <View className="gap-1 px-1">
            <Text variant="callout" tone="muted">
              {t('body', { email: me?.user.email ?? '' })}
            </Text>
          </View>
          <List>
            <ListRow
              icon="language"
              title={t('browser')}
              subtitle={[`${d.client.browser} ${d.client.browser_version}`.trim(), d.client.os].filter(Boolean).join(' · ')}
            />
            <ListRow icon="link" title={t('ip')} subtitle={place ? `${d.ip} · ${place}` : d.ip} />
            <ListRow icon="clock" title={t('expires')} subtitle={formatTime(d.expires_at)} last />
          </List>
          <Text variant="footnote" tone="muted" className="text-center">
            {t('warning')}
          </Text>
          {decide.error ? (
            <Text variant="footnote" tone="danger" className="text-center">
              {errorMessage(isApiError(decide.error) ? decide.error.code : undefined)}
            </Text>
          ) : null}
          <View className="flex-row gap-3">
            <Button
              title={t('reject')}
              variant="secondary"
              size="lg"
              className="flex-1"
              disabled={decide.isPending}
              onPress={() => decide.mutate(false)}
            />
            <Button
              title={t('approve')}
              icon="check"
              size="lg"
              className="flex-1"
              loading={decide.isPending && decide.variables}
              disabled={decide.isPending}
              onPress={() => decide.mutate(true)}
            />
          </View>
        </>
      ) : (
        <EmptyState
          icon="qr"
          title={
            isApiError(failure, 'QR_SESSION_NOT_FOUND') || d
              ? t('expired')
              : errorMessage(isApiError(failure) ? failure.code : undefined)
          }
          action={{ title: t('scan_again'), icon: 'scan', onPress: () => router.replace('/qr-scan') }}
        />
      )}
    </Screen>
  );
}
