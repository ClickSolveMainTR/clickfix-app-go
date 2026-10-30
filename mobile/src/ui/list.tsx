import { useCSSVariable } from 'uniwind';
import type { IconName } from '@/ui/icons';
import { Pressable, StyleSheet, View } from 'react-native';
import Animated from 'react-native-reanimated';

import { cn } from './cn';
import { Icon } from './icon';
import { useRiseIn } from './motion';
import { Text } from './text';

/** Grouped list container (inset, rounded, hairline separators). */
export function List({ children, className }: { children: React.ReactNode; className?: string }) {
  const entering = useRiseIn();
  return (
    <Animated.View entering={entering} className={cn('w-full min-w-0 overflow-hidden rounded-lg bg-card', className)}>
      {children}
    </Animated.View>
  );
}

type RowProps = {
  title: string;
  subtitle?: React.ReactNode;
  titleLines?: number;
  leading?: React.ReactNode;
  /** Concept icon shown at the leading side when no `leading` is passed. */
  icon?: IconName;
  trailing?: React.ReactNode;
  iconTile?: boolean;
  iconTint?: string;
  separatorInset?: number;
  onPress?: () => void;
  disabled?: boolean;
  /** Last row has no separator. */
  last?: boolean;
  tone?: 'default' | 'danger';
};

export function ListRow({
  title,
  subtitle,
  titleLines = 1,
  leading,
  icon,
  trailing,
  onPress,
  last,
  tone = 'default',
  disabled,
  iconTile,
  iconTint,
  separatorInset,
}: RowProps) {
  const body = (
    <View className="w-full min-w-0 min-h-12 flex-row items-center gap-3 px-4 py-3">
      {leading ??
        (icon ? (
          iconTile ? (
            <IconTile name={icon} tint={iconTint} danger={tone === 'danger'} />
          ) : (
            <Icon name={icon} tone={tone === 'danger' ? 'destructive' : 'primary'} size={20} />
          )
        ) : null)}
      <View className="min-w-0 flex-1 gap-0.5">
        <Text variant="body" tone={tone === 'danger' ? 'danger' : 'default'} numberOfLines={titleLines}>
          {title}
        </Text>
        {typeof subtitle !== 'string' ? (
          subtitle
        ) : subtitle ? (
          <Text variant="footnote" tone="muted" numberOfLines={2}>
            {subtitle}
          </Text>
        ) : null}
      </View>
      {!last ? (
        <View
          pointerEvents="none"
          className="absolute bottom-0 right-0 bg-border"
          style={{
            height: StyleSheet.hairlineWidth,
            left: separatorInset ?? (leading ? 64 : icon ? (iconTile ? 57 : 48) : 16),
          }}
        />
      ) : null}
      {trailing}
      {onPress && !trailing ? <Icon name="forward" tone="muted-foreground" size={18} /> : null}
    </View>
  );
  if (!onPress) return body;
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: !!disabled }}
      disabled={disabled}
      onPress={onPress}
      className="w-full min-w-0 active:bg-muted"
      style={{ opacity: disabled ? 0.4 : 1 }}>
      {body}
    </Pressable>
  );
}

export function SectionHeader({ title, className }: { title: string; className?: string }) {
  return (
    <Text
      accessibilityRole="header"
      variant="footnote"
      uppercase={false}
      tone="muted"
      className={cn('px-4 pb-1.5 pt-5', className)}>
      {title}
    </Text>
  );
}

const ICON_TINT: Partial<Record<IconName, string>> = {
  search: 'blue',
  calendar: 'red',
  video: 'green',
  customers: 'indigo',
  chat: 'green',
  alert: 'orange',
  chart: 'blue',
  bug: 'purple',
  megaphone: 'orange',
  book: 'orange',
  channel: 'blue',
  staff: 'indigo',
  sparkles: 'purple',
  wallet: 'green',
  settings: 'gray',
  scan: 'gray',
  building: 'indigo',
  user: 'blue',
  security: 'green',
  receipt: 'gray',
  lockClosed: 'blue',
  signOut: 'orange',
  trash: 'red',
  mail: 'blue',
  phone: 'green',
};

export function IconTile({ name, tint, danger }: { name: IconName; tint?: string; danger?: boolean }) {
  const [fill, onTile] = useCSSVariable([`--color-tile-${danger ? 'red' : ICON_TINT[name] ?? 'blue'}`, '--color-on-tile']);
  return (
    <View
      style={{
        width: 29,
        height: 29,
        borderRadius: 7,
        backgroundColor: tint ?? (typeof fill === 'string' ? fill : undefined),
        alignItems: 'center',
        justifyContent: 'center',
      }}>
      <Icon name={name} size={18} color={typeof onTile === 'string' ? onTile : undefined} weight="medium" />
    </View>
  );
}
