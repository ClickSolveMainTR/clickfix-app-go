"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useTeamsColumns,
  type TeamRowActionHandlers,
} from "@/features/teams/components/teams-columns";
import {
  useDeleteTeam,
  useSkillCatalog,
  useTeamsList,
} from "@/features/teams/hooks/use-teams";
import type { ListTeamsParams, Team } from "@/features/teams/types";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

/**
 * Teams of the active organization: name and color, leads, member count,
 * skills (faceted filter) and working hours ("Organizasyon saatleri" when
 * the team follows the organization's business hours).
 */
export function TeamsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
  });
  const [deleteTarget, setDeleteTarget] = useState<Team | null>(null);
  const skillsQuery = useSkillCatalog();
  const skillOptions = useMemo(
    () => skillsQuery.data?.items ?? [],
    [skillsQuery.data],
  );

  const listParams = useMemo<ListTeamsParams>(() => {
    const skills = filterValues(
      listState.columnFilters.find((filter) => filter.id === "skills")?.value,
    );
    const { limit, offset, q, sort } = listState.params;
    return {
      limit,
      offset,
      q,
      sort,
      // The API filters by one skill.
      skill: skills[0],
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useTeamsList(listParams);
  const remove = useDeleteTeam();

  const openTeam = useCallback(
    (team: Team) => router.push(routes.cms.teams.detail(team.uuid)),
    [router],
  );
  const handlers = useMemo<TeamRowActionHandlers>(
    () => ({ onOpen: openTeam, onDelete: setDeleteTarget }),
    [openTeam],
  );
  const columns = useTeamsColumns(handlers, skillOptions);

  const total = listQuery.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );
  const hasFilters = Boolean(listParams.q) || Boolean(listParams.skill);

  return (
    <EntityPage
      title={t("teams.title")}
      description={t("teams.description")}
      permission={permissions.tenant.teams.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("teams.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("teams.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.cms.teams.create)}
          label={t("teams.actions.create")}
          permission={permissions.tenant.teams.write}
        />
      }
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openTeam}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        errorDescription={t("teams.load_failed")}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={
          hasFilters ? t("teams.empty_filtered_title") : t("teams.empty_title")
        }
        emptyDescription={
          hasFilters
            ? t("teams.empty_filtered_description")
            : t("teams.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{ persistKey: "tenant-teams-v1", viewMode: true }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntityPage>
  );
}
