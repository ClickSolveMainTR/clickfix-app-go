import { router, Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { Pressable, RefreshControl } from 'react-native';
import Swipeable from 'react-native-gesture-handler/ReanimatedSwipeable';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { errorMessage } from '@/core/i18n';
import { ChannelLogoTile, hasChannelLogo } from '@/ui/brand-logo';
import { useChannelColor, channelIcon } from '@/ui/channel';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { StatusPill, type StatusTone } from '@/ui/status-pill';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { toast } from '@/ui/toast';
import { type Channel, useChannels, useCheckChannel } from './api';
export const CHANNEL_TONE: Record<Channel['health']['state'], StatusTone> = {
  ok: 'done',
  warning: 'progress',
  error: 'problem',
  paused: 'neutral',
  unknown: 'neutral',
};
export function ChannelsScreen() {
  const { t } = useTranslation('admin');
  const channels = useChannels();
  const check = useCheckChannel();
  const can = useCan();
  const refresh = useManualRefresh(channels);
  return (
    <>
      <Stack.Screen options={{ title: t('channels.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={channels}>
          {(data) =>
            data.items.length ? (
              <List>
                {[...data.items]
                  .sort(
                    (a, b) =>
                      ({ error: 0, warning: 1, unknown: 2, paused: 3, ok: 4 })[a.health.state] -
                      { error: 0, warning: 1, unknown: 2, paused: 3, ok: 4 }[b.health.state],
                  )
                  .map((c, i) => (
                    <Swipeable
                      key={c.uuid}
                      enabled={can(P.channelsManage)}
                      overshootRight={false}
                      renderRightActions={() => (
                        <Pressable
                          accessibilityRole="button"
                          accessibilityLabel={t('channels.check')}
                          disabled={check.isPending}
                          className="w-24 items-center justify-center gap-1 bg-secondary px-2"
                          onPress={() =>
                            check.mutate(c.uuid, {
                              onSuccess: (r) => toast.success(t(`channels.state_${r.health.state}`)),
                              onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                            })
                          }>
                          <Icon name="refresh" size={21} tone="primary" />
                          <Text variant="caption" className="text-center">
                            {t('channels.check')}
                          </Text>
                        </Pressable>
                      )}>
                      <ChannelRow channel={c} last={i === data.items.length - 1} />
                    </Swipeable>
                  ))}
              </List>
            ) : (
              <EmptyState icon="channel" title={t('channels.empty')} body={t('channels.empty_hint')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
function ChannelRow({ channel: c, last }: { channel: Channel; last: boolean }) {
  const channelColor = useChannelColor();
  const { t } = useTranslation('admin');
  return (
    <ListRow
      iconTile
      leading={hasChannelLogo(c.type) ? <ChannelLogoTile type={c.type} /> : undefined}
      separatorInset={hasChannelLogo(c.type) ? 57 : undefined}
      icon={channelIcon(c.type)}
      iconTint={channelColor(c.type)}
      title={c.name}
      titleLines={2}
      subtitle={`${t(`channels:types.${c.type}`, { defaultValue: c.type })} · ${t('channels.sent', { count: c.health.sent_24h })}`}
      trailing={<StatusPill label={t(`channels.state_${c.health.state}`)} tone={CHANNEL_TONE[c.health.state]} />}
      onPress={() => router.push({ pathname: '/more/channel', params: { uuid: c.uuid } })}
      last={last}
    />
  );
}
