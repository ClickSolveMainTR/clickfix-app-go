import * as Haptics from 'expo-haptics';
import { useEffect, useRef, useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { type AccessibilityActionEvent, Alert, Platform, PlatformColor, Pressable, View } from 'react-native';
import ReanimatedSwipeable, { type SwipeableMethods } from 'react-native-gesture-handler/ReanimatedSwipeable';
import Animated, {
  ReduceMotion,
  useAnimatedStyle,
  useSharedValue,
  withSpring,
  type SharedValue,
} from 'react-native-reanimated';

import { formatShort, formatRelative } from '@/core/format';
import { ChannelLogo, hasChannelLogo } from '@/ui/brand-logo';
import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import type { IconName } from '@/ui/icons';
import { markdownToText } from '@/ui/markdown-core';
import { ActionMenu } from '@/ui/menu';
import type { MenuOption } from '@/ui/menu.types';
import { Text } from '@/ui/text';

import { type Conversation, useMarkRead } from './api';
import { contactLabel, initials, SNOOZE_PRESETS, snoozeUntil, type SnoozePreset } from './helpers';
import { SlaBadge } from './sla-badge';

export type RowAction = 'resolve' | 'reopen' | 'snooze' | 'take' | 'read' | 'open';
type SwipeActionName = Exclude<RowAction, 'open'>;

export function rowActions(
  c: Conversation,
  me: string | undefined,
  can: { manage: boolean; assign: boolean },
): RowAction[] {
  const closed = c.status === 'resolved' || c.status === 'closed';
  const out: RowAction[] = [];
  if (can.assign && me && c.assignee?.uuid !== me) out.push('take');
  if (c.unread_count > 0) out.push('read');
  if (can.manage) out.push(closed ? 'reopen' : 'resolve');
  if (can.manage && !closed && c.status !== 'snoozed') out.push('snooze');
  return out;
}

const ACTION_STYLE: Record<SwipeActionName, { icon: IconName; bg: string; fg: string }> = {
  take: { icon: 'personAdd', bg: 'bg-signal', fg: 'text-signal-foreground' },
  read: { icon: 'confirm', bg: 'bg-secondary', fg: 'text-secondary-foreground' },
  snooze: { icon: 'clock', bg: 'bg-warning/15', fg: 'text-foreground' },
  resolve: { icon: 'check', bg: 'bg-primary', fg: 'text-primary-foreground' },
  reopen: { icon: 'history', bg: 'bg-primary', fg: 'text-primary-foreground' },
};

export function useActionLabel() {
  const { t } = useTranslation('inbox');
  return (a: RowAction) =>
    a === 'take'
      ? t('shortcuts.take')
      : a === 'read'
        ? t('conversation:row.read')
        : a === 'snooze'
          ? t('header.snooze')
          : a === 'resolve'
            ? t('header.resolve')
            : a === 'reopen'
              ? t('header.reopen')
              : t('conversation:row.open');
}

const ACTION_WIDTH = 84;
function SwipeAction({
  action,
  label,
  height,
  onPress,
}: {
  action: SwipeActionName;
  label: string;
  height: number;
  onPress: () => void;
}) {
  const s = ACTION_STYLE[action];
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={label}
      onPress={onPress}
      style={{ width: ACTION_WIDTH, height }}
      className={cn('items-center justify-center gap-1.5 px-1 active:opacity-80', s.bg)}>
      <Icon
        name={s.icon}
        size={22}
        tone={
          action === 'take'
            ? 'signal-foreground'
            : action === 'snooze'
              ? 'warning'
              : action === 'read'
                ? 'secondary-foreground'
                : 'primary-foreground'
        }
      />
      <Text
        variant="caption"
        uppercase={false}
        className={cn('text-center font-sans-semibold leading-[15px]', s.fg)}
        numberOfLines={2}>
        {label}
      </Text>
    </Pressable>
  );
}

