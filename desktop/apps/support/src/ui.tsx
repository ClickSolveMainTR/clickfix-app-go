// Ekranların ortak parçaları.

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

import type { T } from "./i18n";
import { initials } from "./i18n";
import type { Permissions, ResolvedSession } from "./ipc";
import { Shield, Tile } from "./icons";

export interface UiCtx {
  t: T;
  platform: Permissions["platform"];
  version: string;
}

export const Ui = createContext<UiCtx | null>(null);

export function useUi(): UiCtx {
  const c = useContext(Ui);
  if (!c) throw new Error("Ui bağlamı yok");
  return c;
}

/** `**kalın**` işaretini <strong>'a çevirir (metinler i18n.ts'te). */
export function Rich({ text }: { text: string }) {
  const parts = text.split("**");
  return (
    <>
      {parts.map((p, i) => (i % 2 === 1 ? <strong key={i}>{p}</strong> : <span key={i}>{p}</span>))}
    </>
  );
}

export function Brand() {
  const { t } = useUi();
  return (
    <div className="brand">
      <Tile size={34} label="ClickFix" />
      <span className="wordmark">clickfix</span>
      <span className="pill">{t("badge")}</span>
    </div>
  );
}

export function Warning({ strong, large }: { strong?: string; large?: boolean }) {
  const { t } = useUi();
  return (
    <div className={large ? "warning lg" : "warning"}>
      <Shield size={large ? 20 : 18} />
      <p>
        {strong && <strong>{strong} </strong>}
        {t("warning")}
      </p>
    </div>
  );
}

export type StepKey = "identity" | "view" | "control" | "recording";

/** Oturuma göre görünen adımlar: kontrol ve kayıt yalnız istendiyse. */
export function stepsFor(s: ResolvedSession): StepKey[] {
  const steps: StepKey[] = ["identity", "view"];
  if (s.mode_requested === "control") steps.push("control");
  if (s.recording_requested) steps.push("recording");
  return steps;
}

const STEP_LABEL = {
  identity: "stepIdentity",
  view: "stepView",
  control: "stepControl",
  recording: "stepRecording",
} as const;

export function Steps({ steps, active }: { steps: StepKey[]; active: StepKey[] }) {
  const { t } = useUi();
  const firstActive = Math.min(...active.map((a) => steps.indexOf(a)));
  return (
    <div className="steps" aria-label={active.map((a) => t(STEP_LABEL[a])).join(", ")}>
      {steps.map((s, i) => (
        <span
          key={s}
          className={active.includes(s) ? "step active" : i < firstActive ? "step done" : "step"}
          aria-current={active.includes(s) ? "step" : undefined}
        >
          {i + 1} {t(STEP_LABEL[s])}
        </span>
      ))}
    </div>
  );
}

export function Avatar({
  name,
  url,
  size,
}: {
  name: string;
  url: string | null;
  size?: "sm" | "md";
}) {
  return (
    <div className={size ? `avatar ${size}` : "avatar"} aria-hidden="true">
      {url ? <img src={url} alt="" /> : initials(name)}
    </div>
  );
}

export function OrgLogo({ name, url }: { name: string; url: string | null }) {
  return (
    <div className="org-logo" aria-hidden="true">
      {url ? <img src={url} alt="" /> : initials(name)}
    </div>
  );
}

/** Kısayol tuşları (macOS simgeleri ya da Windows adları). */
export function ShortcutKeys() {
  const { platform } = useUi();
  const keys = platform === "darwin" ? ["⌃", "⌥", "⇧", "S"] : ["Ctrl", "Alt", "Shift", "S"];
  return (
    <span className="keys">
      {keys.map((k) => (
        <kbd key={k}>{k}</kbd>
      ))}
    </span>
  );
}

/** `started_at`'tan bu yana geçen süre, saniyede bir güncellenir. */
export function useElapsed(startedAt: string | null): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!startedAt) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [startedAt]);
  if (!startedAt) return 0;
  return Math.max(0, Math.floor((now - Date.parse(startedAt)) / 1000));
}

export function clock(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
}

export function Screen({ className, children }: { className?: string; children: ReactNode }) {
  return <main className={className ? `screen ${className}` : "screen"}>{children}</main>;
}
