// ClickFix Destek arayüzü ile sidecar (`--mode=support`) arasındaki IPC
// sözleşmesi: `desktop/sidecar/IPC.md` (CF-238). Alanlar CF-233 public
// uçlarının (`backend/docs/openapi.yaml`, RemoteSupport*) müşteriye görünen
// kısmıdır. `client_secret`, LiveKit token'ı ve katılma token'ı sidecar'da
// kalır, arayüze hiç gelmez. Sözleşme yalnız geriye uyumlu genişler; bilinmeyen
// alanlar ve olaylar yok sayılır.

/** Oturum için istenen yetki. */
export type Mode = "view" | "control";

/** Backend `ended_reason` değerleri ve sidecar'ın `connection_lost`'u. */
export type EndedReason =
  | "agent"
  | "customer"
  | "declined"
  | "code_expired"
  | "timeout"
  | "attempts"
  | "max_duration"
  | "conversation_closed"
  | "platform"
  | "reported"
  | "connection_lost";

/** `state.session`: RemoteSupportResolved, sırsız. */
export interface ResolvedSession {
  uuid: string;
  organization: {
    uuid: string;
    name: string;
    logo_url: string | null;
    verified: boolean;
  };
  agent: {
    uuid: string;
    name: string;
    avatar_url: string | null;
  };
  mode_requested: Mode;
  /** Organizasyon kaydı açtıysa true; kayıt onayı yalnız o zaman sorulur. */
  recording_requested: boolean;
  max_duration_minutes: number;
  consent_deadline: string;
  warning: string;
  locale: string;
  /** Konuşma özeti (backend henüz döndürmüyor; gelirse gösterilir). */
  conversation?: { number?: string; subject?: string } | null;
  /** Kayıt saklama süresi (gün), gelirse gösterilir. */
  recording_retention_days?: number | null;
}

export type PermissionState = "granted" | "denied" | "not_required" | "unsupported";

export interface Permissions {
  platform: "darwin" | "windows" | "linux";
  screen_recording: PermissionState;
  accessibility: PermissionState;
  /** macOS Ekran Kaydı izni ancak yeniden açılışta tanınır. */
  restart_required: boolean;
}

/** Sidecar oturum aşaması. */
export type Phase =
  | "idle"
  | "resolved"
  | "ready"
  | "starting"
  | "active"
  | "reconnecting"
  | "ending"
  | "ended";

/** Kontrol durumu (tasarımdaki çubuk ve onay ekranı durumları). */
export type ControlState = "view_only" | "requested" | "pending_permission" | "active" | "revoked";

export type Actor = "customer" | "agent" | "system";

/** Oturum bitiş özeti (`state.summary`, yalnız `ended`'da). */
export interface Summary {
  agent_name: string;
  organization_name: string;
  started_at: string | null;
  ended_at: string | null;
  duration_seconds: number;
  control: { granted: boolean; seconds: number; revoked_by: "customer" | null };
  recorded: boolean;
  ended_reason: EndedReason;
  ended_by: "customer" | "agent" | "time" | "platform" | "connection";
  reported: boolean;
}

/** `state` olayı: her değişiklikte tam anlık görüntü. Ekran yalnız bundan çizilir. */
export interface StateEvent {
  phase: Phase;
  session: ResolvedSession | null;
  consent: { view: boolean; control: boolean; recording: boolean | null };
  control: ControlState;
  connection: "none" | "connecting" | "connected" | "reconnecting";
  agent_present: boolean;
  /** Ekran yayınının başladığı an; süre sayacı buradan. */
  started_at: string | null;
  ends_at: string | null;
  permissions: Permissions;
  summary: Summary | null;
}

/** `control` olayı. */
export interface ControlEvent {
  state: ControlState;
  previous: ControlState;
  by?: Actor;
  notice?: "blocked_shortcut" | "rate_limited" | "accessibility_missing";
  at: string;
}

/** Sidecar hata türleri (IPC.md "Hata kodları"); kabuk tamsayı kodu buna çevirir. */
export type ErrorKind =
  | "INVALID_PARAMS"
  | "INTERNAL"
  | "CODE_INVALID"
  | "RATE_LIMITED"
  | "ENDED"
  | "UNAVAILABLE"
  | "NETWORK"
  | "PERMISSION_REQUIRED"
  | "UNSUPPORTED"
  | "INVALID_STATE"
  | "SERVER"
  | "UNAUTHORIZED"
  | "CONFLICT"
  | "CAPTURE_FAILED"
  | "LIVEKIT"
  // Kabuk kodları
  | "NOT_RUNNING"
  | "TIMEOUT"
  | "IPC"
  | "METHOD_NOT_ALLOWED";

/** `error` olayı. */
export interface ErrorEvent {
  code: number;
  kind: ErrorKind;
  message: string;
  fatal: boolean;
}

/** Yöntem hatası (kabuğun `sidecar_call` hatası). */
export interface IpcError {
  /** Hata türü (`ErrorKind`) ya da tanınmayan kod için `RPC_<n>`. */
  code: string;
  message: string;
}

type StateResult = { state: StateEvent };
type Empty = Record<string, never>;

export interface Methods {
  "session.resolve": {
    params: ({ code: string } | { link: string }) & { share_system_info?: boolean };
    result: StateResult;
  };
  "session.consent": {
    params: { view?: true; control?: boolean; recording?: boolean };
    result: StateResult;
  };
  "session.start": { params: Empty; result: StateResult };
  "session.stop": { params: Empty; result: StateResult };
  /** "Bu oturumu ben istemedim" / "Bildir": bildirir ve oturumu bitirir. */
  "session.report": { params: { reason?: string }; result: StateResult };
  "control.revoke": { params: Empty; result: StateResult };
  "permissions.status": { params: Empty; result: Permissions };
  /** macOS izin istemini sidecar'dan açar (izin onu başlatan uygulamaya bağlanır). */
  "permissions.request": {
    params: { kind: "screen_recording" | "accessibility" };
    result: Permissions;
  };
}

export type Method = keyof Methods;

export interface Events {
  state: StateEvent;
  control: ControlEvent;
  error: ErrorEvent;
}

export type EventName = keyof Events;

/** Arayüzün kullandığı IPC yüzeyi; gerçek ve sahte uygulama aynıdır. */
export interface Ipc {
  readonly kind: "sidecar" | "mock";
  call<M extends Method>(method: M, params: Methods[M]["params"]): Promise<Methods[M]["result"]>;
  on<E extends EventName>(event: E, cb: (p: Events[E]) => void): () => void;
}

export class IpcCallError extends Error implements IpcError {
  code: string;
  constructor(e: IpcError) {
    super(e.message);
    this.code = e.code;
  }
}

/** Ekran yayını sürüyor mu (çerçeve ve çubuk görünür)? */
export function isSharing(s: StateEvent | null): boolean {
  return s?.phase === "active" || s?.phase === "reconnecting";
}
