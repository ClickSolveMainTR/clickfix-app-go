import { Linking } from 'react-native';
import { registerWidgetTaskHandler, type WidgetTaskHandlerProps } from 'react-native-android-widget';

import { api, setActiveOrganization, unwrap } from '@/core/api/client';
import { getTokens, loadTokens } from '@/core/auth/tokens';
import { storage } from '@/core/storage';

import type { PresenceStatus } from '../snapshot-core';
import { ANDROID_SNAPSHOT_KEY, readAndroidSnapshot } from '../sync';
import { ANDROID_WIDGETS } from './update';
import { renderInbox, renderPresence } from './widgets';

const STATUSES = new Set<PresenceStatus>(['online', 'away', 'busy']);

/**
 * Presence buttons run in a headless JS task in the app process, so they
 * use the app's own session (no separate widget token on Android); the
 * widget updates optimistically and settles on the server's answer.
 */
async function setPresence(status: PresenceStatus): Promise<boolean> {
  const snap = readAndroidSnapshot();
  if (!snap?.org) return false;
  if (!getTokens()) await loadTokens();
  if (!getTokens()) return false;
  setActiveOrganization(snap.org.slug);
  try {
    const me = await unwrap(
      api.PUT('/v1/tenant/presence/me', {
        params: { header: { 'X-Organization': snap.org.slug } },
        body: { status },
      }),
    );
    storage.set(ANDROID_SNAPSHOT_KEY, JSON.stringify({ ...snap, presence: me.status }));
    return true;
  } catch {
    return false;
  }
}

async function handler({
  widgetInfo,
  widgetAction,
  clickAction,
  clickActionData,
  renderWidget,
}: WidgetTaskHandlerProps) {
  const draw = () => {
    const snap = readAndroidSnapshot();
    renderWidget(
      widgetInfo.widgetName === ANDROID_WIDGETS.presence ? renderPresence(snap) : renderInbox(snap, widgetInfo.height),
    );
  };
  switch (widgetAction) {
    case 'WIDGET_ADDED':
    case 'WIDGET_UPDATE':
    case 'WIDGET_RESIZED':
      draw();
      return;
    case 'WIDGET_CLICK': {
      const status = clickActionData?.status as PresenceStatus | undefined;
      if (clickAction !== 'SET_PRESENCE' || !status || !STATUSES.has(status)) return;
      const snap = readAndroidSnapshot();
      if (snap) renderWidget(renderPresence({ ...snap, presence: status }));
      const ok = await setPresence(status);
      if (ok) draw();
      // Signed out on this phone or offline: the app finishes it.
      else await Linking.openURL(`clickfix:///presence?set=${status}`).catch(() => draw());
      return;
    }
    default:
      return;
  }
}

registerWidgetTaskHandler(handler);
