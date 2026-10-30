"use client";

import { Trophy } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { CalibrationsTab } from "./calibrations-tab";
import { CoachingTab } from "./coaching-tab";
import { LeaderboardTab } from "./leaderboard-tab";
import { RulesTab } from "./rules-tab";

const TABS = ["leaderboard", "rules", "calibrations", "coaching"] as const;
type Tab = (typeof TABS)[number];

/**
 * Agent development (CF-109). Leaderboard, badge rules and the own opt-out
 * need tenant.schedules.read; calibration, coaching and editing rules need
 * tenant.quality.manage.
 */
export function CoachingPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const canGame = permissions.includes(Permission.TenantSchedulesRead);
  const canQuality = permissions.includes(Permission.TenantQualityManage);
  const tabs = TABS.filter((k) =>
    k === "leaderboard" || k === "rules" ? canGame : canQuality,
  );
  const raw = params.get("tab") as Tab;
  const tab: Tab | undefined = tabs.includes(raw) ? raw : tabs[0];

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Trophy className="size-7" />}
        title={t("coaching.title")}
        description={t("coaching.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("coaching.no_organization")}
        </p>
      ) : !tab ? (
        <p className="text-muted-foreground text-sm">
          {t("coaching.forbidden")}
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
                {t(`coaching.tabs.${k}`)}
              </TabsTrigger>
            ))}
          </TabsList>
          {canGame && (
            <>
              <TabsContent value="leaderboard" className="mt-4">
                <LeaderboardTab slug={slug} />
              </TabsContent>
              <TabsContent value="rules" className="mt-4">
                <RulesTab slug={slug} canWrite={canQuality} />
              </TabsContent>
            </>
          )}
          {canQuality && (
            <>
              <TabsContent value="calibrations" className="mt-4">
                <CalibrationsTab slug={slug} />
              </TabsContent>
              <TabsContent value="coaching" className="mt-4">
                <CoachingTab slug={slug} />
              </TabsContent>
            </>
          )}
        </Tabs>
      )}
    </div>
  );
}
