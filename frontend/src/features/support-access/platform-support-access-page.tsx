"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { LogIn } from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
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

  const enter = useCallback(
    async (grant: PlatformSupportGrant) => {
      setEntering(grant.uuid);
      try {
        await hydrateProfile();
        setActive(grant.organization.slug);
        router.push(routes.cms.home);
      } finally {
        setEntering(null);
      }
    },
    [hydrateProfile, router, setActive],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<PlatformSupportGrant>({
          id: "organization",
          // Search matches the organization name, its slug and the reason.
          accessorFn: (g) =>
            `${g.organization.name} ${g.organization.slug} ${g.reason ?? ""}`,
          sortingFn: (a, b) =>
            a.original.organization.name.localeCompare(
              b.original.organization.name,
              locale,
            ),
          labelKey: "support_access.platform.columns.organization",
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => {
            const grant = row.original;
            return (
              <div className="min-w-0 space-y-1">
                <p className="flex flex-wrap items-center gap-2">
                  <span className="truncate font-medium">
                    {grant.organization.name}
                  </span>
                  <code className="text-muted-foreground text-xs">
                    {grant.organization.slug}
                  </code>
                </p>
                {grant.reason ? (
                  <p className="text-muted-foreground text-sm whitespace-normal">
                    “{grant.reason}”
                  </p>
                ) : null}
              </div>
            );
          },
        }),
        createColumn<PlatformSupportGrant>({
          accessorKey: "mode",
          labelKey: "support_access.platform.columns.mode",
          searchable: false,
          cell: ({ row }) => (
            <Badge variant="warning">
              {t(MODE_LABEL_KEYS[row.original.mode])}
            </Badge>
          ),
        }),
        createColumn<PlatformSupportGrant>({
          accessorKey: "expires_at",
          labelKey: "support_access.platform.columns.expires",
          searchable: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground text-sm whitespace-normal">
              {t("support_access.platform.until", {
                date: datetime(row.original.expires_at, undefined, locale),
                remaining: remaining(t, row.original.expires_at, now),
              })}
            </span>
          ),
        }),
        createColumn<PlatformSupportGrant>({
          id: "actions",
          labelKey: "common.actions",
          enableHiding: false,
          enableSorting: false,
          meta: { headerClassName: "text-end", cellClassName: "text-end" },
          cell: ({ row }) => (
            <Button
              type="button"
              size="sm"
              disabled={
                entering !== null ||
                row.original.organization.status === "suspended"
              }
              onClick={() => void enter(row.original)}
            >
              <LogIn className="size-4" />
              {t("support_access.platform.enter")}
            </Button>
          ),
        }),
      ] as ColumnDef<PlatformSupportGrant, unknown>[],
    [enter, entering, locale, now, t],
  );

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
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(g) => g.uuid}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          emptyTitle={t("support_access.platform.empty_title")}
          emptyDescription={t("support_access.platform.empty_description")}
          // A short, unpaged list: search, sort and pages in the browser.
          // Soonest to expire first.
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{
            pagination: { pageIndex: 0, pageSize: 20 },
            sorting: [{ id: "expires_at", desc: false }],
          }}
          features={{
            columnFilters: false,
            rowSelection: false,
            viewMode: false,
          }}
        />
      </EntitySectionCard>
    </EntityPage>
  );
}
