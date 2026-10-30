import { View } from 'react-native';

import { cn } from './cn';
import { Icon } from './icon';
import type { IconName } from './icons';
import { Text } from './text';

export type StatusTone = 'neutral' | 'progress' | 'done' | 'problem';

const tones: Record<StatusTone, { box: string; dot: string }> = {
  neutral: { box: 'bg-secondary', dot: 'bg-muted-foreground' },
  progress: { box: 'bg-warning/15', dot: 'bg-warning' },
  done: { box: 'bg-success/15', dot: 'bg-success' },
  problem: { box: 'bg-destructive/15', dot: 'bg-destructive' },
};

/** State always as text plus color: never color alone. */
export function StatusPill({
  label,
  tone = 'neutral',
  icon,
}: {
  label: string;
  tone?: StatusTone;
  icon?: IconName;
}) {
  return (
    <View
      className={cn('flex-row items-center gap-1.5 self-start rounded-full px-2.5 py-1', tones[tone].box)}>
      <Icon
        name={
          icon ??
          (tone === 'done'
            ? 'accept'
            : tone === 'problem'
              ? 'error'
              : tone === 'progress'
                ? 'clock'
                : 'statusOpen')
        }
        size={13}
        tone={
          tone === 'done'
            ? 'success'
            : tone === 'problem'
              ? 'destructive'
              : tone === 'progress'
                ? 'warning'
                : 'muted-foreground'
        }
      />
      <Text variant="footnote" className="font-sans-semibold">
        {label}
      </Text>
    </View>
  );
}
