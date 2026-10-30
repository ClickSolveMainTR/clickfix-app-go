"use client";

import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useHealthSettings,
  useSaveHealthSettings,
} from "../hooks/use-company-health";
import type { CustomerHealthSettings } from "../services/health.service";

const SIGNALS = [
  "csat",
  "open_conversations",
  "sla_breaches",
  "frequency_trend",
  "billing",
  "usage",
] as const;

/** Settings of the customer health score and churn-risk alerts (CF-87). */
export function CustomerHealthSettingsPage() {
  const { t } = useLocale();
  const { active } = useActiveOrganization();
  const perms = active?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantContactsRead);
  const canWrite = perms.includes(Permission.TenantSettingsWrite);
  const q = useHealthSettings();

  if (!canRead) {
    return (
      <p className="text-muted-foreground p-8 text-sm">
        {t("contacts.no_permission")}
      </p>
    );
  }
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <PageHeader
        title={t("contacts.health.settings.title")}
        description={t("contacts.health.settings.description")}
      />
      {q.isLoading ? (
        <Loading />
      ) : q.isError || !q.data ? (
        <ErrorState
          title={t("contacts.health.settings.error")}
          onRetry={() => void q.refetch()}
        />
      ) : (
        <SettingsForm
          key={JSON.stringify(q.data)}
          initial={q.data}
          canWrite={canWrite}
        />
      )}
    </div>
  );
}

function SettingsForm({
  initial,
  canWrite,
}: {
  initial: CustomerHealthSettings;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const save = useSaveHealthSettings();
  const [form, setForm] = useState(initial);
  const total = SIGNALS.reduce((n, k) => n + (form.weights[k] ?? 0), 0);

  return (
    <Card>
      <CardContent className="grid gap-5 p-5">
        <div className="flex items-center justify-between gap-4">
          <div>
            <Label htmlFor="health-enabled">
              {t("contacts.health.settings.enabled")}
            </Label>
            <p className="text-muted-foreground text-xs">
              {t("contacts.health.settings.enabled_hint")}
            </p>
          </div>
          <Switch
            id="health-enabled"
            checked={form.enabled}
            disabled={!canWrite}
            onCheckedChange={(v) => setForm((f) => ({ ...f, enabled: v }))}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="health-threshold">
            {t("contacts.health.settings.threshold")}
          </Label>
          <Input
            id="health-threshold"
            type="number"
            min={0}
            max={100}
            className="w-28"
            disabled={!canWrite}
            value={form.threshold}
            onChange={(e) =>
              setForm((f) => ({
                ...f,
                threshold: Math.min(
                  100,
                  Math.max(0, Number(e.target.value) || 0),
                ),
              }))
            }
          />
          <p className="text-muted-foreground text-xs">
            {t("contacts.health.settings.threshold_hint")}
          </p>
        </div>
        <div className="grid gap-3">
          <div>
            <p className="text-sm font-medium">
              {t("contacts.health.settings.weights")}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("contacts.health.settings.weights_hint")}
            </p>
          </div>
          {SIGNALS.map((key) => {
            const w = form.weights[key] ?? 0;
            return (
              <div
                key={key}
                className="grid grid-cols-[10rem_1fr_3.5rem] items-center gap-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm">
                    {t(`contacts.health.signal.${key}`)}
                  </p>
                  <p className="text-muted-foreground truncate text-[11px]">
                    {t(`contacts.health.signal_hint.${key}`)}
                  </p>
                </div>
                <Slider
                  value={[w]}
                  min={0}
                  max={100}
                  step={5}
                  disabled={!canWrite}
                  onValueChange={([v]) =>
                    setForm((f) => ({
                      ...f,
                      weights: { ...f.weights, [key]: v ?? 0 },
                    }))
                  }
                />
                <span className="text-muted-foreground text-right text-xs tabular-nums">
                  {total > 0 ? Math.round((w / total) * 100) : 0}%
                </span>
              </div>
            );
          })}
        </div>
        {canWrite && (
          <div className="flex justify-end">
            <Button
              disabled={save.isPending}
              onClick={() =>
                save.mutate(form, {
                  onSuccess: () =>
                    appToast.success(t("contacts.health.settings.saved")),
                })
              }
            >
              {t("contacts.save")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
