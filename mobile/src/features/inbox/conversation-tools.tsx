import { FormSheet } from '@/ui/form-sheet';
import * as Clipboard from 'expo-clipboard';
import { router } from 'expo-router';
import { cloneElement, type ReactElement, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Platform, Pressable, type PressableProps, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { P, useCan } from '@/core/auth/permissions';
import { useSession } from '@/core/auth/session';
import { formatShort } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { type IssueCreate, useCreateIssue } from '@/features/engineering/api';
import {
  cleanPin,
  useStartConversationCall,
  useVideoAvailability,
  useVideoScope,
  videoError,
} from '@/features/video/api';
import { Icon } from '@/ui/icon';
import { ActionMenu } from '@/ui/menu';
import type { MenuOption } from '@/ui/menu.types';
import { OptionSheet } from '@/ui/select';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import {
  type ConversationDetail,
  type OptimisticMessage,
  useAgents,
  useInboxScope,
  useUpdateConversation,
} from './api';
import { type ConversationViewer, useApplyMacro, useMacros, useSplit } from './conversation-api';
import { contactLabel, SNOOZE_PRESETS, snoozeUntil } from './helpers';

const fail = (e: unknown) => toast.error(errorMessage(isApiError(e) ? e.code : undefined));

/**
 * Who else has this conversation open (the panel's viewers bar, CF-79).
 * Someone typing turns it into a warning so two agents never answer at once.
 */
export function ViewersBar({ viewers }: { viewers: ConversationViewer[] }) {
  const { t } = useTranslation('conversation');
  const me = useSession((s) => s.me?.user?.uuid);
  const others = viewers.filter((v) => v.uuid !== me);
  if (!others.length) return null;
  const typing = others.filter((v) => v.typing);
  const names = (typing.length ? typing : others).map((v) => v.name).join(', ');
  return (
    <View
      accessibilityRole="alert"
      accessibilityLiveRegion="polite"
      className={
        typing.length
          ? 'flex-row items-center gap-2 bg-warning/15 px-4 py-1.5'
          : 'flex-row items-center gap-2 bg-secondary px-4 py-1.5'
      }>
      <Icon name={typing.length ? 'edit' : 'show'} size={14} tone={typing.length ? 'warning' : 'muted-foreground'} />
      <Text variant="footnote" numberOfLines={1} className="flex-1">
        {typing.length
          ? t('viewers.typing', { names, count: typing.length })
          : t('viewers.viewing', { names, count: others.length })}
      </Text>
    </View>
  );
}

type MenuAction = 'video' | 'details' | 'take' | 'assign' | 'priority' | 'snooze' | 'macro' | 'issue';

/** The conversation header's native menu, with nested priority and snooze choices. */
export function ConversationMenu({
  conversation: c,
  trigger,
}: {
  conversation: ConversationDetail;
  trigger: ReactElement;
}) {
  const { t } = useTranslation('inbox');
  const { canManage, canAssign, canCanned } = useInboxScope();
  const can = useCan();
  const me = useSession((s) => s.me?.user?.uuid);
  const update = useUpdateConversation(c.uuid);
  const [sub, setSub] = useState<MenuAction | null>(null);
  const agents = useAgents(sub === 'assign');
  const { canUse: canVideo } = useVideoScope();
  const startCall = useStartConversationCall();
  const pinRequired = !!useVideoAvailability().data?.policy.require_pin;
  const call = (pin?: string) =>
    startCall.mutate(
      { conversation: c.uuid, name: contactLabel(c) || t('video:rooms.conversation_room'), pin },
      {
        onSuccess: (room) => {
          setSub(null);
          router.push(`/video/${room.uuid}`);
        },
        onError: (e) => toast.error(videoError(e)),
      },
    );

  const apply = (body: Parameters<typeof update.mutate>[0], done: string) =>
    update.mutate(body, { onSuccess: () => toast.success(done), onError: fail });

  const actions: { value: MenuAction; label: string; show: boolean }[] = [
    { value: 'take', label: t('shortcuts.take'), show: canAssign && !!me && c.assignee?.uuid !== me },
    { value: 'assign', label: t('header.assign'), show: canAssign },
    { value: 'priority', label: t('shortcuts.priority'), show: canManage },
    { value: 'snooze', label: t('header.snooze'), show: canManage && c.status !== 'snoozed' },
    { value: 'macro', label: t('conversation:menu.macro'), show: canCanned },
    { value: 'issue', label: t('conversation:menu.issue'), show: can(P.ticketsWrite) },
    {
      value: 'video',
      label: t('video:rooms.start_from_conversation'),
      show: canVideo && !startCall.isPending,
    },
    { value: 'details', label: t('conversation:menu.details'), show: true },
  ];

  const priorities = ['urgent', 'high', 'normal', 'low'] as const;
  const menuOptions: MenuOption<string>[] = actions
    .filter((a) => a.show)
    .map(({ value, label }) => ({
      value,
      label,
      children:
        value === 'priority'
          ? priorities.map((p) => ({ value: `priority:${p}`, label: t(`priority.${p}`) }))
          : value === 'snooze'
            ? SNOOZE_PRESETS.map((p) => ({
                value: `snooze:${p}`,
                label: t(`snooze.${p}`),
                description: formatShort(snoozeUntil(p).toISOString()),
              }))
            : undefined,
    }));
  const pick = (a: string) => {
    if (a.startsWith('priority:')) {
      const p = a.slice(9) as (typeof priorities)[number];
      return apply({ priority: p }, t(`priority.${p}`));
    }
    if (a.startsWith('snooze:')) {
      const p = a.slice(7) as (typeof SNOOZE_PRESETS)[number];
      return apply({ status: 'snoozed', snoozed_until: snoozeUntil(p).toISOString() }, t('status.snoozed'));
    }
    // Organizations that require a PIN get a one-field sheet first.
    if (a === 'video' && !pinRequired) return call();
    if (a === 'details') return router.push(`/conversation-details/${c.uuid}`);
    if (a === 'take' && me) return apply({ assignee: me }, t('conversation:row.done_take', { name: '' }).trim());
    setSub(a as MenuAction);
  };

  return (
    <>
      <ActionMenu
        title={`#${c.number}`}
        trigger={trigger}
        options={menuOptions}
        value={`priority:${c.priority}`}
        onPick={pick}
      />
      <OptionSheet<string>
        visible={sub === 'assign'}
        title={t('header.assign')}
        searchable
        loading={agents.isFetching}
        value={c.assignee?.uuid ?? null}
        options={[
          { value: '__none', label: t('header.unassign') },
          ...(agents.data?.items ?? []).map((a) => ({ value: a.uuid, label: a.name, description: a.email })),
        ]}
        onClose={() => setSub(null)}
        onPick={(v) => {
          setSub(null);
          apply({ assignee: v === '__none' ? null : v }, t('conversation:menu.assigned'));
        }}
      />
      <MacroSheet conversation={c.uuid} visible={sub === 'macro'} onClose={() => setSub(null)} />
      <IssueSheet conversation={c.uuid} visible={sub === 'issue'} onClose={() => setSub(null)} />
      <CallPinSheet
        visible={sub === 'video'}
        loading={startCall.isPending}
        onStart={call}
        onClose={() => setSub(null)}
      />
    </>
  );
}

function CallPinSheet({
  visible,
  loading,
  onStart,
  onClose,
}: {
  visible: boolean;
  loading: boolean;
  onStart: (pin: string) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('video');
  const [pin, setPin] = useState('');
  return (
    <FormSheet
      visible={visible}
      title={t('rooms.start_from_conversation')}
      onCancel={() => {
        setPin('');
        onClose();
      }}
      detents={['medium']}
      primary={{ label: t('rooms.create'), loading, disabled: pin.length < 4, onPress: () => onStart(pin) }}
      sections={[
        {
          fields: [
            {
              key: 'pin',
              kind: 'text',
              keyboard: 'number',
              label: t('rooms.pin_required'),
              hint: t('links.pin_hint'),
              value: pin,
              onChange: (v) => setPin(cleanPin(v)),
            },
          ],
        },
      ]}
    />
  );
}

/** Direct assignment from the compact status strip, using the existing agent permissions/API. */
export function AssigneeMenu({ conversation: c }: { conversation: ConversationDetail }) {
  const { t } = useTranslation('inbox');
  const { canAssign } = useInboxScope();
  const agents = useAgents(canAssign);
  const update = useUpdateConversation(c.uuid);
  const label = c.assignee?.name ?? t('list.unassigned');
  return (
    <View className="min-w-0 flex-1">
      <ActionMenu
        title={t('header.assign')}
        disabled={!canAssign || update.isPending}
        fullWidth
        value={c.assignee?.uuid ?? '__none'}
        trigger={
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={`${t('header.assign')}: ${label}`}
            className="min-w-0 flex-row items-center gap-1 py-1.5">
            <Icon name="user" size={13} tone="muted-foreground" />
            <Text variant="footnote" tone="muted" numberOfLines={1} className="shrink">
              {label}
            </Text>
            {canAssign ? <Icon name="chevronDown" size={10} tone="muted-foreground" /> : null}
          </Pressable>
        }
        options={[
          { value: '__none', label: t('header.unassign') },
          ...(agents.data?.items ?? []).map((a) => ({ value: a.uuid, label: a.name })),
        ]}
        onPick={(v) => update.mutate({ assignee: v === '__none' ? null : v }, { onError: fail })}
      />
    </View>
  );
}

/** Runs a macro (reply / note / status / priority / assign / tag actions) on the conversation. */
export function MacroSheet({
  conversation,
  visible,
  onClose,
}: {
  conversation: string;
  visible: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation('conversation');
  const macros = useMacros(visible);
  const run = useApplyMacro(conversation);
  return (
    <OptionSheet<string>
      visible={visible}
      title={t('menu.macro')}
      searchable
      loading={macros.isFetching}
      options={(macros.data?.items ?? []).map((m) => ({
        value: m.uuid,
        label: m.name,
        icon: 'macro',
        description: m.description || m.actions.map((a) => t(`macro_action.${a.type}`)).join(' · '),
      }))}
      onClose={onClose}
      onPick={(m) => {
        const macro = macros.data?.items.find((item) => item.uuid === m);
        if (!macro || run.isPending) return;
        Alert.alert(
          macro.name,
          macro.actions
            .map((action) => `${t(`macro_action.${action.type}`)}${action.value ? `: ${String(action.value)}` : ''}`)
            .join('\n'),
          [
            { text: t('common:cancel'), style: 'cancel' },
            {
              text: t('common:confirm'),
              onPress: () => {
                onClose();
                run.mutate(m, {
                  onSuccess: (r) => toast.success(t('menu.macro_done', { count: r.applied.length })),
                  onError: fail,
                });
              },
            },
          ],
        );
      }}
    />
  );
}

type MessageAction = 'copy' | 'one' | 'following' | 'issue' | 'retry' | 'discard';

/** Lift the message bubble on long press; the ellipsis exposes the same actions on tap. */
export function MessageMenu({
  conversation,
  message,
  trigger,
  context,
  onRetry,
  onDiscard,
  retrying,
  preview,
  maxWidth,
  alignment,
}: {
  conversation: string;
  message: OptimisticMessage;
  trigger: ReactElement;
  context?: boolean;
  onRetry?: () => void;
  onDiscard?: () => void;
  retrying?: boolean;
  preview?: ReactElement;
  maxWidth?: number;
  alignment?: 'leading' | 'trailing';
}) {
  const { t } = useTranslation('conversation');
  const { canManage } = useInboxScope();
  const can = useCan();
  const split = useSplit(conversation);
  const [issueFrom, setIssueFrom] = useState<OptimisticMessage | null>(null);
  const real = !!message && !message.optimistic;
  const options: MenuOption<MessageAction>[] = [
    ...(message?.content ? [{ value: 'copy' as const, label: t('message.copy') }] : []),
    ...(canManage && real
      ? [
          {
            value: 'one' as const,
            label: t('message.split'),
            children: [
              { value: 'one' as const, label: t('message.split_one') },
              { value: 'following' as const, label: t('message.split_following') },
            ],
          },
        ]
      : []),
    ...(can(P.ticketsWrite) && real ? [{ value: 'issue' as const, label: t('menu.issue') }] : []),
    ...(onRetry ? [{ value: 'retry' as const, label: t('inbox:delivery.retry'), disabled: retrying }] : []),
    ...(onDiscard ? [{ value: 'discard' as const, label: t('timeline.discard'), destructive: true }] : []),
  ];
  const pick = (action: MessageAction) => {
    const m = message;
    if (!m) return;
    if (action === 'copy') void Clipboard.setStringAsync(m.content).then(() => toast.success(t('message.copied')));
    if (action === 'issue') setIssueFrom(m);
    if (action === 'retry') onRetry?.();
    if (action === 'discard') onDiscard?.();
    if (action === 'one' || action === 'following')
      split.mutate(
        { message_uuid: m.uuid, following: action === 'following' },
        {
          onSuccess: (created) => {
            toast.success(t('message.split_done', { number: created.number }));
            router.push(`/conversation/${created.uuid}`);
          },
          onError: fail,
        },
      );
  };
  return (
    <>
      <ActionMenu
        title={t('message.title')}
        maxWidth={maxWidth}
        alignment={alignment}
        trigger={cloneElement(trigger as ReactElement<PressableProps>, {
          accessibilityActions: [{ name: 'menu', label: t('message.title') }],
          onAccessibilityAction: () => {
            // Native menu actions also remain available in the VoiceOver rotor.
            const flat = options.flatMap((o) => o.children ?? [o]);
            const show = (start: number) => {
              const slice = Platform.OS === 'android' ? flat.slice(start, start + 1) : flat;
              Alert.alert(t('message.title'), undefined, [
                ...slice.map((o) => ({
                  text: o.label,
                  style: o.destructive ? ('destructive' as const) : ('default' as const),
                  onPress: () => {
                    if (!o.disabled) pick(o.value);
                  },
                })),
                ...(Platform.OS === 'android' && start + 1 < flat.length
                  ? [{ text: t('row.more'), onPress: () => show(start + 1) }]
                  : []),
                { text: t('common:cancel'), style: 'cancel' },
              ]);
            };
            show(0);
          },
        })}
        context={context}
        preview={preview}
        options={options}
        onPick={pick}
      />
      <IssueSheet
        conversation={conversation}
        visible={!!issueFrom}
        initial={issueFrom ? { description: issueFrom.content } : undefined}
        onClose={() => setIssueFrom(null)}
      />
    </>
  );
}

/** New engineering issue linked to this conversation; "AI summary" lets the API draft it. */
export function IssueSheet({
  conversation,
  visible,
  initial,
  onClose,
}: {
  conversation: string;
  visible: boolean;
  initial?: { title?: string; description?: string };
  onClose: () => void;
}) {
  const { t } = useTranslation('conversation');
  const create = useCreateIssue();
  const [title, setTitle] = useState('');
  const [description, setDescription] = useState('');
  const [ai, setAi] = useState(true);
  const [priority, setPriority] = useState<IssueCreate['priority']>('normal');
  const [wasVisible, setWasVisible] = useState(visible);
  if (visible !== wasVisible) {
    setWasVisible(visible);
    if (visible) {
      setTitle(initial?.title ?? '');
      setDescription(initial?.description ?? '');
      setAi(!initial?.description);
    }
  }
  const submit = () =>
    create.mutate(
      {
        conversation_uuid: conversation,
        ai_summary: ai,
        priority,
        ...(title.trim() ? { title: title.trim() } : {}),
        ...(description.trim() ? { description: description.trim() } : {}),
      },
      {
        onSuccess: (issue) => {
          onClose();
          toast.success(t('issue.created'));
          router.push(`/engineering/${issue.uuid}`);
        },
        onError: fail,
      },
    );
  return (
    <FormSheet
      visible={visible}
      title={t('menu.issue')}
      onCancel={onClose}
      dirty={!!title || !!description}
      detents={['large']}
      primary={{ label: t('issue.create'), loading: create.isPending, disabled: !ai && !title.trim(), onPress: submit }}
      sections={[
        {
          fields: [
            {
              key: 'ai',
              kind: 'toggle',
              label: t('issue.ai_summary'),
              hint: t('issue.ai_summary_hint'),
              value: ai,
              onChange: setAi,
            },
          ],
        },
        {
          fields: [
            {
              key: 'title',
              kind: 'text',
              label: t('issue.title'),
              value: title,
              onChange: setTitle,
              placeholder: ai ? t('issue.ai_placeholder') : undefined,
            },
            {
              key: 'description',
              kind: 'text',
              label: t('issue.description'),
              value: description,
              onChange: setDescription,
              multiline: true,
            },
            {
              key: 'priority',
              kind: 'pick',
              label: t('issue.priority'),
              value: priority ?? 'normal',
              onChange: (v) => setPriority(v as IssueCreate['priority']),
              options: ['urgent', 'high', 'normal', 'low'].map((value) => ({
                value,
                label: t(`inbox:priority.${value}`),
              })),
            },
          ],
        },
      ]}
    />
  );
}
