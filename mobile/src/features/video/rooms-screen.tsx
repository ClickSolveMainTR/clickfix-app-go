import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { formatRelative, formatShort } from '@/core/format';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { Chips } from '@/ui/chips';
import { EmptyState } from '@/ui/empty-state';
import { SwitchRow } from '@/ui/fields';
import { Input } from '@/ui/input';
import { List, ListRow } from '@/ui/list';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { FormScreen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { toast } from '@/ui/toast';

import { cleanPin, useVideoAvailability, useVideoScope, type VideoRoom, videoError } from './api';

type Filter = 'open' | 'live' | 'ended';

function useRooms(status: Filter) {
  const { org, canUse } = useVideoScope();
  return useQuery({
    queryKey: ['video', org, 'rooms', status] as const,
    enabled: !!org && canUse,
    staleTime: 15_000,
    refetchInterval: 30_000,
    queryFn: () =>
      unwrap(api.GET('/v1/tenant/video/rooms', { params: { header: orgHeader(), query: { status, limit: 50 } } })),
  });
}

function useStartRoom() {
  const { org } = useVideoScope();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { name: string; pin: string }) =>
      unwrap<VideoRoom>(
        api.POST('/v1/tenant/video/rooms', {
          params: { header: orgHeader() },
          body: { name: v.name, pin: v.pin || undefined },
        }),
      ),
    onSettled: () => void qc.invalidateQueries({ queryKey: ['video', org, 'rooms'] }),
  });
}

/**
 * Video calls from the "More" hub (CF-139): rooms going on now or waiting,
 * past ones, and an instant room. Calls from a conversation start in its
 * header; appointment rooms open from the agenda.
 */
export function VideoRoomsScreen() {
  const { t } = useTranslation('video');
  const { canUse } = useVideoScope();
  const [filter, setFilter] = useState<Filter>('live');
  const rooms = useRooms(filter);
  const refresh = useManualRefresh(rooms);
  const start = useStartRoom();
  const [name, setName] = useState('');
  const [pin, setPin] = useState('');
  const [protect, setProtect] = useState(false);
  const pinRequired = !!useVideoAvailability().data?.policy.require_pin;
  const withPin = pinRequired || protect;

  if (!canUse)
    return (
      <>
        <Stack.Screen options={{ title: t('rooms.title') }} />
        <View className="flex-1 justify-center bg-background">
          <EmptyState icon="lockClosed" title={t('hub:video.no_permission')} />
        </View>
      </>
    );

  return (
    <>
      <Stack.Screen options={{ title: t('rooms.title') }} />
      <FormScreen refreshControl={<RefreshControl {...refresh} />}>
        <Card title={t('hub:video.instant')}>
          <Input
            label={t('rooms.name')}
            value={name}
            onChangeText={setName}
            placeholder={t('hub:video.name_placeholder')}
          />
          <SwitchRow
            label={t('rooms.pin_protect')}
            description={pinRequired ? t('rooms.pin_protect_required') : undefined}
            value={withPin}
            onChange={setProtect}
            disabled={pinRequired}
          />
          {withPin ? (
            <Input
              label={t('rooms.pin_required')}
              hint={t('links.pin_hint')}
              value={pin}
              onChangeText={(v) => setPin(cleanPin(v))}
              keyboardType="number-pad"
              maxLength={12}
            />
          ) : null}
          <Button
            title={t('rooms.create')}
            icon="video"
            loading={start.isPending}
            disabled={withPin && pin.length < 4}
            onPress={() =>
              start.mutate(
                { name: name.trim() || t('hub:video.default_name'), pin: withPin ? pin : '' },
                {
                  onSuccess: (room) => {
                    setName('');
                    setPin('');
                    router.push(`/video/${room.uuid}`);
                  },
                  onError: (e) => toast.error(videoError(e)),
                },
              )
            }
          />
        </Card>
        <Chips<Filter>
          value={filter}
          onChange={setFilter}
          options={(['live', 'open', 'ended'] as const).map((s) => ({ value: s, label: t(`status.${s}`) }))}
        />
        <QueryState query={rooms} rows={3}>
          {(d) =>
            d.items.length ? (
              <List>
                {d.items.map((r, i) => (
                  <ListRow
                    key={r.uuid}
                    icon="video"
                    title={r.name}
                    subtitle={[
                      r.scheduled_start_at ? formatShort(r.scheduled_start_at) : null,
                      r.started_at ? formatRelative(r.started_at) : formatRelative(r.created_at),
                      r.waiting ? t('hub:video.waiting', { count: r.waiting }) : null,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                    trailing={
                      <StatusPill
                        label={t(`status.${r.status}`)}
                        tone={r.status === 'live' ? 'progress' : r.status === 'ended' ? 'done' : 'neutral'}
                      />
                    }
                    onPress={r.status === 'ended' ? undefined : () => router.push(`/video/${r.uuid}`)}
                    last={i === d.items.length - 1}
                  />
                ))}
              </List>
            ) : (
              <EmptyState icon="video" title={t('rooms.empty')} />
            )
          }
        </QueryState>
      </FormScreen>
    </>
  );
}
