"use client";

import { BellRing, RefreshCw } from "lucide-react";
import { useId, useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  useCreditPackages,
  useSaveWalletSettings,
} from "@/features/billing/hooks/use-tenant-billing";
import { formatCount, formatMinor } from "@/features/billing/lib/money";
import type { TenantWalletSettings } from "@/features/billing/services/tenant-billing.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function parseLevel(raw: string): number | null {
  const text = raw.trim().replace(/[.\s]/g, "");
  if (!/^\d+$/.test(text)) return null;
  const n = Number(text);
  return Number.isSafeInteger(n) ? n : null;
}

function SettingsForm({
  settings,
  canWrite,
  recommended,
}: {
  settings: TenantWalletSettings;
  canWrite: boolean;
  recommended: number | null;
}) {
  const { t, locale } = useLocale();
  const id = useId();
  const packages = useCreditPackages();
  const save = useSaveWalletSettings();
  const packs = (packages.data?.items ?? []).filter((p) => p.active !== false);

  const [alertMode, setAlertMode] = useState<"recommended" | "custom">(
    settings.alert_threshold === null ? "recommended" : "custom",
  );
  const [alertRaw, setAlertRaw] = useState(
    settings.alert_threshold === null ? "" : String(settings.alert_threshold),
  );
  const [autoTopup, setAutoTopup] = useState(settings.auto_topup);
  const [packageUuid, setPackageUuid] = useState(settings.package_uuid ?? "");
  const [thresholdRaw, setThresholdRaw] = useState(String(settings.threshold));
  // Consent is stored when auto top-up is saved; keep it ticked while on.
  const [consent, setConsent] = useState(settings.auto_topup);

  const pack = packs.find((p) => p.uuid === packageUuid);
  const alertLevel = alertMode === "custom" ? parseLevel(alertRaw) : null;
  const threshold = parseLevel(thresholdRaw);
  const alertInvalid = alertMode === "custom" && alertLevel === null;
  const thresholdInvalid = autoTopup && threshold === null;
  // Turning it on needs an auto-pay card; turning it off is always allowed.
  const toggleDisabled =
    !canWrite || (!settings.auto_topup_available && !autoTopup);
  const valid =
    !alertInvalid &&
    !thresholdInvalid &&
    (!autoTopup || (Boolean(pack) && consent));

  return (
    <form
      className="space-y-6"
      onSubmit={(e) => {
        e.preventDefault();
        if (!valid || !canWrite) return;
        save.mutate({
          alert_threshold: alertLevel,
          auto_topup: autoTopup,
          threshold: threshold ?? 0,
          package_uuid: autoTopup ? packageUuid : undefined,
          consent: autoTopup ? consent : false,
        });
      }}
    >
      <fieldset className="space-y-3" disabled={!canWrite}>
        <legend className="flex items-center gap-2 text-sm font-medium">
          <BellRing className="text-muted-foreground size-4" />
          {t("wallet.settings.alert.title")}
        </legend>
        <p className="text-muted-foreground text-xs">
          {t("wallet.settings.alert.hint")}
        </p>
        <RadioGroup
          value={alertMode}
          onValueChange={(v) =>
            setAlertMode(v === "custom" ? "custom" : "recommended")
          }
          className="gap-2"
          disabled={!canWrite}
        >
          <Label className="flex items-start gap-3 rounded-md border p-2.5 font-normal">
            <RadioGroupItem value="recommended" className="mt-0.5" />
            <span>
              <span className="block font-medium">
                {t("wallet.settings.alert.recommended")}
              </span>
              <span className="text-muted-foreground text-xs">
                {recommended !== null
                  ? t("wallet.settings.alert.recommended_value", {
                      value: formatCount(recommended, locale),
                    })
                  : t("wallet.settings.alert.recommended_hint")}
              </span>
            </span>
          </Label>
          <Label className="flex items-center gap-3 rounded-md border p-2.5 font-normal">
            <RadioGroupItem value="custom" />
            {t("wallet.settings.alert.custom")}
          </Label>
        </RadioGroup>
        {alertMode === "custom" ? (
          <div className="space-y-1.5 ps-1">
            <Label htmlFor={`${id}-alert`}>
              {t("wallet.settings.alert.custom_label")}
            </Label>
            <Input
              id={`${id}-alert`}
              inputMode="numeric"
              className="max-w-40 tabular-nums"
              value={alertRaw}
              aria-invalid={alertInvalid && alertRaw !== ""}
              onChange={(e) => setAlertRaw(e.target.value)}
            />
            {alertInvalid && alertRaw !== "" ? (
              <p className="text-destructive text-xs">
                {t("wallet.settings.invalid_number")}
              </p>
            ) : null}
          </div>
        ) : null}
      </fieldset>

      <fieldset className="space-y-3 border-t pt-5" disabled={!canWrite}>
        <legend className="sr-only">{t("wallet.settings.topup.title")}</legend>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <p className="flex items-center gap-2 text-sm font-medium">
              <RefreshCw className="text-muted-foreground size-4" />
              {t("wallet.settings.topup.title")}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("wallet.settings.topup.hint")}
            </p>
          </div>
          <Switch
            checked={autoTopup}
            disabled={toggleDisabled}
            aria-label={t("wallet.settings.topup.toggle")}
            onCheckedChange={(v) => {
              setAutoTopup(v);
              if (!v) setConsent(false);
              if (v && !packageUuid && packs[0]) setPackageUuid(packs[0].uuid);
            }}
          />
        </div>
        {!settings.auto_topup_available ? (
          <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs">
            {t("wallet.settings.topup.unavailable")}
          </p>
        ) : null}
        {autoTopup ? (
          <div className="space-y-4">
            {packs.length === 0 && packages.isSuccess ? (
              <p className="text-muted-foreground text-sm">
                {t("wallet.settings.topup.no_packages")}
              </p>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor={`${id}-pack`}>
                    {t("wallet.settings.topup.package")}
                  </Label>
                  <Select value={packageUuid} onValueChange={setPackageUuid}>
                    <SelectTrigger id={`${id}-pack`} className="w-full">
                      <SelectValue
                        placeholder={t(
                          "wallet.settings.topup.package_placeholder",
                        )}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {packs.map((p) => (
                        <SelectItem key={p.uuid} value={p.uuid}>
                          {p.name} ·{" "}
                          {formatMinor(p.amount_minor, "TRY", locale)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor={`${id}-threshold`}>
                    {t("wallet.settings.topup.threshold")}
                  </Label>
                  <div className="flex items-center gap-2">
                    <Input
                      id={`${id}-threshold`}
                      inputMode="numeric"
                      className="tabular-nums"
                      value={thresholdRaw}
                      aria-invalid={thresholdInvalid}
                      onChange={(e) => setThresholdRaw(e.target.value)}
                    />
                    <span className="text-muted-foreground shrink-0 text-sm">
                      {t(
                        `wallet.settings.topup.threshold_unit.${pack?.unit ?? "ai_credits"}`,
                      )}
                    </span>
                  </div>
                  {thresholdInvalid ? (
                    <p className="text-destructive text-xs">
                      {t("wallet.settings.invalid_number")}
                    </p>
                  ) : null}
                </div>
              </div>
            )}
            <Label
              className={cn(
                "flex items-start gap-3 rounded-md border p-3 leading-snug font-normal",
                !consent && "border-amber-500/40",
              )}
            >
              <Checkbox
                checked={consent}
                onCheckedChange={(v) => setConsent(v === true)}
                className="mt-0.5"
              />
              <span className="text-sm">
                {t("wallet.settings.topup.consent")}
              </span>
            </Label>
          </div>
        ) : null}
      </fieldset>

      {canWrite ? (
        <div className="flex justify-end border-t pt-4">
          <Button type="submit" disabled={!valid || save.isPending}>
            {t("wallet.settings.save")}
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground border-t pt-4 text-xs">
          {t("wallet.settings.read_only")}
        </p>
      )}
    </form>
  );
}

/** Low AI credit alert level and auto top-up (saved card, explicit consent). */
export function WalletSettingsCard({
  settings,
  canWrite,
  recommended,
}: {
  settings: TenantWalletSettings | undefined;
  canWrite: boolean;
  /** Default alert level: a fifth of the plan's monthly AI credits. */
  recommended: number | null;
}) {
  const { t } = useLocale();
  return (
    <EntitySectionCard
      title={t("wallet.settings.title")}
      description={t("wallet.settings.description")}
    >
      {settings ? (
        <SettingsForm
          // Re-seed the form whenever the saved settings change.
          key={JSON.stringify(settings)}
          settings={settings}
          canWrite={canWrite}
          recommended={recommended}
        />
      ) : (
        <Skeleton className="h-64 w-full" />
      )}
    </EntitySectionCard>
  );
}