/** Each action panel moves in from its own edge; closed panels stay outside the row. */
function SwipePanel({
  drag,
  leading,
  count,
  children,
}: {
  drag: SharedValue<number>;
  leading?: boolean;
  count: number;
  children: React.ReactNode;
}) {
  const width = count * ACTION_WIDTH;
  const style = useAnimatedStyle(() => ({
    transform: [{ translateX: leading ? Math.min(0, drag.value - width) : Math.max(0, drag.value + width) }],
  }));
  return <Animated.View style={[{ width, height: '100%', flexDirection: 'row' }, style]}>{children}</Animated.View>;
}

function channelIcon(channel: string): IconName {
  if (channel === 'email') return 'mail';
  if (channel === 'sms' || channel === 'whatsapp') return 'sms';
  if (channel === 'voice') return 'phone';
  if (channel === 'instagram') return 'camera';
  if (channel === 'slack' || channel === 'teams' || channel === 'google_chat') return 'chat';
  if (channel === 'api') return 'channel';
  return 'chat';
}

export function ConversationRow({
  conversation: c,
  last,
  actions,
  onAction,
  onSnooze,
  selecting = false,
  selected = false,
  onSelect,
  active = false,
}: {
  conversation: Conversation;
  last: boolean;
  actions: RowAction[];
  onAction: (a: RowAction) => void;
  onSnooze?: (preset: SnoozePreset) => void;
  selecting?: boolean;
  selected?: boolean;
  onSelect?: () => void;
  active?: boolean;
}) {
  const { t } = useTranslation('inbox');
  const label = useActionLabel();
  const swipe = useRef<SwipeableMethods>(null);
  const progress = useSharedValue(selecting ? 1 : 0);
  useEffect(() => {
    if (selecting) swipe.current?.close();
    progress.value = withSpring(selecting ? 1 : 0, {
      damping: 22,
      stiffness: 240,
      reduceMotion: ReduceMotion.System,
    });
  }, [selecting, progress]);
  const selectionStyle = useAnimatedStyle(() => ({ width: 34 * progress.value }));
  const circleStyle = useAnimatedStyle(() => ({
    opacity: progress.value,
    transform: [{ translateX: -34 * (1 - progress.value) }],
  }));
  const select = () => {
    void Haptics.selectionAsync();
    onSelect?.();
  };
  const markRead = useMarkRead(c.uuid);
  const [height, setHeight] = useState(100);
  const unread = c.unread_count > 0;
  const name = contactLabel(c);
  const preview = markdownToText(c.search_match?.snippet || c.last_message_preview || c.subject)
    .replace(/\s+/g, ' ')
    .trim();
  const channel = t(`channels:types.${c.channel_type}`, { defaultValue: c.channel_type });
  const urgent = c.priority === 'urgent' || c.priority === 'high';
  const meta = [channel, c.assignee?.name ?? t('list.unassigned'), urgent ? t(`priority.${c.priority}`) : null]
    .filter(Boolean)
    .join(' · ');
  const left = actions.filter((a): a is SwipeActionName => a === 'take' || a === 'read');
  const right = actions.filter((a): a is SwipeActionName => a === 'snooze' || a === 'resolve' || a === 'reopen');
  const run = (a: RowAction) => {
    swipe.current?.close();
    if (a === 'read') markRead();
    else onAction(a);
  };
  const accessibleSnooze = (start = 0) => {
    const presets = SNOOZE_PRESETS.map((p) => ({
      text: `${t(`snooze.${p}`)} · ${formatShort(snoozeUntil(p).toISOString())}`,
      onPress: () => onSnooze?.(p),
    }));
    Alert.alert(label('snooze'), undefined, [
      ...(Platform.OS === 'android' ? presets.slice(start, start + 1) : presets),
      ...(Platform.OS === 'android' && start + 1 < presets.length
        ? [{ text: t('conversation:row.more'), onPress: () => accessibleSnooze(start + 1) }]
        : []),
      { text: t('common:cancel'), style: 'cancel' },
    ]);
  };
  const onA11y = (e: AccessibilityActionEvent) => {
    const n = e.nativeEvent.actionName;
    if (selecting) {
      if (n === 'activate') select();
      return;
    }
    if (n === 'activate') run('open');
    else if (n === 'select') select();
    else if (n === 'snooze' && onSnooze) accessibleSnooze();
    else if (actions.includes(n as RowAction)) run(n as RowAction);
  };
  const options: MenuOption<string>[] = [
    ...['open' as const, ...actions].map((a) => ({
      value: a,
      label: label(a),
      children:
        a === 'snooze' && onSnooze
          ? SNOOZE_PRESETS.map((p) => ({
              value: `snooze:${p}`,
              label: t(`snooze.${p}`),
              description: formatShort(snoozeUntil(p).toISOString()),
            }))
          : undefined,
    })),
    ...(onSelect ? [{ value: 'select', label: t('conversation:bulk.select') }] : []),
  ];
  const pick = (value: string) => {
    swipe.current?.close();
    if (value === 'select') select();
    else if (value.startsWith('snooze:')) onSnooze?.(value.slice(7) as SnoozePreset);
    else run(value as RowAction);
  };
  const button = (a: SwipeActionName) => (
    <SwipeAction action={a} label={label(a)} height={height} onPress={() => run(a)} />
  );
  const renderAction = (a: SwipeActionName) =>
    a === 'snooze' && onSnooze ? (
      <View key={a} style={{ width: ACTION_WIDTH, height }}>
        <ActionMenu
          title={label(a)}
          trigger={button(a)}
          options={SNOOZE_PRESETS.map((p) => ({
            value: p,
            label: t(`snooze.${p}`),
            description: formatShort(snoozeUntil(p).toISOString()),
          }))}
          onPick={(p) => {
            swipe.current?.close();
            onSnooze(p);
          }}
        />
      </View>
    ) : (
      <View key={a}>{button(a)}</View>
    );

  const row = (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={[
        name,
        preview,
        meta,
        formatRelative(c.last_message_at),
        unread ? t('conversation:row.unread', { count: c.unread_count }) : null,
      ]
        .filter(Boolean)
        .join(', ')}
      accessibilityHint={t(selecting ? 'conversation:row.select_hint' : 'conversation:row.hint')}
      accessibilityState={{ selected: selecting ? selected : active }}
      accessibilityActions={
        selecting
          ? [{ name: 'activate', label: t('conversation:bulk.select') }]
          : [
              { name: 'activate', label: label('open') },
              ...actions.map((a) => ({ name: a, label: label(a) })),
              ...(onSelect ? [{ name: 'select', label: t('conversation:bulk.select') }] : []),
            ]
      }
      onAccessibilityAction={onA11y}
      onLayout={(e) => setHeight(e.nativeEvent.layout.height)}
      onPress={() => {
        if (selecting) {
          select();
        } else run('open');
      }}
      onLongPress={
        selecting
          ? undefined
          : () => {
              void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
            }
      }
      delayLongPress={350}
      className={cn('w-full active:bg-muted', selected ? 'bg-primary/5' : active ? 'bg-secondary' : 'bg-card')}>
      <View className={cn('flex-row items-start px-3 py-3.5', !last && 'border-b border-border')}>
        {urgent ? (
          <View
            className={cn(
              'absolute inset-y-3 left-0 w-[3px] rounded-r-full',
              c.priority === 'urgent' ? 'bg-destructive' : 'bg-warning',
            )}
          />
        ) : null}
        <Animated.View
          style={selectionStyle}
          className="overflow-hidden"
          pointerEvents="none"
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants">
          <Animated.View style={circleStyle} className="w-[22px] pt-[11px]">
            <Icon
              name={selected ? 'selectionOn' : 'statusOpen'}
              size={22}
              tone={selected ? 'primary' : 'muted-foreground'}
              iosColor={selected ? undefined : PlatformColor('secondaryLabel')}
            />
          </Animated.View>
        </Animated.View>
        <View className="mr-3 size-11 items-center justify-center rounded-full bg-secondary">
          {name === '—' || name.startsWith('#') ? (
            <Icon name={channelIcon(c.channel_type)} size={21} tone="secondary-foreground" />
          ) : (
            <Text variant="callout" className="font-sans-semibold">
              {initials(name)}
            </Text>
          )}
          <View className="absolute -bottom-0.5 -right-1 size-[19px] items-center justify-center rounded-full border-2 border-card bg-secondary">
            {hasChannelLogo(c.channel_type) ? (
              <ChannelLogo type={c.channel_type} size={11} />
            ) : (
              <Icon name={channelIcon(c.channel_type)} size={11} tone="secondary-foreground" />
            )}
          </View>
        </View>
        <View className="min-w-0 flex-1 gap-1">
          <View className="flex-row items-center gap-2">
            <Text
              variant="callout"
              numberOfLines={1}
              className={cn('min-w-0 flex-1', unread ? 'font-sans-semibold' : 'font-sans-medium')}>
              {name}
            </Text>
            <View className="shrink-0 flex-row items-center gap-1">
              {c.status === 'snoozed' ? <Icon name="clock" size={12} tone="muted-foreground" /> : null}
              <Text variant="caption" uppercase={false} tone={unread ? 'default' : 'muted'} className="tabular-nums">
                {formatRelative(c.last_message_at)}
              </Text>
            </View>
          </View>
          <View className="flex-row items-center gap-2">
            <Text
              variant="footnote"
              numberOfLines={2}
              ellipsizeMode="tail"
              tone={unread ? 'default' : 'muted'}
              className="min-w-0 flex-1 leading-[19px]">
              {c.last_message_direction === 'out' ? `${t('list.you')}: ` : ''}
              {preview}
            </Text>
            {unread ? (
              <View className="min-w-5 items-center rounded-full bg-signal px-1.5 py-0.5">
                <Text variant="caption" className="font-sans-bold text-signal-foreground">
                  {c.unread_count > 99 ? '99+' : String(c.unread_count)}
                </Text>
              </View>
            ) : null}
          </View>
          <View className="flex-row items-center gap-2">
            <Text variant="caption" uppercase={false} tone="muted" numberOfLines={1} className="min-w-0 flex-1">
              {meta}
            </Text>
            <SlaBadge conversation={c} />
          </View>
        </View>
      </View>
    </Pressable>
  );
  const swipeable = (
    <ReanimatedSwipeable
      ref={swipe}
      enabled={!selecting}
      friction={1.7}
      containerStyle={{ width: '100%', minWidth: 0, overflow: 'hidden' }}
      childrenContainerStyle={{ width: '100%', backgroundColor: 'transparent' }}
      overshootLeft={false}
      overshootRight={false}
      leftThreshold={48}
      rightThreshold={48}
      onSwipeableWillOpen={() => void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light)}
      renderLeftActions={
        left.length
          ? (_progress, drag) => (
              <SwipePanel drag={drag} leading count={left.length}>
                {left.map(renderAction)}
              </SwipePanel>
            )
          : undefined
      }
      renderRightActions={
        right.length
          ? (_progress, drag) => (
              <SwipePanel drag={drag} count={right.length}>
                {right.map(renderAction)}
              </SwipePanel>
            )
          : undefined
      }>
      {selecting || Platform.OS === 'ios' ? (
        row
      ) : (
        <ActionMenu
          title={t('conversation:row.more')}
          trigger={row as ReactElement}
          options={options}
          context
          fullWidth
          onPick={pick}
        />
      )}
    </ReanimatedSwipeable>
  );
  // Keep SwiftUI hosting outside the moving row on iOS. Its hosted RN surface
  // stays in one coordinate space while Reanimated translates only the row.
  return Platform.OS === 'ios' ? (
    <ActionMenu
      title={t('conversation:row.more')}
      trigger={swipeable}
      disabled={selecting}
      options={options}
      context
      fullWidth
      onPick={pick}
    />
  ) : (
    swipeable
  );
}
