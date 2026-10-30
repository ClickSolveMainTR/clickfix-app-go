"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  useMemberDirectory,
  type DirectoryOption,
} from "@/features/tenant-members/hooks/use-member-directory";
import { useLocale } from "@/providers/locale-provider";

import type {
  EscalationChannel,
  EscalationPolicy,
  EscalationPolicyInput,
  OncallRota,
} from "../services/oncall.service";

export const CHANNELS: EscalationChannel[] = ["inapp", "push", "email", "sms"];
export type TargetKind = "agent" | "rota" | "team";
const TARGETS: TargetKind[] = ["rota", "agent", "team"];

type StepDraft = {
  after: string;
  kind: TargetKind;
  target: string;
  channels: EscalationChannel[];
};

const PICK = "__pick__";

/**
 * Escalation policy: ordered steps (minutes after the alert, exactly one
 * target, notification channels) and which events open alerts.
 */
export function PolicyDialog({
  policy,
  rotas,
  pending,
  onOpenChange,
  onSubmit,
}: {
  policy: EscalationPolicy | null;
  rotas: OncallRota[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: EscalationPolicyInput) => void;
}) {
  const { t } = useLocale();
  const directory = useMemberDirectory();
  const [name, setName] = useState(policy?.name ?? "");
  const [enabled, setEnabled] = useState(policy?.enabled ?? true);
  const [incidents, setIncidents] = useState(policy?.incidents ?? true);
  const [urgent, setUrgent] = useState(policy?.urgent_conversations ?? false);
  const [steps, setSteps] = useState<StepDraft[]>(
    policy?.steps.length
      ? policy.steps.map((s) => ({
          after: String(s.after_minutes),
          kind: s.rota ? "rota" : s.team ? "team" : "agent",
          target: s.rota ?? s.team ?? s.agent ?? "",
          channels: s.channels,
        }))
      : [{ after: "0", kind: "rota", target: "", channels: ["inapp", "push"] }],
  );

  const options: Record<TargetKind, DirectoryOption[]> = {
    agent: directory.members,
    team: directory.teams,
    rota: rotas.map((r) => ({ value: r.uuid, label: r.name })),
  };
  const minutes = steps.map((s) => Number(s.after));
  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 120 &&
    steps.length > 0 &&
    steps.every(
      (s, i) =>
        s.target &&
        s.channels.length > 0 &&
        Number.isInteger(minutes[i]) &&
        minutes[i]! >= 0 &&
        minutes[i]! <= 10080 &&
        (i === 0 || minutes[i]! >= minutes[i - 1]!),
    );

  const patch = (i: number, p: Partial<StepDraft>) =>
    setSteps((list) => list.map((s, j) => (j === i ? { ...s, ...p } : s)));

  const submit = () =>
    onSubmit({
      name: name.trim(),
      enabled,
      incidents,
      urgent_conversations: urgent,
      steps: steps.map((s) => ({
        after_minutes: Number(s.after),
        [s.kind]: s.target,
        channels: CHANNELS.filter((c) => s.channels.includes(c)),
      })),
    });

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t(policy ? "oncall.policies.edit" : "oncall.policies.new")}
      description={t("oncall.policies.form_hint")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="policy-name">{t("oncall.fields.name")}</Label>
          <Input
            id="policy-name"
            value={name}
            maxLength={120}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          {(
            [
              [
                "policy-enabled",
                enabled,
                setEnabled,
                "oncall.policies.enabled",
              ],
              [
                "policy-incidents",
                incidents,
                setIncidents,
                "oncall.policies.incidents",
              ],
              ["policy-urgent", urgent, setUrgent, "oncall.policies.urgent"],
            ] as const
          ).map(([id, checked, set, label]) => (
            <div key={id} className="flex items-center gap-2">
              <Switch id={id} checked={checked} onCheckedChange={set} />
              <Label htmlFor={id}>{t(label)}</Label>
            </div>
          ))}
        </div>

        <section className="grid gap-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("oncall.policies.steps")}
            </h3>
            <Button
              size="sm"
              variant="outline"
              disabled={steps.length >= 20}
              onClick={() =>
                setSteps((list) => [
                  ...list,
                  {
                    after: String((minutes[minutes.length - 1] ?? 0) + 15),
                    kind: "agent",
                    target: "",
                    channels: ["inapp", "push", "email"],
                  },
                ])
              }
            >
              <Plus className="size-4" />
              {t("oncall.policies.add_step")}
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            {t("oncall.policies.steps_hint")}
          </p>
          {steps.map((s, i) => (
            <div key={i} className="grid gap-3 rounded-md border p-3">
              <div className="grid items-end gap-3 sm:grid-cols-[7rem_9rem_1fr_auto]">
                <div className="grid gap-1.5">
                  <Label htmlFor={`step-${i}-after`} className="text-xs">
                    {t("oncall.fields.after_minutes")}
                  </Label>
                  <Input
                    id={`step-${i}-after`}
                    type="number"
                    min={0}
                    max={10080}
                    value={s.after}
                    onChange={(e) => patch(i, { after: e.target.value })}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label className="text-xs">
                    {t("oncall.fields.target_kind")}
                  </Label>
                  <Select
                    value={s.kind}
                    onValueChange={(v) =>
                      patch(i, { kind: v as TargetKind, target: "" })
                    }
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {TARGETS.map((k) => (
                        <SelectItem key={k} value={k}>
                          {t(`oncall.target.${k}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="grid gap-1.5">
                  <Label className="text-xs">{t("oncall.fields.target")}</Label>
                  <Select
                    value={s.target || PICK}
                    onValueChange={(v) =>
                      patch(i, { target: v === PICK ? "" : v })
                    }
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={PICK} disabled>
                        {t("oncall.fields.pick_target")}
                      </SelectItem>
                      {options[s.kind].map((o) => (
                        <SelectItem key={o.value} value={o.value}>
                          {o.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <Button
                  size="icon"
                  variant="ghost"
                  disabled={steps.length === 1}
                  aria-label={t("oncall.policies.remove_step")}
                  onClick={() =>
                    setSteps((list) => list.filter((_, j) => j !== i))
                  }
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-2">
                {CHANNELS.map((c) => (
                  <label
                    key={c}
                    htmlFor={`step-${i}-${c}`}
                    className="flex items-center gap-2 text-sm"
                  >
                    <Checkbox
                      id={`step-${i}-${c}`}
                      checked={s.channels.includes(c)}
                      onCheckedChange={(on) =>
                        patch(i, {
                          channels: on
                            ? [...s.channels, c]
                            : s.channels.filter((x) => x !== c),
                        })
                      }
                    />
                    {t(`oncall.channel.${c}`)}
                  </label>
                ))}
              </div>
            </div>
          ))}
        </section>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button disabled={pending || !valid} onClick={submit}>
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
