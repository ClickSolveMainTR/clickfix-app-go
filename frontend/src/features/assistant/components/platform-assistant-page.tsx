"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
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
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  platformAssistantService,
  type AssistantSettings,
} from "../services/assistant.service";

const KEY = ["platform-assistant"] as const;
const ROUTES = ["triage", "reply", "complex"] as const;
const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * Platform switch board of ClickFix Asistan (platform.assistant.manage):
 * on/off, aicore model route, per-organization daily limit, extra system
 * prompt, ClickFix's own support organization and the usage report.
 */
export function PlatformAssistantPage() {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const settings = useQuery({
    queryKey: [...KEY, "settings"],
    queryFn: platformAssistantService.settings,
  });
  const usage = useQuery({
    queryKey: [...KEY, "usage"],
    queryFn: () => platformAssistantService.usage(30),
  });
  const nf = new Intl.NumberFormat(locale);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Bot className="size-7" />}
        title={t("assistant.platform.title")}
        description={t("assistant.platform.description")}
      />
      {settings.isLoading ? (
        <Skeleton className="h-96 w-full rounded-xl" />
      ) : null}
      {settings.isError || usage.isError ? (
        <ErrorState
          title={t("common.error_server")}
          onRetry={() => {
            void settings.refetch();
            void usage.refetch();
          }}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {settings.data ? (
        <SettingsForm
          key={settings.data.support_org_uuid ?? "none"}
          initial={settings.data}
          onSaved={(data) => qc.setQueryData([...KEY, "settings"], data)}
        />
      ) : null}
      {usage.data ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">
              {t("assistant.platform.usage")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid grid-cols-2 gap-4 md:grid-cols-4">
              <Stat
                label={t("assistant.platform.calls")}
                value={nf.format(usage.data.calls)}
              />
              <Stat
                label={t("assistant.platform.organizations")}
                value={nf.format(usage.data.organizations)}
              />
              <Stat
                label={t("assistant.platform.tokens")}
                value={nf.format(
                  usage.data.input_tokens + usage.data.output_tokens,
                )}
              />
              <Stat
                label={t("assistant.platform.cost")}
                value={new Intl.NumberFormat(locale, {
                  style: "currency",
                  currency: "USD",
                }).format(usage.data.cost_micros / 1_000_000)}
              />
            </dl>
            <p className="text-muted-foreground mt-3 text-xs">
              {t("assistant.platform.usage_hint")}
            </p>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-xl font-semibold tabular-nums">{value}</dd>
    </div>
  );
}

function SettingsForm({
  initial,
  onSaved,
}: {
  initial: AssistantSettings;
  onSaved: (data: AssistantSettings) => void;
}) {
  const { t } = useLocale();
  const [draft, setDraft] = useState<AssistantSettings>(initial);
  const [supportOrg, setSupportOrg] = useState(initial.support_org_uuid ?? "");
  const save = useMutation({
    mutationFn: (body: AssistantSettings) =>
      platformAssistantService.save(body),
    onSuccess: (data) => {
      onSaved(data);
      appToast.success(t("assistant.platform.saved"));
    },
  });
  const supportValid = supportOrg === "" || UUID_RE.test(supportOrg.trim());

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("assistant.platform.settings")}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-5 md:grid-cols-2">
        <div className="flex items-center justify-between gap-4 md:col-span-2">
          <div>
            <Label htmlFor="assistant-enabled">
              {t("assistant.platform.enabled")}
            </Label>
            <p className="text-muted-foreground text-sm">
              {t("assistant.platform.enabled_hint")}
            </p>
          </div>
          <Switch
            id="assistant-enabled"
            checked={draft.enabled}
            onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
          />
        </div>
        <div className="space-y-1.5">
          <Label>{t("assistant.platform.model_route")}</Label>
          <Select
            value={draft.model_route}
            onValueChange={(v) =>
              setDraft({
                ...draft,
                model_route: v as AssistantSettings["model_route"],
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ROUTES.map((r) => (
                <SelectItem key={r} value={r}>
                  {t(`assistant.platform.routes.${r}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {t("assistant.platform.model_route_hint")}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="assistant-limit">
            {t("assistant.platform.daily_limit")}
          </Label>
          <Input
            id="assistant-limit"
            type="number"
            min={0}
            max={100000}
            value={draft.daily_limit_per_org}
            onChange={(e) =>
              setDraft({
                ...draft,
                daily_limit_per_org: Math.max(0, Number(e.target.value)),
              })
            }
          />
          <p className="text-muted-foreground text-xs">
            {t("assistant.platform.daily_limit_hint")}
          </p>
        </div>
        <div className="space-y-1.5 md:col-span-2">
          <Label htmlFor="assistant-support-org">
            {t("assistant.platform.support_org")}
          </Label>
          <Input
            id="assistant-support-org"
            value={supportOrg}
            placeholder="00000000-0000-0000-0000-000000000000"
            aria-invalid={!supportValid}
            onChange={(e) => setSupportOrg(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("assistant.platform.support_org_hint")}
          </p>
        </div>
        <div className="space-y-1.5 md:col-span-2">
          <Label htmlFor="assistant-prompt">
            {t("assistant.platform.prompt_extra")}
          </Label>
          <Textarea
            id="assistant-prompt"
            rows={5}
            maxLength={4000}
            value={draft.system_prompt_extra}
            onChange={(e) =>
              setDraft({ ...draft, system_prompt_extra: e.target.value })
            }
          />
          <p className="text-muted-foreground text-xs">
            {t("assistant.platform.prompt_extra_hint")}
          </p>
        </div>
        <div className="md:col-span-2">
          <Button
            disabled={save.isPending || !supportValid}
            onClick={() =>
              save.mutate({
                ...draft,
                support_org_uuid: supportOrg.trim() || null,
              })
            }
          >
            {t("common.save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
