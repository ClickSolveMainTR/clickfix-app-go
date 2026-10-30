import { router } from 'expo-router';
import { useTranslation } from 'react-i18next';
import * as Haptics from 'expo-haptics';
import { Pressable } from 'react-native';

import { useGuideTarget } from './guide-target';
import { Icon } from './icon';
import type { IconName } from './icons';

/**
 * Icon action in a native header (`headerRight`): 44 pt target, read by
 * screen readers. `guide` registers it as a guide target (CF-142).
 */
export function HeaderButton({
  icon,
  label,
  onPress,
  guide,
  disabled = false,
}: {
  icon: IconName;
  label: string;
  onPress?: () => void;
  guide?: string;
  disabled?: boolean;
}) {
  const ref = useGuideTarget(guide ?? '', !!guide);
  return (
    <Pressable
      ref={ref}
      accessibilityRole="button"
      accessibilityLabel={label}
      disabled={disabled}
      accessibilityState={{ disabled }}
      style={{ opacity: disabled ? 0.4 : 1 }}
      hitSlop={8}
      onPressIn={() => void Haptics.selectionAsync()}
      onPress={onPress}
      className="size-9 items-center justify-center active:opacity-60">
      <Icon name={icon} tone="primary" size={22} weight="medium" />
    </Pressable>
  );
}

/** Same close affordance for modal routes; pushed routes retain UIKit's back button. */
export function HeaderCloseButton() {
  const { t } = useTranslation();
  return <HeaderButton icon="close" label={t('close')} onPress={() => {
    if (router.canGoBack()) router.back();
    else router.replace('/');
  }} />;
}
