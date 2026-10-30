"use client";

import { Siren } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { AlertsTab } from "./alerts-tab";
import { MonitorsTab } from "./monitors-tab";
import { PoliciesTab } from "./policies-tab";
import { RotasTab } from "./rotas-tab";

const TABS = ["rotas", "policies", "monitors", "alerts"] as const;
type Tab = (typeof TABS)[number];

/**
 * On-call and uptime (CF-104): rotas, escalation policies, HTTP uptime
 * monitors and the alerts/incidents they raise. Read with
 * tenant.schedules.read, change with tenant.schedules.manage; the API also
 * requires organization-wide conversation access (read_all).
 */
export function OncallPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const raw = params.get("tab");
  const tab: Tab = TABS.includes(raw as Tab) ? (raw as Tab) : "rotas";
  const incident = params.get("incident");

  const canRead =
    permissions.includes(Permission.TenantSchedulesRead) &&
    permissions.includes(Permission.TenantConversationsReadAll);
  const canWrite =
    canRead && permissions.includes(Permission.TenantSchedulesManage);
  const canManageComponents =
    permissions.includes(Permission.TenantConversationsRead) &&
    permissions.includes(Permission.TenantConversationsManage);

  const go = (next: Tab, extra?: Record<string, string>) => {
    const q = new URLSearchParams({ tab: next, ...extra });
    router.replace(`${pathname}?${q.toString()}`, { scroll: false });
  };

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Siren className="size-7" />}
        title={t("oncall.title")}
        description={t("oncall.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("oncall.no_organization")}
        </p>
      ) : !canRead ? (
        <p className="text-muted-foreground text-sm">{t("oncall.forbidden")}</p>
      ) : (
        <Tabs value={tab} onValueChange={(v) => go(v as Tab)}>
          <TabsList className="max-w-full overflow-x-auto">
            {TABS.map((k) => (
              <TabsTrigger key={k} value={k}>
                {t(`oncall.tabs.${k}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          <TabsContent value="rotas" className="mt-4">
            <RotasTab slug={slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="policies" className="mt-4">
            <PoliciesTab slug={slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="monitors" className="mt-4">
            <MonitorsTab
              slug={slug}
              canWrite={canWrite}
              canManageComponents={canManageComponents}
            />
          </TabsContent>
          <TabsContent value="alerts" className="mt-4">
            <AlertsTab
              slug={slug}
              focusIncident={incident}
              onOpenIncident={(uuid) => go("alerts", { incident: uuid })}
            />
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}
