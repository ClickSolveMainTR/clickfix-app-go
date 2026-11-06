import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, RefreshControl, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { formatCompactDate } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useMembers } from '@/features/admin/api';
import { Avatar } from '@/ui/avatar';
import { EmptyState } from '@/ui/empty-state';
import { FormSheet } from '@/ui/form-sheet';
import { HeaderButton } from '@/ui/header-button';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { QueryState } from '@/ui/query-state';
import { Screen } from '@/ui/screen';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';
import { useManualRefresh } from '@/ui/use-manual-refresh';

import { type Team, type TeamMember, useTeam, useTeamMutations } from './api';
import { TeamFormSheet } from './team-form-sheet';
import { hoursSummary, matchesWords, personName, WEEK_DAYS } from './teams-core';
import { TeamDot } from './teams-screen';

const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));

/**
 * One team (CF-166), the phone version of the panel's team page
 * (`frontend/src/features/teams/components/team-pages.tsx`,
 * `team-members-tab.tsx`): details and hours, then the members with their
 * team role. With `tenant.teams.write`: edit and delete the team, add
 * members, make one a lead and remove one (confirmed first).
 */
export function TeamDetailScreen() {
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const { t } = useTranslation('team');
  const can = useCan();
  const canWrite = can(P.teamsWrite);
  const query = useTeam(uuid);
  const refresh = useManualRefresh(query);
  const m = useTeamMutations();
  const [editing, setEditing] = useState(false);
  const [adding, setAdding] = useState(false);
  const team = query.data;
  const busy = m.setMembers.isPending || m.removeMember.isPending;

  const confirmRemove = (member: TeamMember) =>
    Alert.alert(t('manage.remove_title', { name: personName(member.user) }), t('manage.remove_body'), [
      { text: t('common:cancel'), style: 'cancel' },
      {
        text: t('manage.remove'),
        style: 'destructive',
        onPress: () =>
          m.removeMember.mutate(
            { uuid, userUuid: member.user.uuid },
            { onSuccess: () => toast.success(t('manage.member_removed')), onError: fail },
          ),
      },
    ]);

  // A member row opens its actions: lead on or off, and removal.
  const memberActions = (member: TeamMember) =>
    Alert.alert(personName(member.user), member.user.email, [
      {
        text: t(member.role === 'lead' ? 'manage.make_member' : 'manage.make_lead'),
        onPress: () =>
          m.setMembers.mutate(
            { uuid, users: [member.user.uuid], role: member.role === 'lead' ? 'member' : 'lead' },
            { onSuccess: () => toast.success(t('manage.member_set')), onError: fail },
          ),
      },
      { text: t('manage.remove'), style: 'destructive', onPress: () => confirmRemove(member) },
      { text: t('common:cancel'), style: 'cancel' },
    ]);

  if (!can(P.teamsRead))
    return (
      <>
        <Stack.Screen options={{ title: t('manage.team') }} />
        <Screen>
          <EmptyState icon="customers" title={t('manage.no_permission')} />
        </Screen>
      </>
    );

  return (
    <>
      <Stack.Screen
        options={{
          title: team?.name ?? t('manage.team'),
          headerRight: () =>
            team && canWrite ? (
              <HeaderButton
                icon="edit"
                label={t('manage.edit')}
                onPress={() => {
                  // The form starts from the team as it is now, not as it was when the screen opened.
                  void query.refetch();
                  setEditing(true);
                }}
              />
            ) : null,
        }}
      />
      {team ? (
        <>
          <TeamFormSheet
            visible={editing}
            team={team}
            onClose={() => setEditing(false)}
            onDeleted={() => router.back()}
          />
          <AddMembersSheet team={team} visible={adding} onClose={() => setAdding(false)} />
        </>
      ) : null}
      <Screen refreshControl={<RefreshControl {...refresh} />}>
        <QueryState query={query}>
          {(d) => {
            const members = d.members ?? [];
            return (
              <View className="gap-4">
                <List>
                  <ListRow
                    leading={<TeamDot color={d.color} size={16} />}
                    title={d.name}
                    titleLines={2}
                    subtitle={d.description || undefined}
                  />
                  <ListRow
                    icon="tag"
                    title={t('manage.field_skills')}
                    subtitle={d.skills.length ? d.skills.join(', ') : t('manage.none')}
                    last
                  />
                </List>
                <Hours team={d} />
                <View>
                  <SectionHeader title={`${t('manage.members')} · ${members.length}`} />
                  <List>
                    {members.map((member, i) => {
                      const name = personName(member.user);
                      return (
                        <ListRow
                          key={member.user.uuid}
                          leading={<Avatar name={name} size={36} />}
                          title={name}
                          subtitle={[
                            member.user.email,
                            t('manage.joined', { date: formatCompactDate(member.joined_at) }),
                            member.skills.join(', '),
                          ]
                            .filter(Boolean)
                            .join(' · ')}
                          trailing={
                            <Text variant="footnote" tone={member.role === 'lead' ? 'default' : 'muted'}>
                              {t(`manage.role_${member.role}`)}
                            </Text>
                          }
                          onPress={canWrite ? () => memberActions(member) : undefined}
                          disabled={busy}
                          last={!canWrite && i === members.length - 1}
                        />
                      );
                    })}
                    {!members.length ? (
                      <ListRow
                        title={t('manage.members_empty')}
                        subtitle={t('manage.members_empty_hint')}
                        last={!canWrite}
                      />
                    ) : null}
                    {canWrite ? (
                      <ListRow icon="personAdd" title={t('manage.add_member')} onPress={() => setAdding(true)} last />
                    ) : null}
                  </List>
                </View>
              </View>
            );
          }}
        </QueryState>
      </Screen>
    </>
  );
}

