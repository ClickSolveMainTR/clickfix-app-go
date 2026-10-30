import { FormSheet } from '@/ui/form-sheet';
import { useQuery } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';

import { api, orgHeader, unwrap } from '@/core/api/client';
import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { formatRelative, formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { useDebounced } from '@/core/use-debounced';
import { useIssues, useLinkIssue } from '@/features/engineering/api';
import { Button } from '@/ui/button';
import { Card } from '@/ui/card';
import { Icon } from '@/ui/icon';
import { List, ListRow } from '@/ui/list';
import { OptionSheet, Select } from '@/ui/select';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { type ConversationDetail, inboxKeys, useInboxScope, useTags, useTeams, useUpdateConversation } from './api';
import {
  useAddReminder,
  useAddTag,
  useCancelReminder,
  useConversationEngineering,
  useCreateSideConversation,
  useLinkConversation,
  useLinks,
  useMerge,
  useReminders,
  useRemoveTag,
  useSideConversations,
} from './conversation-api';
import { IssueSheet } from './conversation-tools';
import { SNOOZE_PRESETS, snoozeUntil } from './helpers';

const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));
const NO_TEAM = '__none';

/** Team assignment (`PATCH team`, `tenant.conversations.assign`). */
export function TeamSelect({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('conversation');
  const { canAssign } = useInboxScope();
  const teams = useTeams(canAssign);
  const update = useUpdateConversation(c.uuid);
  if (!canAssign || !teams.data?.items.length) return null;
  return (
    <Select<string>
      label={t('details.team')}
      icon="customers"
      searchable
      value={c.team?.uuid ?? NO_TEAM}
      options={[
        { value: NO_TEAM, label: t('details.no_team') },
        ...teams.data.items.map((tm) => ({ value: tm.uuid, label: tm.name })),
      ]}
      onChange={(v) => update.mutate({ team: v === NO_TEAM ? null : v }, { onError: fail })}
    />
  );
}

/** Tags with remove (×) and add / create (`/conversations/{uuid}/tags`). */
export function TagsCard({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('conversation');
  const { canManage } = useInboxScope();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState('');
  const tags = useTags(open);
  const add = useAddTag(c.uuid);
  const remove = useRemoveTag(c.uuid);
  const have = new Set(c.tags.map((x) => x.uuid));
  const query = q.trim();
  const exact = tags.data?.items.some((x) => x.name.toLocaleLowerCase('tr') === query.toLocaleLowerCase('tr'));
  const options = [
    ...(tags.data?.items ?? [])
      .filter(
        (x) => !have.has(x.uuid) && (!query || x.name.toLocaleLowerCase('tr').includes(query.toLocaleLowerCase('tr'))),
      )
      .map((x) => ({ value: x.uuid, label: `#${x.name}` })),
    ...(query && !exact ? [{ value: `new:${query}`, label: t('details.create_tag', { name: query }) }] : []),
  ];

  return (
    <Card title={t('inbox:header.tags')}>
      <View className="flex-row flex-wrap gap-2">
        {c.tags.map((tag) => (
          <Pressable
            key={tag.uuid}
            accessibilityRole="button"
            accessibilityLabel={canManage ? t('details.remove_tag', { name: tag.name }) : `#${tag.name}`}
            disabled={!canManage || remove.isPending}
            onPress={() => remove.mutate(tag.uuid, { onError: fail })}
            className="flex-row items-center gap-1 rounded-full bg-secondary px-3 py-1.5">
            <Text variant="footnote" className="font-sans-semibold">
              #{tag.name}
            </Text>
            {canManage ? <Icon name="close" size={13} tone="muted-foreground" /> : null}
          </Pressable>
        ))}
        {canManage ? (
          <Pressable
            accessibilityRole="button"
            onPress={() => {
              setQ('');
              setOpen(true);
            }}
            className="flex-row items-center gap-1 rounded-full border border-dashed border-border px-3 py-1.5">
            <Icon name="add" size={13} tone="primary" />
            <Text variant="footnote" tone="primary" className="font-sans-semibold">
              {t('details.add_tag')}
            </Text>
          </Pressable>
        ) : null}
        {!c.tags.length && !canManage ? (
          <Text variant="footnote" tone="muted">
            {t('details.no_tags')}
          </Text>
        ) : null}
      </View>
      <OptionSheet<string>
        visible={open}
        title={t('details.add_tag')}
        onSearch={setQ}
        loading={tags.isFetching}
        options={options}
        onClose={() => setOpen(false)}
        onPick={(v) => {
          setOpen(false);
          add.mutate(v.startsWith('new:') ? { name: v.slice(4) } : { uuid: v }, { onError: fail });
        }}
      />
    </Card>
  );
}

/** Reminders: "remind me" (always or only if the customer has not replied). */
export function RemindersCard({ uuid }: { uuid: string }) {
  const { t } = useTranslation('conversation');
  const reminders = useReminders(uuid);
  const add = useAddReminder(uuid);
  const cancel = useCancelReminder(uuid);
  const [open, setOpen] = useState(false);
  const items = reminders.data?.items ?? [];
  return (
    <Card title={t('details.reminders')}>
      {items.map((r) => (
        <View key={r.uuid} className="flex-row items-center gap-2">
          <Icon name="bell" size={16} tone="primary" />
          <View className="flex-1">
            <Text variant="footnote" className="font-sans-semibold">
              {formatShort(r.due_at)}
            </Text>
            <Text variant="footnote" tone="muted">
              {[t(`details.reminder_${r.condition}`), r.note].filter(Boolean).join(' · ')}
            </Text>
          </View>
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('details.cancel_reminder')}
            hitSlop={8}
            disabled={cancel.isPending}
            accessibilityState={{ disabled: cancel.isPending }}
            onPress={() => cancel.mutate(r.uuid, { onError: fail })}>
            <Icon name="close" size={16} tone="muted-foreground" />
          </Pressable>
        </View>
      ))}

      <OptionSheet<string>
        trigger={
          <Button
            title={t('details.add_reminder')}
            variant="outline"
            icon="bell"
            loading={add.isPending}
            onPress={() => setOpen(true)}
          />
        }
        disabled={add.isPending}
        visible={open}
        title={t('details.add_reminder')}
        options={SNOOZE_PRESETS.flatMap((p) => [
          {
            value: `always|${p}`,
            label: t(`inbox:snooze.${p}`),
            description: formatShort(snoozeUntil(p).toISOString()),
          },
          {
            value: `no_reply|${p}`,
            label: `${t(`inbox:snooze.${p}`)} · ${t('details.reminder_no_reply')}`,
            description: formatShort(snoozeUntil(p).toISOString()),
          },
        ])}
        onClose={() => setOpen(false)}
        onPick={(v) => {
          setOpen(false);
          const [condition, preset] = v.split('|') as ['always' | 'no_reply', (typeof SNOOZE_PRESETS)[number]];
          add.mutate(
            { due_at: snoozeUntil(preset).toISOString(), condition },
            { onSuccess: () => toast.success(t('details.reminder_set')), onError: fail },
          );
        }}
      />
    </Card>
  );
}

/** Conversations found by text (merge / link pickers). */
function useConversationSearch(q: string, enabled: boolean) {
  const { org } = useInboxScope();
  const query = q.trim();
  return useQuery({
    queryKey: [...inboxKeys.all(org), 'pick', query] as const,
    enabled: !!org && enabled,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
    queryFn: () =>
      unwrap(
        api.GET('/v1/tenant/conversations', {
          params: {
            header: orgHeader(),
            query: { ...(query ? { q: query } : {}), limit: 20, sort: '-last_message_at' },
          },
        }),
      ),
  });
}

type LinkMode = 'merge' | 'related' | 'parent' | 'child' | null;

/** Merged-into banner, linked conversations, and "merge another one here" / "link" actions. */
export function LinksCard({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('conversation');
  const { canManage } = useInboxScope();
  const links = useLinks(c.uuid);
  const link = useLinkConversation(c.uuid);
  const merge = useMerge(c.uuid);
  const [mode, setMode] = useState<LinkMode>(null);
  const [chooser, setChooser] = useState(false);
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const found = useConversationSearch(q, !!mode);
  const data = links.data;
  if (!data) return null;
  const items = data.items;
  if (!items.length && !data.merged_into && !canManage) return null;

  return (
    <Card title={t('details.links')}>
      {data.merged_into ? (
        <Pressable
          onPress={() => router.push(`/conversation/${data.merged_into?.uuid}`)}
          className="rounded-lg bg-warning/15 p-3">
          <Text variant="footnote">
            {t('details.merged_into', { number: data.merged_into.number, subject: data.merged_into.subject })}
          </Text>
        </Pressable>
      ) : null}
      {items.length ? (
        <List>
          {items.map((l, i) => (
            <ListRow
              key={`${l.relation}-${l.uuid}`}
              icon={l.relation === 'merged' ? 'merge' : 'link'}
              title={`#${l.number} ${l.subject}`}
              subtitle={[
                t(`details.relation_${l.relation}`),
                t(`inbox:status.${l.status}`, { defaultValue: l.status }),
                formatRelative(l.last_message_at),
              ].join(' · ')}
              onPress={() => router.push(`/conversation/${l.uuid}`)}
              last={i === items.length - 1}
            />
          ))}
        </List>
      ) : null}
      {canManage ? (
        <View className="flex-row gap-2">
          <OptionSheet<'related' | 'parent' | 'child'>
            trigger={
              <Button
                title={t('details.link')}
                variant="outline"
                icon="link"
                className="flex-1"
                onPress={() => setChooser(true)}
              />
            }
            visible={chooser}
            title={t('details.link')}
            options={(['related', 'parent', 'child'] as const).map((r) => ({
              value: r,
              label: t(`details.relation_${r}`),
              description: t(`details.relation_${r}_hint`),
            }))}
            onClose={() => setChooser(false)}
            onPick={(r) => {
              setChooser(false);
              setMode(r);
            }}
          />
          <Button
            title={t('details.merge')}
            variant="outline"
            icon="merge"
            className="flex-1"
            onPress={() => setMode('merge')}
          />
        </View>
      ) : null}

      <OptionSheet<string>
        visible={!!mode}
        title={mode === 'merge' ? t('details.merge_pick') : t('details.link_pick')}
        onSearch={setText}
        loading={found.isFetching}
        options={(found.data?.items ?? [])
          .filter((x) => x.uuid !== c.uuid)
          .map((x) => ({
            value: x.uuid,
            label: `#${x.number} ${x.subject || x.contact.name}`,
            description: [x.contact.name, t(`inbox:status.${x.status}`), formatRelative(x.last_message_at)].join(' · '),
          }))}
        onClose={() => setMode(null)}
        onPick={(other) => {
          const m = mode;
          setMode(null);
          setText('');
          if (m === 'merge')
            merge.mutate(other, { onSuccess: () => toast.success(t('details.merged')), onError: fail });
          else if (m)
            link.mutate(
              { conversation_uuid: other, relation: m },
              { onSuccess: () => toast.success(t('details.linked')), onError: fail },
            );
        }}
      />
    </Card>
  );
}

/** Side conversations (e-mail threads with third parties, CF-79): list, open, start one. */
export function SideConversationsCard({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('conversation');
  const { canReply } = useInboxScope();
  const sides = useSideConversations(c.uuid);
  const [open, setOpen] = useState(false);
  const items = sides.data?.items ?? [];
  if (!items.length && !canReply) return null;
  return (
    <Card title={t('side.title')}>
      {items.length ? (
        <List>
          {items.map((s, i) => (
            <ListRow
              key={s.uuid}
              icon="mail"
              title={s.subject}
              subtitle={[
                s.participants.join(', '),
                t(`side.status_${s.status}`),
                formatRelative(s.last_message_at),
              ].join(' · ')}
              onPress={() => router.push(`/side-conversation/${s.uuid}`)}
              last={i === items.length - 1}
            />
          ))}
        </List>
      ) : (
        <Text variant="footnote" tone="muted">
          {t('side.empty')}
        </Text>
      )}
      {canReply ? <Button title={t('side.new')} variant="outline" icon="add" onPress={() => setOpen(true)} /> : null}
      <NewSideSheet
        conversation={c.uuid}
        inboxes={sides.data?.inboxes ?? []}
        visible={open}
        onClose={() => setOpen(false)}
      />
    </Card>
  );
}

function NewSideSheet({
  conversation,
  inboxes,
  visible,
  onClose,
}: {
  conversation: string;
  inboxes: { uuid: string; name: string }[];
  visible: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation('conversation');
  const create = useCreateSideConversation(conversation);
  const [to, setTo] = useState('');
  const [subject, setSubject] = useState('');
  const [body, setBody] = useState('');
  const [inbox, setInbox] = useState<string | null>(null);
  const recipients = to
    .split(/[,\s;]+/)
    .map((x) => x.trim())
    .filter(Boolean);
  const valid =
    recipients.length > 0 &&
    recipients.every((r) => /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(r)) &&
    !!subject.trim() &&
    !!body.trim();
  return (
    <FormSheet
      visible={visible}
      title={t('side.new')}
      onCancel={onClose}
      dirty={!!to || !!subject || !!body}
      detents={['large']}
      primary={{
        label: t('side.send'),
        disabled: !valid,
        loading: create.isPending,
        onPress: () =>
          create.mutate(
            { to: recipients, subject: subject.trim(), body: body.trim(), ...(inbox ? { inbox_uuid: inbox } : {}) },
            {
              onSuccess: (s) => {
                onClose();
                setTo('');
                setSubject('');
                setBody('');
                router.push(`/side-conversation/${s.uuid}`);
              },
              onError: fail,
            },
          ),
      }}
      sections={[
        {
          fields: [
            {
              key: 'to',
              kind: 'text',
              label: t('side.to'),
              hint: t('side.to_hint'),
              keyboard: 'email',
              value: to,
              onChange: setTo,
            },
            { key: 'subject', kind: 'text', label: t('side.subject'), value: subject, onChange: setSubject },
            {
              key: 'inbox',
              kind: 'pick',
              label: t('side.from'),
              value: inbox ?? inboxes[0]?.uuid ?? null,
              onChange: setInbox,
              options: inboxes.map((i) => ({ value: i.uuid, label: i.name })),
            },
            { key: 'body', kind: 'text', label: t('side.body'), value: body, onChange: setBody, multiline: true },
          ],
        },
      ]}
    />
  );
}

/** Engineering: suggested issues (link one), escalations, and "new issue". */
export function EngineeringCard({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('conversation');
  const can = useCan();
  const { enabled, suggestions, escalations } = useConversationEngineering(c.uuid);
  const link = useLinkIssue();
  const [creating, setCreating] = useState(false);
  const [picking, setPicking] = useState(false);
  const [text, setText] = useState('');
  const q = useDebounced(text, 300);
  const issues = useIssues(null, q, picking);
  if (!enabled) return null;
  const canWrite = can(P.ticketsWrite);
  const linkTo = (issue: string) =>
    link.mutate(
      { issue, conversation: c.uuid, notifyCustomer: false },
      { onSuccess: () => toast.success(t('issue.linked')), onError: fail },
    );
  const sugg = suggestions.data?.items ?? [];
  const esc = escalations.data?.items ?? [];

  return (
    <Card title={t('issue.card_title')}>
      {esc.map((e) => (
        <View key={e.uuid} className="flex-row items-center gap-2 rounded-lg bg-destructive/10 p-2.5">
          <Icon name="alert" size={16} tone="destructive" />
          <Text variant="footnote" className="flex-1">
            {t('issue.escalation', { tier: e.tier, due: formatShort(e.due_at) })}
            {e.reason ? ` · ${e.reason}` : ''}
          </Text>
        </View>
      ))}
      {sugg.length ? (
        <>
          <Text variant="footnote" tone="muted">
            {t('issue.suggested')}
          </Text>
          <List>
            {sugg.map((i, idx) => (
              <ListRow
                key={i.uuid}
                icon="bug"
                title={i.title}
                subtitle={`${t(`issue.status_${i.status}`)} · ${t(`inbox:priority.${i.priority}`)}`}
                onPress={() => router.push(`/engineering/${i.uuid}`)}
                trailing={
                  canWrite ? (
                    <Pressable
                      accessibilityRole="button"
                      accessibilityLabel={t('issue.link')}
                      hitSlop={8}
                      onPress={() => linkTo(i.uuid)}>
                      <Icon name="link" size={18} tone="primary" />
                    </Pressable>
                  ) : undefined
                }
                last={idx === sugg.length - 1}
              />
            ))}
          </List>
        </>
      ) : null}
      {canWrite ? (
        <View className="flex-row gap-2">
          <Button
            title={t('issue.new')}
            variant="outline"
            icon="bug"
            className="flex-1"
            onPress={() => setCreating(true)}
          />
          <Button
            title={t('issue.link')}
            variant="outline"
            icon="link"
            className="flex-1"
            onPress={() => setPicking(true)}
          />
        </View>
      ) : null}
      <IssueSheet conversation={c.uuid} visible={creating} onClose={() => setCreating(false)} />
      <OptionSheet<string>
        visible={picking}
        title={t('issue.link')}
        onSearch={setText}
        loading={issues.isFetching}
        options={(issues.data?.pages.flatMap((p) => p.items) ?? []).map((i) => ({
          value: i.uuid,
          label: i.title,
          description: `${t(`issue.status_${i.status}`)} · ${t(`inbox:priority.${i.priority}`)}`,
        }))}
        onClose={() => setPicking(false)}
        onPick={(v) => {
          setPicking(false);
          linkTo(v);
        }}
      />
    </Card>
  );
}
