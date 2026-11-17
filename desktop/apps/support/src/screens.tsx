// ClickFix Destek ekranları. Düzen, renk ve metinler onaylı tasarımın
// (CF-231 kanvası) ilgili artboard'larından alınmıştır.

import { useRef, useState, type ClipboardEvent, type KeyboardEvent } from "react";

import { CODE_LENGTH, normalizeCode } from "./code";
import { firstName } from "./i18n";
import {
  ArrowRight,
  Badge,
  Check,
  Clock,
  Eye,
  Flag,
  Monitor,
  Pointer,
  Record,
  Shield,
  Tile,
} from "./icons";
import { isSharing, type Permissions, type ResolvedSession, type StateEvent, type Summary } from "./ipc/types";
import { openLink } from "./native";
import {
  Avatar,
  Brand,
  OrgLogo,
  Rich,
  Screen,
  ShortcutKeys,
  Steps,
  Warning,
  clock,
  stepsFor,
  useElapsed,
  useUi,
} from "./ui";

// 1. Kod girişi

export function CodeScreen(props: {
  initial: string;
  busy: "code" | "link" | null;
  error: string | null;
  onSubmit: (code: string) => void;
  onChange: () => void;
}) {
  const { t, version } = useUi();
  const [digits, setDigits] = useState(() => normalizeCode(props.initial));
  const [hint, setHint] = useState<string | null>(null);
  const refs = useRef<(HTMLInputElement | null)[]>([]);
  const complete = digits.length === CODE_LENGTH;
  const busy = props.busy !== null;

  function focus(i: number) {
    refs.current[Math.max(0, Math.min(CODE_LENGTH - 1, i))]?.focus();
  }

  function setAll(next: string) {
    const v = normalizeCode(next);
    setDigits(v);
    setHint(null);
    props.onChange();
    focus(v.length);
    return v;
  }

  function onInput(i: number, value: string) {
    const typed = value.replace(/\D/g, "");
    if (!typed) return;
    // Bir kutuya birden çok hane gelirse (otomatik doldurma) oradan devam et.
    setAll(digits.slice(0, i) + typed);
  }

  function onKey(i: number, e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Backspace") {
      e.preventDefault();
      const cut = i < digits.length ? i : i - 1;
      if (cut < 0) return;
      setDigits(digits.slice(0, cut));
      props.onChange();
      focus(cut);
    } else if (e.key === "ArrowLeft") {
      focus(i - 1);
    } else if (e.key === "ArrowRight") {
      focus(i + 1);
    } else if (e.key === "Enter" && complete && !busy) {
      props.onSubmit(digits);
    }
  }

  function onPaste(e: ClipboardEvent<HTMLInputElement>) {
    e.preventDefault();
    const v = setAll(e.clipboardData.getData("text"));
    if (v.length === CODE_LENGTH) props.onSubmit(v);
  }

  async function pasteFromClipboard() {
    try {
      const text = await navigator.clipboard.readText();
      const v = normalizeCode(text);
      if (v.length !== CODE_LENGTH) {
        setHint(t("pasteNone"));
        return;
      }
      setAll(v);
    } catch {
      setHint(t("pasteBlocked"));
      focus(0);
    }
  }

  const groups = [0, 1, 2];
  return (
    <Screen className="code">
      <Brand />

      <div className="stack" style={{ gap: 10 }}>
        <div className="eyebrow">
          <span className="live-dot" />
          {t("codeEyebrow")}
        </div>
        <h1 className="title lg">{t("codeTitle")}</h1>
        <p className="lead" style={{ fontSize: 14 }}>
          {t("codeLead")}
        </p>
      </div>

      <form
        className="code-field"
        onSubmit={(e) => {
          e.preventDefault();
          if (complete && !busy) props.onSubmit(digits);
        }}
      >
        <label htmlFor="kod-1" className="code-label">
          {t("codeLabel")}
        </label>
        <div className="digits">
          {groups.map((g) => (
            <div className="digit-group" key={g}>
              {[0, 1, 2].map((d) => {
                const i = g * 3 + d;
                return (
                  <input
                    key={i}
                    id={`kod-${i + 1}`}
                    ref={(el) => {
                      refs.current[i] = el;
                    }}
                    className="digit"
                    inputMode="numeric"
                    autoComplete={i === 0 ? "one-time-code" : "off"}
                    autoFocus={i === Math.min(digits.length, CODE_LENGTH - 1)}
                    aria-label={t("digitLabel", { n: i + 1 })}
                    value={digits[i] ?? ""}
                    disabled={busy}
                    onChange={(e) => onInput(i, e.target.value)}
                    onKeyDown={(e) => onKey(i, e)}
                    onPaste={onPaste}
                    onFocus={(e) => e.currentTarget.select()}
                  />
                );
              })}
              {g < 2 && <span className="digit-sep" aria-hidden="true" />}
            </div>
          ))}
        </div>
        <div className="code-help">
          <span>{t("codeLinkHint")}</span>
          <button type="button" className="link-btn" onClick={pasteFromClipboard} disabled={busy}>
            {t("paste")}
          </button>
        </div>
        {(props.error || hint) && (
          <p className="field-error" role="alert">
            {props.error ?? hint}
          </p>
        )}
        <button type="submit" className="btn primary lg" disabled={!complete || busy} style={{ marginTop: 12 }}>
          {busy ? (
            <>
              <span className="spinner" aria-hidden="true" />
              {props.busy === "link" ? t("resolvingLink") : t("resolving")}
            </>
          ) : (
            <>
              {t("continue")}
              <ArrowRight />
            </>
          )}
        </button>
      </form>

      <div className="spacer" />

      <Warning strong={t("scamStrong")} large />

      <div className="footer">
        <span>{t("version", { v: version })}</span>
        <span style={{ display: "flex", gap: 14 }}>
          <button type="button" onClick={() => void openLink("privacy")}>
            {t("privacy")}
          </button>
          <button type="button" onClick={() => void openLink("help")}>
            {t("help")}
          </button>
        </span>
      </div>
    </Screen>
  );
}

