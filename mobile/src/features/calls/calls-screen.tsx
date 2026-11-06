import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { formatShortWithYear } from '@/core/format';
import { useChannels } from '@/features/admin/api';
import { EmptyState } from '@/ui/empty-state';
import { ListRow } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';

import { useCallScope, useVoiceCalls, type VoiceCall } from './api';
import { callFailed, counterpart, directionLabel, duration, statusLabel, type T } from './core';

/** `t` of the `admin` namespace in the shape the label helpers take. */
export function useCallT(): T {
  const { t } = useTranslation('admin');
  return (key, params) => t(key, params ?? {});
}

/** Compact date and time of a call; the log goes back years, so another year is written out. */
export const callTime = formatShortWithYear;

/** A screen opened without the id it needs (a broken deep link) has nothing to load. */
export function MissingCall({ title, onOpen }: { title: string; onOpen?: () => void }) {
  const t = useCallT();
  return (
    <>
      <Stack.Screen options={{ title }} />
      <Screen>
        <EmptyState
          icon="phone"
          title={t('calls.missing')}
          body={t('calls.missing_hint')}
          action={onOpen ? { title: t('calls.open_log'), icon: 'phone', onPress: onOpen } : undefined}
        />
      </Screen>
    </>
  );
}

/**
 * Call log of one voice channel (CF-169), the phone version of the panel's
 * page (`frontend/src/features/channels/components/voice-calls-page.tsx`):
 * direction, the other number, status, duration and whether it was recorded,
 * newest first, more loading as the list is scrolled. A row opens the call.
 */
export function CallsScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const t = useCallT();
  const { canRead } = useCallScope();
  const channels = useChannels();
  const calls = useVoiceCalls(uuid);
  const channel = channels.data?.items.find((c) => c.uuid === uuid);
  const total = calls.data?.pages[0]?.total;

  if (!canRead)
    return (
      <>
        <Stack.Screen options={{ title: t('calls.title') }} />
        <Screen>
          <EmptyState icon="phone" title={t('calls.no_permission')} />
        </Screen>
      </>
    );

  if (!uuid) return <MissingCall title={t('calls.title')} />;

  return (
    <>
      <Stack.Screen options={{ title: t('calls.title') }} />
      <PagedList<VoiceCall>
        query={calls}
        keyOf={(c) => c.uuid}
        emptyIcon="phone"
        empty={t('calls.empty')}
        emptyBody={t('calls.empty_hint')}
        header={
          <View className="gap-0.5 px-4">
            <Text variant="headline" numberOfLines={2}>
              {channel?.name ?? t('calls.voice_channel')}
            </Text>
            {/* The provider balance, when the channel reports one (the panel shows the same line). */}
            {channel?.health.detail.startsWith('Bakiye:') ? (
              <Text variant="footnote" tone="muted">
                {channel.health.detail}
              </Text>
            ) : null}
            {total ? (
              <Text variant="footnote" tone="muted">
                {t('calls.total', { count: total })}
              </Text>
            ) : null}
          </View>
        }
        renderItem={(c, _i, last) => (
          <ListRow
            icon={c.direction === 'inbound' ? 'income' : c.direction === 'outbound' ? 'expense' : 'phone'}
            title={counterpart(c)}
            subtitle={[
              directionLabel(t, c.direction),
              callTime(c.created_at),
              duration(c.duration_seconds),
              c.recording_message_uuid ? t('calls.recorded') : '',
            ]
              .filter(Boolean)
              .join(' · ')}
            trailing={
              <StatusPill label={statusLabel(t, c.status)} tone={callFailed(c.status) ? 'problem' : 'neutral'} />
            }
            onPress={() => router.push({ pathname: '/more/call', params: { channel: uuid, uuid: c.uuid } })}
            last={last}
          />
        )}
      />
    </>
  );
}
