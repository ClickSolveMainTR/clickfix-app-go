"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowRight, Info } from "lucide-react";
import Link from "next/link";

import { EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { tenantBillingService } from "@/features/billing/services/tenant-billing.service";
import { channelsKeys } from "@/features/channels/hooks/use-channels";
import { channelsService } from "@/features/channels/services/channels.service";
import { AutoRepliesSection } from "@/features/tenant-settings/components/auto-replies-section";
import { autoRepliesService } from "@/features/tenant-settings/services/auto-replies.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useAgentChannelToggle,
  useAgentConfig,
} from "../hooks/use-agent-channels";
import { aiErrorMessage } from "../lib/errors";
import { aiSettingsService } from "../services/ai-settings.service";
import { QueryState } from "./parts";
import { policyKey } from "./security-tab";
import type { AISettingsTabProps } from "./tab-registry";

/** Credits at or below this (without a wallet alert threshold) count as low. */
const LOW_CREDITS = 50;

const AI = routes.cms.settings.ai;
const HOURS = `${routes.cms.settings.sla}?tab=hours`;

/**
 * Persona text that names hours ("09:00", "9 am", "mesai saatleri"). Only a
 * hint to compare with the SLA schedule; the text is not parsed.
 */
const HOURS_IN_TEXT =
  /([01]?\d|2[0-3])[:.][0-5]\d|\b\d{1,2}\s*(am|pm)\b|mesai|çalışma saat|working hours|business hours|opening hours/i;

type Warning = { id: string; href: string };

/**
 * "Genel bakış" tab (CF-243): every connected channel with the AI agent's
 * state on it, the warnings that keep the agent silent, the automatic
 * replies and the working hours link, in one place. The per-channel switch
 * edits the agent's channel-type scope (`tenant.settings.write`).
 */
