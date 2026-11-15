"use client";

import { Trash2 } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityActions, EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { DeleteDepartmentDialog } from "@/features/departments/components/delete-department-dialog";
import { DepartmentForm } from "@/features/departments/components/department-form";
import { DepartmentSharesTab } from "@/features/departments/components/department-shares-tab";
import {
  useCreateDepartment,
  useDepartment,
  useUpdateDepartment,
} from "@/features/departments/hooks/use-departments";
import { TeamColorDot } from "@/features/teams/components/teams-columns";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("departments.forbidden")}
    />
  );
}

/** `/departments/create`. */
export function DepartmentCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const create = useCreateDepartment();
  return (
    <EntityPage
      title={t("departments.create_title")}
      description={t("departments.create_description")}
      permission={permissions.tenant.departments.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("departments.title"), href: routes.cms.departments.root },
        { label: t("departments.create_title") },
      ]}
    >
      <DepartmentForm
        submitLabel={t("departments.actions.create")}
        onCancel={() => router.push(routes.cms.departments.root)}
        onSubmit={async (input) => {
          const department = await create.mutateAsync(input);
          router.replace(routes.cms.departments.detail(department.uuid));
        }}
      />
    </EntityPage>
  );
}

type Tab = "general" | "shares";

/** `/departments/{uuid}`: Genel and Paylaşılan bilgi tabs (`?tab=`). */
export function DepartmentDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { user } = useAuth();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const tab: Tab = searchParams.get("tab") === "shares" ? "shares" : "general";
  const query = useDepartment(uuid);
  const update = useUpdateDepartment(uuid);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const canWrite = can(permissions.tenant.departments.write);
  const department = query.data;
  const title = department?.name ?? t("departments.detail_title");
  // Leads share their department's knowledge; the API checks it again.
  const isLead = Boolean(
    department?.leads.some((lead) => lead.uuid === user?.uuid),
  );

  return (
    <EntityPage
      title={title}
      description={
        department?.description || t("departments.detail_description")
      }
      permission={permissions.tenant.departments.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("departments.title"), href: routes.cms.departments.root },
        { label: title },
      ]}
      actions={
        department && canWrite ? (
          <EntityActions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive"
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 className="size-4" />
              {t("departments.actions.delete")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("departments.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {department ? (
        <Tabs
          value={tab}
          onValueChange={(next) =>
            router.replace(
              next === "shares" ? `${pathname}?tab=shares` : pathname,
              { scroll: false },
            )
          }
          className="space-y-4"
        >
          <TabsList>
            <TabsTrigger value="general">
              <TeamColorDot color={department.color} />
              {t("departments.tabs.general")}
            </TabsTrigger>
            <TabsTrigger value="shares">
              {t("departments.tabs.shares")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="general">
            <DepartmentForm
              department={department}
              disabled={!canWrite}
              submitLabel={t("departments.actions.save")}
              onSubmit={(input) => update.mutateAsync(input)}
            />
          </TabsContent>
          <TabsContent value="shares">
            <DepartmentSharesTab
              department={department}
              canShare={canWrite || isLead}
            />
          </TabsContent>
        </Tabs>
      ) : null}
      <DeleteDepartmentDialog
        department={confirmDelete ? (department ?? null) : null}
        onClose={() => setConfirmDelete(false)}
        onDeleted={() => router.push(routes.cms.departments.root)}
      />
    </EntityPage>
  );
}
