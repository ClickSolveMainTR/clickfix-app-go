// Sidecar olmadan arayüzü gezmek için sahte IPC (desktop/sidecar/IPC.md
// sözleşmesinin bellekteki taklidi). Tarayıcıda (`pnpm dev`) her zaman,
// Tauri'de yalnız geliştirme derlemesinde kullanılır (bkz. `mockSelected`).
//
// Deneme kodları:
//   000 000 000  geçersiz kod (CODE_INVALID)
//   111 111 111  yalnız izleme, kayıt yok
//   222 222 222  kontrol, kayıt yok
//   diğerleri    kontrol + isteğe bağlı kayıt (tasarımdaki örnek)
//
// URL parametreleri: `?platform=darwin|windows`, `?perm=0` (izinler yok).

import type {
  Actor,
  ControlState,
  EndedReason,
  EventName,
  Events,
  Ipc,
  Method,
  Methods,
  Permissions,
  ResolvedSession,
  StateEvent,
  Summary,
} from "./types";
import { IpcCallError } from "./types";

type Listener = (p: never) => void;

const delay = (ms: number) => new Promise((r) => setTimeout(r, ms));

function detectPlatform(): Permissions["platform"] {
  const p = `${navigator.userAgent} ${navigator.platform}`.toLowerCase();
  if (p.includes("win")) return "windows";
  if (p.includes("mac")) return "darwin";
  return "linux";
}

function basePermissions(platform: Permissions["platform"], granted: boolean): Permissions {
  if (platform !== "darwin") {
    return {
      platform,
      screen_recording: "not_required",
      accessibility: "not_required",
      restart_required: false,
    };
  }
  const st = granted ? "granted" : "denied";
  return { platform, screen_recording: st, accessibility: st, restart_required: false };
}

const SAMPLE: Omit<ResolvedSession, "mode_requested" | "recording_requested" | "consent_deadline"> = {
  uuid: "4f6d3a52-8f8f-4d4b-9a59-0c3f6f0d2b11",
  organization: {
    uuid: "0b6f7c1e-2f43-4a51-8f0e-5f4a0c2b9d10",
    name: "Atlas Teknik Servis",
    logo_url: null,
    verified: true,
  },
  agent: { uuid: "c2d1e0f9-8a7b-4c6d-9e5f-1a2b3c4d5e6f", name: "Elif Kaya", avatar_url: null },
  conversation: { number: "48213", subject: "Yazıcı yazdırmıyor" },
  recording_retention_days: 30,
  max_duration_minutes: 60,
  warning:
    "ClickFix çalışanları ve işletmeler sizden asla para transferi, banka girişi veya kart bilgisi istemez.",
  locale: "tr",
};

export interface MockControls {
  /** Temsilci oturumu bitirir. */
  agentEnds(): void;
  /** Temsilci kontrolü bırakır. */
  agentReleasesControl(): void;
  /** İzin durumunu değiştirir (Sequoia'nın aylık yeniden soruşu dahil). */
  setPermission(kind: "screen_recording" | "accessibility", granted: boolean): void;
  setPlatform(p: Permissions["platform"]): void;
}

const ENDED_BY: Record<EndedReason, Summary["ended_by"]> = {
  customer: "customer",
  declined: "customer",
  reported: "customer",
  agent: "agent",
  conversation_closed: "agent",
  timeout: "time",
  max_duration: "time",
  code_expired: "time",
  attempts: "time",
  platform: "platform",
  connection_lost: "connection",
};

