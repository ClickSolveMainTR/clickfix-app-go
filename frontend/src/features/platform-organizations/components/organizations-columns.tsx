"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Bot, Eye, Pause, Pencil, Play, Trash2, Video } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  ACCESS_LABEL_KEYS,
  ORGANIZATION_ACCESS_STATES,
  ORGANIZATION_STATUSES,
  ORGANIZATION_WINDOWS,
  STATUS_LABEL_KEYS,
  WINDOW_LABEL_KEYS,
} from "@/features/platform-organizations/constants";
import { formatCount } from "@/features/billing/lib/money";
import {
  OrganizationAccessChip,
  OrganizationPlanCell,
  OrganizationStatusChip,
  ownerLabel,
} from "@/features/platform-organizations/components/organization-cells";
import type { OrganizationSummary } from "@/features/platform-organizations/types";
import { OrganizationAvatar } from "@/features/organizations";
import { date, datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type OrganizationRowActionHandlers = {
  onView: (org: OrganizationSummary) => void;
  onEdit: (org: OrganizationSummary) => void;
  onToggleStatus: (org: OrganizationSummary) => void;
  onDelete: (org: OrganizationSummary) => void;
};

/** Translated subscription status, the raw value when unknown. */
export function subscriptionLabel(
  t: (key: string) => string,
  status: string,
): string {
  const key = `organizations.subscription.${status}`;
  const label = t(key);
  return label === key ? status : label;
}

function Count({ value }: { value: number }) {
  return <span className="tabular-nums">{value}</span>;
}

/**
 * Columns follow `GET /v1/platform/organizations/meta`: status, access
 * state and access window are server-side faceted filters; sortable keys
 * match `sortable_fields` (name, slug, status, member_count,
 * last_activity_at, access_ends_at, created_at, updated_at).
 */
export function useOrganizationsColumns(
  handlers: OrganizationRowActionHandlers,
) {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<OrganizationSummary>({
          accessorKey: "name",
          labelKey: "organizations.columns.name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex min-w-0 items-center gap-2">
              <OrganizationAvatar
                organization={row.original}
                className="bg-muted text-foreground ring-border size-7 rounded-md text-xs"
              />
              <span className="truncate font-medium">{row.original.name}</span>
            </span>
          ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "slug",
          labelKey: "organizations.columns.slug",
          enableSorting: true,
          defaultHidden: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.slug}
            </code>
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "owner",
          accessorFn: (row) => (row.owner ? ownerLabel(row.owner) : ""),
          labelKey: "organizations.columns.owner",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.owner ? (
              <span className="flex min-w-0 flex-col">
                <span className="truncate">
                  {ownerLabel(row.original.owner)}
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {row.original.owner.email}
                </span>
              </span>
            ) : row.original.pending_invitation_count > 0 ? (
              <Badge variant="warning">
                {t("organizations.owner_invited")}
              </Badge>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "member_count",
          labelKey: "organizations.columns.member_count",
          enableSorting: true,
          cell: ({ row }) => <Count value={row.original.member_count} />,
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "billable_seats",
          labelKey: "organizations.columns.billable_seats",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => <Count value={row.original.billable_seats} />,
        }),
        createColumn<OrganizationSummary>({
          id: "plan",
          accessorFn: (row) => row.plan_name || row.plan_code,
          labelKey: "organizations.columns.plan",
          enableSorting: false,
          cell: ({ row }) => <OrganizationPlanCell org={row.original} />,
        }),
        createColumn<OrganizationSummary>({
          id: "credits",
          accessorFn: (row) => row.ai_credits,
          labelKey: "commerce.org.list.credits",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col gap-0.5 text-xs leading-tight tabular-nums">
              <span className="flex items-center gap-1.5">
                <Bot className="text-muted-foreground size-3.5" />
                <span
                  className={
                    row.original.ai_credits <= 0
                      ? "text-muted-foreground"
                      : "font-medium"
                  }
                >
                  {formatCount(row.original.ai_credits, locale)}
                </span>
                <span className="text-muted-foreground">
                  {t("commerce.org.list.ai_short")}
                </span>
              </span>
              <span className="flex items-center gap-1.5">
                <Video className="text-muted-foreground size-3.5" />
                <span
                  className={
                    row.original.video_minutes <= 0
                      ? "text-muted-foreground"
                      : "font-medium"
                  }
                >
                  {formatCount(row.original.video_minutes, locale)}
                </span>
                <span className="text-muted-foreground">
                  {t("commerce.org.list.minutes_short")}
                </span>
              </span>
            </span>
          ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "last_activity_at",
          labelKey: "organizations.columns.last_activity_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.last_activity_at ? (
              <time
                dateTime={row.original.last_activity_at}
                title={datetime(
                  row.original.last_activity_at,
                  undefined,
                  locale,
                )}
              >
                {relativeDatetime(row.original.last_activity_at, locale)}
              </time>
            ) : (
              <span className="text-muted-foreground">
                {t("organizations.no_activity")}
              </span>
            ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "status",
          labelKey: "organizations.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_STATUSES.map((value) => ({
            value,
            labelKey: STATUS_LABEL_KEYS[value],
            label: t(STATUS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <OrganizationStatusChip status={row.original.status} />
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "access",
          accessorFn: (row) => row.access_state,
          labelKey: "organizations.columns.access",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_ACCESS_STATES.map((value) => ({
            value,
            labelKey: ACCESS_LABEL_KEYS[value],
            label: t(ACCESS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <OrganizationAccessChip state={row.original.access_state} />
          ),
        }),
        createColumn<OrganizationSummary>({
          id: "window",
          accessorFn: (row) => row.access_ends_at ?? "",
          labelKey: "organizations.columns.access_ends_at",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: ORGANIZATION_WINDOWS.map((value) => ({
            value,
            labelKey: WINDOW_LABEL_KEYS[value],
            label: t(WINDOW_LABEL_KEYS[value]),
          })),
          cell: ({ row }) =>
            row.original.access_ends_at ? (
              date(row.original.access_ends_at, undefined, locale)
            ) : (
              <span className="text-muted-foreground">
                {t("organizations.window.unlimited")}
              </span>
            ),
        }),
        createColumn<OrganizationSummary>({
          id: "region",
          accessorFn: (row) =>
            [row.country_code, row.locale, row.currency, row.timezone].join(
              " ",
            ),
          labelKey: "organizations.columns.region",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-xs">
              {row.original.country_code} · {row.original.locale.toUpperCase()}{" "}
              · {row.original.currency} · {row.original.timezone}
            </span>
          ),
        }),
        createColumn<OrganizationSummary>({
          accessorKey: "created_at",
          labelKey: "organizations.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<OrganizationSummary>({
          id: "actions",
          labelKey: "organizations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const org = row.original;
            const write = permissions.organizations.write;
            const actions: EntityRowAction[] = [
              {
                id: "view",
                label: t("organizations.actions.view"),
                icon: Eye,
                onSelect: () => handlers.onView(org),
              },
              {
                id: "edit",
                label: t("organizations.actions.edit"),
                icon: Pencil,
                permission: write,
                onSelect: () => handlers.onEdit(org),
              },
              {
                id: "toggle-status",
                label:
                  org.status === "active"
                    ? t("organizations.actions.suspend")
                    : t("organizations.actions.activate"),
                icon: org.status === "active" ? Pause : Play,
                permission: write,
                onSelect: () => handlers.onToggleStatus(org),
              },
              {
                id: "delete",
                label: t("organizations.actions.delete"),
                icon: Trash2,
                permission: write,
                variant: "destructive",
                separatorBefore: true,
                onSelect: () => handlers.onDelete(org),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<OrganizationSummary, unknown>[],
    [handlers, locale, t],
  );
}
