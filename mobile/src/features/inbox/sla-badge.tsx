import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { cn } from '@/ui/cn';
import { Icon } from '@/ui/icon';
import { Text } from '@/ui/text';

import { formatSlaTime, slaBadge } from './helpers';

/** Re-renders every interval so countdowns stay fresh. */
export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

/**
 * The nearest SLA deadline (panel `SlaBadge`, CF-46): amber when it is
 * close, red once it passed, nothing for paused or finished clocks.
 */
export function SlaBadge({ conversation }: { conversation: Parameters<typeof slaBadge>[0] }) {
  const { t, i18n } = useTranslation('sla');
  const now = useNow();
  const b = slaBadge(conversation, now);
  if (!b) return null;
  const time = formatSlaTime(b.ms / 1000, i18n.language);
  const label = `${t(`metric.${b.metric}`)}: ${b.tone === 'overdue' ? t('badge.overdue', { time }) : t('badge.left', { time })}`;
  return (
    <View
      accessibilityLabel={label}
      className={cn(
        'shrink-0 flex-row items-center gap-0.5 rounded-md px-1.5 py-0.5',
        b.tone === 'overdue' ? 'bg-destructive/10' : b.tone === 'soon' ? 'bg-warning/15' : 'bg-muted',
      )}>
      <Icon
        name={b.tone === 'overdue' ? 'timerOff' : 'timer'}
        size={12}
        tone={b.tone === 'overdue' ? 'destructive' : b.tone === 'soon' ? 'warning' : 'muted-foreground'}
      />
      <Text
        variant="caption"
        tone={b.tone === 'overdue' ? 'danger' : b.tone === 'soon' ? 'default' : 'muted'}
        className="font-sans-semibold tabular-nums">
        {t(b.tone === 'overdue' ? 'badge.overdue' : 'badge.left', { time })}
      </Text>
    </View>
  );
}
