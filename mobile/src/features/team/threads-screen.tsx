import { HeaderActions } from '@/features/guides/help-menu';
import { FormSheet } from '@/ui/form-sheet';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatRelative } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { useCreateThread, useTeamPresence, useThreads } from './api';

/**
 * Team chat (CF-106): direct and group threads and "swarm" rooms opened from
 * a conversation. Unread counts per thread; a new thread picks members.
 */
export function ThreadsScreen() {
  const { t } = useTranslation('team');
  const threads = useThreads();
  const refresh = useManualRefresh(threads);
  const [creating, setCreating] = useState(false);
  return (
    <>
      <Stack.Screen
        options={{
          title: t('chat.title'),
          headerRight: () => (
            <HeaderActions>
              {<HeaderButton icon="add" label={t('chat.new')} onPress={() => setCreating(true)} />}
            </HeaderActions>
          ),
        }}
      />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={threads}>
          {(d) =>
            d.items.length ? (
              <List>
                {d.items.map((th, i) => (
                  <ListRow
                    key={th.uuid}
                    icon={th.kind === 'direct' ? 'user' : th.kind === 'swarm' ? 'alert' : 'chat'}
                    title={th.title || t(`chat.kind_${th.kind}`)}
                    subtitle={[
                      t(`chat.kind_${th.kind}`),
                      th.closed_at ? t('chat.closed') : null,
                      th.updated_at ? formatRelative(th.updated_at) : null,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                    trailing={
                      th.unread ? (
                        <View className="min-w-6 items-center rounded-full bg-signal px-1.5 py-0.5">
                          <Text variant="caption" className="font-sans-bold text-signal-foreground">
                            {th.unread > 99 ? '99+' : th.unread}
                          </Text>
                        </View>
                      ) : undefined
                    }
                    onPress={() =>
                      router.push({
                        pathname: '/team-chat/[uuid]',
                        params: { uuid: th.uuid, title: th.title },
                      })
                    }
                    last={i === d.items.length - 1}
                  />
                ))}
              </List>
            ) : (
              <EmptyState
                icon="chat"
                title={t('chat.empty')}
                body={t('chat.empty_hint')}
                action={{ title: t('chat.new'), onPress: () => setCreating(true), icon: 'add' }}
              />
            )
          }
        </QueryState>
      </Screen>
      {creating ? <NewThread onDone={() => setCreating(false)} /> : null}
    </>
  );
}

function NewThread({ onDone }: { onDone: () => void }) {
  const { t } = useTranslation('team');
  const me = useSession((s) => s.me?.user?.uuid);
  const presence = useTeamPresence();
  const create = useCreateThread();
  const [members, setMembers] = useState<string[]>([]);
  const [title, setTitle] = useState('');
  const direct = members.length === 1;
  const [search, setSearch] = useState('');
  const options = (presence.data?.items ?? []).filter((m) => m.user_uuid !== me);
  const shown = options.filter((m) => m.name.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()));
  const start = () => {
    if (!members.length || create.isPending) return;
    create.mutate(
      { kind: direct ? 'direct' : 'group', members, ...(title.trim() ? { title: title.trim() } : {}) },
      {
        onSuccess: (th) => {
          onDone();
          router.push({ pathname: '/team-chat/[uuid]', params: { uuid: th.uuid, title: th.title } });
        },
        onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
      },
    );
  };
  return (
    <FormSheet
      visible
      title={t('chat.new')}
      onCancel={onDone}
      dirty={members.length > 0 || !!title}
      detents={['large']}
      searchable
      onSearch={setSearch}
      primary={{ label: t('chat.start'), disabled: !members.length, loading: create.isPending, onPress: start }}
      sections={[
        {
          title: t('chat.to'),
          fields: [
            {
              key: 'members',
              kind: 'multi',
              label: t('chat.to'),
              values: members,
              onChange: setMembers,
              options: shown.map((m) => ({ value: m.user_uuid, label: m.name })),
            },
          ],
        },
        ...(!direct && members.length > 1
          ? [
              {
                fields: [
                  {
                    key: 'title',
                    kind: 'text' as const,
                    label: t('chat.thread_title'),
                    value: title,
                    onChange: setTitle,
                  },
                ],
              },
            ]
          : []),
      ]}
    />
  );
}
