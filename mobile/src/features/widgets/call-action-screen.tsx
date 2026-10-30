import { router, useLocalSearchParams } from 'expo-router';
import { useEffect } from 'react';

import { runCallAction } from './call-controls';

/** `clickfix:///call/{end|mute}` from the call Live Activity: run it and step back to the call. */
export default function CallActionScreen() {
  const { action } = useLocalSearchParams<{ action: string }>();
  useEffect(() => {
    runCallAction(action ?? '');
    if (router.canGoBack()) router.back();
    else router.replace('/');
  }, [action]);
  return null;
}
