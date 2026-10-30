import { router, Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ActivityIndicator, Linking, Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { Card } from '@/ui/card';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { QueryState } from '@/ui/query-state';
import { OptionSheet, Select } from '@/ui/select';
import { Screen } from '@/ui/screen';
import { StatusPill } from '@/ui/status-pill';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type ConversationDetail,
  type ConversationUpdate,
  useAgents,
  useContact,
  useConversation,
  useConversationSla,
  useInboxScope,
  useUpdateConversation,
} from './api';
import {
  EngineeringCard,
  LinksCard,
  RemindersCard,
  SideConversationsCard,
  TagsCard,
  TeamSelect,
} from './conversation-details-extra';
import { contactLabel, formatDuration, SNOOZE_PRESETS, type SnoozePreset, snoozeUntil } from './helpers';

const STATUSES = ['open', 'pending', 'resolved', 'closed'] as const;
const PRIORITIES = ['urgent', 'high', 'normal', 'low'] as const;
const UNASSIGNED = '__none__';

/**
 * Everything around one conversation (the panel's context panel and header
 * menus, CF-80): assignee, status, priority and snooze for members who may
 * change them, the contact and company, SLA clocks, tags and participants.
 */
export function ConversationDetailsScreen() {
  const { t } = useTranslation('conversation');
  const { uuid } = useLocalSearchParams<{ uuid: string }>();
  const conversation = useConversation(uuid);
  return (
    <>
      <Stack.Screen options={{ title: t('details.title') }} />
      <Screen>
        <QueryState query={conversation}>
          {(c) => (
            <>
              <Manage conversation={c} />
              <TagsCard conversation={c} />
              <ContactCard conversation={c} />
              <SlaCard uuid={c.uuid} />
              <RemindersCard uuid={c.uuid} />
              <SideConversationsCard conversation={c} />
              <LinksCard conversation={c} />
              <EngineeringCard conversation={c} />
              <Facts conversation={c} />
            </>
          )}
        </QueryState>
      </Screen>
    </>
  );
}

