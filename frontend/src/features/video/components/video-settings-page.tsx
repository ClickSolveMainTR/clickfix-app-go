"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Video } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useVideoScope,
  videoErrorKey,
  videoKeys,
} from "../lib/use-video-scope";
import { videoService, type VideoOrgSettings } from "../services/video.service";
import { PolicyFields, type PolicyDraft } from "./policy-fields";
import { RecordingsList } from "./video-room-page";
import { AvailabilityNotice } from "./video-rooms-page";

/**
 * Organization video settings (tenant.video.manage): switch the module,
 * tighten the platform/plan policy, recording storage, minutes and
 * recordings.
 */
export function VideoSettingsPage() {
  const { t } = useLocale();
  const { slug, canManage } = useVideoScope();
  const settings = useQuery({
    queryKey: videoKeys.settings(slug),
    queryFn: () => videoService.settings(slug),
    enabled: Boolean(slug && canManage),
  });
  if (!canManage) {
    return (
      <p className="text-muted-foreground text-sm">{t("video.forbidden")}</p>
    );
  }
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Video className="size-7" />}
        title={t("video.settings.title")}
        description={t("video.settings.description")}
      />
      {settings.data ? (
        <AvailabilityNotice a={settings.data.availability} />
      ) : null}
      {settings.data ? (
        <Tabs defaultValue="policy">
          <TabsList>
            <TabsTrigger value="policy">
              {t("video.settings.tabs.policy")}
            </TabsTrigger>
            <TabsTrigger value="minutes">
              {t("video.settings.tabs.minutes")}
            </TabsTrigger>
            <TabsTrigger value="recordings">
              {t("video.settings.tabs.recordings")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="policy" className="mt-4">
            <SettingsForm initial={settings.data} />
          </TabsContent>
          <TabsContent value="minutes" className="mt-4">
            <MinutesTab
              balance={settings.data.availability.balance}
              unlimited={settings.data.availability.unlimited}
            />
          </TabsContent>
          <TabsContent value="recordings" className="mt-4">
            <Card>
              <CardContent className="p-4">
                <RecordingsList />
              </CardContent>
            </Card>
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  );
}

function SettingsForm({ initial }: { initial: VideoOrgSettings }) {
  const { t } = useLocale();
  const { slug } = useVideoScope();
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(initial.enabled);
  const [policy, setPolicy] = useState<PolicyDraft>(
    initial.policy as PolicyDraft,
  );
  const [mode, setMode] = useState(initial.storage.mode);
  const [storage, setStorage] = useState({
    endpoint: initial.storage.endpoint,
    region: initial.storage.region,
    bucket: initial.storage.bucket,
    prefix: initial.storage.prefix,
    path_style: initial.storage.path_style,
    access_key: "",
    secret_key: "",
  });
  const save = useMutation({
    mutationFn: () =>
      videoService.saveSettings(slug, {
        enabled,
        policy,
        storage:
          mode === "custom" ? { mode, ...storage } : { mode: "platform" },
      }),
    onSuccess: () => {
      appToast.success(t("video.settings.saved"));
      void qc.invalidateQueries({ queryKey: ["video", slug] });
    },
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  const field = (k: keyof typeof storage, label: string, type = "text") => (
    <div className="space-y-1.5">
      <Label htmlFor={`s3-${k}`}>{label}</Label>
      <Input
        id={`s3-${k}`}
        type={type}
        autoComplete="off"
        value={String(storage[k])}
        placeholder={
          (k === "access_key" && initial.storage.has_access_key) ||
          (k === "secret_key" && initial.storage.has_secret_key)
            ? t("video.settings.storage.keep_secret")
            : undefined
        }
        onChange={(e) => setStorage({ ...storage, [k]: e.target.value })}
      />
    </div>
  );
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="space-y-4 p-4">
          <label className="flex items-center justify-between gap-3 text-sm font-medium">
            {t("video.settings.enabled")}
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </label>
          <p className="text-muted-foreground text-xs">
            {t("video.settings.tighten_hint")}
          </p>
          <PolicyFields
            value={policy}
            onChange={setPolicy}
            limits={initial.availability.limits}
          />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("video.settings.storage.title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex items-start justify-between gap-3 text-sm">
            <span>
              {t("video.settings.storage.custom")}
              <span className="text-muted-foreground block text-xs">
                {t("video.settings.storage.hint", {
                  hours: initial.availability.policy.recording_retention_hours,
                })}
              </span>
            </span>
            <Switch
              checked={mode === "custom"}
              onCheckedChange={(v) => setMode(v ? "custom" : "platform")}
            />
          </label>
          {mode === "custom" ? (
            <div className="grid gap-3 md:grid-cols-2">
              {field("endpoint", t("video.settings.storage.endpoint"))}
              {field("region", t("video.settings.storage.region"))}
              {field("bucket", t("video.settings.storage.bucket"))}
              {field("prefix", t("video.settings.storage.prefix"))}
              {field("access_key", t("video.settings.storage.access_key"))}
              {field(
                "secret_key",
                t("video.settings.storage.secret_key"),
                "password",
              )}
              <label className="flex items-center gap-2 text-sm">
                <Switch
                  checked={storage.path_style}
                  onCheckedChange={(v) =>
                    setStorage({ ...storage, path_style: v })
                  }
                />
                {t("video.settings.storage.path_style")}
              </label>
            </div>
          ) : null}
        </CardContent>
      </Card>
      <Button onClick={() => save.mutate()} disabled={save.isPending}>
        {t("video.settings.save")}
      </Button>
    </div>
  );
}

function MinutesTab({
  balance,
  unlimited,
}: {
  balance: number;
  unlimited: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useVideoScope();
  const packages = useQuery({
    queryKey: videoKeys.packages(slug),
    queryFn: () => videoService.packages(slug),
    enabled: Boolean(slug),
  });
  const buy = useMutation({
    mutationFn: (uuid: string) => videoService.purchase(slug, uuid),
    onSuccess: () => appToast.success(t("video.minutes.purchased")),
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  const items = (packages.data?.items ?? []).filter(
    (p) => p.unit === "video_minutes",
  );
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="p-4 text-sm">
          {unlimited
            ? t("video.minutes.unlimited")
            : t("video.minutes.balance", { balance })}
          <p className="text-muted-foreground mt-1 text-xs">
            {t("video.minutes.model")}
          </p>
        </CardContent>
      </Card>
      <div className="grid gap-3 md:grid-cols-3">
        {items.map((p) => (
          <Card key={p.uuid}>
            <CardContent className="space-y-2 p-4">
              <p className="font-medium">{p.name}</p>
              <p className="text-2xl font-semibold">
                {t("video.minutes.amount", { minutes: p.credits ?? 0 })}
              </p>
              <p className="text-muted-foreground text-sm">
                {((p.amount_minor ?? 0) / 100).toLocaleString(undefined, {
                  style: "currency",
                  currency: "TRY",
                })}
                {p.expires_days
                  ? ` · ${t("video.minutes.valid_days", { days: p.expires_days })}`
                  : ""}
              </p>
              <Button
                size="sm"
                className="w-full"
                disabled={buy.isPending}
                onClick={() => p.uuid && buy.mutate(p.uuid)}
              >
                {t("video.minutes.buy")}
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>
      {items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("video.minutes.no_packages")}
        </p>
      ) : null}
      <p className="text-muted-foreground text-xs">
        {t("video.minutes.pay_hint")}{" "}
        <Link className="underline" href={routes.cms.billing}>
          {t("video.minutes.billing_link")}
        </Link>
      </p>
    </div>
  );
}
