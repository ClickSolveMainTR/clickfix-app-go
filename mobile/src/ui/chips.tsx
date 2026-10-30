import type { ReactNode } from 'react';
import { Pressable, ScrollView, View } from 'react-native';

import { cn } from './cn';
import { Icon } from './icon';
import type { IconName } from './icons';
import { Text } from './text';

/** Single-choice filter chips in a horizontal row. */
export function Chips<T extends string>({
  options,
  value,
  onChange,
  trailing,
}: {
  options: { value: T; label: string; count?: number; icon?: IconName }[];
  trailing?: ReactNode;
  value: T;
  onChange: (v: T) => void;
}) {
  return (
    <ScrollView
      horizontal
      style={{ flexGrow: 0, minWidth: 0, width: '100%' }}
      showsHorizontalScrollIndicator={false}
      contentInsetAdjustmentBehavior="never"
      contentContainerClassName="items-center gap-2">
      {options.map((o) => {
        const on = o.value === value;
        return (
          <Pressable
            key={o.value}
            accessibilityRole="button"
            accessibilityState={{ selected: on }}
            onPress={() => onChange(o.value)}
            className={cn(
              'flex-row items-center gap-1.5 rounded-full border px-3 py-1.5',
              on ? 'border-primary bg-primary' : 'border-border bg-card',
            )}>
            {o.icon ? (
              <Icon name={o.icon} size={14} tone={on ? 'primary-foreground' : 'muted-foreground'} />
            ) : null}
            <Text
              variant="footnote"
              className={cn('font-sans-semibold', on ? 'text-primary-foreground' : 'text-foreground')}>
              {o.label}
            </Text>
            {o.count != null && o.count > 0 ? (
              <View
                className={cn(
                  'min-w-4 items-center rounded-full px-1 py-0.5',
                  on ? 'bg-primary-foreground/15' : 'bg-secondary',
                )}>
                <Text
                  variant="caption"
                  className={cn(
                    'font-sans-semibold tabular-nums',
                    on ? 'text-primary-foreground' : 'text-muted-foreground',
                  )}>
                  {o.count > 99 ? '99+' : o.count}
                </Text>
              </View>
            ) : null}
          </Pressable>
        );
      })}
      {trailing}
    </ScrollView>
  );
}
