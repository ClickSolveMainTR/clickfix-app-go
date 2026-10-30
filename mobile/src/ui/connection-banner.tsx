import { useTranslation } from 'react-i18next';
import { View } from 'react-native';
import Animated, { FadeIn, FadeOut } from 'react-native-reanimated';

import { useConnection } from '@/core/network';

import { cn } from './cn';
import { Icon } from './icon';
import { Text } from './text';

/**
 * One strip that says why data may be stale: the phone is offline, or it is
 * online but the server does not answer (learned from real requests, never
 * from the realtime socket). Nothing shows while all is well (CF-79).
 */
export function ConnectionBanner({ offlineText, className }: { offlineText: string; className?: string }) {
  const { t } = useTranslation();
  const state = useConnection();
  if (state === 'ok') return null;
  return (
    <Animated.View entering={FadeIn} exiting={FadeOut}>
      <View
        accessibilityRole="alert"
        accessibilityLiveRegion="polite"
        className={cn('flex-row items-center gap-2 bg-warning/15 px-3 py-2', className)}>
        <Icon name="offline" size={16} tone="warning" />
        <Text variant="footnote" className="flex-1">
          {state === 'offline' ? offlineText : t('common:unreachable_banner')}
        </Text>
      </View>
    </Animated.View>
  );
}
