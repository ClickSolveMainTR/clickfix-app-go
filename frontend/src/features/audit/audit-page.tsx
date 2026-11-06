"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Download, FileJson } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  activityLabel,
  useAuditExport,
  useAuditFacets,
  useAuditList,
  type AuditEvent,
  type AuditFilters,
  type ListAuditParams,
} from "@/features/audit/audit";
import { useTenantMembersList } from "@/features/tenant-members/hooks/use-tenant-members";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const MEMBER_PARAMS = { limit: 100, offset: 0, sort: "name" };

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function actorName(event: AuditEvent) {
  if (!event.actor) return null;
  return (
    `${event.actor.name} ${event.actor.surname}`.trim() || event.actor.email
  );
}

function PayloadPopover({ payload }: { payload: Record<string, unknown> }) {
  const { t } = useLocale();
  const keys = Object.keys(payload);
  if (!keys.length) return <span className="text-muted-foreground">—</span>;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button type="button" variant="ghost" size="sm" className="h-7 gap-1">
          <FileJson className="size-3.5" />
          {t("audit.payload_fields", { count: keys.length })}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-96 max-w-[90vw]">
        <p className="mb-2 text-sm font-medium">{t("audit.columns.payload")}</p>
        <pre className="bg-muted max-h-80 overflow-auto rounded-md p-3 text-xs">
          {JSON.stringify(payload, null, 2)}
        </pre>
      </PopoverContent>
    </Popover>
  );
}

/**
 * Organization audit log (CF-12): who did what and when, filterable by
 * actor, resource, action and a date range, with CSV export. Actions taken
 * through ClickFix support access carry a "Destek" badge.
 */
export function AuditPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 50,
  });
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const facets = useAuditFacets();
  const members = useTenantMembersList(MEMBER_PARAMS);
  const exportCsv = useAuditExport();

  const filters = useMemo<AuditFilters>(() => {
    const column = (id: string) => {
      const values = filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
      return values.length ? values.join(",") : undefined;
    };
    return {
      q: listState.params.q,
      actor: column("actor"),
      resource: column("resource"),
      action: column("action"),
      from: from || undefined,
      to: to || undefined,
    };
  }, [from, listState.columnFilters, listState.params.q, to]);

  const listParams = useMemo<ListAuditParams>(
    () => ({
      ...filters,
      limit: listState.params.limit,
      offset: listState.params.offset,
      sort: listState.params.sort,
    }),
    [filters, listState.params],
  );
  const listQuery = useAuditList(listParams);

  const resourceOptions = useMemo(
    () =>
      (facets.data?.items ?? []).map((facet) => ({
        value: facet.resource,
        label: activityLabel(t, "resources", facet.resource),
      })),
    [facets.data?.items, t],
  );
  const actionOptions = useMemo(() => {
    const chosen = filters.resource?.split(",") ?? [];
    const actions = new Set<string>();
    for (const facet of facets.data?.items ?? []) {
      if (chosen.length && !chosen.includes(facet.resource)) continue;
      facet.actions.forEach((action) => actions.add(action));
    }
    return [...actions].sort().map((value) => ({
      value,
      label: activityLabel(t, "actions", value),
    }));
  }, [facets.data?.items, filters.resource, t]);
  const actorOptions = useMemo(
    () =>
      (members.data?.items ?? []).map((m) => ({
        value: m.user.uuid,
        label: `${m.user.name} ${m.user.surname}`.trim() || m.user.email,
      })),
    [members.data?.items],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<AuditEvent>({
          accessorKey: "created_at",
          labelKey: "audit.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, undefined, locale)}
            </span>
          ),
        }),
        createColumn<AuditEvent>({
          id: "actor",
          accessorFn: (row) => actorName(row) ?? "",
          labelKey: "audit.columns.actor",
          enableSorting: false,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: actorOptions,
          cell: ({ row }) => {
            const name = actorName(row.original);
            const viaSupport = row.original.payload?.via_support === true;
            return (
              <span className="flex min-w-0 items-center gap-1.5">
                <span className="truncate">
                  {name ?? (
                    <span className="text-muted-foreground">
                      {t("audit.system_actor")}
                    </span>
                  )}
                </span>
                {viaSupport ? (
                  <Badge variant="warning" className="shrink-0">
                    {t("audit.via_support")}
                  </Badge>
                ) : null}
              </span>
            );
          },
        }),
        createColumn<AuditEvent>({
          accessorKey: "action",
          labelKey: "audit.columns.action",
          enableSorting: false,
          gridPrimary: true,
          filterVariant: "faceted",
          filterOptions: actionOptions,
          cell: ({ row }) => (
            <span className="font-medium" title={row.original.action}>
              {activityLabel(t, "actions", row.original.action)}
            </span>
          ),
        }),
        createColumn<AuditEvent>({
          accessorKey: "resource",
          labelKey: "audit.columns.resource",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: resourceOptions,
          cell: ({ row }) => (
            <span title={row.original.resource}>
              {activityLabel(t, "resources", row.original.resource)}
            </span>
          ),
        }),
        createColumn<AuditEvent>({
          id: "payload",
          accessorFn: (row) => JSON.stringify(row.payload),
          labelKey: "audit.columns.payload",
          enableSorting: false,
          cell: ({ row }) => (
            <PayloadPopover payload={row.original.payload ?? {}} />
          ),
        }),
        createColumn<AuditEvent>({
          id: "ip_address",
          accessorFn: (row) => row.ip_address ?? "",
          labelKey: "audit.columns.ip_address",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.ip_address ? (
              <code className="text-xs">{row.original.ip_address}</code>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
      ] as ColumnDef<AuditEvent, unknown>[],
    [actionOptions, actorOptions, locale, resourceOptions, t],
  );

  const total = listQuery.data?.total ?? 0;
  const hasFilters = Object.values(filters).some(Boolean);

  return (
    <EntityPage
      title={t("audit.title")}
      description={t("audit.description")}
      permission={permissions.tenant.audit.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("audit.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.cms.settings.organization,
        },
        { label: t("audit.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("audit.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters ? t("audit.empty_filtered_title") : t("audit.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("audit.empty_filtered_description")
            : t("audit.empty_description")
        }
        total={total}
        state={listState.tableState}
        features={{ persistKey: "tenant-audit-v1" }}
        toolbarExtra={
          <>
            <DatePicker
              value={from}
              onChange={setFrom}
              placeholder={t("audit.from")}
              className="w-40"
            />
            <DatePicker
              value={to}
              onChange={setTo}
              placeholder={t("audit.to")}
              className="w-40"
            />
            {from || to ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => {
                  setFrom("");
                  setTo("");
                }}
              >
                {t("audit.clear_dates")}
              </Button>
            ) : null}
            {can(permissions.tenant.audit.export) ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={exportCsv.isPending}
                onClick={() =>
                  exportCsv.mutate(filters, {
                    onError: () => appToast.error(t("audit.export_failed")),
                  })
                }
              >
                <Download className="size-4" />
                {t("audit.export_csv")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
