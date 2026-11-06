"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import {
  humanLabel,
  ISSUE_PRIORITIES,
  ISSUE_STATUSES,
  issuePriorityTone,
  issueStatusTone,
} from "@/features/engineering/lib/issue-display";
import type { Issue } from "@/features/engineering/services/engineering.service";
import { relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * `status` and `priority` are server-side filters; the API sorts by latest
 * change only, so no column sorts.
 */
export function useIssuesColumns() {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Issue>({
          accessorKey: "title",
          labelKey: "engineering.columns.title",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.title}</span>
          ),
        }),
        createColumn<Issue>({
          accessorKey: "status",
          labelKey: "engineering.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ISSUE_STATUSES.map((value) => ({
            value,
            labelKey: `engineering.status.${value}`,
            label: value,
          })),
          gridSecondary: true,
          cell: ({ row }) => (
            <StatusChip
              label={t(`engineering.status.${row.original.status}`)}
              tone={issueStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Issue>({
          accessorKey: "priority",
          labelKey: "engineering.columns.priority",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ISSUE_PRIORITIES.map((value) => ({
            value,
            labelKey: `inbox.priority.${value}`,
            label: value,
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`inbox.priority.${row.original.priority}`)}
              tone={issuePriorityTone(row.original.priority)}
            />
          ),
        }),
        createColumn<Issue>({
          id: "labels",
          accessorFn: (row) => row.labels.join(", "),
          labelKey: "engineering.columns.labels",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.labels.length ? (
              <div className="flex flex-wrap gap-1">
                {row.original.labels.map((label) => (
                  <Badge
                    key={label}
                    variant="secondary"
                    className="font-normal"
                  >
                    {humanLabel(label, locale)}
                  </Badge>
                ))}
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Issue>({
          accessorKey: "updated_at",
          labelKey: "engineering.columns.updated_at",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-sm">
              {relativeDatetime(row.original.updated_at, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<Issue, unknown>[],
    [locale, t],
  );
}
