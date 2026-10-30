"use client";

import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { useOrganizationActivity } from "@/features/platform-organizations/hooks/use-organizations";
import type { OrganizationActivityEntry } from "@/features/platform-organizations/types";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function actorName(entry: OrganizationActivityEntry) {
  if (!entry.actor) return null;
  return (
    `${entry.actor.name} ${entry.actor.surname}`.trim() || entry.actor.email
  );
}

/**
 * "Denetim": platform operations on the organization (plan changes, credit
 * adjustments, status changes) and everything recorded inside it.
 */
export function OrganizationAuditTab({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(20);
  const query = useOrganizationActivity(uuid, limit);
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const describe = (entry: OrganizationActivityEntry) => {
    const key = `activity.actions.${entry.action}`;
    const label = t(key);
    return label === key ? entry.action : label;
  };
  return (
    <EntitySectionCard
      title={t("commerce.org.tabs.audit")}
      description={t("commerce.org.audit.description")}
      badge={query.data ? total : undefined}
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
          className="py-8"
          title={t("organizations.activity.empty")}
        />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry),
              description:
                actorName(entry) ?? t("organizations.activity.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setLimit((value) => value + 20)}
            >
              {t("organizations.activity.more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}
