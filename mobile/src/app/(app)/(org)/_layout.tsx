import { HeaderCloseButton } from '@/ui/header-button';
import { ShakeToFeedback } from '@/features/feedback/shake';
import { HelpMenuButton } from '@/features/guides/help-menu';
import { GuideHost } from '@/features/guides/host';
import { LARGE_TITLES } from '@/features/shell/large-titles';
import { Keyboard } from 'react-native';
import { Stack } from 'expo-router';
import { useTranslation } from 'react-i18next';

import { useMyPresence } from '@/features/presence/presence';
import { useWidgetSurfaces } from '@/features/widgets/use-widget-surfaces';

/** Screens of the active organization: the tabs plus the records they open (pushed over the tabs). */
export default function OrgLayout() {
  const { t } = useTranslation();
  // Keeps the agent present (routing, team status) while the app is open.
  useMyPresence();
  // Home / Lock Screen widgets, Live Activities and Siri follow this organization.
  useWidgetSurfaces();
  return (
    <>
      <Stack
        screenListeners={{ blur: Keyboard.dismiss, transitionStart: Keyboard.dismiss }}
        screenOptions={{
          headerBackTitle: t('common:back'),
          headerLargeTitle: LARGE_TITLES,
          headerBackButtonDisplayMode: 'minimal',
          headerShadowVisible: false,
          headerRight: () => <HelpMenuButton />,
        }}>
        <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
        <Stack.Screen name="conversation/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="conversation-details/[uuid]" options={{ title: '', presentation: 'modal', headerLargeTitle: false, headerLeft: () => <HeaderCloseButton /> }} />
        <Stack.Screen name="appointment/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="video/[uuid]" options={{ headerShown: false, presentation: 'fullScreenModal' }} />
        <Stack.Screen name="contact/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="company/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="article/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="knowledge/[collection]" options={{ title: '' }} />
        <Stack.Screen name="search" options={{ title: '' }} />
        <Stack.Screen name="side-conversation/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="team-chat/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="engineering/[uuid]" options={{ title: '' }} />
        <Stack.Screen name="assistant" options={{ title: '' }} />
        <Stack.Screen name="setup-guide" options={{ title: '' }} />
        <Stack.Screen name="new-conversation" options={{ headerShown: false }} />
        <Stack.Screen name="presence" options={{ headerShown: false, presentation: 'transparentModal' }} />
        <Stack.Screen name="call/[action]" options={{ headerShown: false, presentation: 'transparentModal' }} />
      </Stack>
      <GuideHost />
      <ShakeToFeedback />
    </>
  );
}
