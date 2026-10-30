import type { components } from "@/generated/api";
import { apiClient, unwrap } from "@/lib/api";

export type NotificationPreferences =
  components["schemas"]["NotificationPreferences"];
export type NotificationPreferencesUpdate =
  components["schemas"]["NotificationPreferencesUpdate"];
export type NotificationEventType =
  components["schemas"]["NotificationEventType"];

export const notificationPreferencesService = {
  async get() {
    return unwrap<NotificationPreferences>(
      await apiClient.GET("/v1/notification-preferences"),
    );
  },

  /** Merge update: omitted fields keep their stored value. */
  async update(body: NotificationPreferencesUpdate) {
    return unwrap<NotificationPreferences>(
      await apiClient.PUT("/v1/notification-preferences", { body }),
    );
  },

  async events() {
    return unwrap<{ items: NotificationEventType[]; channels: string[] }>(
      await apiClient.GET("/v1/notification-preferences/events"),
    );
  },
};