// 2. Kimlik doğrulama

export function IdentityScreen(props: {
  session: ResolvedSession;
  busy: boolean;
  error: string | null;
  onConfirm: () => void;
  onCancel: () => void;
  onReport: () => void;
}) {
  const { t } = useUi();
  const s = props.session;
  const conv = s.conversation;
  return (
    <Screen>
      <Steps steps={stepsFor(s)} active={["identity"]} />

      <div className="stack">
        <h1 className="title">{t("idTitle")}</h1>
        <p className="lead">{t("idLead")}</p>
      </div>

      <section className="card" aria-label={s.organization.name}>
        <div className="card-row dashed">
          <OrgLogo name={s.organization.name} url={s.organization.logo_url} />
          <div className="stack" style={{ flex: 1, minWidth: 0, gap: 5 }}>
            <span className="org-name">{s.organization.name}</span>
            {s.organization.verified ? (
              <span className="verified">
                <Badge />
                {t("verified")}
              </span>
            ) : (
              <span className="verified no">
                <Shield size={13} />
                {t("unverified")}
              </span>
            )}
          </div>
        </div>
        <div className="card-row">
          <div className="avatar-wrap">
            <Avatar name={s.agent.name} url={s.agent.avatar_url} />
          </div>
          <div className="stack" style={{ flex: 1, gap: 2 }}>
            <span style={{ fontSize: 15, fontWeight: 600 }}>{s.agent.name}</span>
            <span className="muted" style={{ fontSize: 12.5 }}>
              {t("agentRole")}
            </span>
          </div>
        </div>
        <div className="card-meta">
          {conv && (conv.number || conv.subject) && (
            <div className="meta-row">
              <span className="muted">{t("conversation")}</span>
              <span className="fm">
                {[conv.number && `#${conv.number}`, conv.subject].filter(Boolean).join(" · ")}
              </span>
            </div>
          )}
          <div className="meta-row">
            <span className="muted">{t("requested")}</span>
            <span className="chips">
              <span className="chip">
                <Eye />
                {t("chipView")}
              </span>
              {s.mode_requested === "control" && (
                <span className="chip">
                  <Pointer />
                  {t("chipControl")}
                </span>
              )}
              {s.recording_requested && (
                <span className="chip">
                  <Record />
                  {t("chipRecording")}
                </span>
              )}
            </span>
          </div>
        </div>
      </section>

      <Warning />

      {props.error && (
        <p className="field-error" role="alert">
          {props.error}
        </p>
      )}

      <div className="spacer" />

      <div className="actions">
        <button className="btn secondary grow-1" onClick={props.onCancel} disabled={props.busy}>
          {t("cancel")}
        </button>
        <button className="btn primary grow-2" onClick={props.onConfirm} disabled={props.busy}>
          {t("idConfirm")}
        </button>
      </div>
      <button className="link-btn danger" onClick={props.onReport} disabled={props.busy}>
        <Flag />
        {t("reportLink")}
      </button>
    </Screen>
  );
}

