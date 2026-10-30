import { router, useLocalSearchParams } from 'expo-router';
import { useEffect } from 'react';

import { PRESENCE_STATUSES, type PresenceStatus, useSetPresence } from '@/features/presence/presence';

/**
 * `clickfix:///presence?set={status}`: the Android Presence widget and the iOS
 * 17 intents that open the app change the status here, then step back.
 */
export default function PresenceLinkScreen() {
  const { set } = useLocalSearchParams<{ set?: string }>();
  const setPresence = useSetPresence();
  const { mutate } = setPresence;
  useEffect(() => {
    if (set && (PRESENCE_STATUSES as string[]).includes(set)) mutate(set as PresenceStatus);
    if (router.canGoBack()) router.back();
    else router.replace('/');
  }, [set, mutate]);
  return null;
}
