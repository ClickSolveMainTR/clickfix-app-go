import { useState } from 'react';
import * as Haptics from 'expo-haptics';
import { useTranslation } from 'react-i18next';
import { Platform, Pressable, View } from 'react-native';

import { isApiError } from '@/core/api/errors';
import { useSession } from '@/core/auth/session';
import { formatDate, formatShort, formatMessageTime } from '@/core/format';
import { errorMessage } from '@/core/i18n';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';
import { toast } from '@/ui/toast';

import { type OptimisticMessage, useInboxScope, useRetryMessage } from '../api';
import { MessageAttachments } from '../attachments';
import { MessageMenu } from '../conversation-tools';
import { eventText } from '../helpers';
import { discardFailed, requeueFailed } from '../outbox';
import { relativeDay, type TimelineMark } from '../timeline';
import { MessageBody } from './message-body';

export const MessageBubble = Platform.OS === 'ios' ? View : Pressable;

const dayCache = new Map<string, string>();
export function dayKey(iso: string) {
  let day = dayCache.get(iso);
  if (day === undefined) {
    if (dayCache.size > 2000) dayCache.clear();
    day = formatDate(iso);
    dayCache.set(iso, day);
  }
  return day;
}

function DaySeparator({ iso }: { iso: string }) {
  const { t } = useTranslation('conversation');
  const now = new Date();
  const rel = relativeDay(
    dayKey(iso),
    dayKey(now.toISOString()),
    dayKey(new Date(now.getTime() - 86_400_000).toISOString()),
  );
  return (
    <View accessibilityRole="header" className="items-center py-3">
      <View className="rounded-full bg-secondary px-3 py-1">
        <Text variant="caption" uppercase={false} tone="muted" className="font-sans-medium">
          {rel ? t(`timeline.${rel}`) : formatDate(iso)}
        </Text>
      </View>
    </View>
  );
}

/** Delivery is conveyed by shape as well as color, and named for screen readers. */
function Delivery({ status, queued }: { status: string; queued?: boolean }) {
  const { t } = useTranslation('inbox');
  const waiting = queued || status === 'queued' || status === 'sending';
  const double = status === 'delivered' || status === 'read';
  const tone = status === 'read' ? 'signal-text' : status === 'failed' ? 'destructive' : 'muted-foreground';
  return (
    <View
      accessible
      accessibilityLabel={waiting ? t('conversation:queued') : t(`delivery.${status}`)}
      style={{ width: double ? 22 : 15, height: 15 }}>
      <Icon name={waiting ? 'clock' : status === 'failed' ? 'error' : 'check'} size={15} tone={tone} />
      {double ? (
        <View className="absolute left-[7px] top-0">
          <Icon name="check" size={15} tone={tone} />
        </View>
      ) : null}
    </View>
  );
}

