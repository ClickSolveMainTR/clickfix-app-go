import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { formatRelative } from '@/core/format';
import { PresenceCard } from '@/features/presence/presence-card';
import { ShiftCard } from '@/features/presence/shift-card';
import { EmptyState } from '@/ui/empty-state';
import { Avatar } from '@/ui/avatar';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { useManualRefresh } from '@/ui/use-manual-refresh';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';

import { type PresenceMember, useTeamPresence } from './api';
import { PRESENCE_DOT } from './names';

const ORDER = ['online', 'busy', 'away', 'offline'] as const;

/** Team status (the panel's /team-status): everyone by availability, with my status and shift on top. */
export function TeamStatusScreen() {
  const { t } = useTranslation('team');
  const presence = useTeamPresence();
  const refresh = useManualRefresh(presence);
  return (
    <>
      <Stack.Screen options={{ title: t('status.title') }} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <PresenceCard />
        <ShiftCard />
        <QueryState query={presence}>
          {(d) => (
            <View>
              {!d.items.length ? <EmptyState icon="customers" title={t('common:no_results')} /> : null}
              {ORDER.map((st) => {
                const members = d.items
                  .filter((m) => m.status === st)
                  .sort((a, b) => a.name.localeCompare(b.name, 'tr'));
                if (!members.length) return null;
                return (
                  <View key={st}>
                    <SectionHeader title={`${t(`status.${st}`)} · ${members.length}`} />
                    <List>
                      {members.map((m, i) => (
                        <MemberRow key={m.user_uuid} member={m} last={i === members.length - 1} />
                      ))}
                    </List>
                  </View>
                );
              })}
            </View>
          )}
        </QueryState>
      </Screen>
    </>
  );
}

function MemberRow({ member: m, last }: { member: PresenceMember; last: boolean }) {
  const { t } = useTranslation('team');
  return (
    <ListRow
      leading={<Avatar name={m.name} size={36} dot={PRESENCE_DOT[m.status]} />}
      title={m.name}
      subtitle={
        m.status === 'online'
          ? t('status.online')
          : m.last_seen_at
            ? t('status.last_seen', { at: formatRelative(m.last_seen_at) })
            : t(`status.${m.status}`)
      }
      last={last}
    />
  );
}
