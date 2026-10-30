import { Stack, useLocalSearchParams } from 'expo-router';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import { P, useCan } from '@/core/auth/permissions';
import { isApiError } from '@/core/api/errors';
import { formatRelative } from '@/core/format';
import { roleDescription, roleLabel } from '@/core/roles';
import { errorMessage } from '@/core/i18n';
import { useTeamPresence } from '@/features/team/api';
import { PRESENCE_DOT } from '@/features/team/names';
import { Avatar } from '@/ui/avatar';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { Icon } from '@/ui/icon';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useMember, useMemberTeams, useRoles, useSetMemberRoles } from './api';
export function MemberScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('admin');
  const can = useCan();
  const member = useMember(uuid);
  const roles = useRoles(can(P.membersWrite));
  const teams = useMemberTeams(uuid);
  const presence = useTeamPresence().data?.items.find((m) => m.user_uuid === uuid);
  const update = useSetMemberRoles(uuid);
  return (
    <>
      <Stack.Screen options={{ title: t('members.title') }} />
      <Screen>
        <QueryState query={member}>
          {(m) => {
            const name = [m.user.name, m.user.surname].filter(Boolean).join(' ') || m.user.email;
            return (
              <View className="gap-4">
                <List>
                  <ListRow
                    leading={
                      <Avatar name={name} size={44} dot={PRESENCE_DOT[presence?.status ?? 'offline']} />
                    }
                    title={name}
                    subtitle={m.user.email}
                  />
                  <ListRow
                    title={t('team:status.title')}
                    subtitle={
                      presence?.last_seen_at
                        ? t('team:status.last_seen', { at: formatRelative(presence.last_seen_at) })
                        : undefined
                    }
                    trailing={<Text tone="muted">{t(`team:status.${presence?.status ?? 'offline'}`)}</Text>}
                    last
                  />
                </List>
                <View>
                  <SectionHeader title={t('members.roles')} />
                  <List>
                    {(can(P.membersWrite) ? roles.data?.items ?? m.roles : m.roles).map((role, index, all) => {
                      const selected = m.roles.some((r) => r.uuid === role.uuid);
                      const editable =
                        can(P.membersWrite) && !update.isPending && !(selected && m.roles.length === 1);
                      return (
                        <ListRow
                          key={role.uuid}
                          title={roleLabel(role)}
                          subtitle={roleDescription(role)}
                          trailing={
                            selected ? <Icon name="check" tone="primary" size={20} /> : <View className="w-5" />
                          }
                          last={index === all.length - 1}
                          onPress={
                            editable
                              ? () => update.mutate(
                                  selected
                                    ? m.roles.filter((r) => r.uuid !== role.uuid).map((r) => r.uuid)
                                    : [...m.roles.map((r) => r.uuid), role.uuid],
                                  { onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)) },
                                )
                              : undefined
                          }
                        />
                      );
                    })}
                  </List>
                  <Text variant="footnote" tone="muted" className="px-4 pt-2">
                    {t('conversation:bulk.selected', { count: m.roles.length })}
                  </Text>
                </View>
                {can(P.teamsRead) ? (
                  <View>
                    <SectionHeader title={t('members.teams')} />
                    <QueryState query={teams}>
                      {(d) => (
                        <List>
                          {d.items.length ? (
                            d.items.map((team, i) => (
                              <ListRow
                                key={team.uuid}
                                icon="customers"
                                title={team.name}
                                last={i === d.items.length - 1}
                              />
                            ))
                          ) : (
                            <ListRow title={t('common:no_results')} last />
                          )}
                        </List>
                      )}
                    </QueryState>
                  </View>
                ) : null}
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}
