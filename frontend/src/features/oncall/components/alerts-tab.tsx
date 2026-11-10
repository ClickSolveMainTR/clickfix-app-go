"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { BellRing, Check, MessagesSquare, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntityTable, useServerListState } from "@/components/entity";
import { createColumn } from "@/components/tables";
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
  const listState = useServerListState({ initialPageSize: 20 });
  const { limit, offset } = listState.params;
  const alerts = useQuery({
    queryKey: [...key, limit, offset],
    queryFn: () => oncallService.alertsPage(slug, { limit, offset }),
    refetchInterval: 30_000,
    placeholderData: keepPreviousData,
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

  const policyName = useMemo(() => {
    const names = new Map((policies.data ?? []).map((p) => [p.uuid, p.name]));
    return (uuid: string) => names.get(uuid) ?? uuid.slice(0, 8);
  }, [policies.data]);
  const incidentTitle = useMemo(() => {
    const titles = new Map(
      (incidents.data ?? []).map((i) => [i.uuid, i.title]),
    );
    return (uuid: string) => titles.get(uuid) ?? uuid.slice(0, 8);
  }, [incidents.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<OncallAlert>({
          id: "state",
          labelKey: "oncall.alerts.columns.state",
          cell: ({ row }) => <AlertState alert={row.original} />,
        }),
        createColumn<OncallAlert>({
          id: "alert",
          labelKey: "oncall.alerts.columns.alert",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const a = row.original;
            return (
              <div className="min-w-0 space-y-1">
                <span className="text-sm font-medium">
                  {policyName(a.policy_uuid)}
                </span>
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
            );
          },
        }),
        createColumn<OncallAlert>({
          id: "actions",
          labelKey: "common.actions",
          enableHiding: false,
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row }) =>
            !row.original.acknowledged_at && !row.original.resolved_at ? (
              <Button
                size="sm"
                variant="outline"
                disabled={ack.isPending}
                onClick={() => ack.mutate(row.original.uuid)}
              >
                <Check className="size-4" />
                {t("oncall.alerts.acknowledge")}
              </Button>
            ) : null,
        }),
      ] as ColumnDef<OncallAlert, unknown>[],
    [ack, incidentTitle, locale, onOpenIncident, policyName, t],
  );

  return (
    <div className="grid gap-6 xl:grid-cols-2">
      <section className="space-y-3">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <BellRing className="size-4" />
          {t("oncall.alerts.title")}
        </h2>
        <EntityTable
          columns={columns}
          data={alerts.data?.items ?? []}
          getRowId={(a) => a.uuid}
          isLoading={alerts.isLoading}
          isError={alerts.isError}
          errorTitle={t("oncall.error.title")}
          errorDescription={t("oncall.error.description")}
          onRetry={() => void alerts.refetch()}
          emptyTitle={t("oncall.alerts.empty_title")}
          emptyDescription={t("oncall.alerts.empty_description")}
          total={alerts.data?.total ?? 0}
          state={listState.tableState}
          features={{
            // The endpoint takes `limit` and `offset` only.
            toolbar: false,
            sorting: false,
            globalFilter: false,
            columnFilters: false,
            rowSelection: false,
            viewMode: false,
          }}
        />
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
