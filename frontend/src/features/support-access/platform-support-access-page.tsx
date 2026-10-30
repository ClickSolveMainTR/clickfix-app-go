"use client";

import { LogIn } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganizationStore } from "@/features/organizations";
import {
  MODE_LABEL_KEYS,
  remaining,
  usePlatformSupportGrants,
  type PlatformSupportGrant,
} from "@/features/support-access/support-access";
import { useNow } from "@/features/support-access/use-now";
import { datetime } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/platform/support-access`: organizations that granted ClickFix support
 * access. "Organizasyona gir" refreshes `/me` (the grant then appears as a
 * `support` membership), makes it the active organization and opens `/`.
 */
export function PlatformSupportAccessPage() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const now = useNow();
  const { hydrateProfile } = useAuth();
  const setActive = useActiveOrganizationStore((s) => s.setSlug);
  const query = usePlatformSupportGrants();
  const [entering, setEntering] = useState<string | null>(null);
  const items = query.data?.items ?? [];

  const enter = async (grant: PlatformSupportGrant) => {
    setEntering(grant.uuid);
    try {
      await hydrateProfile();
      setActive(grant.organization.slug);
      router.push(routes.cms.home);
    } finally {
      setEntering(null);
    }
  };

  return (
    <EntityPage
      title={t("support_access.platform.title")}
      description={t("support_access.platform.description")}
      permission={permissions.support.access}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("support_access.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("support_access.platform.title") },
      ]}
    >
      <EntitySectionCard
        title={t("support_access.platform.list_title")}
        badge={query.data ? items.length : undefined}
      >
        {query.isLoading ? <Loading label={t("common.loading")} /> : null}
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {query.data && items.length === 0 ? (
          <EmptyState
            title={t("support_access.platform.empty_title")}
            description={t("support_access.platform.empty_description")}
          />
        ) : null}
        {items.length ? (
          <ul className="divide-y rounded-lg border">
            {items.map((grant) => (
              <li
                key={grant.uuid}
                className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center"
              >
                <div className="min-w-0 flex-1 space-y-1">
                  <p className="flex flex-wrap items-center gap-2">
                    <span className="truncate font-medium">
                      {grant.organization.name}
                    </span>
                    <code className="text-muted-foreground text-xs">
                      {grant.organization.slug}
                    </code>
                    <Badge variant="warning">
                      {t(MODE_LABEL_KEYS[grant.mode])}
                    </Badge>
                  </p>
                  <p className="text-muted-foreground text-sm">
                    {t("support_access.platform.until", {
                      date: datetime(grant.expires_at, undefined, locale),
                      remaining: remaining(t, grant.expires_at, now),
                    })}
                  </p>
                  {grant.reason ? (
                    <p className="text-muted-foreground text-sm">
                      “{grant.reason}”
                    </p>
                  ) : null}
                </div>
                <Button
                  type="button"
                  size="sm"
                  disabled={
                    entering !== null ||
                    grant.organization.status === "suspended"
                  }
                  onClick={() => void enter(grant)}
                >
                  <LogIn className="size-4" />
                  {t("support_access.platform.enter")}
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </EntitySectionCard>
    </EntityPage>
  );
}
