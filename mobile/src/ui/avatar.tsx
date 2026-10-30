import { View } from 'react-native';

import { cn } from './cn';
import { Text } from './text';

/** Two-letter initials for avatars (Turkish-aware upper case). */
export function initialsOf(name: string | null | undefined): string {
  const parts = (name ?? '').trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return '?';
  const first = parts[0]?.[0] ?? '';
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? '') : '';
  return (first + last).toLocaleUpperCase('tr');
}

/** Initials in a circle; `dot` adds a presence / state dot (a class like `bg-success`). */
export function Avatar({
  name,
  size = 40,
  dot,
  className,
}: {
  name: string | null | undefined;
  size?: number;
  dot?: string;
  className?: string;
}) {
  return (
    <View
      importantForAccessibility="no-hide-descendants"
      accessibilityElementsHidden
      className={cn('items-center justify-center rounded-full bg-secondary', className)}
      style={{ width: size, height: size }}>
      <Text variant={size >= 48 ? 'headline' : 'footnote'} className="font-sans-semibold">
        {initialsOf(name)}
      </Text>
      {dot ? (
        <View
          className={cn('absolute -bottom-0.5 -right-0.5 rounded-full border-2 border-card', dot)}
          style={{ width: Math.max(10, size / 4), height: Math.max(10, size / 4) }}
        />
      ) : null}
    </View>
  );
}
