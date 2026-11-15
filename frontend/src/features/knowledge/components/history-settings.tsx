"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  historyKeys,
  historyService,
  type HistorySettingsInput,
} from "../services/history.service";

const NUMBER_FIELDS = [
  { key: "evidence_threshold", min: 2, max: 100, step: 1 },
  { key: "cluster_similarity", min: 0.5, max: 0.99, step: 0.01 },
  { key: "resolution_similarity", min: 0.5, max: 0.99, step: 0.01 },
  { key: "rejection_similarity", min: 0.5, max: 0.99, step: 0.01 },
  { key: "raw_retention_hours", min: 1, max: 720, step: 1 },
  { key: "archive_retention_days", min: 0, max: 3650, step: 1 },
] as const;

/** Evidence threshold, similarities, retention and the nightly run. */
export function HistorySettingsCard({ canManage }: { canManage: boolean }) {
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  const settings = useQuery({
    queryKey: historyKeys.settings(slug),
    queryFn: () => historyService.settings(slug),
    enabled: Boolean(slug),
  });
  if (!settings.data) return null;
  const { nightly_watermark: _w, updated_at, ...input } = settings.data;
  // Re-keyed on save so the form restarts from the stored values.
  return (
    <SettingsForm
      key={updated_at ?? "defaults"}
      slug={slug}
      initial={input}
      canManage={canManage}
    />
  );
}

function SettingsForm({
  slug,
  initial,
  canManage,
}: {
  slug: string;
  initial: HistorySettingsInput;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [form, setForm] = useState<HistorySettingsInput>(initial);
  const save = useMutation({
    mutationFn: (body: HistorySettingsInput) =>
      historyService.saveSettings(slug, body),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: historyKeys.settings(slug) });
      appToast.success(t("knowledge.history.settings_saved"));
    },
  });
  const set = <K extends keyof HistorySettingsInput>(
    key: K,
    value: HistorySettingsInput[K],
  ) => setForm({ ...form, [key]: value });

  return (
    <section className="space-y-3">
      <h3 className="font-medium">{t("knowledge.history.settings")}</h3>
      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="grid gap-4 sm:grid-cols-2">
            {NUMBER_FIELDS.map((f) => (
              <div key={f.key} className="space-y-1">
                <Label htmlFor={`history-${f.key}`}>
                  {t(`knowledge.history.field.${f.key}`)}
                </Label>
                <Input
                  id={`history-${f.key}`}
                  type="number"
                  min={f.min}
                  max={f.max}
                  step={f.step}
                  disabled={!canManage}
                  value={form[f.key]}
                  onChange={(e) => set(f.key, Number(e.target.value))}
                />
                <p className="text-muted-foreground text-xs">
                  {t(`knowledge.history.hint.${f.key}`)}
                </p>
              </div>
            ))}
          </div>
          <div className="space-y-2 border-t pt-4">
            <div className="flex items-center gap-2">
              <Switch
                id="history-nightly"
                checked={form.nightly_enabled}
                disabled={!canManage}
                onCheckedChange={(v) =>
                  setForm({
                    ...form,
                    nightly_enabled: v,
                    nightly_consent_analysis: v
                      ? form.nightly_consent_analysis
                      : false,
                  })
                }
              />
              <Label htmlFor="history-nightly">
                {t("knowledge.history.nightly")}
              </Label>
            </div>
            {form.nightly_enabled && (
              <>
                <div className="flex items-start gap-2">
                  <Checkbox
                    id="history-nightly-analysis"
                    checked={form.nightly_consent_analysis}
                    disabled={!canManage}
                    onCheckedChange={(v) =>
                      set("nightly_consent_analysis", v === true)
                    }
                  />
                  <Label
                    htmlFor="history-nightly-analysis"
                    className="leading-snug font-normal"
                  >
                    {t("knowledge.history.nightly_consent_analysis")}
                  </Label>
                </div>
                <div className="flex items-start gap-2">
                  <Checkbox
                    id="history-nightly-training"
                    checked={form.nightly_consent_training}
                    disabled={!canManage}
                    onCheckedChange={(v) =>
                      set("nightly_consent_training", v === true)
                    }
                  />
                  <Label
                    htmlFor="history-nightly-training"
                    className="leading-snug font-normal"
                  >
                    {t("knowledge.history.consent_training")}
                  </Label>
                </div>
              </>
            )}
          </div>
          {canManage && (
            <Button
              size="sm"
              disabled={
                save.isPending ||
                (form.nightly_enabled && !form.nightly_consent_analysis)
              }
              onClick={() => save.mutate(form)}
            >
              {t("knowledge.save")}
            </Button>
          )}
        </CardContent>
      </Card>
    </section>
  );
}
