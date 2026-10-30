import { roleDescription, roleLabel } from '@/core/roles';
import { useTeamPresence } from '@/features/team/api';
import { PRESENCE_DOT } from '@/features/team/names';
import { FormSheet } from '@/ui/form-sheet';
import { HeaderButton } from '@/ui/header-button';
import { router, Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { formatRelative } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useDebounced } from '@/core/use-debounced';
import { Avatar } from '@/ui/avatar';
import { List, ListRow, SectionHeader } from '@/ui/list';
import { PagedList } from '@/ui/paged-list';
import { SearchField } from '@/ui/search-field';
import { toast } from '@/ui/toast';

import { type Member, useInvitations, useInvite, useMembers, useRoles } from './api';

/** Members and pending invitations; with `tenant.members.write`, invite by e-mail with roles. */
export function MembersScreen() {
  const { t } = useTranslation('admin');
  const can = useCan();
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const members = useMembers(q);
  const invitations = useInvitations();
  const presence = useTeamPresence();
  const [inviteOpen, setInviteOpen] = useState(false);
  const pending = invitations.data?.items ?? [];
  return (
    <>
      <Stack.Screen
        options={{
          title: t('members.title'),
          headerRight: () =>
            can(P.membersWrite) ? (
              <HeaderButton icon="personAdd" label={t('members.invite')} onPress={() => setInviteOpen(true)} />
            ) : null,
        }}
      />
      <InviteCard open={inviteOpen} onClose={() => setInviteOpen(false)} />
      <PagedList<Member>
        query={members}
        keyOf={(m) => m.user.uuid}
        emptyIcon="staff"
        empty={t('members.empty')}
        header={
          <View className="gap-3">
            {pending.length ? (
              <View>
                <SectionHeader title={t('members.pending', { count: pending.length })} />
                <List>
                  {pending.map((i, idx) => (
                    <ListRow
                      key={i.uuid}
                      icon="mail"
                      title={i.email}
                      subtitle={[
                        i.roles.map(roleLabel).join(', '),
                        t('members.expires', { at: formatRelative(i.expires_at) }),
                      ].join(' · ')}
                      last={idx === pending.length - 1}
                    />
                  ))}
                </List>
              </View>
            ) : null}
            <SearchField value={text} onChangeText={setText} placeholder={t('members.search')} />
          </View>
        }
        renderItem={(m, _i, last) => {
          const name = [m.user.name, m.user.surname].filter(Boolean).join(' ') || m.user.email;
          const state = presence.data?.items.find((p) => p.user_uuid === m.user.uuid);
          return (
            <ListRow
              onPress={() => router.push({ pathname: '/more/member', params: { uuid: m.user.uuid } })}
              leading={<Avatar name={name} size={36} dot={PRESENCE_DOT[state?.status ?? 'offline']} />}
              title={name}
              subtitle={[
                m.roles.map(roleLabel).join(', '),
                state?.last_seen_at
                  ? t('team:status.last_seen', { at: formatRelative(state.last_seen_at) })
                  : m.last_login_at
                    ? t('members.last_login', { at: formatRelative(m.last_login_at) })
                    : null,
              ]
                .filter(Boolean)
                .join(' · ')}
              last={last}
            />
          );
        }}
      />
    </>
  );
}

function InviteCard({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useTranslation('admin');
  const roles = useRoles(open);
  const invite = useInvite();
  const [email, setEmail] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const valid = /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim()) && selected.length > 0;
  return (
    <FormSheet
      visible={open}
      title={t('members.invite')}
      onCancel={() => {
        setEmail('');
        setSelected([]);
        onClose();
      }}
      dirty={!!email || selected.length > 0}
      primary={{
        label: t('members.send'),
        disabled: !valid,
        loading: invite.isPending,
        onPress: () =>
          invite.mutate(
            { email: email.trim(), role_uuids: selected },
            {
              onSuccess: () => {
                toast.success(t('members.sent', { email: email.trim() }));
                setEmail('');
                setSelected([]);
                onClose();
              },
              onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
            },
          ),
      }}
      sections={[
        {
          fields: [
            {
              key: 'email',
              kind: 'text',
              label: t('members.email'),
              value: email,
              onChange: setEmail,
              keyboard: 'email',
            },
          ],
          footer: t('conversation:form.invite_hint'),
        },
        {
          title: t('members.roles'),
          fields: [
            {
              key: 'roles',
              kind: 'multi',
              label: t('members.roles'),
              values: selected,
              onChange: setSelected,
              options: (roles.data?.items ?? []).map((r) => ({
                value: r.uuid,
                label: roleLabel(r),
                description: roleDescription(r),
              })),
            },
          ],
          footer: roles.isLoading
            ? t('common:loading')
            : roles.isError
              ? errorMessage(undefined)
              : t('conversation:bulk.selected', { count: selected.length }),
        },
      ]}
    />
  );
}
