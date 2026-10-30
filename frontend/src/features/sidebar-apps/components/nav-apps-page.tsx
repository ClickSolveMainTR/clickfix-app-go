"use client";

import { Blocks } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { usePlacedApps } from "../hooks/use-sidebar-apps";
import { appIcon } from "../lib/protocol";
import { AppFrame } from "./app-frame";

/**
 * Apps registered with the `nav` placement. They launch without
 * conversation or contact context, so they can show data but cannot act on
 * a conversation.
 */
export function NavAppsPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const apps = usePlacedApps("nav");
  const items = apps.data;
  const current = items.find((a) => a.uuid === params.get("app")) ?? items[0];

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Blocks className="size-7" />}
        title={t("apps.nav.title")}
        description={t("apps.nav.description")}
      />
      {apps.isLoading && <Loading label={t("common.loading")} />}
      {apps.isError && (
        <ErrorState
          title={t("apps.error.title")}
          description={t("apps.error.description")}
          onRetry={() => void apps.refetch()}
        />
      )}
      {apps.isSuccess && items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-10 text-center text-sm">
            {t("apps.nav.empty")}
          </CardContent>
        </Card>
      )}
      {current && (
        <div className="grid gap-4 lg:grid-cols-[14rem_minmax(0,1fr)]">
          <nav
            className="flex gap-1 overflow-x-auto lg:flex-col"
            aria-label={t("apps.nav.title")}
          >
            {items.map((app) => {
              const Icon = appIcon(app.icon);
              return (
                <button
                  key={app.uuid}
                  type="button"
                  aria-current={app.uuid === current.uuid ? "page" : undefined}
                  onClick={() =>
                    router.replace(routes.cms.apps.detail(app.uuid))
                  }
                  className={cn(
                    "hover:bg-muted flex shrink-0 items-center gap-2 rounded-md px-3 py-2 text-left text-sm",
                    app.uuid === current.uuid && "bg-muted font-medium",
                  )}
                >
                  <Icon className="text-muted-foreground size-4" />
                  <span className="truncate">{app.name}</span>
                </button>
              );
            })}
          </nav>
          <AppFrame
            key={current.uuid}
            app={current}
            context={{}}
            height="calc(100svh - 14rem)"
          />
        </div>
      )}
    </div>
  );
}
