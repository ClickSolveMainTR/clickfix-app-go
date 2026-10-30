import { CameraView, useCameraPermissions } from 'expo-camera';
import * as Haptics from 'expo-haptics';
import { type Href, router, useIsFocused } from 'expo-router';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Linking, View } from 'react-native';

import { EmptyState } from '@/ui/empty-state';
import { Text } from '@/ui/text';
import { appRoute } from '@/features/push/links';

/**
 * Scans the QR on the web panel's sign-in page (`{panel}/login/qr/{id}`) and
 * opens the approval screen (ported from olexfilms-ecosystem
 * `mobile/src/features/scan/scan-screen.tsx`). The camera runs only while the
 * screen is focused; any other code shows a hint instead.
 */
export function QrScanScreen() {
  const { t } = useTranslation('qr');
  const focused = useIsFocused();
  const [permission, requestPermission] = useCameraPermissions();
  const [unknown, setUnknown] = useState(false);
  const busy = useRef(false);

  if (!permission) return <View className="flex-1 bg-ink" />;

  if (!permission.granted) {
    return (
      <View className="flex-1 justify-center bg-background">
        <EmptyState
          icon="scan"
          title={t('permission_title')}
          body={t('permission_body')}
          action={
            permission.canAskAgain
              ? { title: t('permission_button'), onPress: () => void requestPermission() }
              : { title: t('settings:push_open_settings'), onPress: () => void Linking.openSettings() }
          }
        />
      </View>
    );
  }

  const onScanned = ({ data }: { data: string }) => {
    if (busy.current) return;
    const route = appRoute(data);
    if (!route?.startsWith('/qr-login/')) {
      setUnknown(true);
      return;
    }
    busy.current = true;
    void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
    router.replace(route as Href);
  };

  return (
    <View className="flex-1 bg-ink">
      {focused ? (
        <CameraView
          style={{ flex: 1 }}
          facing="back"
          barcodeScannerSettings={{ barcodeTypes: ['qr'] }}
          onBarcodeScanned={onScanned}
        />
      ) : null}
      <View pointerEvents="none" className="absolute inset-0 items-center justify-center px-8">
        <View className="size-64 rounded-2xl border-[3px] border-signal" />
        <Text variant="callout" className="mt-5 text-center text-on-tile/90">
          {unknown ? t('scan_unknown') : t('scan_hint')}
        </Text>
      </View>
    </View>
  );
}
