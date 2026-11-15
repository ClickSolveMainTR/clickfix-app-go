"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import type { Department } from "@/features/departments/types";
import { TeamColorDot } from "@/features/teams/components/teams-columns";
import { personName } from "@/features/teams/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export type DepartmentRowActionHandlers = {
  onOpen: (department: Department) => void;
  onDelete: (department: Department) => void;
};

/** Columns follow `GET /v1/tenant/departments/meta` (CF-215). */
export function useDepartmentsColumns(handlers: DepartmentRowActionHandlers) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.departments.write);
  return useMemo(
    () =>
      [
        createColumn<Department>({
          accessorKey: "name",
          labelKey: "departments.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 flex-col">
              <span className="flex min-w-0 items-center gap-2">
                <TeamColorDot color={row.original.color} />
                <span className="truncate font-medium">
                  {row.original.name}
                </span>
                {row.original.widget_visible ? null : (
                  <Badge variant="outline">
                    {t("departments.badges.hidden_in_widget")}
                  </Badge>
                )}
              </span>
              {row.original.description ? (
                <span className="text-muted-foreground truncate text-xs">
                  {row.original.description}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<Department>({
          id: "teams",
          accessorFn: (row) => row.teams.map((team) => team.name).join(", "),
          labelKey: "departments.columns.teams",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.teams.length ? (
              <span className="flex flex-wrap gap-1">
                {row.original.teams.map((team) => (
                  <Badge key={team.uuid} variant="secondary">
                    {team.name}
                  </Badge>
                ))}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<Department>({
          id: "leads",
          accessorFn: (row) => row.leads.map(personName).join(", "),
          labelKey: "departments.columns.leads",
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
        createColumn<Department>({
          id: "sla_policy_uuid",
          accessorFn: (row) => (row.sla_policy_uuid ? "1" : ""),
          labelKey: "departments.columns.sla",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.sla_policy_uuid ? (
              <Badge variant="outline">{t("departments.badges.own_sla")}</Badge>
            ) : (
              <span className="text-muted-foreground">
                {t("departments.badges.org_sla")}
              </span>
            ),
        }),
        createColumn<Department>({
          id: "ai",
          accessorFn: (row) => (row.ai.enabled ? "1" : ""),
          labelKey: "departments.columns.ai",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.ai.enabled ? (
              <Badge variant="secondary">{t("departments.badges.ai_on")}</Badge>
            ) : (
              <Badge variant="outline">{t("departments.badges.ai_off")}</Badge>
            ),
        }),
        createColumn<Department>({
          accessorKey: "created_at",
          labelKey: "departments.columns.created_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Department>({
          id: "actions",
          labelKey: "departments.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: canWrite
                  ? t("departments.actions.edit")
                  : t("departments.actions.view"),
                icon: canWrite ? Pencil : Eye,
                onSelect: () => handlers.onOpen(row.original),
              },
              {
                id: "delete",
                label: t("departments.actions.delete"),
                icon: Trash2,
                permission: permissions.tenant.departments.write,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => handlers.onDelete(row.original),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Department, unknown>[],
    [canWrite, handlers, locale, t],
  );
}