export function OverviewTab({ slug, canWrite }: AISettingsTabProps) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canBilling = can(permissions.billing.tenantRead);

  const agent = useAgentConfig(slug);
  const policy = useQuery({
    queryKey: policyKey(slug),
    queryFn: () => aiSettingsService.policy(slug),
  });
  const channels = useQuery({
    queryKey: channelsKeys.list(slug),
    queryFn: () => channelsService.list(slug).then((r) => r.items),
  });
  const replies = useQuery({
    queryKey: ["tenant", slug, "organization", "auto-replies"],
    queryFn: () => autoRepliesService.get(slug),
  });
  const wallet = useQuery({
    queryKey: ["tenant", slug, "ai", "overview", "wallet"],
    queryFn: () => tenantBillingService.wallet(slug, { limit: 1, offset: 0 }),
    enabled: canBilling,
    retry: false,
  });
  const toggle = useAgentChannelToggle(slug);

  if (!agent.data || !channels.data) {
    return (
      <QueryState
        loading={agent.isLoading || channels.isLoading}
        error={agent.isError || channels.isError}
        onRetry={() => {
          void agent.refetch();
          void channels.refetch();
        }}
      />
    );
  }

  const cfg = agent.data.config;
  const instances = channels.data;
  const enabledTypes = new Set(cfg.channels);
  const connectedTypes = new Set(instances.map((c) => c.type));
  const offTypes = [...connectedTypes].filter((x) => !enabledTypes.has(x));
  const typeLabel = (type: string) => {
    const key = `channels.types.${type}`;
    const label = t(key);
    return label === key ? type : label;
  };
  const sameType = (type: string) =>
    instances.filter((c) => c.type === type).length;

  const warnings: Warning[] = [];
  if (policy.data && !policy.data.enabled)
    warnings.push({ id: "policy_off", href: `${AI}?tab=security` });
  if (cfg.mode === "off")
    warnings.push({ id: "mode_off", href: `${AI}?tab=agent` });
  if (instances.length > 0 && !instances.some((c) => enabledTypes.has(c.type)))
    warnings.push({ id: "no_channels", href: `${AI}?tab=agent` });
  else if (offTypes.length > 0)
    warnings.push({ id: "channels_off", href: `${AI}?tab=agent` });
  if (wallet.data) {
    const threshold = wallet.data.settings.alert_threshold ?? LOW_CREDITS;
    if (wallet.data.ai_credits <= 0)
      warnings.push({ id: "credits_empty", href: routes.cms.billing });
    else if (wallet.data.ai_credits <= threshold)
      warnings.push({ id: "credits_low", href: routes.cms.billing });
  }
  const personaHours = HOURS_IN_TEXT.test(cfg.persona);

  const onToggle = (type: string, enabled: boolean) =>
    toggle.mutate(
      { type, enabled },
      {
        onSuccess: () =>
          appToast.success(
            t(
              enabled
                ? "ai_settings.overview.channel_enabled"
                : "ai_settings.overview.channel_disabled",
              { channel: typeLabel(type) },
            ),
          ),
        onError: (error) => appToast.error(aiErrorMessage(error, t)),
      },
    );

  const onOff = (on: boolean) =>
    on ? t("ai_settings.overview.on") : t("ai_settings.overview.off");

  return (
    <div className="space-y-6">
      {warnings.length > 0 && (
        <div className="grid gap-3 lg:grid-cols-2">
          {warnings.map((w) => (
            <Alert
              key={w.id}
              variant={
                w.id === "policy_off" || w.id === "credits_empty"
                  ? "destructive"
                  : "default"
              }
            >
              <AlertTriangle className="size-4" />
              <AlertTitle>
                {t(`ai_settings.overview.warning.${w.id}`)}
              </AlertTitle>
              <AlertDescription className="space-y-2">
                <p>
                  {t(`ai_settings.overview.warning.${w.id}_hint`, {
                    channels: offTypes.map(typeLabel).join(", "),
                  })}
                </p>
                <Link
                  href={w.href}
                  className="text-foreground inline-flex items-center gap-1 text-sm font-medium underline underline-offset-4"
                >
                  {t(`ai_settings.overview.warning.${w.id}_link`)}
                  <ArrowRight className="size-3.5" />
                </Link>
              </AlertDescription>
            </Alert>
          ))}
        </div>
      )}
      {personaHours && (
        <Alert>
          <Info className="size-4" />
          <AlertDescription>
            <p>
              {t("ai_settings.overview.persona_hours")}{" "}
              <Link
                href={HOURS}
                className="text-foreground underline underline-offset-4"
              >
                {t("ai_settings.overview.hours_link")}
              </Link>
            </p>
          </AlertDescription>
        </Alert>
      )}

      <EntitySectionCard
        title={t("ai_settings.overview.channels_title")}
        description={t("ai_settings.overview.channels_hint")}
        badge={instances.length}
      >
        {instances.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("ai_settings.overview.no_instances")}{" "}
            <Link
              href={routes.cms.channels}
              className="text-foreground underline underline-offset-4"
            >
              {t("ai_settings.overview.connect_channel")}
            </Link>
          </p>
        ) : (
          <ul className="divide-y">
            {instances.map((c) => {
              const on = enabledTypes.has(c.type);
              const id = `ai-channel-${c.uuid}`;
              return (
                <li
                  key={c.uuid}
                  className="flex flex-col gap-3 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0 space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate text-sm font-medium">
                        {c.name}
                      </span>
                      <Badge variant="outline" className="font-normal">
                        {typeLabel(c.type)}
                      </Badge>
                      {c.status !== "active" && (
                        <Badge variant="secondary" className="font-normal">
                          {t(`channels.status.${c.status}`)}
                        </Badge>
                      )}
                    </div>
                    <dl className="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs">
                      <div className="flex gap-1">
                        <dt>{t("ai_settings.overview.agent")}:</dt>
                        <dd className={on ? "text-foreground" : undefined}>
                          {onOff(on)}
                        </dd>
                      </div>
                      <div className="flex gap-1">
                        <dt>{t("ai_settings.overview.mode")}:</dt>
                        <dd>{t(`ai_settings.agent.mode.${cfg.mode}`)}</dd>
                      </div>
                      <div className="flex gap-1">
                        <dt>{t("ai_settings.overview.hours")}:</dt>
                        <dd>
                          {cfg.business_hours_only
                            ? t("ai_settings.overview.hours_only")
                            : t("ai_settings.overview.hours_always")}
                        </dd>
                      </div>
                      {replies.data && (
                        <>
                          <div className="flex gap-1">
                            <dt>{t("ai_settings.overview.reply_ack")}:</dt>
                            <dd>{onOff(replies.data.ack.enabled)}</dd>
                          </div>
                          <div className="flex gap-1">
                            <dt>{t("ai_settings.overview.reply_offline")}:</dt>
                            <dd>{onOff(replies.data.offline.enabled)}</dd>
                          </div>
                        </>
                      )}
                    </dl>
                    {sameType(c.type) > 1 && (
                      <p className="text-muted-foreground text-xs">
                        {t("ai_settings.overview.same_type_note", {
                          channel: typeLabel(c.type),
                          count: sameType(c.type),
                        })}
                      </p>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Switch
                      id={id}
                      checked={on}
                      disabled={!canWrite || toggle.isPending}
                      aria-label={t("ai_settings.overview.toggle", {
                        channel: c.name,
                      })}
                      onCheckedChange={(checked) => onToggle(c.type, checked)}
                    />
                    <label htmlFor={id} className="text-sm">
                      {t("ai_settings.overview.toggle_label")}
                    </label>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
        <p className="text-muted-foreground mt-4 text-xs">
          {t("ai_settings.overview.type_scope_note")}
        </p>
      </EntitySectionCard>

      <EntitySectionCard
        title={t("ai_settings.overview.hours_title")}
        description={t("ai_settings.overview.hours_hint")}
      >
        <Link
          href={HOURS}
          className="text-foreground inline-flex items-center gap-1 text-sm font-medium underline underline-offset-4"
        >
          {t("ai_settings.overview.hours_link")}
          <ArrowRight className="size-3.5" />
        </Link>
      </EntitySectionCard>

      <AutoRepliesSection disabled={!canWrite} />
    </div>
  );
}