function Manage({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('inbox');
  const { canManage, canAssign } = useInboxScope();
  const me = useSession((s) => s.me?.user);
  const agents = useAgents(canAssign);
  const update = useUpdateConversation(c.uuid);
  const [snoozeOpen, setSnoozeOpen] = useState(false);

  const apply = (body: ConversationUpdate, done?: string) =>
    update.mutate(body, {
      onSuccess: () => done && toast.success(done),
      onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
    });

  const agentOptions = [
    { value: UNASSIGNED, label: t('list.unassigned') },
    ...(agents.data?.items ?? []).map((a) => ({
      value: a.uuid,
      label: a.uuid === me?.uuid ? `${a.name} (${t('header.you')})` : a.name,
      description: a.email,
    })),
  ];
  const mine = !!me?.uuid && c.assignee?.uuid === me.uuid;

  return (
    <Card title={t('context.details')}>
      <View className="flex-row flex-wrap items-center gap-2">
        <StatusPill
          label={t(`status.${c.status}`)}
          tone={c.status === 'resolved' || c.status === 'closed' ? 'done' : 'progress'}
        />
        {c.snoozed_until ? (
          <Text variant="footnote" tone="muted">
            {t('status.snoozed')} · {formatShort(c.snoozed_until)}
          </Text>
        ) : null}
        {update.isPending ? <ActivityIndicator /> : null}
      </View>
      {canAssign ? (
        <>
          <Select
            label={t('header.assign')}
            icon="user"
            value={c.assignee?.uuid ?? UNASSIGNED}
            options={agentOptions}
            searchable
            onChange={(v) => apply({ assignee: v === UNASSIGNED ? null : v })}
          />
          {!mine && me?.uuid ? (
            <QuickAction
              icon="personAdd"
              label={t('shortcuts.take')}
              onPress={() => apply({ assignee: me.uuid })}
            />
          ) : null}
        </>
      ) : null}
      <TeamSelect conversation={c} />
      {canManage ? (
        <>
          <Select
            label={t('context.status')}
            icon="check"
            value={c.status === 'snoozed' ? null : c.status}
            placeholder={t('status.snoozed')}
            options={STATUSES.map((s) => ({ value: s, label: t(`status.${s}`) }))}
            onChange={(v) => apply({ status: v })}
          />
          <Select
            label={t('shortcuts.priority')}
            icon="up"
            value={c.priority}
            options={PRIORITIES.map((p) => ({ value: p, label: t(`priority.${p}`) }))}
            onChange={(v) => apply({ priority: v })}
          />
          <OptionSheet<SnoozePreset>
            trigger={
              <QuickAction icon="clock" label={t('header.snooze')} onPress={() => setSnoozeOpen(true)} />
            }
            visible={snoozeOpen}
            title={t('header.snooze')}
            options={SNOOZE_PRESETS.map((p) => ({
              value: p,
              label: t(`snooze.${p}`),
              description: formatShort(snoozeUntil(p).toISOString()),
            }))}
            onClose={() => setSnoozeOpen(false)}
            onPick={(p) => {
              setSnoozeOpen(false);
              apply({ status: 'snoozed', snoozed_until: snoozeUntil(p).toISOString() }, t('status.snoozed'));
            }}
          />
        </>
      ) : null}
    </Card>
  );
}

function QuickAction({ icon, label, onPress }: { icon: IconName; label: string; onPress: () => void }) {
  return (
    <Pressable
      accessibilityRole="button"
      onPress={onPress}
      className="min-h-11 flex-row items-center gap-2 rounded-lg border border-border px-3 active:bg-muted">
      <Icon name={icon} size={18} tone="primary" />
      <Text tone="primary" className="font-sans-semibold">
        {label}
      </Text>
    </Pressable>
  );
}

function Row({
  label,
  value,
  onPress,
}: {
  label: string;
  value: string | null | undefined;
  onPress?: () => void;
}) {
  if (!value) return null;
  return (
    <Pressable
      disabled={!onPress}
      accessibilityRole={onPress ? 'link' : undefined}
      onPress={onPress}
      className="flex-row items-start justify-between gap-3">
      <Text variant="footnote" tone="muted">
        {label}
      </Text>
      <Text
        variant="footnote"
        tone={onPress ? 'primary' : 'default'}
        className="flex-1 text-right"
        selectable>
        {value}
      </Text>
    </Pressable>
  );
}

function ContactCard({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('contacts');
  const { canContacts } = useInboxScope();
  const contact = useContact(canContacts ? c.contact.uuid : undefined);
  const d = contact.data;
  const email = d?.email ?? c.contact.email;
  const phone = d?.phone ?? c.contact.phone;
  return (
    <Card title={t('detail')}>
      <Text variant="headline">{contactLabel(c)}</Text>
      <View className="gap-2">
        <Row
          label={t('fields.email')}
          value={email}
          onPress={email ? () => void Linking.openURL(`mailto:${email}`) : undefined}
        />
        <Row
          label={t('fields.phone')}
          value={phone}
          onPress={phone ? () => void Linking.openURL(`tel:${phone}`) : undefined}
        />
        <Row label={t('fields.company')} value={d?.company?.name ?? c.company?.name} />
        <Row label={t('fields.job_title')} value={d?.job_title} />
        <Row label={t('columns.conversations')} value={d ? String(d.conversation_count) : null} />
        <Row label={t('columns.last_seen')} value={d?.last_seen_at ? formatShort(d.last_seen_at) : null} />
      </View>
      {d?.notes ? (
        <Text variant="footnote" tone="muted">
          {d.notes}
        </Text>
      ) : null}
      {canContacts ? (
        <View className="flex-row gap-2">
          <QuickAction
            icon="user"
            label={t('detail')}
            onPress={() => router.push(`/contact/${c.contact.uuid}`)}
          />
          {c.company ? (
            <QuickAction
              icon="building"
              label={c.company.name}
              onPress={() => router.push(`/company/${c.company?.uuid}`)}
            />
          ) : null}
        </View>
      ) : null}
    </Card>
  );
}

function SlaCard({ uuid }: { uuid: string }) {
  const { t, i18n } = useTranslation('sla');
  const sla = useConversationSla(uuid);
  const metrics = sla.data?.metrics ?? [];
  if (!sla.data?.policy && metrics.length === 0) return null;
  return (
    <Card title={sla.data?.policy?.name ?? t('metric.resolution')}>
      {metrics.map((m) => {
        const late = m.status === 'breached' || (m.status === 'running' && m.remaining_seconds < 0);
        const time = formatDuration(m.remaining_seconds, i18n.language);
        return (
          <View key={`${m.metric}-${m.cycle}`} className="flex-row items-center justify-between gap-3">
            <Text variant="footnote">{t(`metric.${m.metric}`)}</Text>
            <Text
              variant="footnote"
              tone={late ? 'danger' : m.status === 'met' ? 'success' : 'muted'}
              className={cn('tabular-nums', late && 'font-sans-semibold')}>
              {m.status === 'running'
                ? m.remaining_seconds < 0
                  ? t('badge.overdue', { time })
                  : t('badge.left', { time })
                : t(`status.${m.status}`)}
            </Text>
          </View>
        );
      })}
    </Card>
  );
}

function Facts({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('inbox');
  return (
    <Card title={t('context.label')}>
      <View className="gap-2">
        <Row label={t('context.created')} value={formatShort(c.created_at)} />
        <Row
          label={t('context.first_response')}
          value={c.first_response_at ? formatShort(c.first_response_at) : null}
        />
        <Row label={t('context.messages')} value={String(c.message_count)} />
        <Row label={t('context.participants')} value={c.participants.map((p) => p.name).join(', ')} />
      </View>
    </Card>
  );
}
