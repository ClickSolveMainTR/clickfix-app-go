"use client";

import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
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
import { useLocale } from "@/providers/locale-provider";

import { METRIC_MAX, METRICS } from "../lib/format";
import type {
  GameMetric,
  GameRule,
  GameRuleInput,
} from "../services/coaching.service";

function isInt(raw: string, min: number, max: number) {
  const n = Number(raw);
  return raw.trim() !== "" && Number.isInteger(n) && n >= min && n <= max;
}

/**
 * Badge rule: a KPI threshold over the leaderboard window that awards the
 * badge and its points. Rules are saved by name, so an existing rule keeps
 * its name (renaming would create another rule).
 */
export function RuleDialog({
  rule,
  names,
  pending,
  onOpenChange,
  onSubmit,
}: {
  rule: GameRule | null;
  /** Names already used, to keep "new" from overwriting a rule. */
  names: string[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: GameRuleInput) => void;
}) {
  const { t } = useLocale();
  const [name, setName] = useState(rule?.name ?? "");
  const [metric, setMetric] = useState<GameMetric>(
    rule?.metric ?? "resolutions",
  );
  const [threshold, setThreshold] = useState(String(rule?.threshold ?? 10));
  const [points, setPoints] = useState(String(rule?.points ?? 10));
  const [minSamples, setMinSamples] = useState(String(rule?.min_samples ?? 1));
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);

  const trimmed = name.trim();
  const duplicate = !rule && names.includes(trimmed);
  const thresholdNum = Number(threshold);
  const valid =
    trimmed.length > 0 &&
    trimmed.length <= 200 &&
    !duplicate &&
    threshold.trim() !== "" &&
    Number.isFinite(thresholdNum) &&
    thresholdNum >= 0 &&
    thresholdNum <= METRIC_MAX[metric] &&
    isInt(points, 0, 10000) &&
    isInt(minSamples, 1, 100000);

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t(rule ? "coaching.rules.edit" : "coaching.rules.new")}
      description={t("coaching.rules.form_hint")}
      className="sm:max-w-lg"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="rule-name">{t("coaching.rules.name")}</Label>
          <Input
            id="rule-name"
            value={name}
            maxLength={200}
            disabled={Boolean(rule)}
            aria-invalid={duplicate}
            onChange={(e) => setName(e.target.value)}
          />
          {duplicate && (
            <p className="text-destructive text-xs">
              {t("coaching.rules.duplicate")}
            </p>
          )}
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="rule-metric">{t("coaching.rules.metric")}</Label>
            <Select
              value={metric}
              onValueChange={(v) => setMetric(v as GameMetric)}
            >
              <SelectTrigger id="rule-metric" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {METRICS.map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(`coaching.metric.${m}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="rule-threshold">
              {t("coaching.rules.threshold")}
            </Label>
            <Input
              id="rule-threshold"
              type="number"
              inputMode="decimal"
              min={0}
              max={METRIC_MAX[metric]}
              step={metric === "csat" ? 0.1 : 1}
              value={threshold}
              onChange={(e) => setThreshold(e.target.value)}
            />
          </div>
        </div>
        <p className="text-muted-foreground -mt-2 text-xs">
          {t(`coaching.metric_hint.${metric}`)}
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="rule-points">{t("coaching.rules.points")}</Label>
            <Input
              id="rule-points"
              type="number"
              min={0}
              max={10000}
              value={points}
              onChange={(e) => setPoints(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="rule-samples">
              {t("coaching.rules.min_samples")}
            </Label>
            <Input
              id="rule-samples"
              type="number"
              min={1}
              max={100000}
              value={minSamples}
              onChange={(e) => setMinSamples(e.target.value)}
            />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="rule-enabled"
            checked={enabled}
            onCheckedChange={setEnabled}
          />
          <Label htmlFor="rule-enabled">{t("coaching.rules.enabled")}</Label>
        </div>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button
          disabled={pending || !valid}
          onClick={() =>
            onSubmit({
              name: trimmed,
              metric,
              threshold: thresholdNum,
              points: Number(points),
              min_samples: Number(minSamples),
              enabled,
            })
          }
        >
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