/** Working hours: the organization's, or the team's own week in its time zone. */
function Hours({ team }: { team: Team }) {
  const { t } = useTranslation('team');
  const hours = hoursSummary(team);
  const days = team.working_hours?.days ?? [];
  return (
    <View>
      <SectionHeader title={t('manage.section_hours')} />
      <List>
        {hours.kind === 'org' ? (
          <ListRow icon="clock" title={t('manage.hours_org')} subtitle={t('manage.inherit_hours_hint')} last />
        ) : (
          <>
            <ListRow
              icon="clock"
              title={t('manage.field_timezone')}
              subtitle={team.timezone || t('manage.timezone_org')}
            />
            {WEEK_DAYS.map((day, i) => {
              const row = days.find((d) => d.day === day);
              return (
                <ListRow
                  key={day}
                  title={t(`manage.day_${day}`)}
                  trailing={
                    <Text tone={row?.open ? 'default' : 'muted'}>
                      {row?.open ? `${row.start} – ${row.end}` : t('manage.day_closed')}
                    </Text>
                  }
                  last={i === WEEK_DAYS.length - 1}
                />
              );
            })}
          </>
        )}
      </List>
    </View>
  );
}

/**
 * "Add member": organization members who are not in the team yet, from the
 * endpoint the panel's picker reads (`GET /v1/tenant/members`, so it needs
 * `tenant.members.read`). Every page is loaded while the sheet is open and
 * the search runs here, over name, surname and e-mail word by word: the
 * API's own `q` does not find a full name. Everyone picked joins as a
 * member; the lead role is set on the member's row afterwards.
 */
function AddMembersSheet({ team, visible, onClose }: { team: Team; visible: boolean; onClose: () => void }) {
  const { t } = useTranslation('team');
  const can = useCan();
  const m = useTeamMutations();
  const [search, setSearch] = useState('');
  const org = useMembers('');
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = org;
  useEffect(() => {
    if (visible && hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [visible, hasNextPage, isFetchingNextPage, fetchNextPage]);
  const [picked, setPicked] = useState<string[]>([]);
  const inTeam = new Set((team.members ?? []).map((x) => x.user.uuid));
  // After a partly failed add the team is read again: who got in leaves the selection.
  const pending = picked.filter((uuid) => !inTeam.has(uuid));
  const candidates = (org.data?.pages.flatMap((p) => p.items) ?? []).filter((x) => !inTeam.has(x.user.uuid));
  const options = candidates
    .filter((x) => matchesWords(`${x.user.name} ${x.user.surname} ${x.user.email}`, search))
    .map((x) => ({ value: x.user.uuid, label: personName(x.user), description: x.user.email }));
  const loading = org.isLoading || !!hasNextPage;
  const close = () => {
    setPicked([]);
    setSearch('');
    onClose();
  };
  return (
    <FormSheet
      visible={visible}
      title={t('manage.add_member')}
      onCancel={close}
      dirty={pending.length > 0}
      detents={['large']}
      searchable
      onSearch={setSearch}
      primary={{
        label: t('manage.add'),
        disabled: !pending.length,
        loading: m.setMembers.isPending,
        onPress: () =>
          m.setMembers.mutate(
            { uuid: team.uuid, users: pending, role: 'member' },
            {
              onSuccess: () => {
                toast.success(t('manage.members_added', { count: pending.length }));
                close();
              },
              onError: fail,
            },
          ),
      }}
      sections={[
        {
          title: t('manage.add_member_title'),
          fields: [
            {
              key: 'members',
              kind: 'multi',
              label: t('manage.add_member_title'),
              values: pending,
              onChange: setPicked,
              options,
            },
          ],
          footer: !can(P.membersRead)
            ? t('manage.add_no_permission')
            : org.isError
              ? errorMessage(undefined)
              : loading
                ? t('common:loading')
                : options.length
                  ? t('conversation:bulk.selected', { count: pending.length })
                  : candidates.length
                    ? t('common:no_results')
                    : t('manage.add_empty'),
        },
      ]}
    />
  );
}
