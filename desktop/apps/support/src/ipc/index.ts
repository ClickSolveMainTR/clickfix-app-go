import { invoke, isTauri } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

import { createMockIpc, type MockControls } from "./mock";
import type { EventName, Events, Ipc, IpcError } from "./types";
import { IpcCallError } from "./types";

export * from "./types";

/** Rust kabuğunun sidecar olaylarını ilettiği Tauri olayı. */
const SIDECAR_EVENT = "support://sidecar";

interface RawEvent {
  event: string;
  params: unknown;
}

function toIpcError(err: unknown): IpcCallError {
  if (err && typeof err === "object" && "code" in err && "message" in err) {
    return new IpcCallError(err as IpcError);
  }
  return new IpcCallError({ code: "IPC", message: String(err) });
}

/** Rust `sidecar_call` komutu üzerinden gerçek sidecar. */
function createSidecarIpc(): Ipc {
  const listeners = new Map<string, Set<(p: unknown) => void>>();
  void listen<RawEvent>(SIDECAR_EVENT, (e) => {
    for (const cb of listeners.get(e.payload.event) ?? []) cb(e.payload.params);
  });
  return {
    kind: "sidecar",
    async call(method, params) {
      try {
        return await invoke("sidecar_call", { method, params });
      } catch (err) {
        throw toIpcError(err);
      }
    },
    on<E extends EventName>(event: E, cb: (p: Events[E]) => void) {
      let set = listeners.get(event);
      if (!set) listeners.set(event, (set = new Set()));
      const fn = cb as (p: unknown) => void;
      set.add(fn);
      return () => set.delete(fn);
    },
  };
}

/**
 * Sahte IPC ne zaman kullanılır:
 * - Tauri dışında (tarayıcı önizlemesi) her zaman;
 * - geliştirme derlemesinde `VITE_SUPPORT_IPC` `sidecar` değilse (CF-238
 *   birleşene kadar sidecar bu yöntemleri bilmiyor).
 * Sürüm derlemesi her zaman gerçek sidecar'ı kullanır.
 */
export function mockSelected(): boolean {
  if (!isTauri()) return true;
  return import.meta.env.DEV && import.meta.env.VITE_SUPPORT_IPC !== "sidecar";
}

let instance: (Ipc & { controls?: MockControls }) | null = null;

export function ipc(): Ipc & { controls?: MockControls } {
  instance ??= mockSelected() ? createMockIpc() : createSidecarIpc();
  return instance;
}