/** "Bu oturumu ben istemedim" onay sayfası. */
export function ReportSheet(props: { busy: boolean; onConfirm: () => void; onBack: () => void }) {
  const { t } = useUi();
  return (
    <div className="sheet-backdrop" role="presentation" onClick={props.onBack}>
      <div
        className="sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby="report-title"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="report-title" className="title md" style={{ margin: 0 }}>
          {t("reportTitle")}
        </h2>
        <p className="lead">{t("reportBody")}</p>
        <button className="btn danger" onClick={props.onConfirm} disabled={props.busy} autoFocus>
          <Flag />
          {t("reportConfirm")}
        </button>
        <button className="btn secondary" onClick={props.onBack} disabled={props.busy}>
          {t("reportBack")}
        </button>
      </div>
    </div>
  );
}

function AgentLine({ s }: { s: ResolvedSession }) {
  return (
    <div className="agent-line">
      <Avatar name={s.agent.name} url={s.agent.avatar_url} size="sm" />
      <span>
        <strong>{s.agent.name}</strong> · {s.organization.name}
      </span>
    </div>
  );
}

// 3. Görme onayı

export function ViewConsentScreen(props: {
  session: ResolvedSession;
  busy: boolean;
  error: string | null;
  onAccept: () => void;
  onDecline: () => void;
}) {
  const { t } = useUi();
  const s = props.session;
  const agent = firstName(s.agent.name);
  const org = s.organization.name;
  const [checked, setChecked] = useState(false);
  return (
    <Screen>
      <Steps steps={stepsFor(s)} active={["view"]} />
      <AgentLine s={s} />
      <h1 className="title">{t("viewTitle", { agent })}</h1>

      <div className="info-card">
        <div className="screen-thumb" aria-hidden="true">
          <span style={{ left: 8, top: 8, width: 44, height: 30, background: "var(--input)" }} />
          <span style={{ left: 46, top: 20, width: 38, height: 28, background: "var(--border)" }} />
        </div>
        <div className="stack">
          <span style={{ fontWeight: 600 }}>{t("viewSees")}</span>
          <span className="muted" style={{ lineHeight: 1.45 }}>
            {t("viewNotShared", { agent })}
          </span>
        </div>
      </div>

      <details className="notice">
        <summary>{t("notice")}</summary>
        <p>{t("noticeBody", { org })}</p>
      </details>

      <label className="consent">
        <input type="checkbox" checked={checked} onChange={(e) => setChecked(e.target.checked)} />
        <span>
          <Rich text={t("viewConsent", { org })} />
        </span>
      </label>

      <div className="note">
        <Clock />
        <span>
          <Rich text={t("viewDuration", { min: s.max_duration_minutes })} />
        </span>
      </div>

      {props.error && (
        <p className="field-error" role="alert">
          {props.error}
        </p>
      )}

      <div className="spacer" />

      <div className="actions">
        <button className="btn secondary grow-1" onClick={props.onDecline} disabled={props.busy}>
          {t("decline")}
        </button>
        <button className="btn primary grow-2" onClick={props.onAccept} disabled={!checked || props.busy}>
          <Monitor />
          {t("viewAccept")}
        </button>
      </div>
    </Screen>
  );
}

// 4. Kontrol ve kayıt onayı

