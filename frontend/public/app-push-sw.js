/* global self, clients */
/**
 * Web Push service worker. The backend sends lock-screen-safe text and, in
 * `data` (notifications/usecase/push.go WebPushContent, CF-157):
 * notification_uuid, action_url (app path), tag, category and up to two
 * actions: "open" and "read". A click (or "open") opens action_url in an
 * existing tab when there is one and marks the notification read; "read"
 * only marks it read.
 */
const FALLBACK_URL = "/platform/notifications";
const ACTIONS = new Set(["open", "read"]);
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function readPayload(event) {
  try {
    const payload = event.data ? event.data.json() : {};
    return {
      title: payload.title || "ClickFix",
      body: payload.body || "",
      data:
        payload.data && typeof payload.data === "object" ? payload.data : {},
    };
  } catch {
    return {
      title: "ClickFix",
      body: event.data ? event.data.text() : "",
      data: {},
    };
  }
}

/** Only the known actions with a title; the browser shows at most maxActions. */
function notificationActions(data) {
  if (!Array.isArray(data.actions)) return [];
  const max = (self.Notification && self.Notification.maxActions) || 2;
  return data.actions
    .filter(
      (a) =>
        a &&
        ACTIONS.has(a.action) &&
        typeof a.title === "string" &&
        a.title.trim() !== "",
    )
    .slice(0, max)
    .map((a) => ({ action: a.action, title: a.title }));
}

function resolveNotificationUrl(data) {
  if (
    data &&
    typeof data.action_url === "string" &&
    data.action_url.startsWith("/") &&
    !data.action_url.startsWith("//")
  ) {
    return data.action_url;
  }
  return FALLBACK_URL;
}

/** Marks the notification read through the same-origin API (session cookie). */
function markRead(data) {
  const id = data && data.notification_uuid;
  if (typeof id !== "string" || !UUID.test(id)) return Promise.resolve();
  return fetch(`/api/v1/notifications/${id}/read`, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  }).catch(() => undefined);
}

/** Opens url in an existing app tab (navigated there) or a new one. */
function openUrl(url) {
  const target = new URL(url, self.location.origin).href;
  return clients
    .matchAll({ type: "window", includeUncontrolled: true })
    .then((windowClients) => {
      const same = windowClients.filter(
        (c) => new URL(c.url).origin === self.location.origin,
      );
      const exact = same.find((c) => c.url === target);
      if (exact && "focus" in exact) return exact.focus();
      const tab = same[0];
      if (tab && "navigate" in tab) {
        return tab
          .navigate(target)
          .then((c) => (c || tab).focus())
          .catch(() =>
            clients.openWindow ? clients.openWindow(target) : undefined,
          );
      }
      return clients.openWindow ? clients.openWindow(target) : undefined;
    });
}

self.addEventListener("push", (event) => {
  const { title, body, data } = readPayload(event);
  const options = { body, data, actions: notificationActions(data) };
  if (typeof data.tag === "string" && data.tag) options.tag = data.tag;
  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const data = event.notification.data || {};
  if (event.action === "read") {
    event.waitUntil(markRead(data));
    return;
  }
  event.waitUntil(
    Promise.all([openUrl(resolveNotificationUrl(data)), markRead(data)]),
  );
});
