import { captureScreen } from 'react-native-view-shot';
import { create } from 'zustand';

import { readJSON, writeJSON } from '@/core/storage';

/** Shake to send feedback is on unless the member turned it off on this phone. */
const PREF_KEY = 'pref.feedback';
const saved = readJSON<{ shake: boolean }>(PREF_KEY);

type Feedback = {
  shake: boolean;
  open: boolean;
  /** Local file of the screen as it was when feedback started, if one was taken. */
  screenshot: string | null;
  /** Route pattern of that screen (`/conversation/[uuid]`). */
  route: string;
};

export const useFeedback = create<Feedback>(() => ({
  shake: saved?.shake ?? true,
  open: false,
  screenshot: null,
  route: '',
}));

export function setShakeEnabled(shake: boolean) {
  useFeedback.setState({ shake });
  writeJSON(PREF_KEY, { shake });
}

/** Grabs the whole screen as a temporary JPEG; null when the platform refuses. */
async function screenshot(): Promise<string | null> {
  try {
    return await captureScreen({ format: 'jpg', quality: 0.7, result: 'tmpfile' });
  } catch {
    return null;
  }
}

/**
 * Opens the feedback sheet. With `capture` the current screen is taken
 * first (shake, header menu), so the sheet never appears in its own picture.
 */
export async function openFeedback({ route, capture }: { route: string; capture: boolean }) {
  if (useFeedback.getState().open) return;
  const shot = capture ? await screenshot() : null;
  useFeedback.setState({ open: true, screenshot: shot, route });
}

export function closeFeedback() {
  useFeedback.setState({ open: false, screenshot: null });
}
