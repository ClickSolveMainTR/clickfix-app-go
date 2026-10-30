"use client";

import { BarChart3 } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { CustomReportsTab } from "./custom-reports-tab";
import { ScorecardsTab } from "./scorecards-tab";
import { SchedulesTab } from "./schedules-tab";

/**
 * Reports (CF-112): agent scorecards, saved custom reports and scheduled
 * CSV/XLSX e-mail deliveries. The API requires conversations read +
 * read_all; saving and scheduling also need conversations manage.
 */
export function ReportsPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canRead =
    permissions.includes(Permission.TenantConversationsRead) &&
    permissions.includes(Permission.TenantConversationsReadAll);
  const canWrite =
    canRead && permissions.includes(Permission.TenantConversationsManage);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<BarChart3 className="size-7" />}
        title={t("reports.title")}
        description={t("reports.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("reports.no_organization")}
        </p>
      ) : !canRead ? (
        <p className="text-muted-foreground text-sm">
          {t("reports.forbidden")}
        </p>
      ) : (
        <Tabs defaultValue="scorecards">
          <TabsList className="max-w-full overflow-x-auto">
            <TabsTrigger value="scorecards">
              {t("reports.tabs.scorecards")}
            </TabsTrigger>
            <TabsTrigger value="custom">{t("reports.tabs.custom")}</TabsTrigger>
            <TabsTrigger value="schedules">
              {t("reports.tabs.schedules")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="scorecards" className="mt-4">
            <ScorecardsTab slug={slug} />
          </TabsContent>
          <TabsContent value="custom" className="mt-4">
            <CustomReportsTab slug={slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="schedules" className="mt-4">
            <SchedulesTab slug={slug} canWrite={canWrite} />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
