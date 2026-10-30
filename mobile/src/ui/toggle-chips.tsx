import * as Haptics from 'expo-haptics';
import { Pressable, View } from 'react-native';

import { cn } from './cn';
import { Icon } from './icon';
import { Text } from './text';

/** Multi-choice chips that wrap (filters with several values, e.g. priorities, tags). */
export function ToggleChips<T extends string>({
  options,
  values,
  onToggle,
}: {
  options: { value: T; label: string }[];
  values: T[];
  onToggle: (v: T) => void;
}) {
  return (
    <View className="flex-row flex-wrap gap-2">
      {options.map((o) => {
        const on = values.includes(o.value);
        return (
          <Pressable
            key={o.value}
            accessibilityRole="checkbox"
            accessibilityState={{ checked: on }}
            onPress={() => {
              void Haptics.selectionAsync();
              onToggle(o.value);
            }}
            className={cn(
              'flex-row items-center gap-1 rounded-full border px-3 py-1.5',
              on ? 'border-primary bg-primary' : 'border-border bg-card',
            )}>
            {on ? <Icon name="check" size={13} tone="primary-foreground" /> : null}
            <Text
              variant="footnote"
              className={cn('font-sans-semibold', on ? 'text-primary-foreground' : 'text-foreground')}>
              {o.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}
