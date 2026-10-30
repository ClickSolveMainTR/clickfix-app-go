import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';
import { P, useCan } from '@/core/auth/permissions';
import { isApiError } from '@/core/api/errors';
import { errorMessage } from '@/core/i18n';
import { formatShort } from '@/core/format';
import { EmptyState } from '@/ui/empty-state';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Button } from '@/ui/button';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { useChannels, useCheckChannel, useChannelHealthHistory } from './api';
import { CHANNEL_TONE } from './channels-screen';
export function ChannelScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('admin');
  const channels = useChannels();
  const channel = channels.data?.items.find((c) => c.uuid === uuid);
  const history = useChannelHealthHistory(uuid);
  const check = useCheckChannel();
  const can = useCan();
  const refresh = useManualRefresh(channels);
  return (
    <>
      <Stack.Screen options={{ title: channel?.name ?? t('channels.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={channels}>
          {() =>
            channel ? (
              <View className="gap-4">
                <List>
                  <ListRow
                    title={channel.name}
                    titleLines={2}
                    subtitle={channel.health.detail}
                    trailing={
                      <StatusPill
                        label={t(`channels.state_${channel.health.state}`)}
                        tone={CHANNEL_TONE[channel.health.state]}
                      />
                    }
                  />
                  <ListRow
                    title={t('channels.last_error')}
                    subtitle={channel.last_error || t('channels.no_error')}
                    last
                  />
                </List>
                {can(P.channelsManage) ? (
                  <Button
                    title={t('channels.check')}
                    icon="refresh"
                    variant="outline"
                    loading={check.isPending}
                    onPress={() =>
                      check.mutate(uuid, {
                        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
                      })
                    }
                  />
                ) : null}
                <View>
                  <SectionHeader title={t('channels.history')} />
                  <List>
                    {(history.data ?? []).map((item, i, all) => (
                      <ListRow
                        key={item.at}
                        title={formatShort(item.at)}
                        subtitle={item.health.detail || item.error}
                        trailing={
                          <StatusPill
                            label={t(`channels.state_${item.health.state}`)}
                            tone={CHANNEL_TONE[item.health.state]}
                          />
                        }
                        last={i === all.length - 1}
                      />
                    ))}
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('channels.history_hint')}
                  </Text>
                </View>
              </View>
            ) : (
              <EmptyState icon="channel" title={t('common:no_results')} />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
