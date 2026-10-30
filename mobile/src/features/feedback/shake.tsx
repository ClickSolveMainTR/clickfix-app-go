import * as Haptics from 'expo-haptics';
import { Accelerometer } from 'expo-sensors';
import { useEffect, useState } from 'react';
import { AppState } from 'react-native';

import { useHelpAvailability } from '@/features/guides/api';
import { useRoutePattern } from '@/features/guides/host';
import { useGuideTour } from '@/features/guides/store';
import { useAppLock } from '@/features/security/app-lock';

import { FeedbackSheet } from './feedback-sheet';
import { createShakeDetector } from './shake-core';
import { openFeedback, useFeedback } from './store';

const SAMPLE_MS = 100;

/** True while the app is in the foreground (the accelerometer is off otherwise). */
function useForeground() {
  const [active, setActive] = useState(AppState.currentState === 'active');
  useEffect(() => {
    const sub = AppState.addEventListener('change', (s) => setActive(s === 'active'));
    return () => sub.remove();
  }, []);
  return active;
}

/**
 * Shake to send feedback (CF-142): a shake takes a screenshot of the screen
 * and opens the feedback sheet. Listens only in the foreground, while the
 * setting is on, the server takes feedback, and no sheet, guide or app lock
 * is up. Mounted once in the organization shell, with the sheet.
 */
export function ShakeToFeedback() {
  const route = useRoutePattern();
  const enabled = useFeedback((s) => s.shake && !s.open);
  const available = useHelpAvailability((s) => s.feedback);
  const guiding = useGuideTour((s) => !!s.active);
  const locked = useAppLock((s) => s.locked);
  const foreground = useForeground();
  const listening = enabled && available && !guiding && !locked && foreground;

  useEffect(() => {
    if (!listening) return;
    const detect = createShakeDetector();
    let sub: { remove: () => void } | null = null;
    let cancelled = false;
    void Accelerometer.isAvailableAsync().then((ok) => {
      if (!ok || cancelled) return;
      Accelerometer.setUpdateInterval(SAMPLE_MS);
      sub = Accelerometer.addListener((s) => {
        if (!detect(s, Date.now())) return;
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Warning);
        void openFeedback({ route, capture: true });
      });
    });
    return () => {
      cancelled = true;
      sub?.remove();
    };
  }, [listening, route]);

  return <FeedbackSheet />;
}
