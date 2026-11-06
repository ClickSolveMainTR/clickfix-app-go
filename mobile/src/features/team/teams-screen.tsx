import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RefreshControl, View } from 'react-native';

import { P, useCan } from '@/core/auth/permissions';
import { useDebounced } from '@/core/use-debounced';
import { useTeams } from '@/features/inbox/api';
import { EmptyState } from '@/ui/empty-state';
import { HeaderButton } from '@/ui/header-button';
import { List, ListRow } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { SearchField } from '@/ui/search-field';
import { Text } from '@/ui/text';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import type { Team } from './api';
import { TeamFormSheet } from './team-form-sheet';
import { hoursSummary, personName } from './teams-core';

const openTeam = (team: Team) => router.push({ pathname: '/more/team-detail', params: { uuid: team.uuid } });

/** Team color as a dot; a team without a color gets the neutral one. */
export function TeamDot({ color, size = 12 }: { color: string; size?: number }) {
  return (
    <View
      className="rounded-full bg-muted-foreground"
      style={[{ width: size, height: size }, color ? { backgroundColor: color } : null]}
    />
  );
}

/**
 * Teams of the organization (CF-166), the phone version of the panel's
 * Teams page (`frontend/src/features/teams/components/teams-page.tsx`):
 * search, leads, member count, skills and working hours per team.
 * `tenant.teams.read` opens the screen; creating needs `tenant.teams.write`.
 */
export function TeamsScreen() {
  const { t } = useTranslation('team');
  const can = useCan();
  const canRead = can(P.teamsRead);
  const canWrite = can(P.teamsWrite);
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const list = useTeams(true, q);
  const refresh = useManualRefresh(list);
  const [creating, setCreating] = useState(false);

  if (!canRead)
    return (
      <>
        <Stack.Screen options={{ title: t('manage.title') }} />
        <Screen>
          <EmptyState icon="customers" title={t('manage.no_permission')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen
        options={{
          title: t('manage.title'),
          headerRight: () =>
            canWrite ? <HeaderButton icon="add" label={t('manage.new')} onPress={() => setCreating(true)} /> : null,
        }}
      />
      {/* Like the panel, a new team opens on its page, where members are added. */}
      <TeamFormSheet visible={creating} team={null} onClose={() => setCreating(false)} onSaved={openTeam} />
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <SearchField value={text} onChangeText={setText} placeholder={t('manage.search')} />
        <QueryState query={list}>
          {(data) =>
            data.items.length ? (
              <View className="gap-2">
                <List>
                  {data.items.map((team, i) => {
                    const hours = hoursSummary(team);
                    return (
                      <ListRow
                        key={team.uuid}
                        leading={<TeamDot color={team.color} />}
                        title={team.name}
                        subtitle={
                          <View className="gap-0.5">
                            <Text variant="footnote" tone="muted" numberOfLines={1}>
                              {[
                                t('manage.member_count', { count: team.member_count }),
                                team.leads.length
                                  ? t('manage.leads', { names: team.leads.map(personName).join(', ') })
                                  : null,
                              ]
                                .filter(Boolean)
                                .join(' · ')}
                            </Text>
                            <Text variant="footnote" tone="muted" numberOfLines={1}>
                              {[
                                hours.kind === 'org'
                                  ? t('manage.hours_org')
                                  : hours.open
                                    ? t('manage.hours_own', { count: hours.open })
                                    : t('manage.hours_closed'),
                                team.skills.join(', '),
                              ]
                                .filter(Boolean)
                                .join(' · ')}
                            </Text>
                          </View>
                        }
                        onPress={() => openTeam(team)}
                        last={i === data.items.length - 1}
                      />
                    );
                  })}
                </List>
                {data.total > data.items.length ? (
                  <Text variant="footnote" tone="muted" className="px-4">
                    {t('manage.more_hint', { shown: data.items.length, total: data.total })}
                  </Text>
                ) : null}
              </View>
            ) : q.trim() ? (
              <EmptyState icon="search" title={t('common:no_results')} />
            ) : (
              <EmptyState
                icon="customers"
                title={t('manage.empty')}
                body={t('manage.empty_hint')}
                action={
                  canWrite ? { title: t('manage.new'), icon: 'add', onPress: () => setCreating(true) } : undefined
                }
              />
            )
          }
        </QueryState>
      </Screen>
    </>
  );
}
