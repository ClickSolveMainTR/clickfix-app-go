import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { View } from 'react-native';

import { useSession } from '@/core/auth/session';
import { BrandMark } from '@/ui/brand';
import { Button } from '@/ui/button';
import { Text } from '@/ui/text';

import { startAppLock, unlock, useAppLock } from './app-lock';

/**
 * Covers the whole app while the lock is on and not yet passed (CF-82). The
 * check starts by itself; "Sign out" is the way out for someone who cannot
 * pass it (the next sign-in needs the password anyway).
 */
export function LockScreen() {
  const { t } = useTranslation('security');
  const signedIn = useSession((s) => s.status === 'signedIn');
  const locked = useAppLock((s) => s.locked);
  const show = signedIn && locked;

  useEffect(() => startAppLock(), []);
  useEffect(() => {
    if (show) void unlock();
  }, [show]);

  if (!show) return null;
  return (
    <View
      className="absolute inset-0 items-center justify-center gap-6 bg-background px-8"
      accessibilityViewIsModal
      importantForAccessibility="yes">
      <BrandMark size={72} />
      <View className="items-center gap-2">
        <Text variant="title" className="text-center">
          {t('locked_title')}
        </Text>
        <Text tone="muted" className="text-center">
          {t('locked_body')}
        </Text>
      </View>
      <View className="w-full gap-3">
        <Button title={t('unlock')} icon="lockClosed" onPress={() => void unlock()} />
        <Button
          title={t('sign_out')}
          icon="signOut"
          variant="secondary"
          onPress={() => void useSession.getState().signOut()}
        />
      </View>
    </View>
  );
}
