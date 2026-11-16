// ClickFix Destek akışı (docs/design/masaustu.md §5 "Akış"):
//
//   kod / bağlantı → [macOS: Ekran Kaydı izni] → kimlik → görme onayı
//   → [kontrol ve kayıt onayı] → [macOS: Erişilebilirlik] → paylaşım → bitiş
//
// Ekran Kaydı izni macOS'ta uygulamanın yeniden başlamasını ister. Çözülmüş
// (tek kullanımlık) bir oturum yeniden başlatmada kaybolacağı için izin, kod
// sunucuya gönderilmeden önce istenir; girilen kod en çok 15 dakika yerelde
// tutulur ve yeniden açılışta kendiliğinden kullanılır.
//
// Paylaşım durumunu sidecar'ın `state` olayı belirler. Çerçeve ve çubuk
// pencerelerini Rust kabuğu aynı olaydan çizer; bu pencere yalnız özet ve
// yedek düğmeler gösterir.

import { isTauri } from "@tauri-apps/api/core";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { normalizeCode } from "./code";
import { firstName, systemLang, translator, type Lang, type T } from "./i18n";
import {
  ipc,
  IpcCallError,
  isSharing,
  type Permissions,
  type ResolvedSession,
  type StateEvent,
} from "./ipc";
import {
  mockOverlay,
  onBarAction,
  onJoinToken,
  openPermissionSettings,
  quit,
  relaunch,
  takeJoinToken,
  type BarAction,
} from "./native";
import { Bar, Frame } from "./overlay";
import {
  CodeScreen,
  ConnectingScreen,
  ControlConsentScreen,
  EndedScreen,
  IdentityScreen,
  PermissionScreen,
  ReportSheet,
  SharingScreen,
  ViewConsentScreen,
  type EndedBy,
} from "./screens";
import { Ui } from "./ui";

declare const __APP_VERSION__: string;

type View =
  | { k: "boot" }
  | { k: "code" }
  | { k: "perm-screen" }
  | { k: "identity" }
  | { k: "view" }
  | { k: "control" }
  | { k: "perm-ax" }
  | { k: "connecting" }
  | { k: "sharing" }
  | { k: "ended" };

type Pending = { kind: "code" | "token"; value: string; at: number };

const PENDING_KEY = "clickfix-support:pending";
const PENDING_TTL = 15 * 60_000;

function savePending(p: Pending | null) {
  try {
    if (p) localStorage.setItem(PENDING_KEY, JSON.stringify(p));
    else localStorage.removeItem(PENDING_KEY);
  } catch {
    // Depolama yoksa yeniden başlatmada kod yeniden girilir.
  }
}

function loadPending(now: number): Pending | null {
  try {
    const raw = localStorage.getItem(PENDING_KEY);
    if (!raw) return null;
    const p = JSON.parse(raw) as Pending;
    if (now - p.at > PENDING_TTL) {
      localStorage.removeItem(PENDING_KEY);
      return null;
    }
    return p;
  } catch {
    return null;
  }
}

/** Hata türünden (desktop/sidecar/IPC.md "Hata kodları") müşteri metni. */
function errorText(t: T, err: unknown): string {
  const code = err instanceof IpcCallError ? err.code : "";
  switch (code) {
    case "CODE_INVALID":
      return t("errCodeInvalid");
    case "RATE_LIMITED":
      return t("errRateLimited");
    case "UNAVAILABLE":
      return t("errDisabled");
    case "ENDED":
    case "UNAUTHORIZED":
      return t("errEnded");
    case "NETWORK":
    case "LIVEKIT":
      return t("errNetwork");
    case "UNSUPPORTED":
      return t("errUnsupported");
    case "CAPTURE_FAILED":
      return t("errCapture");
    case "NOT_RUNNING":
    case "TIMEOUT":
    case "IPC":
      return t("errSidecar");
    default:
      return t("errGeneric");
  }
}

function needsScreenPermission(p: Permissions): boolean {
  return p.platform === "darwin" && p.screen_recording === "denied";
}

function needsAxPermission(p: Permissions): boolean {
  return p.platform === "darwin" && p.accessibility === "denied";
}

type Theme = "system" | "light" | "dark";

