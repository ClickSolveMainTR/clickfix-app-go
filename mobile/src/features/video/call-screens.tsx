import { FormSheet, type FormField } from '@/ui/form-sheet';
import { useMutation, useQuery } from '@tanstack/react-query';
import { router, Stack, useLocalSearchParams } from 'expo-router';
import { Component, lazy, Suspense, useEffect, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { NativeModules, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { Button } from '@/ui/button';
import { EmptyState } from '@/ui/empty-state';
import { Skeleton } from '@/ui/skeleton';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { endRoom, joinAsMember, publicVideo, useLobby, useLobbyAction, videoError, type VideoJoinGrant } from './api';

// Loaded on demand: the module touches the LiveKit / WebRTC native modules.
// A binary built before those modules has no WebRTCModule; importing then throws
// while the module evaluates, so check first and let NativeGuard show the fallback.
const CallView = lazy(async () => {
  if (!NativeModules.WebRTCModule) throw new Error('VIDEO_NATIVE_MISSING');
  const mod = await import('./call-view');
  if (!mod?.default) throw new Error('VIDEO_NATIVE_MISSING');
  return mod;
});

/** Shown instead of crashing when this binary predates the native video modules. */
class NativeGuard extends Component<{ children: ReactNode; fallback: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  render() {
    return this.state.failed ? this.props.fallback : this.props.children;
  }
}

function Call({ grant, onLeave, toolbar }: { grant: VideoJoinGrant; onLeave: () => void; toolbar?: ReactNode }) {
  const { t } = useTranslation('video');
  return (
    <>
      <Stack.Screen options={{ headerShown: false }} />
      <NativeGuard
        fallback={
          <EmptyState icon="error" title={t('mobile.update_required')} body={t('mobile.update_required_body')}
            action={{ title: t('common:close'), icon: 'close', onPress: onLeave }} />
        }>
        <Suspense
          fallback={
            <View className="flex-1 gap-4 bg-background p-6">
              <Skeleton className="h-48" />
              <Skeleton className="h-12" />
            </View>
          }>
          <CallView grant={grant} onLeave={onLeave} toolbar={toolbar} />
        </Suspense>
      </NativeGuard>
    </>
  );
}

function leave() {
  if (router.canGoBack()) router.back();
  else router.replace('/');
}

/** Organization member joining a room (inbox action, push or `/video/{uuid}` link). */
export function AgentCallScreen() {
  const { t } = useTranslation('video');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const join = useMutation({ mutationFn: () => joinAsMember(uuid) });
  const { mutate } = join;
  useEffect(() => mutate(), [mutate]);

  if (join.data?.token) {
    const host = join.data.role === 'host';
    return (
      <Call
        grant={join.data}
        onLeave={leave}
        toolbar={
          <>
            <LobbyButton room={uuid} />
            {host ? (
              <Button
                variant="destructive"
                title={t('rooms.end')}
                onPress={() => void endRoom(uuid).then(leave, (e: unknown) => toast.error(videoError(e)))}
              />
            ) : null}
          </>
        }
      />
    );
  }
  return (
    <View className="flex-1 items-center justify-center bg-background p-6">
      {join.isError ? (
        <EmptyState
          icon="error"
          title={videoError(join.error)}
          action={{ title: t('common:retry'), icon: 'refresh', onPress: () => join.mutate() }}
        />
      ) : (
        <>
          <Skeleton className="h-20 w-full" />
          <Button title={t('common:close')} variant="ghost" onPress={leave} />
        </>
      )}
    </View>
  );
}

function LobbyButton({ room }: { room: string }) {
  const { t } = useTranslation('video');
  const lobby = useLobby(room, true);
  const act = useLobbyAction(room);
  const waiting = (lobby.data?.items ?? []).filter((p) => p.status === 'waiting');
  if (waiting.length === 0) return null;
  const next = waiting[0];
  return (
    <View className="flex-1 flex-row items-center gap-2 rounded-full bg-warning/25 px-3 py-1">
      <Text variant="footnote" className="flex-1 text-on-tile" numberOfLines={1}>
        {t('lobby.waiting_count', { count: waiting.length })}: {next.display_name}
      </Text>
      <Button
        title={t('lobby.admit')}
        disabled={act.isPending}
        onPress={() => act.mutate({ participant: next.uuid, action: 'admit' })}
      />
      <Button
        variant="secondary"
        title={t('lobby.deny')}
        disabled={act.isPending}
        onPress={() => act.mutate({ participant: next.uuid, action: 'deny' })}
      />
    </View>
  );
}

function publicError(e: unknown) {
  return isApiError(e) ? videoError(e) : videoError(undefined);
}

/** Customer joining from a signed link (`/r/{code}`), signed in or not. */
export function GuestCallScreen() {
  const { t } = useTranslation('video');
  const { code } = useLocalSearchParams<{ code: string }>();
  const [name, setName] = useState('');
  const [pin, setPin] = useState('');
  const [grant, setGrant] = useState<VideoJoinGrant | null>(null);
  const info = useQuery({
    queryKey: ['video-public', code],
    queryFn: () => publicVideo.room(code),
    retry: false,
  });
  const join = useMutation({
    mutationFn: () => publicVideo.join(code, { name: name.trim(), pin: info.data?.pin_required ? pin : undefined }),
    onSuccess: setGrant,
  });

  // Lobby: poll the ticket until the host admits (or denies).
  const ticket = grant?.status === 'waiting' ? grant.ticket : undefined;
  useEffect(() => {
    if (!ticket) return;
    const id = setInterval(() => {
      publicVideo.ticket(ticket).then(
        (next) => next.status !== 'waiting' && setGrant({ ...next, ticket }),
        (e: unknown) => {
          toast.error(publicError(e));
          setGrant(null);
        },
      );
    }, 3_000);
    return () => clearInterval(id);
  }, [ticket]);

  if (grant?.status === 'admitted' && grant.token) {
    return <Call grant={grant} onLeave={leave} />;
  }
  if (info.isPending)
    return (
      <View className="flex-1 gap-4 bg-background p-6">
        <Skeleton className="h-48" />
        <Skeleton className="h-12" />
      </View>
    );
  if (!info.data)
    return (
      <EmptyState
        icon="error"
        title={publicError(info.error)}
        action={{ title: t('common:retry'), icon: 'refresh', onPress: () => void info.refetch() }}
      />
    );
  if (info.data.link_role === 'agent') {
    return <EmptyState icon="info" title={t('guest.agent_link')} />;
  }
  if (grant) {
    return (
      <EmptyState
        icon={grant.status === 'waiting' ? 'clock' : 'info'}
        title={grant.status === 'waiting' ? t('guest.waiting') : t(`guest.status.${grant.status}`)}
      />
    );
  }
  return (
    <FormSheet
      visible
      title={info.data.name}
      onCancel={() => {
        if (router.canGoBack()) router.back();
        else router.replace('/');
      }}
      dirty={!!name || !!pin}
      primary={{
        label: t('guest.join'),
        loading: join.isPending,
        disabled: !name.trim() || (!!info.data.pin_required && !pin.trim()),
        onPress: () => join.mutate(),
      }}
      sections={[
        {
          fields: [
            { key: 'name', kind: 'text', label: t('guest.name'), value: name, onChange: setName },
            ...(info.data.pin_required
              ? [
                  {
                    key: 'pin',
                    kind: 'text' as const,
                    label: t('guest.pin'),
                    value: pin,
                    onChange: setPin,
                    keyboard: 'number' as const,
                  },
                ]
              : []),
          ],
          footer: join.isError ? publicError(join.error) : t('guest.permissions_hint'),
        },
        {
          fields: [
            { key: 'org', kind: 'info', label: info.data.organization_name, value: info.data.name },
            ...(
              [
                info.data.e2ee ? 'guest.e2ee' : null,
                info.data.recording ? 'guest.recording_notice' : null,
                info.data.lobby ? 'guest.lobby_notice' : null,
              ].filter(Boolean) as string[]
            ).map((key): FormField => ({ key, kind: 'info', label: t(key), value: '' })),
          ],
        },
      ]}
    />
  );
}
