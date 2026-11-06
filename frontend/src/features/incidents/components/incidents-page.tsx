"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityDrawer,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  IncidentSeverityBadge,
  IncidentStatusChip,
  IncidentVisibilityBadge,
} from "@/features/oncall/components/incident-badges";
import {
  oncallService,
  type StatusIncident,
} from "@/features/oncall/services/oncall.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime, relativeDatetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { latestUpdates, useIncidentsColumns } from "./incidents-columns";

/**
 * Service incidents and maintenance of the active organization, read-only
 * (CF-162): what agents need to answer customers during an outage. Opening
 * and updating incidents stays under On-call.
 */
export function IncidentsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const listState = useServerListState({ initialPageSize: 20 });
  const columns = useIncidentsColumns();
  const [open, setOpen] = useState<StatusIncident | null>(null);
  const { limit, offset } = listState.params;

  const listQuery = useQuery({
    queryKey: ["tenant", slug, "status", "incidents", "page", limit, offset],
    queryFn: () => oncallService.incidentsPage(slug, { limit, offset }),
    enabled: Boolean(slug),
    placeholderData: keepPreviousData,
    refetchInterval: 2 * 60_000,
  });

  const pageCount = Math.max(
    1,
    Math.ceil((listQuery.data?.total ?? 0) / (limit || 20)),
  );

  return (
    <EntityPage
      title={t("oncall.incidents.page_title")}
      description={t("oncall.incidents.page_description")}
      permission={permissions.tenant.conversations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("oncall.incidents.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("oncall.incidents.page_title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid ?? ""}
        onRowClick={setOpen}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("oncall.incidents.unavailable")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("oncall.incidents.empty_title")}
        emptyDescription={t("oncall.incidents.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-incidents-v1",
          sorting: false,
          rowSelection: false,
          columnFilters: false,
          globalFilter: false,
          facetedFilters: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <EntityDrawer
        open={Boolean(open)}
        onOpenChange={(next) => !next && setOpen(null)}
        title={open?.title ?? ""}
      >
        {open ? <IncidentDetail incident={open} /> : null}
      </EntityDrawer>
    </EntityPage>
  );
}

/** Same fields as the mobile incident card: badges, times, last 3 updates. */
function IncidentDetail({ incident }: { incident: StatusIncident }) {
  const { t, locale } = useLocale();
  const at = (value: string) => datetime(value, "dd.MM.yyyy HH:mm", locale);
  const updates = latestUpdates(incident, 3);
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <IncidentStatusChip incident={incident} />
        <IncidentSeverityBadge incident={incident} />
        <IncidentVisibilityBadge incident={incident} />
      </div>
      <p className="text-muted-foreground text-sm">
        {[
          incident.started_at
            ? t("oncall.incidents.started", { at: at(incident.started_at) })
            : null,
          incident.resolved_at
            ? t("oncall.incidents.resolved", { at: at(incident.resolved_at) })
            : null,
        ]
          .filter(Boolean)
          .join(" · ")}
      </p>
      <section className="space-y-3">
        <h3 className="text-sm font-semibold">
          {t("oncall.incidents.updates")}
        </h3>
        {updates.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("oncall.incidents.no_updates")}
          </p>
        ) : (
          <ul className="space-y-3">
            {updates.map((u) => (
              <li key={u.uuid} className="space-y-0.5 border-l-2 pl-3">
                <p className="text-xs font-semibold">
                  {[
                    u.status,
                    u.created_at
                      ? relativeDatetime(u.created_at, locale)
                      : null,
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
                {u.body ? (
                  <p className="text-sm whitespace-pre-wrap">{u.body}</p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
