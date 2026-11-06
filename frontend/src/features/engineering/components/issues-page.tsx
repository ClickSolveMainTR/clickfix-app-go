"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useIssuesColumns } from "@/features/engineering/components/issues-columns";
import { useIssues } from "@/features/engineering/hooks/use-engineering";
import {
  ISSUE_PRIORITIES,
  ISSUE_STATUSES,
} from "@/features/engineering/lib/issue-display";
import type {
  Issue,
  IssuePriority,
  IssueStatus,
  ListIssuesParams,
} from "@/features/engineering/services/engineering.service";
import { useLocale } from "@/providers/locale-provider";

/** "active" hides done and canceled requests (the mobile default). */
type StatusFilter = "active" | "all" | IssueStatus;
type PriorityFilter = "all" | IssuePriority;

/**
 * Product requests of the active organization: search, one status and one
 * priority filter, newest change first. Ported from the mobile
 * `IssuesScreen`, including its "active" default.
 */
export function IssuesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "",
    initialPageSize: 20,
  });

  const [status, setStatus] = useState<StatusFilter>("active");
  const [priority, setPriority] = useState<PriorityFilter>("all");
  const { setPagination } = listState;
  const firstPage = () => setPagination((prev) => ({ ...prev, pageIndex: 0 }));

  const listParams = useMemo<ListIssuesParams>(() => {
    const { limit, offset, q } = listState.params;
    return {
      limit,
      offset,
      q,
      status: status === "active" || status === "all" ? undefined : status,
      priority: priority === "all" ? undefined : priority,
    };
  }, [listState.params, priority, status]);

  const listQuery = useIssues(listParams);
  // The API has no "open" filter, so done and canceled rows are dropped from
  // the loaded page, as on mobile; such a page can be shorter than its size.
  const rows = useMemo(() => {
    const items = listQuery.data?.items ?? [];
    return status === "active"
      ? items.filter((i) => i.status !== "done" && i.status !== "canceled")
      : items;
  }, [listQuery.data?.items, status]);
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
    (status !== "active" && status !== "all") ||
    priority !== "all";

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
        data={rows}
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
          <>
            <Select
              value={status}
              onValueChange={(next) => {
                setStatus(next as StatusFilter);
                firstPage();
              }}
            >
              <SelectTrigger
                className="h-8 w-auto"
                aria-label={t("engineering.columns.status")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="active">
                  {t("engineering.filter.active")}
                </SelectItem>
                <SelectItem value="all">
                  {t("engineering.filter.all_statuses")}
                </SelectItem>
                {ISSUE_STATUSES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(`engineering.status.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={priority}
              onValueChange={(next) => {
                setPriority(next as PriorityFilter);
                firstPage();
              }}
            >
              <SelectTrigger
                className="h-8 w-auto"
                aria-label={t("engineering.columns.priority")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  {t("engineering.filter.all_priorities")}
                </SelectItem>
                {ISSUE_PRIORITIES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(`inbox.priority.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
