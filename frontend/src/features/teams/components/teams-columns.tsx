"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { personName, type Team } from "@/features/teams/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type TeamRowActionHandlers = {
  onOpen: (team: Team) => void;
  onDelete: (team: Team) => void;
};

export function TeamColorDot({ color }: { color: string }) {
  return (
    <span
      aria-hidden
      className="ring-border inline-block size-2.5 shrink-0 rounded-full ring-1"
      style={{ backgroundColor: color || "var(--muted-foreground)" }}
    />
  );
}

/** "Organizasyon saatleri" or a short summary of the team's own hours. */
export function useWorkingHoursLabel() {
  const { t } = useLocale();
  return (team: Pick<Team, "working_hours">) => {
    if (!team.working_hours) return t("teams.hours.organization");
    const open = team.working_hours.days.filter((day) => day.open).length;
    return open
      ? t("teams.hours.custom", { count: open })
      : t("teams.hours.closed");
  };
}

/**
 * Columns follow `GET /v1/tenant/teams/meta`: `skills` is a server-side
 * faceted filter (`skill`), `name` and `created_at` sort.
 */
export function useTeamsColumns(
  handlers: TeamRowActionHandlers,
  skillOptions: string[],
) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.teams.write);
  const hoursLabel = useWorkingHoursLabel();

  return useMemo(
    () =>
      [
        createColumn<Team>({
          accessorKey: "name",
          labelKey: "teams.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 items-center gap-2">
              <TeamColorDot color={row.original.color} />
              <span className="truncate font-medium">{row.original.name}</span>
            </span>
          ),
        }),
        createColumn<Team>({
          id: "leads",
          accessorFn: (row) => row.leads.map(personName).join(", "),
          labelKey: "teams.columns.leads",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.leads.length ? (
              <span className="truncate">
                {row.original.leads.map(personName).join(", ")}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Team>({
          accessorKey: "member_count",
          labelKey: "teams.columns.member_count",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.member_count}</span>
          ),
        }),
        createColumn<Team>({
          id: "skills",
          accessorFn: (row) => row.skills.join(","),
          labelKey: "teams.columns.skills",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: skillOptions.map((value) => ({
            value,
            label: value,
          })),
          cell: ({ row }) => {
            const skills = row.original.skills;
            if (!skills.length) {
              return <span className="text-muted-foreground">—</span>;
            }
            return (
              <div className="flex flex-wrap gap-1">
                {skills.slice(0, 3).map((skill) => (
                  <Badge key={skill} variant="secondary">
                    {skill}
                  </Badge>
                ))}
                {skills.length > 3 ? (
                  <Badge variant="outline">+{skills.length - 3}</Badge>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<Team>({
          id: "working_hours",
          accessorFn: (row) => hoursLabel(row),
          labelKey: "teams.columns.working_hours",
          enableSorting: false,
          cell: ({ row }) => (
            <span
              className={
                row.original.working_hours ? undefined : "text-muted-foreground"
              }
            >
              {hoursLabel(row.original)}
            </span>
          ),
        }),
        createColumn<Team>({
          accessorKey: "created_at",
          labelKey: "teams.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Team>({
          id: "actions",
          labelKey: "teams.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: canWrite
                  ? t("teams.actions.edit")
                  : t("teams.actions.view"),
                icon: canWrite ? Pencil : Eye,
                onSelect: () => handlers.onOpen(row.original),
              },
              {
                id: "delete",
                label: t("teams.actions.delete"),
                icon: Trash2,
                permission: permissions.tenant.teams.write,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => handlers.onDelete(row.original),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Team, unknown>[],
    [canWrite, handlers, hoursLabel, locale, skillOptions, t],
  );
}
