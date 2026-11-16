import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { transform } from "esbuild";

const { code } = await transform(
  await readFile(new URL("./bridge.ts", import.meta.url), "utf8"),
  { loader: "ts", format: "esm" },
);
const bridge = await import(
  `data:text/javascript;base64,${Buffer.from(code).toString("base64")}`
);

/** A fake window: records invoke calls and DOM listeners. */
function fakeWindow({ info, invoke } = {}) {
  const calls = [];
  const listeners = new Map();
  const w = {
    addEventListener(type, fn) {
      listeners.set(type, [...(listeners.get(type) ?? []), fn]);
    },
    removeEventListener(type, fn) {
      listeners.set(
        type,
        (listeners.get(type) ?? []).filter((f) => f !== fn),
      );
    },
    dispatch(type, detail) {
      for (const fn of listeners.get(type) ?? []) fn({ detail });
    },
  };
  if (info !== undefined) w.__CLICKFIX_DESKTOP__ = info;
  if (invoke !== undefined) {
    w.__TAURI_INTERNALS__ = {
      invoke: async (cmd, args) => {
        calls.push([cmd, args]);
        return invoke(cmd, args);
      },
    };
  }
  return { w, calls, listeners };
}

function withWindow(w, fn) {
  const had = "window" in globalThis;
  const prev = globalThis.window;
  globalThis.window = w;
  return Promise.resolve()
    .then(fn)
    .finally(() => {
      if (had) globalThis.window = prev;
      else delete globalThis.window;
    });
}

test("no-op without a window (server render)", async () => {
  assert.equal(bridge.isDesktop(), false);
  assert.equal(bridge.desktopInfo(), null);
  assert.equal(await bridge.notify({ title: "x" }), false);
  assert.equal(await bridge.version(), null);
  assert.equal(await bridge.openExternal("https://example.com"), false);
  await bridge.setBadge(3);
  await bridge.setPresence("busy");
  await bridge.signOut();
  const off = bridge.onDesktopCommand(() => {});
  off();
});

test("no-op in a normal browser, even with Tauri internals but no ClickFix flag", async () => {
  for (const env of [
    {},
    { invoke: () => true },
    { info: { bridge: 2, version: "9.0.0" }, invoke: () => true },
    { info: { bridge: 1, version: "0.1.0" } },
  ]) {
    const { w, calls, listeners } = fakeWindow(env);
    await withWindow(w, async () => {
      assert.equal(bridge.isDesktop(), false);
      assert.equal(await bridge.notify({ title: "Yeni mesaj" }), false);
      await bridge.setBadge(5);
      await bridge.setPresence("online");
      await bridge.signOut();
      assert.equal(await bridge.openExternal("https://example.com"), false);
      bridge.onDesktopCommand(() => assert.fail("no events outside desktop"));
      w.dispatch(bridge.DESKTOP_EVENT, { type: "command-palette" });
    });
    assert.deepEqual(calls, []);
    assert.equal(listeners.size, 0);
  }
});

test("calls the app commands on the desktop", async () => {
  const { w, calls } = fakeWindow({
    info: { bridge: 1, version: "0.1.0", os: "macos" },
    invoke: (cmd) =>
      cmd === "notify"
        ? true
        : cmd === "version"
          ? { bridge: 1, version: "0.1.0", os: "macos" }
          : null,
  });
  await withWindow(w, async () => {
    assert.equal(bridge.isDesktop(), true);
    assert.deepEqual(bridge.desktopInfo(), {
      bridge: 1,
      version: "0.1.0",
      os: "macos",
    });
    assert.equal(
      await bridge.notify({
        title: " Yeni mesaj ",
        body: "Deniz Kaya",
        path: "/inbox?c=1",
      }),
      true,
    );
    await bridge.notify({ title: "x", path: "//evil.test" });
    assert.equal(await bridge.notify({ title: "  " }), false);
    await bridge.setBadge(12.7);
    await bridge.setBadge(-1);
    await bridge.setBadge(1e9);
    await bridge.setPresence("busy");
    await bridge.setPresence("dancing");
    assert.equal(await bridge.openExternal("https://example.com/a"), true);
    assert.deepEqual(await bridge.version(), {
      bridge: 1,
      version: "0.1.0",
      os: "macos",
    });
    await bridge.signOut();
  });
  assert.deepEqual(calls, [
    ["notify", { title: "Yeni mesaj", body: "Deniz Kaya", path: "/inbox?c=1" }],
    ["notify", { title: "x", body: null, path: null }],
    ["set_badge", { count: 12 }],
    ["set_badge", { count: 0 }],
    ["set_badge", { count: 9999 }],
    ["set_presence", { status: "busy" }],
    ["set_presence", { status: null }],
    ["open_external", { url: "https://example.com/a" }],
    ["version", undefined],
    ["sign_out", undefined],
  ]);
});

test("app errors never reach the web app", async () => {
  const { w } = fakeWindow({
    info: { bridge: 1, version: "0.1.0" },
    invoke: () => {
      throw new Error("izin yok");
    },
  });
  await withWindow(w, async () => {
    assert.equal(await bridge.notify({ title: "x" }), false);
    assert.equal(await bridge.version(), null);
    await bridge.setBadge(1);
  });
});

test("desktop commands are validated", async () => {
  const { w, listeners } = fakeWindow({
    info: { bridge: 1, version: "0.1.0" },
    invoke: () => null,
  });
  const seen = [];
  await withWindow(w, () => {
    const off = bridge.onDesktopCommand((c) => seen.push(c));
    w.dispatch(bridge.DESKTOP_EVENT, { type: "command-palette" });
    w.dispatch(bridge.DESKTOP_EVENT, { type: "presence", status: "away" });
    w.dispatch(bridge.DESKTOP_EVENT, { type: "presence", status: "x" });
    w.dispatch(bridge.DESKTOP_EVENT, { type: "sign-out" });
    w.dispatch(bridge.DESKTOP_EVENT, { type: "eval", code: "1" });
    w.dispatch(bridge.DESKTOP_EVENT, null);
    off();
    w.dispatch(bridge.DESKTOP_EVENT, { type: "command-palette" });
  });
  assert.deepEqual(seen, [
    { type: "command-palette" },
    { type: "presence", status: "away" },
    { type: "sign-out" },
  ]);
  assert.equal(listeners.get(bridge.DESKTOP_EVENT).length, 0);
});

test("desktopPath keeps in-app paths only", () => {
  assert.equal(bridge.desktopPath("/inbox?c=1"), "/inbox?c=1");
  assert.equal(bridge.desktopPath(" /a "), "/a");
  for (const bad of [
    "//evil.test",
    "https://evil.test",
    "/\\evil.test",
    "",
    null,
    undefined,
    `/${"a".repeat(2048)}`,
  ]) {
    assert.equal(bridge.desktopPath(bad), null, String(bad));
  }
});
