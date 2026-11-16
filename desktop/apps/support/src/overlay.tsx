// Paylaşım göstergeleri: ekran kenarı çerçevesi ve her zaman üstte duran,
// sürüklenebilir durum çubuğu (tasarım: "Destek: paylaşım"). Tauri'de ayrı,
// şeffaf pencerelerde (bar.html, frame.html), tarayıcı önizlemesinde ana
// sayfanın üstünde çizilir.

import type { T } from "./i18n";
import { initials } from "./i18n";
import { Cursor, Grip, StopSquare } from "./icons";
import type { Permissions, StateEvent } from "./ipc";
import { clock, useElapsed } from "./ui";

export function Frame({ t }: { t: T }) {
  return (
    <div className="ov-frame" aria-hidden="true">
      <div className="ov-frame-tab">{t("frameTab")}</div>
    </div>
  );
}

export function Bar(props: {
  t: T;
  platform: Permissions["platform"];
  state: StateEvent;
  /** Tauri penceresinde sürükleme bölgesi işaretlenir. */
  native: boolean;
  onStop: () => void;
  onRevoke: () => void;
}) {
  const { t, state } = props;
  const s = state.session;
  const control = state.control === "active";
  const elapsed = useElapsed(state.started_at);
  const agent = s?.agent.name ?? "";
  const drag = props.native ? { "data-tauri-drag-region": true } : {};

  return (
    <div className="ov-bar-wrap">
      <div className="ov-bar" role="status" aria-live="polite" {...drag}>
        <span className="ov-grip" title={t("barDrag")} {...drag}>
          <Grip />
        </span>
        <div className="ov-avatar" {...drag}>
          {s?.agent.avatar_url ? <img src={s.agent.avatar_url} alt="" /> : initials(agent)}
          <span className="ov-avatar-dot">
            <span />
          </span>
        </div>
        <div className="ov-text" {...drag}>
          <span className="ov-headline">
            {control ? t("barControlling", { agent }) : t("barSeeing", { agent })}
          </span>
          <span className="ov-sub">{t("barVerified", { org: s?.organization.name ?? "" })}</span>
        </div>
        <span className={control ? "ov-chip on" : "ov-chip"}>
          <Cursor width={14} height={9} />
          {control ? t("chipAgentControl") : t("chipViewOnly")}
        </span>
        <span className="ov-time">{clock(elapsed)}</span>
        {control && (
          <button className="ov-revoke" onClick={props.onRevoke}>
            {t("revoke")}
          </button>
        )}
        <button className="ov-stop" onClick={props.onStop}>
          <StopSquare />
          {t("stop")}
        </button>
      </div>
      <div className="ov-hint">{props.platform === "darwin" ? t("barHintMac") : t("barHintWin")}</div>
    </div>
  );
}
