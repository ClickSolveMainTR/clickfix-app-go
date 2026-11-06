"use client";

import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { createColumn } from "@/components/tables";
import {
  IncidentSeverityBadge,
  IncidentStatusChip,
  IncidentVisibilityBadge,
} from "@/features/oncall/components/incident-badges";
import type { StatusIncident } from "@/features/oncall/services/oncall.service";
import { datetime, relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Updates of one incident, newest first. */
export function latestUpdates(incident: StatusIncident, count: number) {
  return [...(incident.updates ?? [])]
    .sort((a, b) => (b.created_at ?? "").localeCompare(a.created_at ?? ""))
    .slice(0, count);
}

/**
 * Read-only incident columns. The API takes neither `sort` nor `q`, so no
 * column sorts or filters.
 */
export function useIncidentsColumns() {
  const { locale } = useLocale();

  return useMemo<ColumnDef<StatusIncident>[]>(() => {
    const when = (value?: string | null) =>
      value ? datetime(value, "dd.MM.yyyy HH:mm", locale) : "—";
    return [
      createColumn<StatusIncident>({
        id: "status",
        accessorKey: "status",
        labelKey: "oncall.incidents.columns.status",
        enableSorting: false,
        cell: ({ row }) => <IncidentStatusChip incident={row.original} />,
      }),
      createColumn<StatusIncident>({
        id: "severity",
        accessorKey: "severity",
        labelKey: "oncall.incidents.columns.severity",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.severity ? (
            <IncidentSeverityBadge incident={row.original} />
          ) : (
            "—"
          ),
      }),
      createColumn<StatusIncident>({
        id: "title",
        accessorKey: "title",
        labelKey: "oncall.incidents.columns.title",
        enableSorting: false,
        enableHiding: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.title}</span>
        ),
      }),
      createColumn<StatusIncident>({
        id: "public",
        accessorKey: "public",
        labelKey: "oncall.incidents.columns.visibility",
        enableSorting: false,
        cell: ({ row }) => <IncidentVisibilityBadge incident={row.original} />,
      }),
      createColumn<StatusIncident>({
        id: "started_at",
        accessorKey: "started_at",
        labelKey: "oncall.incidents.columns.started_at",
        enableSorting: false,
        gridSecondary: true,
        cell: ({ row }) => when(row.original.started_at),
      }),
      createColumn<StatusIncident>({
        id: "resolved_at",
        accessorKey: "resolved_at",
        labelKey: "oncall.incidents.columns.resolved_at",
        enableSorting: false,
        cell: ({ row }) => when(row.original.resolved_at),
      }),
      createColumn<StatusIncident>({
        id: "last_update",
        labelKey: "oncall.incidents.columns.last_update",
        enableSorting: false,
        cell: ({ row }) => {
          const [last] = latestUpdates(row.original, 1);
          if (!last) return "—";
          return (
            <div className="max-w-md min-w-0">
              <p className="text-muted-foreground text-xs">
                {[
                  last.status,
                  last.created_at
                    ? relativeDatetime(last.created_at, locale)
                    : null,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </p>
              {last.body ? (
                <p className="line-clamp-2 text-sm">{last.body}</p>
              ) : null}
            </div>
          );
        },
      }),
    ];
  }, [locale]);
}