export function MessageRow({
  message: m,
  mark,
  conversationUuid,
  maxWidth,
  eventCount = 1,
}: {
  message: OptimisticMessage;
  mark: TimelineMark | undefined;
  conversationUuid: string;
  maxWidth: number;
  eventCount?: number;
}) {
  const { t } = useTranslation('inbox');
  const { org } = useInboxScope();
  const me = useSession((s) => s.me?.user?.uuid);
  const retry = useRetryMessage(conversationUuid);
  const [bubbleWidth, setBubbleWidth] = useState(maxWidth);
  const day = mark?.dayStart ? <DaySeparator iso={m.created_at} /> : null;

  if (m.kind === 'event') {
    const event = eventText(m.attributes, (key, params) => t(key, params));
    const label =
      eventCount > 1
        ? m.attributes?.event === 'merged_from'
          ? t('conversation:events.merged_count', { count: eventCount })
          : t('conversation:events.repeated', { event, count: eventCount })
        : event;
    return (
      <View>
        {day}
        <View className="flex-row items-center justify-center gap-2 px-4 py-2.5">
          <View className="h-px w-4 bg-border" />
          <Text variant="caption" uppercase={false} tone="muted" className="shrink text-center leading-[16px]">
            {label} · {formatMessageTime(m.created_at)}
          </Text>
          <View className="h-px w-4 bg-border" />
        </View>
      </View>
    );
  }

  const note = m.private || m.direction === 'internal';
  const inbound = !note && m.direction === 'in';
  const failed = m.status === 'failed';
  const body = m.kind === 'unsupported' ? t('timeline.unsupported') : m.content;
  const failedNote = failed && m.optimistic && m.error && m.error !== 'not_sent' ? errorMessage(m.error) : null;
  const runEnd = mark?.runEnd ?? true;
  const showMeta = runEnd || failed || m.queued;
  const sender = inbound || (m.sender.uuid && m.sender.uuid !== me) ? m.sender.name : null;
  const a11y = [
    note ? t('timeline.private_note') : sender || t('list.you'),
    body,
    m.attachments.length ? t('conversation:timeline.attachments', { count: m.attachments.length }) : null,
    formatShort(m.created_at),
    !inbound && !note ? (m.queued ? t('conversation:queued') : t(`delivery.${m.status}`)) : null,
    failedNote,
  ]
    .filter(Boolean)
    .join(', ');
  const retryMessage = failed
    ? () => {
        void Haptics.selectionAsync();
        if (m.optimistic) {
          if (m.client_id) requeueFailed(org, conversationUuid, m.client_id);
        } else
          retry.mutate(m.uuid, {
            onError: (e) => toast.error(errorMessage(isApiError(e) ? e.code : undefined)),
          });
      }
    : undefined;
  const bubbleClass = messageBubbleClass({ inbound, note, runEnd, failed });
  const menuProps = {
    conversation: conversationUuid,
    message: m,
    onRetry: retryMessage,
    onDiscard:
      failed && m.optimistic && m.client_id ? () => discardFailed(org, conversationUuid, m.client_id!) : undefined,
    retrying: retry.isPending,
  };

  return (
    <View className={mark?.runStart === false ? 'pt-0.5' : 'pt-2.5'}>
      {day}
      <View
        style={{ maxWidth, alignSelf: inbound ? 'flex-start' : 'flex-end', flexShrink: 1 }}
        className="min-w-0 gap-1">
        {note && mark?.runStart !== false ? (
          <View className="flex-row items-center gap-1 self-end px-1">
            <Icon name="lockClosed" size={11} tone="warning" />
            <Text variant="caption" uppercase={false} tone="muted" className="shrink font-sans-medium">
              {[t('conversation:timeline.note_label'), m.sender.name].filter(Boolean).join(' · ')}
            </Text>
          </View>
        ) : null}
        <MessageMenu
          {...menuProps}
          context
          preview={
            <View
              className={cn(bubbleClass, 'overflow-hidden')}
              style={{ width: Math.min(bubbleWidth, maxWidth), maxHeight: 320, borderCurve: 'continuous' }}>
              {body ? <MessageBody body={body} preview /> : null}
              {failedNote ? (
                <Text variant="footnote" tone="danger">
                  {failedNote}
                </Text>
              ) : null}
              {m.attachments.length ? <MessageAttachments items={m.attachments} onPrimary={false} /> : null}
            </View>
          }
          maxWidth={maxWidth}
          alignment={inbound ? 'leading' : 'trailing'}
          trigger={
            <MessageBubble
              accessible
              accessibilityLabel={a11y}
              onLayout={({ nativeEvent }) => setBubbleWidth(nativeEvent.layout.width)}
              className={bubbleClass}
              style={{
                maxWidth,
                alignSelf: inbound ? 'flex-start' : 'flex-end',
                flexShrink: 1,
                borderCurve: 'continuous',
              }}>
              {body ? <MessageBody key={m.client_id ?? m.uuid} body={body} /> : null}
              {failedNote ? (
                <Text variant="footnote" tone="danger" className="shrink pt-1">
                  {failedNote}
                </Text>
              ) : null}
              {m.attachments.length > 0 ? <MessageAttachments items={m.attachments} onPrimary={false} /> : null}
            </MessageBubble>
          }
        />
        {showMeta ? (
          <View className={cn('flex-row items-center gap-1 px-1 pb-0.5', inbound ? 'self-start' : 'self-end')}>
            <Text variant="caption" uppercase={false} tone="muted" className="shrink leading-[15px] tabular-nums">
              {[sender, formatMessageTime(m.created_at)].filter(Boolean).join(' · ')}
            </Text>
            {failed ? (
              <MessageMenu
                {...menuProps}
                trigger={
                  <Pressable accessibilityRole="button" accessibilityLabel={t('delivery.failed')} hitSlop={12}>
                    <Delivery status="failed" />
                  </Pressable>
                }
              />
            ) : !inbound && !note ? (
              <Delivery status={m.status} queued={m.queued} />
            ) : null}
          </View>
        ) : null}
      </View>
    </View>
  );
}

export function messageBubbleClass({
  inbound = false,
  note = false,
  runEnd = true,
  failed = false,
}: {
  inbound?: boolean;
  note?: boolean;
  runEnd?: boolean;
  failed?: boolean;
}) {
  return cn(
    'min-w-0 shrink rounded-[18px] px-3.5 py-2.5',
    inbound && 'border border-border bg-card',
    !inbound && !note && 'border border-success/20 bg-signal-soft',
    note && 'border border-dashed border-warning/60 bg-warning/10',
    runEnd && inbound && 'rounded-bl-md',
    runEnd && !inbound && 'rounded-br-md',
    failed && 'border border-destructive/60',
  );
}
