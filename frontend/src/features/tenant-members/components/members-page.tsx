"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { MemberSkillsDrawer } from "@/features/teams/components/member-skills-drawer";
import { MemberAccessDrawer } from "@/features/tenant-members/components/member-access-drawer";
import type { MemberRowActionHandlers } from "@/features/tenant-members/components/member-row-actions";
import { useMembersColumns } from "@/features/tenant-members/components/members-columns";
import { TenantInvitationsTab } from "@/features/tenant-members/components/tenant-invitations-tab";
import {
  useRemoveTenantMember,
  useTenantMembersList,
  useTenantSeats,
} from "@/features/tenant-members/hooks/use-tenant-members";
import type {
  ListTenantMembersParams,
  TenantMember,
} from "@/features/tenant-members/types";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

type MembersTab = "members" | "invitations";

/**
 * Organization members: roles, status and last sign-in, with server-side
 * search / sort / faceted filters and a drawer to change roles. The
 * "Davetler" tab lists staff invitations; the active tab is mirrored to
 * `?tab=`.
 */
export function MembersPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const tab: MembersTab =
    searchParams.get("tab") === "invitations" ? "invitations" : "members";
  const setTab = (next: string) => {
    router.replace(
      next === "invitations" ? `${pathname}?tab=invitations` : pathname,
      {
        scroll: false,
      },
    );
  };

  const listState = useServerListState({
    initialSort: "created_at",
    initialPageSize: 20,
  });
  const [editing, setEditing] = useState<TenantMember | null>(null);
  const [removing, setRemoving] = useState<TenantMember | null>(null);
  const [skillsOf, setSkillsOf] = useState<TenantMember | null>(null);

  const listParams = useMemo<ListTenantMembersParams>(() => {
    const column = (id: string) =>
      filterValues(
        listState.columnFilters.find((filter) => filter.id === id)?.value,
      );
    const roles = column("roles");
    return {
      ...listState.params,
      role: roles.length ? roles.join(",") : undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useTenantMembersList(listParams);
  const rolesQuery = useAllTenantRoles(can(permissions.tenant.roles.read));
  const remove = useRemoveTenantMember();
  const seats = useTenantSeats().data;

  const openAccess = useCallback(
    (member: TenantMember) => {
      if (can(permissions.tenant.members.write)) setEditing(member);
    },
    [can],
  );
  const handlers = useMemo<MemberRowActionHandlers>(
    () => ({
      onEditAccess: setEditing,
      onRemove: setRemoving,
      onEditSkills: setSkillsOf,
    }),
    [],
  );
  const columns = useMembersColumns({
    handlers,
    roles: rolesQuery.data?.items ?? [],
  });

  const total = listQuery.data?.total ?? 0;
  const pageCount = useMemo(() => {
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listState.pagination.pageSize, total]);
  const hasFilters = Boolean(listParams.q) || Boolean(listParams.role);

  const removingName = removing
    ? `${removing.user.name} ${removing.user.surname}`.trim() ||
      removing.user.email
    : "";

  return (
    <EntityPage
      title={t("tenant_members.title")}
      description={t("tenant_members.description")}
      permission={permissions.tenant.members.read}
      actions={
        seats ? (
          <Badge variant="outline" className="tabular-nums">
            {t("tenant_members.seats", {
              billable: seats.billable,
              total: seats.total,
            })}
          </Badge>
        ) : null
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tenant_members.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.cms.settings.organization,
        },
        { label: t("tenant_members.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList>
          <TabsTrigger value="members">
            {t("tenant_members.tabs.members")}
          </TabsTrigger>
          <TabsTrigger value="invitations">
            {t("tenant_members.tabs.invitations")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="members">
          <EntityTable
            columns={columns}
            data={listQuery.data?.items ?? []}
            getRowId={(row) => row.user.uuid}
            onRowClick={openAccess}
            isLoading={listQuery.isLoading}
            isError={listQuery.isError}
            errorDescription={t("tenant_members.load_failed")}
            onRetry={() => void listQuery.refetch()}
            emptyTitle={
              hasFilters
                ? t("tenant_members.empty_filtered_title")
                : t("tenant_members.empty_title")
            }
            emptyDescription={
              hasFilters
                ? t("tenant_members.empty_filtered_description")
                : t("tenant_members.empty_description")
            }
            pageCount={pageCount}
            state={listState.tableState}
            features={{ persistKey: "tenant-members-v1", viewMode: true }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void listQuery.refetch()}
                refreshDisabled={listQuery.isFetching}
              />
            }
          />
        </TabsContent>
        <TabsContent value="invitations">
          <TenantInvitationsTab active={tab === "invitations"} />
        </TabsContent>
      </Tabs>

      <MemberAccessDrawer
        member={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />

      <MemberSkillsDrawer
        member={skillsOf}
        onOpenChange={(open) => {
          if (!open) setSkillsOf(null);
        }}
      />

      <ConfirmDialog
        open={Boolean(removing)}
        title={t("tenant_members.remove.title", { name: removingName })}
        description={t("tenant_members.remove.description", {
          name: removingName,
        })}
        confirmLabel={t("tenant_members.actions.remove")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setRemoving(null)}
        onConfirm={() => {
          if (!removing) return;
          remove.mutate(removing.user.uuid, {
            onSettled: () => setRemoving(null),
          });
        }}
      />
    </EntityPage>
  );
}
