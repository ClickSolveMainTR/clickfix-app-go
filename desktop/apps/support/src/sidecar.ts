import { invoke, isTauri } from "@tauri-apps/api/core";

export interface SidecarInfo {
  version: string;
  mode: string;
  go: string;
  os: string;
  arch: string;
}

/** Sidecar sürüm bilgisini getirir; Tauri dışında (tarayıcıda) null döner. */
export async function sidecarVersion(): Promise<SidecarInfo | null> {
  if (!isTauri()) return null;
  return invoke<SidecarInfo>("sidecar_version");
}