export function createMockIpc(): Ipc & { controls: MockControls } {
  const listeners = new Map<EventName, Set<Listener>>();
  const query = new URLSearchParams(location.search);
  let perms = basePermissions(
    (query.get("platform") as Permissions["platform"] | null) ?? detectPlatform(),
    query.get("perm") !== "0",
  );

  const idle = (): StateEvent => ({
    phase: "idle",
    session: null,
    consent: { view: false, control: false, recording: null },
    control: "view_only",
    connection: "none",
    agent_present: false,
    started_at: null,
    ends_at: null,
    permissions: perms,
    summary: null,
  });

  let state = idle();
  let controlSince: number | null = null;
  let controlSeconds = 0;
  let controlGranted = false;
  let revokedBy: "customer" | null = null;

  function emit<E extends EventName>(event: E, payload: Events[E]) {
    for (const cb of listeners.get(event) ?? []) (cb as (p: Events[E]) => void)(payload);
  }

  function setState(patch: Partial<StateEvent>) {
    state = { ...state, ...patch, permissions: perms };
    emit("state", state);
    return { state };
  }

  function setControl(next: ControlState, by?: Actor) {
    const previous = state.control;
    if (previous === next) return;
    if (previous === "active" && controlSince) {
      controlSeconds += Math.round((Date.now() - controlSince) / 1000);
      controlSince = null;
    }
    if (next === "active") {
      controlGranted = true;
      controlSince = Date.now();
    }
    state = { ...state, control: next };
    emit("control", { state: next, previous, by, at: new Date().toISOString() });
  }

  /** Kontrol onayı verilmişse kapının durumu (izin ve yayına göre). */
  function grantedControl(): ControlState {
    if (perms.platform === "darwin" && perms.accessibility !== "granted") return "pending_permission";
    return "active";
  }

  function end(reason: EndedReason, reported = false) {
    if (state.phase === "ended" || state.phase === "idle") return;
    if (state.control === "active" || state.control === "pending_permission") {
      setControl(state.consent.control ? state.control : "view_only", "system");
    }
    if (controlSince) {
      controlSeconds += Math.round((Date.now() - controlSince) / 1000);
      controlSince = null;
    }
    const s = state.session;
    const endedAt = new Date().toISOString();
    const summary: Summary = {
      agent_name: s?.agent.name ?? "",
      organization_name: s?.organization.name ?? "",
      started_at: state.started_at,
      ended_at: endedAt,
      duration_seconds: state.started_at
        ? Math.max(0, Math.round((Date.now() - Date.parse(state.started_at)) / 1000))
        : 0,
      control: { granted: controlGranted, seconds: controlSeconds, revoked_by: revokedBy },
      recorded: state.consent.recording === true && state.started_at !== null,
      ended_reason: reason,
      ended_by: ENDED_BY[reason],
      reported,
    };
    setState({ phase: "ended", connection: "none", agent_present: false, summary });
  }

  function requireSession() {
    if (!state.session || state.phase === "ended" || state.phase === "idle") {
      throw new IpcCallError({ code: "ENDED", message: "Oturum bitti." });
    }
    return state.session;
  }

  const handlers: {
    [M in Method]: (p: Methods[M]["params"]) => Promise<Methods[M]["result"]>;
  } = {
    async "session.resolve"(p) {
      await delay(500);
      if (state.phase !== "idle" && state.phase !== "ended") {
        throw new IpcCallError({ code: "INVALID_STATE", message: "Başka bir oturum sürüyor." });
      }
      const code = "code" in p ? p.code.replace(/[\s-]/g, "") : "";
      if (code === "000000000") {
        throw new IpcCallError({
          code: "CODE_INVALID",
          message: "Kod geçersiz, kullanılmış ya da süresi dolmuş.",
        });
      }
      const mode = code === "111111111" ? "view" : "control";
      const session: ResolvedSession = {
        ...SAMPLE,
        consent_deadline: new Date(Date.now() + 15 * 60_000).toISOString(),
        mode_requested: mode,
        recording_requested: code !== "111111111" && code !== "222222222",
      };
      controlSeconds = 0;
      controlGranted = false;
      revokedBy = null;
      state = {
        ...idle(),
        phase: "resolved",
        session,
        control: mode === "control" ? "requested" : "view_only",
      };
      emit("state", state);
      return { state };
    },
    async "session.consent"(p) {
      await delay(250);
      const s = requireSession();
      const consent = { ...state.consent };
      let phase = state.phase;
      if (p.view) {
        consent.view = true;
        if (phase === "resolved") phase = "ready";
      }
      if (p.recording !== undefined && s.recording_requested) consent.recording = p.recording;
      if (p.control !== undefined && s.mode_requested === "control") {
        if (p.control) {
          consent.control = true;
          revokedBy = null;
          setControl(phase === "active" ? grantedControl() : "requested", "customer");
        } else {
          const wasGiven = consent.control;
          consent.control = false;
          if (wasGiven) revokedBy = "customer";
          setControl(wasGiven ? "revoked" : "view_only", "customer");
        }
      }
      return setState({ phase, consent });
    },
    async "session.start"() {
      requireSession();
      if (state.phase !== "ready") {
        throw new IpcCallError({ code: "INVALID_STATE", message: "Önce izleme onayı gerekir." });
      }
      if (perms.platform === "darwin" && perms.screen_recording !== "granted") {
        throw new IpcCallError({ code: "PERMISSION_REQUIRED", message: "Ekran Kaydı izni gerekir." });
      }
      const result = setState({ phase: "starting", connection: "connecting" });
      void (async () => {
        await delay(900);
        if (state.phase !== "starting") return;
        setState({
          phase: "active",
          connection: "connected",
          agent_present: true,
          started_at: new Date().toISOString(),
          ends_at: new Date(Date.now() + (state.session?.max_duration_minutes ?? 60) * 60_000).toISOString(),
        });
        if (state.consent.control) {
          setControl(grantedControl(), "customer");
          emit("state", state);
        }
      })();
      return result;
    },
    async "session.stop"() {
      await delay(150);
      if (state.phase === "idle") return { state };
      setState({ phase: "ending" });
      await delay(150);
      end(state.started_at ? "customer" : "declined");
      return { state };
    },
    async "session.report"() {
      await delay(300);
      if (state.phase === "ended" && state.summary) {
        return setState({ summary: { ...state.summary, reported: true } });
      }
      end("reported", true);
      return { state };
    },
    async "control.revoke"() {
      await delay(120);
      requireSession();
      if (state.consent.control) {
        revokedBy = "customer";
        setControl("revoked", "customer");
        return setState({ consent: { ...state.consent, control: false } });
      }
      return { state };
    },
    async "permissions.status"() {
      await delay(120);
      if (state.control === "pending_permission" && perms.accessibility === "granted") {
        setControl("active", "system");
        setState({});
      }
      return perms;
    },
    async "permissions.request"() {
      await delay(120);
      return perms;
    },
  };

  return {
    kind: "mock",
    call(method, p) {
      return handlers[method](p as never) as never;
    },
    on(event, cb) {
      let set = listeners.get(event);
      if (!set) listeners.set(event, (set = new Set()));
      set.add(cb as Listener);
      return () => set.delete(cb as Listener);
    },
    controls: {
      agentEnds: () => end("agent"),
      agentReleasesControl() {
        if (state.control !== "active") return;
        setControl("view_only", "agent");
        setState({ consent: { ...state.consent, control: false } });
      },
      setPermission(kind, granted) {
        if (perms.platform !== "darwin") return;
        perms = { ...perms, [kind]: granted ? "granted" : "denied" };
      },
      setPlatform(p) {
        perms = basePermissions(p, false);
      },
    },
  };
}
