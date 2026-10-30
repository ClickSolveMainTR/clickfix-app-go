import '@/global.css';

import { PersistQueryClientProvider } from '@tanstack/react-query-persist-client';
import { useFonts } from 'expo-font';
import { Stack, ThemeProvider } from 'expo-router';
import * as SplashScreen from 'expo-splash-screen';
import { StatusBar } from 'expo-status-bar';
import { useEffect } from 'react';
import { I18nextProvider } from 'react-i18next';
import { GestureHandlerRootView } from 'react-native-gesture-handler';
import { KeyboardProvider } from 'react-native-keyboard-controller';

import { useSession } from '@/core/auth/session';
import { FONTS } from '@/core/fonts';
import { i18n } from '@/core/i18n';
import { startNetworkWatch } from '@/core/network';
import { applyThemePreference } from '@/core/preferences';
import { persistOptions, queryClient } from '@/core/query';
import { startUpdateChecks } from '@/core/updates';
import { PushNotifications } from '@/features/push/push-notifications';
import { LockScreen } from '@/features/security/lock-screen';
import { useNavigationTheme } from '@/features/shell/navigation-theme';
import { ErrorFallback } from '@/features/shell/error-fallback';
import { LaunchVeil } from '@/features/shell/launch-veil';
import { UpdateBanner } from '@/features/shell/update-banner';
import { Toaster } from '@/ui/toast';
import { UndoBar } from '@/ui/undo-bar';

// Keep the splash only until the stored session is read (local, no network wait).
void SplashScreen.preventAutoHideAsync();
applyThemePreference();
startNetworkWatch();

// A render error must never reach React Native's fatal handler (release builds abort on it):
// each screen falls back on its own so navigation still works, and the root catches the rest.
export const ErrorBoundary = ErrorFallback;
export const unstable_settings = { screenErrorBoundary: ErrorFallback };

export default function RootLayout() {
  const status = useSession((s) => s.status);
  const theme = useNavigationTheme();
  // Bundled with the app, so loading is local; on failure the system font stands in.
  const [fontsLoaded, fontError] = useFonts(FONTS);
  const fontsReady = fontsLoaded || !!fontError;

  useEffect(() => {
    // The stored sign-in is read before the splash hides, so the first screen is the right one.
    void useSession.getState().hydrate();
  }, []);

  // OTA: check on launch and each foreground (throttled); no-op in dev builds and Expo Go.
  useEffect(() => startUpdateChecks(), []);

  useEffect(() => {
    if (status !== 'loading' && fontsReady) void SplashScreen.hideAsync();
  }, [status, fontsReady]);

  if (status === 'loading' || !fontsReady) return null;

  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <KeyboardProvider>
        <I18nextProvider i18n={i18n}>
          <PersistQueryClientProvider client={queryClient} persistOptions={persistOptions}>
            <ThemeProvider value={theme}>
              <StatusBar style="auto" />
              <Stack screenOptions={{ headerShown: false }}>
                <Stack.Protected guard={status === 'signedIn'}>
                  <Stack.Screen name="(app)" />
                </Stack.Protected>
                <Stack.Protected guard={status === 'signedOut'}>
                  <Stack.Screen name="(public)" />
                </Stack.Protected>
                {/* Video join links (CF-139) work signed in or out. */}
                <Stack.Screen name="r/[code]" options={{ headerShown: true, title: '', presentation: 'fullScreenModal' }} />
              </Stack>
              <UpdateBanner />
              <PushNotifications />
              <UndoBar />
              <Toaster />
              <LockScreen />
              <LaunchVeil />
            </ThemeProvider>
          </PersistQueryClientProvider>
        </I18nextProvider>
      </KeyboardProvider>
    </GestureHandlerRootView>
  );
}
