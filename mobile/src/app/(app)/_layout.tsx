import { HeaderCloseButton } from '@/ui/header-button';
import { LARGE_TITLES } from '@/features/shell/large-titles';
import { useQuery } from '@tanstack/react-query';
import { Keyboard } from 'react-native';
import { Stack } from 'expo-router';
import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';

import { useSession } from '@/core/auth/session';
import { startOutbox } from '@/features/inbox/outbox';

/**
 * Signed-in shell. The cached session renders first; /me refreshes it in the
 * background (launch and foreground). Inbox screens (`(org)`) need an active
 * organization; until one is picked only the picker is reachable. QR sign-in
 * approval works without one (it signs in the user, not an organization).
 */
export default function AppLayout() {
  const { t } = useTranslation();
  const hasOrganization = useSession((s) => !!s.organization);

  useQuery({
    queryKey: ['me'],
    queryFn: () => useSession.getState().refreshMe(),
    staleTime: 60_000,
    refetchOnWindowFocus: true,
  });

  // Replies queued offline (or before the app was closed) go out once signed in and online.
  useEffect(() => startOutbox(), []);

  return (
    <Stack
      screenListeners={{ blur: Keyboard.dismiss, transitionStart: Keyboard.dismiss }}
      screenOptions={{ headerBackTitle: t('common:back'), headerBackButtonDisplayMode: 'minimal', headerLargeTitle: LARGE_TITLES, headerShadowVisible: false }}>
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Protected guard={hasOrganization}>
        <Stack.Screen name="(org)" options={{ headerShown: false }} />
      </Stack.Protected>
      <Stack.Screen
        name="organizations"
        options={{
          title: t('auth:pick_org_title'),
          presentation: hasOrganization ? 'modal' : 'card',
          headerShown: hasOrganization,
          headerLargeTitle: false,
          headerLeft: hasOrganization ? () => <HeaderCloseButton /> : undefined,
        }}
      />
      <Stack.Screen name="qr-scan" options={{ title: t('qr:scan_title'), presentation: 'modal', headerLargeTitle: false, headerLeft: () => <HeaderCloseButton /> }} />
      <Stack.Screen name="qr-login/[id]" options={{ title: t('qr:title'), presentation: 'modal', headerLargeTitle: false, headerLeft: () => <HeaderCloseButton /> }} />
      <Stack.Screen name="delete-account" options={{ title: t('settings:delete_account') }} />
    </Stack>
  );
}
