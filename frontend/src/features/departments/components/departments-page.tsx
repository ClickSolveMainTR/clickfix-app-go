"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useDepartmentsColumns,
  type DepartmentRowActionHandlers,
} from "@/features/departments/components/departments-columns";
import { DeleteDepartmentDialog } from "@/features/departments/components/delete-department-dialog";
import { useDepartmentsList } from "@/features/departments/hooks/use-departments";
import type {
  Department,
  ListDepartmentsParams,
} from "@/features/departments/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Departments of the active organization (CF-215): business units such as
 * technical support or sales, with their teams, leads, SLA and AI settings.
 */
export function DepartmentsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({ initialPageSize: 20 });
  const [deleteTarget, setDeleteTarget] = useState<Department | null>(null);
  const listParams = useMemo<ListDepartmentsParams>(() => {
    const { limit, offset, q } = listState.params;
    return { limit, offset, q };
  }, [listState.params]);
  const listQuery = useDepartmentsList(listParams);
  const open = useCallback(
    (department: Department) =>
      router.push(routes.cms.departments.detail(department.uuid)),
    [router],
  );
  const handlers = useMemo<DepartmentRowActionHandlers>(
    () => ({ onOpen: open, onDelete: setDeleteTarget }),
    [open],
  );
  const columns = useDepartmentsColumns(handlers);
  const hasFilters = Boolean(listParams.q);

  return (
    <EntityPage
      title={t("departments.title")}
      description={t("departments.description")}
      permission={permissions.tenant.departments.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("departments.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("departments.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.cms.departments.create)}
          label={t("departments.actions.create")}
          permission={permissions.tenant.departments.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={open}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("departments.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters
            ? t("departments.empty_filtered_title")
            : t("departments.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("departments.empty_filtered_description")
            : t("departments.empty_description")
        }
        total={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: "tenant-departments-v1", viewMode: true }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
      <DeleteDepartmentDialog
        department={deleteTarget}
        onClose={() => setDeleteTarget(null)}
      />
    </EntityPage>
  );
}
