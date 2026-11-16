"use client";

import { realtimeConfig } from "@/config/realtime";
import { isLiveSurfaceChannel } from "@/features/notifications/constants";
import { useNotificationRealtimeInvalidate } from "@/features/notifications/hooks/use-notification-realtime";
import {
  enrichLiveNotification,
  liveNotificationFromMessage,
} from "@/features/notifications/lib/live-notification";
import { useNotificationLiveStore } from "@/features/notifications/lib/notification-live-store";
import { useRealtimeEvent } from "@/hooks/use-realtime";
import { notify as desktopNotify } from "@/lib/desktop/bridge";
import { RealtimeEvents } from "@/lib/realtime/events";

const LIVE_EVENT_TYPES = new Set<string>([
  RealtimeEvents.NotificationItemCreated,
]);

/**
 * Centrifugo live notifications land in the corner host.
 * Inbox / table stay in-app; realtime is the on-screen channel. In
 * ClickFix Masaüstü the same notification also becomes a native one.
 */
export function useNotificationLiveToasts(enabled = true) {
  const active = enabled && realtimeConfig.enabled;
  const push = useNotificationLiveStore((state) => state.push);

  useNotificationRealtimeInvalidate(enabled);

  useRealtimeEvent(
    "*",
    (message) => {
      if (!LIVE_EVENT_TYPES.has(message.type)) return;
      const incoming = liveNotificationFromMessage(message);
      if (!incoming || !isLiveSurfaceChannel(incoming.channel)) return;

      void (async () => {
        const notification = await enrichLiveNotification(incoming);
        push(notification);
        // ClickFix Masaüstü: native notification while the window is in the
        // background; a no-op in the browser (CF-236).
        void desktopNotify({
          title: notification.title,
          body: notification.body,
          path: notification.action_url,
        });
      })();
    },
    { enabled: active },
  );
}
