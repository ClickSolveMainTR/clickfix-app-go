// Rust kabuğunun pencere, izin ve derin bağlantı komutları. Tauri dışında
// (tarayıcı önizlemesi) hepsi zararsız no-op'tur.

import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

import type { StateEvent } from "./ipc";

export type BarAction = "stop" | "revoke" | "emergency";
export type PermissionPane = "screen_recording" | "accessibility";

/** Soğuk açılışta gelen `clickfix-support://join?t=...` token'ı (bir kez). */
export async function takeJoinToken(): Promise<string | null> {
  if (!isTauri()) {
    return new URLSearchParams(location.search).get("t");
  }
  return invoke<string | null>("take_join_token");
}

/** Uygulama açıkken gelen derin bağlantılar. */
export function onJoinToken(cb: (token: string) => void): () => void {
  if (!isTauri()) return () => {};
  const off = listen<string>("support://join", (e) => cb(e.payload));
  return () => void off.then((f) => f());
}

/** Paylaşım çubuğundan ya da acil kısayoldan gelen istek. `handled` false ise
 * (sahte IPC) isteği bu pencere uygular. */
export function onBarAction(cb: (a: BarAction, handled: boolean) => void): () => void {
  if (!isTauri()) return () => {};
  const off = listen<{ action: BarAction; handled: boolean }>("support://bar-action", (e) =>
    cb(e.payload.action, e.payload.handled),
  );
  return () => void off.then((f) => f());
}

/** Sahte IPC'de çerçeve ve çubuk pencerelerini durumla sürer (yalnız geliştirme). */
export async function mockOverlay(state: StateEvent): Promise<void> {
  if (!isTauri()) return;
  await invoke("mock_overlay", { state }).catch(() => {});
}

export async function openPermissionSettings(pane: PermissionPane): Promise<void> {
  if (!isTauri()) return;
  await invoke("open_permission_settings", { pane });
}

export async function relaunch(): Promise<void> {
  if (!isTauri()) {
    location.reload();
    return;
  }
  await invoke("relaunch");
}

export async function quit(): Promise<void> {
  if (!isTauri()) {
    location.reload();
    return;
  }
  await invoke("quit");
}

export type HelpLink = "privacy" | "help" | "webview2";

export async function openLink(link: HelpLink): Promise<void> {
  if (!isTauri()) return;
  await invoke("open_link", { link });
}

// Paylaşım çubuğu penceresi

export async function barAction(action: BarAction): Promise<void> {
  if (!isTauri()) return;
  await invoke("bar_action", { action });
}

export async function overlayState(): Promise<StateEvent | null> {
  if (!isTauri()) return null;
  return invoke<StateEvent | null>("overlay_state");
}

export function onOverlayState(cb: (s: StateEvent) => void): () => void {
  if (!isTauri()) return () => {};
  const off = listen<StateEvent>("support://overlay", (e) => cb(e.payload));
  return () => void off.then((f) => f());
}
