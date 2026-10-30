import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Pressable, View } from 'react-native';
import Animated, { FadeInDown, FadeOutDown } from 'react-native-reanimated';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { create } from 'zustand';

import { Text } from './text';

type Bar = { id: number; message: string; onUndo: () => void };

const useBar = create<{ current: Bar | null }>(() => ({ current: null }));
let seq = 0;

/** "Done · Undo" bar above the tab bar (Gmail / Front after a bulk action); 6 s, then gone. */
export const undoBar = {
  show: (message: string, onUndo: () => void) => useBar.setState({ current: { id: ++seq, message, onUndo } }),
  hide: () => useBar.setState({ current: null }),
};

export function UndoBar() {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const current = useBar((s) => s.current);
  useEffect(() => {
    if (!current) return;
    const id = setTimeout(() => useBar.setState((s) => (s.current?.id === current.id ? { current: null } : s)), 6000);
    return () => clearTimeout(id);
  }, [current]);
  if (!current) return null;
  return (
    <Animated.View
      key={current.id}
      entering={FadeInDown}
      exiting={FadeOutDown}
      className="absolute inset-x-4"
      style={{ bottom: insets.bottom + 64 }}>
      <View
        accessibilityLiveRegion="polite"
        className="flex-row items-center gap-3 rounded-xl bg-ink px-4 py-3"
        style={{ borderCurve: 'continuous', boxShadow: '0 6px 20px rgba(2, 6, 24, 0.2)' }}>
        <Text variant="callout" className="flex-1 font-sans-medium text-ink-foreground">
          {current.message}
        </Text>
        <Pressable
          accessibilityRole="button"
          hitSlop={10}
          onPress={() => {
            undoBar.hide();
            current.onUndo();
          }}>
          <Text variant="callout" className="font-sans-bold text-signal">
            {t('undo')}
          </Text>
        </Pressable>
      </View>
    </Animated.View>
  );
}