export function ControlConsentScreen(props: {
  session: ResolvedSession;
  state: StateEvent | null;
  busy: boolean;
  error: string | null;
  onSubmit: (c: { control: boolean; recording: boolean }) => void;
}) {
  const { t } = useUi();
  const s = props.session;
  const agent = firstName(s.agent.name);
  const wantsControl = s.mode_requested === "control";
  const [control, setControl] = useState(false);
  const [recording, setRecording] = useState(false);
  const sharing = isSharing(props.state);
  const elapsed = useElapsed(sharing ? (props.state?.started_at ?? null) : null);
  const days = s.recording_retention_days;

  const active = [wantsControl && "control", s.recording_requested && "recording"].filter(
    Boolean,
  ) as ("control" | "recording")[];

  return (
    <Screen>
      <Steps steps={stepsFor(s)} active={active} />

      {sharing && (
        <div className="live-banner" role="status">
          <span className="live-dot" />
          {t("liveSeeing", { agent, time: clock(elapsed) })}
        </div>
      )}

      {wantsControl ? (
        <>
          <h1 className="title md">{t("controlTitle", { agent })}</h1>
          <div className="can-grid">
            <div>
              <span className="can-label" style={{ color: "var(--signal-text)" }}>
                {t("can")}
              </span>
              <span>{t("canBody")}</span>
            </div>
            <div>
              <span className="can-label" style={{ color: "var(--danger)" }}>
                {t("cannot")}
              </span>
              <span>{t("cannotBody")}</span>
            </div>
          </div>
          <div className="note">
            <ShortcutKeys />
            <span>{t("shortcutHint")}</span>
          </div>
          <label className="consent compact">
            <input type="checkbox" checked={control} onChange={(e) => setControl(e.target.checked)} />
            <span>{t("controlConsent", { agent: s.agent.name })}</span>
          </label>
        </>
      ) : (
        <h1 className="title md">{t("recordingTitle", { org: s.organization.name })}</h1>
      )}

      {s.recording_requested && (
        <>
          {wantsControl && <div className="divider" />}
          <label className="consent plain">
            <input type="checkbox" checked={recording} onChange={(e) => setRecording(e.target.checked)} />
            <span className="stack" style={{ gap: 3 }}>
              <span>
                <strong>{t("recordingConsent")}</strong>
                <span className="optional">{t("optional")}</span>
              </span>
              <span className="muted" style={{ fontSize: 12 }}>
                {days ? t("recordingHint", { days }) : t("recordingHintNoDays")}
              </span>
            </span>
          </label>
        </>
      )}

      {props.error && (
        <p className="field-error" role="alert">
          {props.error}
        </p>
      )}

      <div className="spacer" />

      {wantsControl ? (
        <div className="actions">
          <button
            className="btn secondary grow-1"
            onClick={() => props.onSubmit({ control: false, recording })}
            disabled={props.busy}
          >
            {t("viewOnly")}
          </button>
          <button
            className="btn primary grow-13"
            onClick={() => props.onSubmit({ control: true, recording })}
            disabled={!control || props.busy}
          >
            <Pointer size={15} />
            {t("allowControl")}
          </button>
        </div>
      ) : (
        <button
          className="btn primary"
          onClick={() => props.onSubmit({ control: false, recording })}
          disabled={props.busy}
        >
          {t("continue")}
          <ArrowRight />
        </button>
      )}
    </Screen>
  );
}

// 5. macOS izinleri

