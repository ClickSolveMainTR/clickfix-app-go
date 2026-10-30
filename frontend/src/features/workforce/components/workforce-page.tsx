"use client";

import { CalendarClock } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { ForecastTab } from "./forecast-tab";
import { TimesheetsTab } from "./timesheets-tab";

const TABS = ["forecast", "timesheets"] as const;
type Tab = (typeof TABS)[number];

/**
 * Workforce (CF-107): volume forecast with staffing gaps against scheduled
 * shifts (tenant.schedules.read) and agent timesheets with export
 * (tenant.schedules.manage).
 */
export function WorkforcePage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const canRead = permissions.includes(Permission.TenantSchedulesRead);
  const canManage = permissions.includes(Permission.TenantSchedulesManage);
  const tabs = TABS.filter((k) => k !== "timesheets" || canManage);
  const raw = params.get("tab") as Tab;
  const tab: Tab = tabs.includes(raw) ? raw : "forecast";

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<CalendarClock className="size-7" />}
        title={t("workforce.title")}
        description={t("workforce.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("workforce.no_organization")}
        </p>
      ) : !canRead ? (
        <p className="text-muted-foreground text-sm">
          {t("workforce.forbidden")}
        </p>
      ) : (
        <Tabs
          value={tab}
          onValueChange={(v) =>
            router.replace(`${pathname}?tab=${v}`, { scroll: false })
          }
        >
          <TabsList className="max-w-full overflow-x-auto">
            {tabs.map((k) => (
              <TabsTrigger key={k} value={k}>
                {t(`workforce.tabs.${k}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="forecast" className="mt-4">
            <ForecastTab slug={slug} />
          </TabsContent>
          {canManage && (
            <TabsContent value="timesheets" className="mt-4">
              <TimesheetsTab slug={slug} />
            </TabsContent>
          )}
        </Tabs>
      )}
    </div>
  );
}