export function App() {
  const api = ipc();
  const [lang, setLang] = useState<Lang>(systemLang);
  const [theme, setTheme] = useState<Theme>("system");
  const t = useMemo(() => translator(lang), [lang]);

  const [view, setView] = useState<View>({ k: "boot" });
  const [session, setSession] = useState<ResolvedSession | null>(null);
  const [state, setState] = useState<StateEvent | null>(null);
  const [perms, setPerms] = useState<Permissions | null>(null);
  const [busy, setBusy] = useState<"code" | "link" | "action" | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [reportOpen, setReportOpen] = useState(false);
  const [permMissing, setPermMissing] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [initialCode, setInitialCode] = useState("");
  const [localEnd, setLocalEnd] = useState<EndedBy>(null);
  const viewRef = useRef<View>(view);
  const pending = useRef<Pending | null>(null);

  useEffect(() => {
    viewRef.current = view;
  }, [view]);

  useEffect(() => {
    document.documentElement.lang = lang;
    document.title = t("appName");
  }, [lang, t]);

  useEffect(() => {
    if (theme === "system") delete document.documentElement.dataset.theme;
    else document.documentElement.dataset.theme = theme;
  }, [theme]);

  const refreshPerms = useCallback(async () => {
    const p = await api.call("permissions.status", {});
    setPerms(p);
    return p;
  }, [api]);

  /** Kodu ya da bağlantı token'ını çözer (izin denetimi dahil). */
  const resolve = useCallback(
    async (p: Pending) => {
      setError(null);
      setBusy(p.kind === "token" ? "link" : "code");
      try {
        const current = await refreshPerms();
        if (needsScreenPermission(current)) {
          pending.current = p;
          savePending(p);
          setPermMissing(false);
          setView({ k: "perm-screen" });
          return;
        }
        savePending(null);
        pending.current = null;
        const { state: s } = await api.call(
          "session.resolve",
          p.kind === "token" ? { link: p.value } : { code: p.value },
        );
        setLocalEnd(null);
        setState(s);
        setSession(s.session);
        setView({ k: "identity" });
      } catch (err) {
        savePending(null);
        setError(errorText(t, err));
        setView({ k: "code" });
      } finally {
        setBusy(null);
      }
    },
    [api, refreshPerms, t],
  );

  // Açılış: derin bağlantı, yeniden başlatmadan kalan kod ya da boş kod ekranı.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const token = await takeJoinToken().catch(() => null);
      if (cancelled) return;
      if (token) {
        void resolve({ kind: "token", value: token, at: Date.now() });
        return;
      }
      const p = loadPending(Date.now());
      if (p) {
        if (p.kind === "code") setInitialCode(p.value);
        void resolve(p);
        return;
      }
      setView({ k: "code" });
    })();
    return () => {
      cancelled = true;
    };
  }, [resolve]);

  // Uygulama açıkken gelen derin bağlantı: yalnız oturum yokken kabul edilir.
  useEffect(
    () =>
      onJoinToken((token) => {
        const k = viewRef.current.k;
        if (k === "code" || k === "ended" || k === "perm-screen") {
          void resolve({ kind: "token", value: token, at: Date.now() });
        }
      }),
    [resolve],
  );

  // Sidecar olayları.
  useEffect(() => {
    const offState = api.on("state", (s) => {
      setState(s);
      if (s.session) setSession(s.session);
      if (api.kind === "mock") void mockOverlay(s);
      if (isSharing(s)) {
        setView((v) => (v.k === "connecting" ? { k: "sharing" } : v));
      } else if (s.phase === "ready") {
        // Başlatma hatası (`starting` → `ready`): onay ekranına dön.
        setView((v) => (v.k === "connecting" ? { k: "control" } : v));
      } else if (s.phase === "ended") {
        setReportOpen(false);
        setBusy(null);
        setView({ k: "ended" });
      }
    });
    const offControl = api.on("control", (c) => {
      if (c.state === "active") setNotice(null);
      else if (c.previous === "active" && c.by === "agent") setNotice("agent");
      else if (c.state === "revoked" && c.by === "customer") setNotice("you");
    });
    // Ölümcül hatada ardından `ended` durumu gelir; ekranı o çizer.
    const offError = api.on("error", (e) => {
      if (!e.fatal) setError(errorText(t, new IpcCallError({ code: e.kind, message: e.message })));
    });
    return () => {
      offState();
      offControl();
      offError();
    };
  }, [api, t]);

  const stop = useCallback(
    async (kind: "stop" | "emergency") => {
      setLocalEnd(kind);
      try {
        await api.call("session.stop", {});
      } catch (err) {
        setError(errorText(t, err));
      }
    },
    [api, t],
  );

  const revoke = useCallback(async () => {
    try {
      await api.call("control.revoke", {});
    } catch (err) {
      setError(errorText(t, err));
    }
  }, [api, t]);

  // Çubuk ve acil kısayol: gerçek sidecar'da Rust doğrudan uygular; sahte
  // IPC'de istek bu pencereye gelir.
  useEffect(
    () =>
      onBarAction((a: BarAction, handled: boolean) => {
        if (a === "emergency" || a === "stop") setLocalEnd(a);
        if (handled) return;
        if (a === "revoke") void revoke();
        else void stop(a === "emergency" ? "emergency" : "stop");
      }),
    [revoke, stop],
  );

  async function act(fn: () => Promise<void>) {
    setBusy("action");
    setError(null);
    try {
      await fn();
    } catch (err) {
      setError(errorText(t, err));
    } finally {
      setBusy(null);
    }
  }

  async function begin(control: boolean, back: View) {
    const p = perms ?? (await refreshPerms());
    if (control && needsAxPermission(p)) {
      setPermMissing(false);
      setView({ k: "perm-ax" });
      return;
    }
    await startSharing(back);
  }

  /** Yayını başlatır; hızlı denetim hatasında (izin, platform) önceki ekrana döner. */
  async function startSharing(back: View) {
    setView({ k: "connecting" });
    try {
      await api.call("session.start", {});
    } catch (err) {
      setView(back);
      throw err;
    }
  }

  async function decline() {
    setLocalEnd("declined");
    await api.call("session.stop", {});
  }

  async function report() {
    await api.call("session.report", {});
  }

  /** İzin istemi sidecar'dan açılır (izin onu başlatan uygulamaya bağlanır),
   * ardından kabuk Sistem Ayarları'nın ilgili bölmesini açar. */
  function openSettings(pane: "screen_recording" | "accessibility") {
    void act(async () => {
      setPerms(await api.call("permissions.request", { kind: pane }));
      await openPermissionSettings(pane);
      // Sahte IPC: Sistem Ayarları'nda anahtarın açıldığını varsay.
      api.controls?.setPermission(pane, true);
    });
  }

  function closeApp() {
    void quit();
  }

  function resetToCode() {
    setSession(null);
    setState(null);
    setError(null);
    setInitialCode("");
    setView({ k: "code" });
  }

  const platform = perms?.platform ?? (navigator.userAgent.includes("Win") ? "windows" : "darwin");
  const ui = useMemo(() => ({ t, platform, version: __APP_VERSION__ }), [t, platform]);
  const agentFirst = session ? firstName(session.agent.name) : "";
  const noticeText =
    notice === "agent"
      ? t("controlRevokedByAgent", { agent: agentFirst })
      : notice === "you"
        ? t("controlRevokedByYou", { agent: agentFirst })
        : null;
  const preview = !isTauri() && view.k === "sharing" && isSharing(state);

  let screen: ReactNode = null;
  switch (view.k) {
    case "boot":
      screen = null;
      break;
    case "code":
      screen = (
        <CodeScreen
          key={initialCode}
          initial={initialCode}
          busy={busy === "code" || busy === "link" ? busy : null}
          error={error}
          onChange={() => setError(null)}
          onSubmit={(code) => void resolve({ kind: "code", value: normalizeCode(code), at: Date.now() })}
        />
      );
      break;
    case "perm-screen":
      screen = (
        <PermissionScreen
          kind="screen_recording"
          perms={perms}
          agentName={null}
          controlWanted={false}
          missingAfterCheck={permMissing}
          busy={busy !== null}
          onOpenSettings={() => openSettings("screen_recording")}
          onRecheck={() =>
            void act(async () => {
              const p = await refreshPerms();
              if (needsScreenPermission(p) || p.restart_required) setPermMissing(true);
              else if (pending.current) await resolve(pending.current);
            })
          }
          onRestart={() => void relaunch()}
          onSkip={null}
        />
      );
      break;
    case "identity":
      if (!session) break;
      screen = (
        <IdentityScreen
          session={session}
          busy={busy !== null}
          error={error}
          onConfirm={() => {
            setError(null);
            setView({ k: "view" });
          }}
          onCancel={() => void act(decline)}
          onReport={() => setReportOpen(true)}
        />
      );
      break;
    case "view":
      if (!session) break;
      screen = (
        <ViewConsentScreen
          session={session}
          busy={busy !== null}
          error={error}
          onDecline={() => void act(decline)}
          onAccept={() =>
            void act(async () => {
              await api.call("session.consent", { view: true });
              if (session.mode_requested === "control" || session.recording_requested) {
                setView({ k: "control" });
              } else {
                await begin(false, { k: "view" });
              }
            })
          }
        />
      );
      break;
    case "control":
      if (!session) break;
      screen = (
        <ControlConsentScreen
          session={session}
          state={state}
          busy={busy !== null}
          error={error}
          onSubmit={(c) =>
            void act(async () => {
              const params: { control?: boolean; recording?: boolean } = {};
              if (session.mode_requested === "control") params.control = c.control;
              if (session.recording_requested) params.recording = c.recording;
              await api.call("session.consent", params);
              await begin(c.control, { k: "control" });
            })
          }
        />
      );
      break;
    case "perm-ax":
      if (!session) break;
      screen = (
        <PermissionScreen
          kind="accessibility"
          perms={perms}
          agentName={session.agent.name}
          controlWanted
          missingAfterCheck={permMissing}
          busy={busy !== null}
          onOpenSettings={() => openSettings("accessibility")}
          onRecheck={() =>
            void act(async () => {
              const p = await refreshPerms();
              if (needsAxPermission(p)) {
                setPermMissing(true);
                return;
              }
              await startSharing({ k: "perm-ax" });
            })
          }
          onRestart={() => void relaunch()}
          onSkip={() =>
            void act(async () => {
              await api.call("session.consent", { control: false });
              await startSharing({ k: "perm-ax" });
            })
          }
        />
      );
      break;
    case "connecting":
      screen = <ConnectingScreen />;
      break;
    case "sharing":
      if (!session || !state) break;
      screen = (
        <SharingScreen
          session={session}
          state={state}
          notice={noticeText}
          onStop={() => void stop("stop")}
          onRevoke={() => void revoke()}
        />
      );
      break;
    case "ended":
      screen = (
        <EndedScreen
          session={session}
          summary={state?.summary ?? null}
          localEnd={localEnd}
          busy={busy !== null}
          onClose={session ? closeApp : resetToCode}
          onReport={
            session && state?.summary?.started_at
              ? () =>
                  void act(async () => {
                    await report();
                  })
              : null
          }
        />
      );
      break;
  }

  return (
    <Ui.Provider value={ui}>
      {screen}
      {reportOpen && (
        <ReportSheet
          busy={busy !== null}
          onBack={() => setReportOpen(false)}
          onConfirm={() =>
            void act(async () => {
              await report();
              setReportOpen(false);
            })
          }
        />
      )}
      {preview && state && (
        <>
          <Frame t={t} />
          <div className="ov-preview-bar">
            <Bar
              t={t}
              platform={platform}
              state={state}
              native={false}
              onStop={() => void stop("stop")}
              onRevoke={() => void revoke()}
            />
          </div>
        </>
      )}
      {api.kind === "mock" && (
        <DevPanel
          t={t}
          lang={lang}
          setLang={setLang}
          theme={theme}
          setTheme={setTheme}
          platform={platform}
          onPlatform={(p) => {
            api.controls?.setPlatform(p);
            void refreshPerms();
          }}
          onAgentEnds={() => api.controls?.agentEnds()}
          onAgentReleases={() => api.controls?.agentReleasesControl()}
          onRevokeScreen={() => {
            api.controls?.setPermission("screen_recording", false);
            void refreshPerms();
          }}
          onGrantAll={() => {
            api.controls?.setPermission("screen_recording", true);
            api.controls?.setPermission("accessibility", true);
          }}
          onEmergency={() => void stop("emergency")}
        />
      )}
    </Ui.Provider>
  );
}