export function PermissionScreen(props: {
  kind: "screen_recording" | "accessibility";
  perms: Permissions | null;
  agentName: string | null;
  controlWanted: boolean;
  missingAfterCheck: boolean;
  busy: boolean;
  onOpenSettings: () => void;
  onRecheck: () => void;
  onRestart: () => void;
  onSkip: (() => void) | null;
}) {
  const { t } = useUi();
  const ax = props.kind === "accessibility";
  const agent = props.agentName ? firstName(props.agentName) : null;
  const screenOk = props.perms?.screen_recording === "granted";
  const axOk = props.perms?.accessibility === "granted";

  const rows = [
    {
      n: "1",
      name: t("permScreen"),
      why: agent ? t("permScreenWhy", { agent }) : t("permScreenWhyGeneric"),
      st: screenOk || ax ? "ok" : "now",
    },
    {
      n: "2",
      name: t("permAx"),
      why: t("permAxWhy"),
      st: ax ? (axOk ? "ok" : "now") : props.controlWanted ? "next" : "skip",
    },
  ] as const;
  const stateLabel = { ok: t("permGranted"), now: t("permWaiting"), next: t("permNext"), skip: t("permSkipped") };
  const pane = ax ? t("permAx") : t("permScreen");

  return (
    <Screen>
      <div className="stack" style={{ gap: 8 }}>
        {rows.map((r) => (
          <div key={r.n} className={`perm ${r.st}`}>
            <span className="perm-n">{r.n}</span>
            <span className="stack" style={{ flex: 1, gap: 1 }}>
              <span style={{ fontSize: 13.5, fontWeight: 600 }}>{r.name}</span>
              <span className="muted" style={{ fontSize: 11.5 }}>
                {r.why}
              </span>
            </span>
            <span className="perm-state">{stateLabel[r.st]}</span>
          </div>
        ))}
      </div>

      <div className="stack">
        <h1 className="title md">{ax ? t("permAxTitle") : t("permScreenTitle")}</h1>
        <p className="lead" style={{ fontSize: 13 }}>
          {ax ? t("permAxLead", { agent: agent ?? "" }) : t("permScreenLead")}
        </p>
      </div>

      <div className="settings-mock" aria-hidden="true">
        <div className="settings-bar">
          <i />
          <i />
          <i />
          <span style={{ marginLeft: 8 }}>{t("permPath", { pane })}</span>
        </div>
        <div style={{ padding: "6px 12px" }}>
          <div className="settings-row" style={{ borderBottom: "1px solid var(--border)", opacity: 0.55 }}>
            <span style={{ width: 22, height: 22, borderRadius: 6, background: "#3b6fd8" }} />
            <span style={{ flex: 1 }}>{t("permOtherApp")}</span>
            <span className="toggle" />
          </div>
          <div className="settings-row">
            <Tile size={22} />
            <span style={{ flex: 1, fontWeight: 600 }}>{t("appName")}</span>
            <span className="toggle on" />
            <svg
              viewBox="0 0 1280 744"
              width="30"
              height="18"
              style={{ position: "absolute", right: -14, top: 24, filter: "drop-shadow(0 2px 4px rgba(0,0,0,0.5))" }}
            >
              <polygon fill="#03ff13" stroke="#000000" strokeWidth="40" points="64 744 0 0 1280 168 288 176 64 744" />
            </svg>
          </div>
        </div>
      </div>

      <ol className="howto">
        {[t("permStep1"), t("permStep2"), ax ? t("permStep3Ax") : t("permStep3Screen")].map((text, i) => (
          <li key={i}>
            <span className="n">0{i + 1}</span>
            <span>{text}</span>
          </li>
        ))}
      </ol>

      {props.missingAfterCheck && (
        <p className="field-error" role="alert">
          {t("permStillMissing")}
        </p>
      )}

      <div className="spacer" />

      <p className="small-note">{t("permMonthly")}</p>
      <div className="actions">
        {props.missingAfterCheck && !ax ? (
          <button className="btn secondary grow-1" onClick={props.onRestart} disabled={props.busy}>
            {t("permRestart")}
          </button>
        ) : (
          <button className="btn secondary grow-1" onClick={props.onRecheck} disabled={props.busy}>
            {t("permRecheck")}
          </button>
        )}
        <button className="btn primary grow-14" onClick={props.onOpenSettings} disabled={props.busy}>
          {t("permOpen")}
        </button>
      </div>
      {props.onSkip && (
        <button className="link-btn" style={{ alignSelf: "center" }} onClick={props.onSkip} disabled={props.busy}>
          {t("permSkipControl")}
        </button>
      )}
    </Screen>
  );
}

// 6. Bağlanma ve paylaşım (ana pencere)

export function ConnectingScreen() {
  const { t } = useUi();
  return (
    <Screen className="center">
      <div className="hero" style={{ marginTop: 120 }}>
        <span className="spinner" aria-hidden="true" style={{ width: 28, height: 28 }} />
        <p className="lead" role="status">
          {t("connecting")}
        </p>
      </div>
    </Screen>
  );
}

export function SharingScreen(props: {
  session: ResolvedSession;
  state: StateEvent;
  notice: string | null;
  onStop: () => void;
  onRevoke: () => void;
}) {
  const { t, platform } = useUi();
  const s = props.session;
  const agent = s.agent.name;
  const control = props.state.control === "active";
  const elapsed = useElapsed(props.state.started_at);
  return (
    <Screen>
      <Brand />
      <div className="live-banner" role="status">
        <span className="live-dot" />
        {control ? t("barControlling", { agent }) : t("barSeeing", { agent })}
        <span className="spacer" />
        <span className="fm">{clock(elapsed)}</span>
      </div>
      <h1 className="title">{t("sharingTitle")}</h1>
      <p className="lead">{t("sharingLead")}</p>
      <AgentLine s={s} />
      {props.notice && (
        <p className="toast" role="status">
          {props.notice}
        </p>
      )}
      <div className="note">
        <ShortcutKeys />
        <span>{t("shortcutHint")}</span>
      </div>
      <div className="spacer" />
      <Warning />
      <div className="actions">
        {control && (
          <button className="btn secondary grow-1" onClick={props.onRevoke}>
            {t("revoke")}
          </button>
        )}
        <button className="btn danger grow-1" onClick={props.onStop}>
          {t("stop")}
        </button>
      </div>
      <p className="small-note" style={{ textAlign: "center" }}>
        {platform === "darwin" ? t("barHintMac") : t("barHintWin")}
      </p>
    </Screen>
  );
}

