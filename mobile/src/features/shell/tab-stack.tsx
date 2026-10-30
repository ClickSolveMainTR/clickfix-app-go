import { HelpMenuButton } from '@/features/guides/help-menu';
import { Stack } from 'expo-router';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Keyboard } from 'react-native';
import { LARGE_TITLES } from './large-titles';
import { HeaderCollapsedContext, HeaderScrollContext } from './header-scroll';

/** A single native bar preserves glass controls, safe areas and swipe navigation. */
export function TabStack() {
  const { t } = useTranslation();
  const [collapsed, setCollapsed] = useState(false);
  return (
    <HeaderCollapsedContext value={collapsed}>
      <HeaderScrollContext value={(offset) => setCollapsed(offset > 24)}>
        <Stack
          screenListeners={{ blur: Keyboard.dismiss, transitionStart: Keyboard.dismiss }}
          screenOptions={{
            headerLargeTitle: LARGE_TITLES,
            headerBackTitle: t('hub:more.title'),
            headerBackButtonDisplayMode: 'minimal',
            headerShadowVisible: true,
            headerRight: () => <HelpMenuButton />,
          }}>
          <Stack.Screen name="index" options={{ headerLargeTitle: false }} />
        </Stack>
      </HeaderScrollContext>
    </HeaderCollapsedContext>
  );
}
