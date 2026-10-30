"use client";

import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, CheckCircle2, RefreshCw } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type PlatformHealth = components["schemas"]["PlatformHealth"];
type AIProvider = components["schemas"]["PlatformHealthAIProvider"];
type MailHealth = components["schemas"]["PlatformHealthMail"];

const REFRESH_MS = 30_000;

const AI_TONE: Record<
  AIProvider["status"],
  "success" | "warning" | "danger" | "default"
> = {
  ok: "success",
  degraded: "warning",
  down: "danger",
  not_configured: "default",
};

const AI_LABEL_KEYS: Record<AIProvider["status"], string> = {
  ok: "platform_health.status.ok",
  degraded: "platform_health.status.degraded",
  down: "platform_health.status.down",
  not_configured: "platform_health.status.not_configured",
};

const MAIL_TONE: Record<
  MailHealth["status"],
  "success" | "warning" | "danger" | "default"
> = {
  ok: "success",
  down: "danger",
  not_configured: "warning",
  unknown: "default",
};

const MAIL_LABEL_KEYS: Record<MailHealth["status"], string> = {
  ok: "platform_health.status.ok",
  down: "platform_health.status.down",
  not_configured: "platform_health.status.not_configured",
  unknown: "platform_health.status.unknown",
};

function Tile({
  label,
  value,
  tone,
}: {
  label: string;
  value: ReactNode;
  tone?: "warning" | "danger";
}) {
  return (
    <div
      className={cn(
        "rounded-md border px-3 py-2",
        tone === "danger" && "border-destructive/40 bg-destructive/5",
        tone === "warning" && "border-amber-500/40 bg-amber-500/5",
      )}
    >
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="text-lg font-semibold tabular-nums">{value}</div>
    </div>
  );
}

/**
 * `/platform/health` ("Sistem sağlığı"): organizations, job queues, outbox,
 * channels, AI providers and the e-mail transport; refreshes every 30 seconds.
 */
