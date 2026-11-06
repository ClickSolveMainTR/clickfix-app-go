"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useIssuesColumns } from "@/features/engineering/components/issues-columns";
import { useIssues } from "@/features/engineering/hooks/use-engineering";
import type {
  Issue,
  IssuePriority,
  IssueStatus,
  ListIssuesParams,
} from "@/features/engineering/services/engineering.service";
import { useLocale } from "@/providers/locale-provider";

function firstValue(value: unknown): string | undefined {
  if (Array.isArray(value)) return value.map(String).find(Boolean);
  return typeof value === "string" && value ? value : undefined;
}

/**
 * Product requests of the active organization: search, status and priority
 * filters, newest change first. Ported from the mobile `IssuesScreen`.
 */
export function IssuesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
  });

  const listParams = useMemo<ListIssuesParams>(() => {
    const column = (id: string) =>
      firstValue(listState.columnFilters.find((f) => f.id === id)?.value);
    const { limit, offset, q } = listState.params;
    return {
      limit,
      offset,
      q,
      // The API filters by one status and one priority.
      status: column("status") as IssueStatus | undefined,
      priority: column("priority") as IssuePriority | undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useIssues(listParams);
  const columns = useIssuesColumns();
  const openIssue = useCallback(
    (issue: Issue) => router.push(routes.cms.engineering.detail(issue.uuid)),
    [router],
  );

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters =
    Boolean(listParams.q) ||
    Boolean(listParams.status) ||
    Boolean(listParams.priority);

  return (
    <EntityPage
      title={t("engineering.title")}
      description={t("engineering.description")}
      permission={permissions.tenant.tickets.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("engineering.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("engineering.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openIssue}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("engineering.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("engineering.empty_filtered_title")
            : t("engineering.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("engineering.empty_filtered_description")
            : t("engineering.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "tenant-engineering-issues-v1" }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
