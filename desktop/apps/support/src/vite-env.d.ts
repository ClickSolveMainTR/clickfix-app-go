/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** `sidecar`: geliştirme derlemesinde de gerçek sidecar'ı kullan. */
  readonly VITE_SUPPORT_IPC?: string;
}