export function PlatformHealthPage() {
  const { t, locale } = useLocale();
  const query = useQuery({
    queryKey: ["platform", "health"],
    queryFn: () =>
      platformRequest<PlatformHealth>("GET", "/v1/platform/health"),
    refetchInterval: REFRESH_MS,
  });
  const data = query.data;
  const n = (value: number) => value.toLocaleString(locale);

  return (
    <EntityPage
      title={t("platform_health.title")}
      description={t("platform_health.description")}
      permission={permissions.organizations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("platform_health.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("platform_health.title") },
      ]}
      actions={
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={query.isFetching}
          onClick={() => void query.refetch()}
        >
          <RefreshCw
            className={cn("size-4", query.isFetching && "animate-spin")}
          />
          {t("platform_health.refresh")}
        </Button>
      }
    >
      {query.isLoading ? <Skeleton className="h-40 w-full" /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("platform_health.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {data ? (
        <div className="space-y-6">
          <EntitySectionCard
            title={
              data.status === "ok"
                ? t("platform_health.overall_ok")
                : t("platform_health.overall_degraded")
            }
          >
            <div className="space-y-2">
              <StatusChip
                label={t(
                  data.status === "ok"
                    ? "platform_health.status.ok"
                    : "platform_health.status.degraded",
                )}
                tone={data.status === "ok" ? "success" : "warning"}
              />
              <p className="text-muted-foreground text-sm">
                {t("platform_health.generated", {
                  date: datetime(data.generated_at, undefined, locale),
                  hours: data.window_hours,
                })}
              </p>
              {data.issues.length ? (
                <ul className="space-y-1">
                  {data.issues.map((issue) => (
                    <li
                      key={issue}
                      className="flex items-start gap-2 text-sm text-amber-700 dark:text-amber-400"
                    >
                      <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                      <code className="text-xs">{issue}</code>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="flex items-center gap-2 text-sm">
                  <CheckCircle2 className="size-4 text-emerald-600" />
                  {t("platform_health.no_issues")}
                </p>
              )}
            </div>
          </EntitySectionCard>

          <div className="grid gap-6 lg:grid-cols-2">
            <EntitySectionCard
              title={t("platform_health.sections.organizations")}
            >
              <div className="grid grid-cols-3 gap-2">
                <Tile
                  label={t("platform_health.organizations.active")}
                  value={n(data.organizations.active)}
                />
                <Tile
                  label={t("platform_health.organizations.suspended")}
                  value={n(data.organizations.suspended)}
                  tone={data.organizations.suspended ? "warning" : undefined}
                />
                <Tile
                  label={t("platform_health.organizations.created_last_week")}
                  value={n(data.organizations.created_last_week)}
                />
              </div>
            </EntitySectionCard>

            <EntitySectionCard title={t("platform_health.sections.outbox")}>
              <div className="grid grid-cols-3 gap-2">
                <Tile
                  label={t("platform_health.outbox.pending")}
                  value={n(data.outbox.pending)}
                />
                <Tile
                  label={t("platform_health.outbox.failed")}
                  value={n(data.outbox.failed)}
                  tone={data.outbox.failed ? "danger" : undefined}
                />
                <Tile
                  label={t("platform_health.outbox.oldest_pending")}
                  value={t("platform_health.seconds", {
                    count: data.outbox.oldest_pending_seconds,
                  })}
                  tone={
                    data.outbox.oldest_pending_seconds > 300
                      ? "warning"
                      : undefined
                  }
                />
              </div>
            </EntitySectionCard>
          </div>

          <EntitySectionCard title={t("platform_health.sections.queues")}>
            {data.queue_error ? (
              <p className="text-destructive mb-3 flex items-center gap-2 text-sm">
                <AlertTriangle className="size-4" />
                {t("platform_health.queue_error")}
              </p>
            ) : null}
            {data.queues.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("platform_health.empty")}
              </p>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead className="text-muted-foreground text-xs">
                    <tr className="border-b text-left">
                      <th className="py-2 pr-3 font-medium">
                        {t("platform_health.queues.name")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.pending")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.active")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.scheduled")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.retry")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.archived")}
                      </th>
                      <th className="py-2 pr-3 text-right font-medium">
                        {t("platform_health.queues.processed_today")}
                      </th>
                      <th className="py-2 text-right font-medium">
                        {t("platform_health.queues.failed_today")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.queues.map((queue) => (
                      <tr
                        key={queue.name}
                        className="border-b tabular-nums last:border-b-0"
                      >
                        <td className="py-2 pr-3 font-medium">
                          {queue.name}
                          {queue.paused ? (
                            <span className="ms-2 text-xs text-amber-600">
                              {t("platform_health.queues.paused")}
                            </span>
                          ) : null}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.pending)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.active)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.scheduled)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.retry)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.archived)}
                        </td>
                        <td className="py-2 pr-3 text-right">
                          {n(queue.processed_today)}
                        </td>
                        <td
                          className={cn(
                            "py-2 text-right",
                            queue.failed_today > 0 && "text-destructive",
                          )}
                        >
                          {n(queue.failed_today)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </EntitySectionCard>

          <div className="grid gap-6 lg:grid-cols-2">
            <EntitySectionCard title={t("platform_health.sections.channels")}>
              {data.channels.length === 0 ? (
                <p className="text-muted-foreground text-sm">
                  {t("platform_health.channels.empty")}
                </p>
              ) : (
                <ul className="space-y-3">
                  {data.channels.map((channel) => (
                    <li key={channel.type} className="rounded-md border p-3">
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-medium">{channel.type}</span>
                        <span
                          className={cn(
                            "text-sm tabular-nums",
                            channel.error_rate > 0.05 && "text-destructive",
                          )}
                        >
                          {t("platform_health.channels.error_rate", {
                            rate: (channel.error_rate * 100).toFixed(1),
                          })}
                        </span>
                      </div>
                      <p className="text-muted-foreground mt-1 text-xs">
                        {t("platform_health.channels.summary", {
                          instances: channel.instances,
                          errors: channel.instances_with_errors,
                          total: channel.outbound_total,
                          failed: channel.outbound_failed,
                        })}
                      </p>
                    </li>
                  ))}
                </ul>
              )}
            </EntitySectionCard>

            <EntitySectionCard title={t("platform_health.sections.mail")}>
              <div className="space-y-3">
                <div className="flex items-center justify-between gap-2 rounded-md border p-3">
                  <span className="min-w-0">
                    <span className="block font-medium">
                      {data.mail.configured
                        ? `${data.mail.host}:${data.mail.port} (${data.mail.tls_mode})`
                        : t("platform_health.mail.not_configured")}
                    </span>
                    <span className="text-muted-foreground block truncate text-xs">
                      {data.mail.cause
                        ? t("platform_health.mail.cause", {
                            cause: data.mail.cause,
                          })
                        : data.mail.checked_at
                          ? t("platform_health.mail.checked", {
                              date: datetime(
                                data.mail.checked_at,
                                undefined,
                                locale,
                              ),
                            })
                          : t("platform_health.mail.not_checked")}
                    </span>
                  </span>
                  <StatusChip
                    label={t(MAIL_LABEL_KEYS[data.mail.status])}
                    tone={MAIL_TONE[data.mail.status]}
                  />
                </div>
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                  <Tile
                    label={t("platform_health.mail.sent")}
                    value={n(data.mail.sent)}
                  />
                  <Tile
                    label={t("platform_health.mail.pending")}
                    value={n(data.mail.pending)}
                  />
                  <Tile
                    label={t("platform_health.mail.retrying")}
                    value={n(data.mail.retrying)}
                    tone={data.mail.retrying > 0 ? "warning" : undefined}
                  />
                  <Tile
                    label={t("platform_health.mail.dead_lettered")}
                    value={n(data.mail.dead_lettered)}
                    tone={data.mail.dead_lettered > 0 ? "danger" : undefined}
                  />
                </div>
                {data.mail.last_error ? (
                  <p className="text-muted-foreground text-xs break-words">
                    {t("platform_health.mail.last_error", {
                      error: data.mail.last_error,
                    })}
                  </p>
                ) : null}
              </div>
            </EntitySectionCard>

            <EntitySectionCard title={t("platform_health.sections.ai")}>
              {data.ai_providers.length === 0 ? (
                <EmptyState title={t("platform_health.ai.empty")} />
              ) : (
                <ul className="space-y-2">
                  {data.ai_providers.map((provider) => (
                    <li
                      key={provider.name}
                      className="flex items-center justify-between gap-2 rounded-md border p-3"
                    >
                      <span className="min-w-0">
                        <span className="block font-medium">
                          {provider.name}
                        </span>
                        {provider.detail ? (
                          <span className="text-muted-foreground block truncate text-xs">
                            {provider.detail}
                          </span>
                        ) : null}
                      </span>
                      <StatusChip
                        label={t(AI_LABEL_KEYS[provider.status])}
                        tone={AI_TONE[provider.status]}
                      />
                    </li>
                  ))}
                </ul>
              )}
            </EntitySectionCard>
          </div>
        </div>
      ) : null}
    </EntityPage>
  );
}
