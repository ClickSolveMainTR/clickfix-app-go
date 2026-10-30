// Service worker behaviour (public/app-push-sw.js, CF-157) under node --test:
// the worker runs in a vm context with stubbed self / clients / fetch.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const source = readFileSync(
  new URL("../../../public/app-push-sw.js", import.meta.url),
  "utf8",
);
const ORIGIN = "https://getclickfix.com";
const ID = "2f10a4fa-6dfe-44f9-995b-f346b98a674a";

function load({ windows = [] } = {}) {
  const listeners = {};
  const shown = [];
  const fetches = [];
  const opened = [];
  const self = {
    location: { origin: ORIGIN },
    Notification: { maxActions: 2 },
    registration: {
      showNotification: (title, options) => {
        shown.push({ title, options });
        return Promise.resolve();
      },
    },
    addEventListener: (name, fn) => {
      listeners[name] = fn;
    },
  };
  const clients = {
    matchAll: () => Promise.resolve(windows),
    openWindow: (url) => {
      opened.push(url);
      return Promise.resolve();
    },
  };
  const fetch = (url, init) => {
    fetches.push({ url, init });
    return Promise.resolve({ ok: true });
  };
  vm.runInNewContext(source, {
    self,
    clients,
    fetch,
    URL,
    Set,
    Array,
    Promise,
  });
  async function dispatch(name, event) {
    let pending;
    listeners[name]({ ...event, waitUntil: (p) => (pending = p) });
    await pending;
  }
  return { dispatch, shown, fetches, opened };
}

function pushEvent(payload) {
  return { data: { json: () => payload, text: () => JSON.stringify(payload) } };
}

function clickEvent(data, action = "") {
  return { action, notification: { data, close: () => undefined } };
}

test("shows the safe text with tag and known actions only", async () => {
  const sw = load();
  await sw.dispatch(
    "push",
    pushEvent({
      title: "Yeni mesaj",
      body: "WhatsApp · #42",
      data: {
        notification_uuid: ID,
        tag: `notification:${ID}`,
        action_url: "/inbox?c=x",
        actions: [
          { action: "open", title: "Aç" },
          { action: "read", title: "Okundu" },
          { action: "delete", title: "Sil" },
        ],
      },
    }),
  );
  const [n] = sw.shown;
  assert.equal(n.title, "Yeni mesaj");
  assert.equal(n.options.body, "WhatsApp · #42");
  assert.equal(n.options.tag, `notification:${ID}`);
  assert.deepEqual(
    n.options.actions.map((a) => a.action),
    ["open", "read"],
  );
});

test("old payloads without actions still show", async () => {
  const sw = load();
  await sw.dispatch("push", pushEvent({ title: "Bildirim", body: "x" }));
  assert.equal(sw.shown[0].title, "Bildirim");
  assert.deepEqual(Array.from(sw.shown[0].options.actions), []);
});

test("read marks the notification read and opens nothing", async () => {
  const sw = load();
  await sw.dispatch(
    "notificationclick",
    clickEvent({ notification_uuid: ID, action_url: "/inbox?c=x" }, "read"),
  );
  assert.equal(sw.opened.length, 0);
  assert.equal(sw.fetches.length, 1);
  assert.equal(sw.fetches[0].url, `/api/v1/notifications/${ID}/read`);
  assert.equal(sw.fetches[0].init.method, "POST");
  assert.equal(sw.fetches[0].init.credentials, "same-origin");
});

test("a click navigates an open app tab to the target", async () => {
  const navigated = [];
  const tab = {
    url: `${ORIGIN}/dashboard`,
    navigate: (url) => {
      navigated.push(url);
      return Promise.resolve(tab);
    },
    focus: () => Promise.resolve(tab),
  };
  const sw = load({ windows: [tab] });
  await sw.dispatch(
    "notificationclick",
    clickEvent({ notification_uuid: ID, action_url: "/inbox?c=x" }, "open"),
  );
  assert.deepEqual(navigated, [`${ORIGIN}/inbox?c=x`]);
  assert.equal(sw.opened.length, 0);
  assert.equal(sw.fetches.length, 1, "opening also marks read");
});

test("without a tab a new window opens; unsafe urls fall back", async () => {
  const sw = load();
  await sw.dispatch(
    "notificationclick",
    clickEvent({
      notification_uuid: "not-a-uuid",
      action_url: "//evil.test/x",
    }),
  );
  assert.deepEqual(sw.opened, [`${ORIGIN}/platform/notifications`]);
  assert.equal(sw.fetches.length, 0, "no read call without a valid id");
});
