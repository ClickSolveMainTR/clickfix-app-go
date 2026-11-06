"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BellRing, Check, MessagesSquare, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useEffect } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { cn, datetime, relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useOncallError } from "../lib/use-oncall-error";
import { IncidentSeverityBadge, IncidentStatusChip } from "./incident-badges";
import {
  oncallService,
  type OncallAlert,
  type StatusIncident,
} from "../services/oncall.service";

/** Anchor id of one incident row; alerts link to it. */
export const incidentAnchor = (uuid: string) => `incident-${uuid}`;

/**
 * Alerts opened by incidents and urgent conversations, with acknowledge,
 * next to the organization's incidents they point at.
 */
export function AlertsTab({
  slug,
  focusIncident,
  onOpenIncident,
}: {
  slug: string;
  focusIncident: string | null;
  onOpenIncident: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const onError = useOncallError();
  const key = ["tenant", slug, "oncall", "alerts"] as const;
  const alerts = useQuery({
    queryKey: key,
    queryFn: () => oncallService.alerts(slug),
    refetchInterval: 30_000,
  });
  const policies = useQuery({
    queryKey: ["tenant", slug, "oncall", "policies"],
    queryFn: () => oncallService.policies(slug),
  });
  const incidents = useQuery({
    queryKey: ["tenant", slug, "status", "incidents"],
    queryFn: () => oncallService.incidents(slug),
  });
  const ack = useMutation({
    mutationFn: (uuid: string) => oncallService.acknowledge(slug, uuid),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: key });
      appToast.success(t("oncall.alerts.acknowledged"));
    },
    onError,
  });

  useEffect(() => {
    if (!focusIncident || !incidents.data) return;
    document
      .getElementById(incidentAnchor(focusIncident))
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  }, [focusIncident, incidents.data]);

  if (alerts.isLoading) return <Loading label={t("common.loading")} />;
  if (alerts.isError)
    return (
      <ErrorState
        title={t("oncall.error.title")}
        description={t("oncall.error.description")}
        onRetry={() => void alerts.refetch()}
      />
    );
  const items = alerts.data ?? [];
  const policyName = (uuid: string) =>
    policies.data?.find((p) => p.uuid === uuid)?.name ?? uuid.slice(0, 8);
  const incidentTitle = (uuid: string) =>
    incidents.data?.find((i) => i.uuid === uuid)?.title ?? uuid.slice(0, 8);

  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <section className="space-y-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <BellRing className="size-4" />
          {t("oncall.alerts.title")}
        </h2>
        {items.length === 0 ? (
          <EmptyState
            icon={<BellRing className="size-6" />}
            title={t("oncall.alerts.empty_title")}
            description={t("oncall.alerts.empty_description")}
          />
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((a) => (
              <li key={a.uuid} className="flex flex-wrap items-start gap-3 p-3">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <AlertState alert={a} />
                    <span className="text-sm font-medium">
                      {policyName(a.policy_uuid)}
                    </span>
                  </div>
                  <div className="text-sm">
                    {a.incident_uuid ? (
                      <button
                        type="button"
                        className="text-primary inline-flex items-center gap-1 hover:underline"
                        onClick={() => onOpenIncident(a.incident_uuid!)}
                      >
                        <TriangleAlert className="size-3.5" />
                        {incidentTitle(a.incident_uuid)}
                      </button>
                    ) : a.conversation_uuid ? (
                      <Link
                        href={`/inbox?c=${a.conversation_uuid}`}
                        className="text-primary inline-flex items-center gap-1 hover:underline"
                      >
                        <MessagesSquare className="size-3.5" />
                        {t("oncall.alerts.urgent_conversation")}
                      </Link>
                    ) : null}
                  </div>
                  <p className="text-muted-foreground text-xs">
                    {t("oncall.alerts.opened", {
                      time: relativeDatetime(a.created_at, locale),
                    })}
                    {!a.acknowledged_at &&
                      !a.resolved_at &&
                      ` · ${t("oncall.alerts.next_step", {
                        step: a.next_step + 1,
                        time: datetime(a.next_notify_at, "HH:mm", locale),
                      })}`}
                  </p>
                </div>
                {!a.acknowledged_at && !a.resolved_at && (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={ack.isPending}
                    onClick={() => ack.mutate(a.uuid)}
                  >
                    <Check className="size-4" />
                    {t("oncall.alerts.acknowledge")}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        )}
      </section>
      <section className="space-y-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <TriangleAlert className="size-4" />
          {t("oncall.incidents.title")}
        </h2>
        {incidents.isError ? (
          <p className="text-muted-foreground text-sm">
            {t("oncall.incidents.unavailable")}
          </p>
        ) : (incidents.data ?? []).length === 0 ? (
          <p className="text-muted-foreground rounded-md border p-3 text-sm">
            {incidents.isLoading ? "…" : t("oncall.incidents.empty")}
          </p>
        ) : (
          <ul className="divide-y rounded-md border">
            {(incidents.data ?? []).map((i) => (
              <IncidentRow
                key={i.uuid}
                incident={i}
                focused={i.uuid === focusIncident}
              />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function AlertState({ alert }: { alert: OncallAlert }) {
  const { t } = useLocale();
  if (alert.resolved_at)
    return (
      <StatusChip label={t("oncall.alerts.state.resolved")} tone="success" />
    );
  if (alert.acknowledged_at)
    return <StatusChip label={t("oncall.alerts.state.acknowledged")} />;
  return <StatusChip label={t("oncall.alerts.state.open")} tone="danger" />;
}

function IncidentRow({
  incident,
  focused,
}: {
  incident: StatusIncident;
  focused: boolean;
}) {
  const { t, locale } = useLocale();
  return (
    <li
      id={incident.uuid ? incidentAnchor(incident.uuid) : undefined}
      className={cn(
        "space-y-1 p-3 transition-colors",
        focused && "bg-primary/10",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <IncidentStatusChip incident={incident} />
        <IncidentSeverityBadge incident={incident} />
        <span className="text-sm font-medium">{incident.title}</span>
      </div>
      <p className="text-muted-foreground text-xs">
        {incident.started_at &&
          datetime(incident.started_at, "dd.MM.yyyy HH:mm", locale)}
        {incident.resolved_at &&
          ` → ${datetime(incident.resolved_at, "dd.MM.yyyy HH:mm", locale)}`}
        {incident.public === false && ` · ${t("oncall.incidents.private")}`}
      </p>
    </li>
  );
}
