"use client";

import { Trash2 } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityPage,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TeamForm } from "@/features/teams/components/team-form";
import { TeamMembersTab } from "@/features/teams/components/team-members-tab";
import { TeamColorDot } from "@/features/teams/components/teams-columns";
import {
  useCreateTeam,
  useDeleteTeam,
  useTeam,
  useUpdateTeam,
} from "@/features/teams/hooks/use-teams";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("teams.forbidden")}
    />
  );
}

/** `/teams/create`: details and hours; members are added on the team page. */
export function TeamCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateTeam();
  return (
    <EntityPage
      title={t("teams.create_title")}
      description={t("teams.create_description")}
      permission={permissions.tenant.teams.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("teams.title"), href: routes.cms.teams.root },
        { label: t("teams.create_title") },
      ]}
    >
      <TeamForm
        submitLabel={t("teams.actions.create")}
        onCancel={() => router.push(routes.cms.teams.root)}
        onSubmit={async (input) => {
          const team = await create.mutateAsync(input);
          router.replace(`${routes.cms.teams.detail(team.uuid)}?tab=members`);
        }}
      />
    </EntityPage>
  );
}

type Tab = "general" | "members";

/** `/teams/{uuid}`: Genel (details + hours) and Üyeler tabs (`?tab=`). */
export function TeamDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const tab: Tab =
    searchParams.get("tab") === "members" ? "members" : "general";
  const query = useTeam(uuid);
  const update = useUpdateTeam(uuid);
  const remove = useDeleteTeam();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const canWrite = can(permissions.tenant.teams.write);
  const team = query.data;
  const title = team?.name ?? t("teams.detail_title");

  return (
    <EntityPage
      title={title}
      description={team?.description || t("teams.detail_description")}
      permission={permissions.tenant.teams.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("teams.title"), href: routes.cms.teams.root },
        { label: title },
      ]}
      actions={
        team && canWrite ? (
          <EntityActions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive"
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 className="size-4" />
              {t("teams.actions.delete")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("teams.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {team ? (
        <Tabs
          value={tab}
          onValueChange={(next) =>
            router.replace(
              next === "members" ? `${pathname}?tab=members` : pathname,
              { scroll: false },
            )
          }
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="general">
              <TeamColorDot color={team.color} />
              {t("teams.tabs.general")}
            </TabsTrigger>
            <TabsTrigger value="members">
              {t("teams.tabs.members")} ({team.member_count})
            </TabsTrigger>
          </TabsList>
          <TabsContent value="general">
            <TeamForm
              team={team}
              disabled={!canWrite}
              submitLabel={t("teams.actions.save")}
              onSubmit={(input) => update.mutateAsync(input)}
            />
          </TabsContent>
          <TabsContent value="members">
            <TeamMembersTab team={team} />
          </TabsContent>
        </Tabs>
      ) : null}

      <EntityDeleteDialog
        open={confirmDelete}
        softDelete={false}
        entityLabel={team?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() =>
          remove.mutate(uuid, {
            onSuccess: () => router.push(routes.cms.teams.root),
            onSettled: () => setConfirmDelete(false),
          })
        }
      />
    </EntityPage>
  );
}
