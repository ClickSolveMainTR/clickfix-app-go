"use client";

import { NotificationLiveHost } from "@/features/notifications/components/notification-live-host";
import { useNotificationLiveToasts } from "@/features/notifications/hooks/use-notification-live-toasts";

/**
 * Listens for live notifications and renders the corner cards. Own
 * notifications are open to every signed-in user (organization staff carry
 * no platform role).
 */
export function NotificationLiveBridge() {
  useNotificationLiveToasts(true);
  return <NotificationLiveHost />;
}