// 7. Bitiş özeti

export type EndedBy = "stop" | "emergency" | "declined" | null;

export function EndedScreen(props: {
  session: ResolvedSession | null;
  summary: Summary | null;
  localEnd: EndedBy;
  busy: boolean;
  onClose: () => void;
  onReport: (() => void) | null;
}) {
  const { t } = useUi();
  const sum = props.summary;
  const s = props.session;
  const reported = sum?.reported === true || sum?.ended_reason === "reported";
  const started = Boolean(sum?.started_at);

  const total = sum?.duration_seconds ?? 0;
  const ctrl = sum?.control;
  const ctrlMin = Math.max(1, Math.round((ctrl?.seconds ?? 0) / 60));
  const controlText =
    !ctrl?.granted || ctrl.seconds === 0
      ? t("controlNone")
      : ctrl.revoked_by === "customer"
        ? t("controlYouRevoked", { m: ctrlMin })
        : t("controlUsed", { m: ctrlMin });
  const days = s?.recording_retention_days;
  const recordingText = sum?.recorded
    ? days
      ? t("recordingYesDays", { days })
      : t("recordingYes")
    : t("recordingNo");

  const endedBy = (() => {
    if (reported) return t("byReported");
    switch (sum?.ended_by) {
      case "time":
        return t("byTimeout");
      case "agent":
        return t("byAgent");
      case "customer":
        if (props.localEnd === "emergency") return t("byYouEmergency");
        if (props.localEnd === "stop") return t("byYouStop");
        return t("byYou");
      case "connection":
        return t("byConnection");
      default:
        return t("bySystem");
    }
  })();

  const title = reported ? t("reportedTitle") : started ? t("endedTitle") : t("notStartedTitle");
  const body = reported ? t("reportedBody") : started ? t("endedBody") : t("notStartedBody");

  const rows: [string, string, boolean][] = [];
  const agentName = sum?.agent_name ?? s?.agent.name;
  const orgName = sum?.organization_name ?? s?.organization.name;
  if (agentName) rows.push([t("rowAgent"), `${agentName} · ${orgName ?? ""}`, false]);
  if (started) {
    rows.push([t("rowDuration"), t("duration", { m: Math.floor(total / 60), s: total % 60 }), true]);
    if (s?.mode_requested === "control") rows.push([t("rowControl"), controlText, false]);
    if (s?.recording_requested) rows.push([t("rowRecording"), recordingText, false]);
  }
  if (sum) rows.push([t("rowEndedBy"), endedBy, false]);

  return (
    <Screen className="center">
      <div className="hero">
        <div className={reported ? "done-mark warn" : "done-mark"}>
          {reported ? <Shield size={30} /> : <Check />}
        </div>
        <h1 className="title" style={{ fontSize: 26 }}>
          {title}
        </h1>
        <p className="lead">{body}</p>
      </div>

      {rows.length > 0 && (
        <div className="summary">
          {rows.map(([k, v, mono]) => (
            <div className="summary-row" key={k}>
              <span className="muted">{k}</span>
              <span className={mono ? "fm" : undefined}>{v}</span>
            </div>
          ))}
        </div>
      )}

      <div className="spacer" />

      <p className="small-note" style={{ textAlign: "center", fontSize: 12 }}>
        {t("endedUninstall")}
      </p>
      <button className="btn primary" onClick={props.onClose} autoFocus>
        {t("close")}
      </button>
      {props.onReport && !reported && (
        <button className="link-btn danger" onClick={props.onReport} disabled={props.busy}>
          {t("endedReport")}
        </button>
      )}
    </Screen>
  );
}
