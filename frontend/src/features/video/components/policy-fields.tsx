"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";

import type { VideoPolicy } from "../services/video.service";

export const POLICY_SWITCHES = [
  "enabled",
  "allow_camera",
  "allow_screen_share",
  "allow_chat",
  "allow_recording",
  "allow_guest_join",
  "require_pin",
  "require_lobby",
  "e2ee_required",
] as const;

export const POLICY_NUMBERS = [
  "max_duration_minutes",
  "max_participants",
  "recording_retention_hours",
  "max_concurrent_recordings",
  "max_recording_minutes",
  "low_balance_minutes",
] as const;

type SwitchKey = (typeof POLICY_SWITCHES)[number];
type NumberKey = (typeof POLICY_NUMBERS)[number];
export type PolicyDraft = Partial<
  Record<SwitchKey, boolean> & Record<NumberKey, number>
>;

/**
 * Policy editor shared by the platform page (full policy, plan overrides)
 * and the organization page (tighten-only: `limits` bounds every field).
 */
export function PolicyFields({
  value,
  onChange,
  limits,
  partial,
  disabled,
}: {
  value: PolicyDraft;
  onChange: (next: PolicyDraft) => void;
  /** Organization mode: values the organization can only tighten. */
  limits?: VideoPolicy;
  /** Plan override mode: unset fields inherit (shown as "inherit"). */
  partial?: boolean;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const set = (k: keyof PolicyDraft, v: boolean | number | undefined) => {
    const next = { ...value };
    if (v === undefined) delete next[k];
    else (next as Record<string, boolean | number>)[k] = v;
    onChange(next);
  };
  // Organizations may only switch off allow_* / enabled and switch on require_*.
  const locked = (k: SwitchKey) => {
    if (!limits) return false;
    if (k.startsWith("allow_") || k === "enabled") return !limits[k];
    return Boolean(limits[k]);
  };
  return (
    <div className="grid gap-4 md:grid-cols-2">
      {POLICY_SWITCHES.map((k) => (
        <label
          key={k}
          className="flex items-start justify-between gap-3 rounded-md border p-3 text-sm"
        >
          <span>
            {t(`video.policy.${k}`)}
            <span className="text-muted-foreground block text-xs">
              {t(`video.policy.${k}_hint`)}
            </span>
          </span>
          <Switch
            disabled={disabled || locked(k)}
            checked={value[k] ?? (limits ? limits[k] : false)}
            onCheckedChange={(v) => set(k, v)}
          />
        </label>
      ))}
      {POLICY_NUMBERS.map((k) => (
        <div key={k} className="space-y-1.5">
          <Label htmlFor={`policy-${k}`}>
            {t(`video.policy.${k}`)}
            {limits ? ` (≤ ${limits[k]})` : ""}
          </Label>
          <Input
            id={`policy-${k}`}
            type="number"
            min={0}
            max={limits ? limits[k] : undefined}
            disabled={disabled}
            placeholder={partial ? t("video.policy.inherit") : undefined}
            value={value[k] ?? ""}
            onChange={(e) =>
              set(k, e.target.value === "" ? undefined : Number(e.target.value))
            }
          />
        </div>
      ))}
    </div>
  );
}
