"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MonitorUp } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { errorKey } from "../lib/access";
import {
  remoteSupportKeys,
  useRemoteSupportScope,
} from "../lib/use-remote-support";
import {
  remoteSupportService,
  type RemoteSupportSettings,
} from "../services/remote-support.service";

/**
 * Organization remote support settings (tenant.remote_support.settings):
 * on/off (off by default), control, recording, longest session and
 * recording retention. Explains why sessions cannot start (media server,
 * plan, platform kill switch).
 */
export function RemoteSupportSettingsPage() {
  const { t } = useLocale();
  const { slug, settings: canManage } = useRemoteSupportScope();
  const query = useQuery({
    queryKey: remoteSupportKeys.settings(slug),
    queryFn: () => remoteSupportService.settings(slug),
    enabled: Boolean(slug && canManage),
  });
  if (!canManage) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("remote_support.forbidden")}
      </p>
    );
  }
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MonitorUp className="size-7" />}
        title={t("remote_support.settings.title")}
        description={t("remote_support.settings.description")}
      />
      {query.data ? <Readiness s={query.data} /> : null}
      {query.data ? <SettingsForm initial={query.data} /> : null}
    </div>
  );
}

function Readiness({ s }: { s: RemoteSupportSettings }) {
  const { t } = useLocale();
  const problems: string[] = [];
  if (!s.configured) problems.push(t("remote_support.settings.not_configured"));
  if (s.platform_state !== "active")
    problems.push(
      t(`remote_support.settings.platform_${s.platform_state}`, {
        reason: s.platform_reason || "—",
      }),
    );
  if (!s.enabled) problems.push(t("remote_support.settings.off_hint"));
  return (
    <div className="space-y-2">
      {!s.entitled ? (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardContent className="space-y-1 p-4 text-sm" role="status">
            <p className="font-medium">
              {t("remote_support.settings.not_entitled")}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("remote_support.settings.upgrade_hint")}{" "}
              <Link className="underline" href={routes.cms.billing}>
                {t("remote_support.settings.upgrade_link")}
              </Link>
            </p>
          </CardContent>
        </Card>
      ) : null}
      {problems.length ? (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardContent className="p-4 text-sm" role="status">
            <ul className="list-disc space-y-1 pl-5">
              {problems.map((p) => (
                <li key={p}>{p}</li>
              ))}
            </ul>
          </CardContent>
        </Card>
      ) : s.ready ? (
        <p
          className="rounded-md bg-emerald-500/10 px-3 py-2 text-sm text-emerald-800 dark:text-emerald-200"
          role="status"
        >
          {t("remote_support.settings.ready")}
        </p>
      ) : null}
    </div>
  );
}

function SettingsForm({ initial }: { initial: RemoteSupportSettings }) {
  const { t } = useLocale();
  const { slug } = useRemoteSupportScope();
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(initial.enabled);
  const [allowControl, setAllowControl] = useState(initial.allow_control);
  const [recording, setRecording] = useState(initial.recording);
  const [maxMinutes, setMaxMinutes] = useState(
    String(initial.max_duration_minutes),
  );
  const [retention, setRetention] = useState(
    String(initial.recording_retention_days),
  );
  const minutes = Number(maxMinutes);
  const days = Number(retention);
  const invalidMinutes =
    !Number.isInteger(minutes) || minutes < 5 || minutes > 240;
  const invalidDays = !Number.isInteger(days) || days < 1 || days > 365;
  const save = useMutation({
    mutationFn: () =>
      remoteSupportService.saveSettings(slug, {
        enabled,
        allow_control: allowControl,
        recording,
        max_duration_minutes: minutes,
        recording_retention_days: days,
      }),
    onSuccess: (next) => {
      qc.setQueryData(remoteSupportKeys.settings(slug), next);
      appToast.success(t("remote_support.settings.saved"));
    },
    onError: (err) =>
      appToast.error(t(`remote_support.errors.${errorKey(err)}`)),
  });
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (!invalidMinutes && !invalidDays) save.mutate();
      }}
    >
      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="flex items-start justify-between gap-3">
            <Label htmlFor="rs-enabled" className="block">
              {t("remote_support.settings.enabled")}
              <span className="text-muted-foreground block text-xs font-normal">
                {t("remote_support.settings.enabled_hint")}
              </span>
            </Label>
            <Switch
              id="rs-enabled"
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
          <div className="flex items-start justify-between gap-3">
            <Label htmlFor="rs-control" className="block">
              {t("remote_support.settings.allow_control")}
              <span className="text-muted-foreground block text-xs font-normal">
                {t("remote_support.settings.allow_control_hint")}
              </span>
            </Label>
            <Switch
              id="rs-control"
              checked={allowControl}
              onCheckedChange={setAllowControl}
            />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="rs-max">
                {t("remote_support.settings.max_duration")}
              </Label>
              <Input
                id="rs-max"
                type="number"
                inputMode="numeric"
                min={5}
                max={240}
                value={maxMinutes}
                aria-invalid={invalidMinutes}
                aria-describedby="rs-max-hint"
                onChange={(e) => setMaxMinutes(e.target.value)}
              />
              <p id="rs-max-hint" className="text-muted-foreground text-xs">
                {t("remote_support.settings.max_duration_hint")}
              </p>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("remote_support.settings.recording_title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="rs-recording">
              {t("remote_support.settings.recording")}
            </Label>
            <Select
              value={recording}
              onValueChange={(v) => setRecording(v as "off" | "optional")}
            >
              <SelectTrigger id="rs-recording" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="off">
                  {t("remote_support.settings.recording_off")}
                </SelectItem>
                <SelectItem value="optional">
                  {t("remote_support.settings.recording_optional")}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className="text-muted-foreground text-xs">
              {t("remote_support.settings.recording_hint")}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="rs-retention">
              {t("remote_support.settings.retention")}
            </Label>
            <Input
              id="rs-retention"
              type="number"
              inputMode="numeric"
              min={1}
              max={365}
              value={retention}
              disabled={recording === "off"}
              aria-invalid={invalidDays}
              aria-describedby="rs-retention-hint"
              onChange={(e) => setRetention(e.target.value)}
            />
            <p id="rs-retention-hint" className="text-muted-foreground text-xs">
              {t("remote_support.settings.retention_hint")}
            </p>
          </div>
        </CardContent>
      </Card>
      <Button
        type="submit"
        disabled={save.isPending || invalidMinutes || invalidDays}
      >
        {t("remote_support.settings.save")}
      </Button>
    </form>
  );
}