/** Sahte IPC ile bütün ekranları gezmek için küçük panel. */
function DevPanel(props: {
  t: T;
  lang: Lang;
  setLang: (l: Lang) => void;
  theme: Theme;
  setTheme: (t: Theme) => void;
  platform: Permissions["platform"];
  onPlatform: (p: Permissions["platform"]) => void;
  onAgentEnds: () => void;
  onAgentReleases: () => void;
  onRevokeScreen: () => void;
  onGrantAll: () => void;
  onEmergency: () => void;
}) {
  const { t } = props;
  const pick = <V extends string>(value: V, current: V, set: (v: V) => void, label: string) => (
    <button key={value} aria-pressed={value === current} onClick={() => set(value)}>
      {label}
    </button>
  );
  return (
    <details className="dev">
      <summary>{t("devTitle")}</summary>
      <div className="dev-body">
        <div className="dev-row">
          <span>{t("devTheme")}</span>
          {pick("system", props.theme, props.setTheme, t("system"))}
          {pick("light", props.theme, props.setTheme, t("light"))}
          {pick("dark", props.theme, props.setTheme, t("dark"))}
        </div>
        <div className="dev-row">
          <span>{t("devLang")}</span>
          {pick("tr", props.lang, props.setLang, "TR")}
          {pick("en", props.lang, props.setLang, "EN")}
        </div>
        <div className="dev-row">
          <span>{t("devPlatform")}</span>
          {pick("darwin", props.platform, props.onPlatform, "macOS")}
          {pick("windows", props.platform, props.onPlatform, "Windows")}
        </div>
        <div className="dev-row">
          <button onClick={props.onAgentReleases}>{t("devAgentReleases")}</button>
          <button onClick={props.onAgentEnds}>{t("devAgentEnds")}</button>
        </div>
        <div className="dev-row">
          <button onClick={props.onEmergency}>Ctrl+Alt+Shift+S</button>
          <button onClick={props.onRevokeScreen}>{t("devRevokeScreen")}</button>
          <button onClick={props.onGrantAll}>{t("permGranted")}</button>
        </div>
        <p className="muted" style={{ margin: 0, lineHeight: 1.4 }}>
          {t("devCodes")}
        </p>
      </div>
    </details>
  );
}
