"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2, Video } from "lucide-react";
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { platformBillingService } from "@/features/billing/services/platform-billing.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { videoErrorKey } from "../lib/use-video-scope";
import {
  platformVideoService,
  type VideoPlatformSettings,
  type VideoPolicy,
} from "../services/video.service";
import { PolicyFields, type PolicyDraft } from "./policy-fields";

const KEY = ["platform-video"] as const;

/**
 * Platform video module (platform.video.manage): platform policy, plan
 * overrides and the video-minute credit packages organizations buy.
 */
export function PlatformVideoPage() {
  const { t } = useLocale();
  const settings = useQuery({
    queryKey: KEY,
    queryFn: platformVideoService.settings,
  });
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Video className="size-7" />}
        title={t("video.platform.title")}
        description={t("video.platform.description")}
      />
      {settings.data && !settings.data.configured ? (
        <Card className="border-amber-500/40 bg-amber-500/5">
          <CardContent className="p-4 text-sm">
            {t("video.availability.not_configured")}
          </CardContent>
        </Card>
      ) : null}
      {settings.data ? (
        <Tabs defaultValue="policy">
          <TabsList>
            <TabsTrigger value="policy">
              {t("video.platform.tabs.policy")}
            </TabsTrigger>
            <TabsTrigger value="plans">
              {t("video.platform.tabs.plans")}
            </TabsTrigger>
            <TabsTrigger value="packages">
              {t("video.platform.tabs.packages")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="policy" className="mt-4">
            <PlatformPolicy data={settings.data} />
          </TabsContent>
          <TabsContent value="plans" className="mt-4">
            <PlanOverrides data={settings.data} />
          </TabsContent>
          <TabsContent value="packages" className="mt-4">
            <MinutePackages />
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  );
}

function PlatformPolicy({ data }: { data: VideoPlatformSettings }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [policy, setPolicy] = useState<PolicyDraft>(data.policy);
  const save = useMutation({
    mutationFn: () =>
      platformVideoService.save({ ...data.policy, ...policy } as VideoPolicy),
    onSuccess: (next) => {
      qc.setQueryData(KEY, next);
      appToast.success(t("video.settings.saved"));
    },
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  return (
    <Card>
      <CardContent className="space-y-4 p-4">
        <p className="text-muted-foreground text-xs">
          {t("video.platform.policy_hint")}
        </p>
        <PolicyFields value={policy} onChange={setPolicy} />
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          {t("video.settings.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

function PlanOverrides({ data }: { data: VideoPlatformSettings }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const plans = useQuery({
    queryKey: ["platform-video", "plans"],
    queryFn: () => platformBillingService.plans({ limit: 100, offset: 0 }),
  });
  const [plan, setPlan] = useState("");
  const [draft, setDraft] = useState<PolicyDraft>({});
  const choose = (uuid: string) => {
    setPlan(uuid);
    const existing = data.plans.find((p) => p.plan_uuid === uuid);
    setDraft((existing?.policy as PolicyDraft) ?? {});
  };
  const onDone = (next: VideoPlatformSettings) => {
    qc.setQueryData(KEY, next);
    appToast.success(t("video.settings.saved"));
  };
  const save = useMutation({
    mutationFn: () => platformVideoService.savePlan(plan, draft),
    onSuccess: onDone,
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => platformVideoService.deletePlan(uuid),
    onSuccess: onDone,
  });
  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("video.platform.plans_title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          {data.plans.length === 0 ? (
            <p className="text-muted-foreground">
              {t("video.platform.no_overrides")}
            </p>
          ) : null}
          {data.plans.map((p) => (
            <div key={p.plan_uuid} className="flex items-center gap-2">
              <button
                type="button"
                className="font-medium hover:underline"
                onClick={() => choose(p.plan_uuid)}
              >
                {p.name}
              </button>
              <span className="text-muted-foreground text-xs">
                {t("video.platform.override_count", {
                  count: Object.keys(p.policy).length,
                })}
              </span>
              <Button
                disabled={remove.isPending}
                size="icon"
                variant="ghost"
                className="ml-auto"
                aria-label={t("video.platform.remove_override")}
                onClick={() => remove.mutate(p.plan_uuid)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="max-w-sm space-y-1.5">
            <Label>{t("video.platform.plan")}</Label>
            <Select value={plan} onValueChange={choose}>
              <SelectTrigger>
                <SelectValue
                  placeholder={t("video.platform.plan_placeholder")}
                />
              </SelectTrigger>
              <SelectContent>
                {(plans.data?.items ?? []).map((p) => (
                  <SelectItem key={p.uuid} value={p.uuid}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {plan ? (
            <>
              <p className="text-muted-foreground text-xs">
                {t("video.platform.override_hint")}
              </p>
              <PolicyFields value={draft} onChange={setDraft} partial />
              <div className="flex gap-2">
                <Button onClick={() => save.mutate()} disabled={save.isPending}>
                  {t("video.settings.save")}
                </Button>
                <Button variant="outline" onClick={() => setDraft({})}>
                  {t("video.platform.clear")}
                </Button>
              </div>
            </>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}

function MinutePackages() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const list = useQuery({
    queryKey: ["platform-video", "packages"],
    queryFn: platformVideoService.packages,
  });
  const [form, setForm] = useState({
    code: "",
    name: "",
    minutes: "",
    price: "",
    days: "",
  });
  const create = useMutation({
    mutationFn: () =>
      platformVideoService.createPackage({
        unit: "video_minutes",
        code: form.code.trim(),
        name: form.name.trim(),
        credits: Number(form.minutes),
        amount_minor: Math.round(Number(form.price.replace(",", ".")) * 100),
        expires_days: form.days ? Number(form.days) : undefined,
      }),
    onSuccess: () => {
      setForm({ code: "", name: "", minutes: "", price: "", days: "" });
      void qc.invalidateQueries({ queryKey: ["platform-video", "packages"] });
      appToast.success(t("video.settings.saved"));
    },
    onError: (err) => appToast.error(t(`video.errors.${videoErrorKey(err)}`)),
  });
  const items = (list.data?.items ?? []).filter(
    (p) => p.unit === "video_minutes",
  );
  const input = (k: keyof typeof form, label: string, type = "text") => (
    <div className="space-y-1.5">
      <Label htmlFor={`pkg-${k}`}>{label}</Label>
      <Input
        id={`pkg-${k}`}
        type={type}
        value={form[k]}
        onChange={(e) => setForm({ ...form, [k]: e.target.value })}
      />
    </div>
  );
  return (
    <div className="space-y-4">
      <Card>
        <CardContent className="space-y-2 p-4 text-sm">
          {items.length === 0 ? (
            <p className="text-muted-foreground">
              {t("video.minutes.no_packages")}
            </p>
          ) : null}
          {items.map((p) => (
            <div key={p.uuid} className="flex flex-wrap gap-3">
              <span className="font-medium">{p.name}</span>
              <span className="text-muted-foreground">{p.code}</span>
              <span>
                {t("video.minutes.amount", { minutes: p.credits ?? 0 })}
              </span>
              <span>
                {((p.amount_minor ?? 0) / 100).toLocaleString(undefined, {
                  style: "currency",
                  currency: "TRY",
                })}
              </span>
              {p.expires_days ? (
                <span>
                  {t("video.minutes.valid_days", { days: p.expires_days })}
                </span>
              ) : null}
            </div>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("video.platform.new_package")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 md:grid-cols-3">
            {input("code", t("video.platform.package.code"))}
            {input("name", t("video.platform.package.name"))}
            {input("minutes", t("video.platform.package.minutes"), "number")}
            {input("price", t("video.platform.package.price"))}
            {input("days", t("video.platform.package.days"), "number")}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("video.platform.package.hint")}
          </p>
          <Button
            disabled={
              create.isPending ||
              !form.code ||
              !form.name ||
              !Number(form.minutes) ||
              !Number(form.price.replace(",", "."))
            }
            onClick={() => create.mutate()}
          >
            {t("video.platform.package.create")}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
