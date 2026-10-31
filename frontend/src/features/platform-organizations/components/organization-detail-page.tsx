"use client";

import {
  AlertTriangle,
  Database,
  Bot,
  Building2,
  Pencil,
  Trash2,
  Users,
  Video,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityHeader,
  EntityPage,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { platformAIRunsService, RunLog } from "@/features/ai-settings";
import {
  OrganizationSubscriptionTab,
  OrganizationUsageTab,
} from "@/features/billing/components/platform/organization-billing-tab";
import { OrganizationWalletTab } from "@/features/billing/components/platform/organization-wallet-tab";
import { formatCount } from "@/features/billing/lib/money";
import { OrganizationAuditTab } from "@/features/platform-organizations/components/organization-audit-tab";
import {
  OrganizationAccessChip,
  OrganizationPlanCell,
  OrganizationStatusChip,
} from "@/features/platform-organizations/components/organization-cells";
import { OrganizationGeneralTab } from "@/features/platform-organizations/components/organization-general-tab";
import { OrganizationInvoicesTab } from "@/features/platform-organizations/components/organization-invoices-tab";
import { OrganizationMembersTab } from "@/features/platform-organizations/components/organization-members-tab";
import {
  useDeleteOrganization,
  useOrganization,
} from "@/features/platform-organizations/hooks/use-organizations";
import {
  clearCreationNotice,
  readCreationNotice,
  type CreationNotice,
} from "@/features/platform-organizations/lib/creation-notice";
import { PROFILE_FIELD_KEYS } from "@/features/platform-organizations/schemas/organization-wizard";
import { OrganizationAvatar } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TABS = [
  "general",
  "subscription",
  "wallet",
  "usage",
  "invoices",
  "members",
  "audit",
  "ai",
] as const;
type Tab = (typeof TABS)[number];

const TAB_LABEL_KEYS: Record<Tab, string> = {
  general: "commerce.org.tabs.general",
  subscription: "commerce.org.tabs.subscription",
  wallet: "commerce.org.tabs.wallet",
  usage: "commerce.org.tabs.usage",
  invoices: "commerce.org.tabs.invoices",
  members: "commerce.org.tabs.members",
  audit: "commerce.org.tabs.audit",
  ai: "commerce.org.tabs.ai",
};

/** `?tab=` values of the previous layout, still linked from elsewhere. */
const LEGACY_TABS: Record<string, Tab> = {
  billing: "subscription",
  activity: "audit",
  status: "general",
};

function resolveTab(value: string | undefined): Tab {
  const v = value ?? "";
  if (v in LEGACY_TABS) return LEGACY_TABS[v];
  return (TABS as readonly string[]).includes(v) ? (v as Tab) : "general";
}

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("organizations.forbidden")}
    />
  );
}

function QuickFact({
  icon,
  label,
  children,
}: {
  icon: ReactNode;
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-w-0 items-start gap-3">
      <span className="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg">
        {icon}
      </span>
      <div className="min-w-0 space-y-0.5">
        <p className="text-muted-foreground text-xs">{label}</p>
        <div className="text-sm">{children}</div>
      </div>
    </div>
  );
}

/** Warning left by the create wizard (profile fields or opening credits). */
function CreationBanner({
  notice,
  onDismiss,
}: {
  notice: CreationNotice;
  onDismiss: () => void;
}) {
  const { t } = useLocale();
  const fields = notice.profileFields.map((field) => {
    const key = PROFILE_FIELD_KEYS[field];
    return key ? t(key) : field;
  });
  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm"
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-700 dark:text-amber-300" />
      <div className="min-w-0 flex-1 space-y-1">
        <p className="font-medium">{t("commerce.wizard.notice.title")}</p>
        {fields.length ? (
          <p>
            {t("commerce.wizard.notice.profile", {
              fields: fields.join(", "),
            })}
          </p>
        ) : null}
        {notice.failedCredits.length ? (
          <p>{t("commerce.wizard.notice.credits")}</p>
        ) : null}
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7"
        aria-label={t("commerce.org.common.dismiss")}
        onClick={onDismiss}
      >
        <X className="size-4" />
      </Button>
    </div>
  );
}

/** Demo logins left by the create wizard (shown once). */
function DemoBanner({
  demo,
  onDismiss,
}: {
  demo: NonNullable<CreationNotice["demo"]>;
  onDismiss: () => void;
}) {
  const { t } = useLocale();
  return (
    <div
      role="status"
      className="bg-primary/5 border-primary/20 flex items-start gap-3 rounded-xl border px-4 py-3 text-sm"
    >
      <Database className="text-primary mt-0.5 size-4 shrink-0" />
      <div className="min-w-0 flex-1 space-y-2">
        <p className="font-medium">{t("commerce.wizard.demo.notice_title")}</p>
        <p>{t("commerce.wizard.demo.notice_body")}</p>
        <p>
          {t("commerce.wizard.demo.password")}:{" "}
          <code className="bg-muted rounded px-1.5 py-0.5 text-xs select-all">
            {demo.password}
          </code>
        </p>
        <ul className="grid gap-1 sm:grid-cols-2">
          {demo.accounts.map((account) => (
            <li key={account.email} className="min-w-0 truncate">
              <span className="font-mono text-xs select-all">
                {account.email}
              </span>{" "}
              <span className="text-muted-foreground text-xs">
                · {account.name} · {account.role}
              </span>
            </li>
          ))}
        </ul>
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7"
        aria-label={t("commerce.org.common.dismiss")}
        onClick={onDismiss}
      >
        <X className="size-4" />
      </Button>
    </div>
  );
}

/**
 * Platform organization page: header with status, plan and balances; tabs
 * Genel / Abonelik / Cüzdan ve krediler / Kullanım / Faturalar / Üyeler /
 * Denetim / Yapay zeka (`?tab=`, old values are mapped).
 */
export function OrganizationDetailPage({
  uuid,
  initialTab,
}: {
  uuid: string;
  initialTab?: string;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const query = useOrganization(uuid);
  const remove = useDeleteOrganization();
  const [tab, setTab] = useState<Tab>(() => resolveTab(initialTab));
  const [confirmDelete, setConfirmDelete] = useState(false);
  // Read once on the client; the effect below only clears storage.
  const [notice, setNotice] = useState<CreationNotice | null>(() =>
    typeof window === "undefined" ? null : readCreationNotice(uuid),
  );
  useEffect(() => {
    clearCreationNotice(uuid);
  }, [uuid]);
  const openTab = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };
  const org = query.data;
  const title = org?.name ?? t("organizations.detail_title");
  const canWrite = can(permissions.organizations.write);
  // AI run traces are masked logs (backend: platform.logs.read).
  const canAIRuns = can(permissions.logs.read);
  const canSubscriptions = can(permissions.billing.subscriptionsRead);
  const canUsage = can(permissions.billing.entitlementsRead);
  const visible = (value: Tab) => {
    switch (value) {
      case "subscription":
      case "wallet":
      case "invoices":
        return canSubscriptions;
      case "usage":
        return canUsage;
      case "ai":
        return canAIRuns;
      default:
        return true;
    }
  };

  return (
    <EntityPage
      title={title}
      description={t("commerce.org.detail_description")}
      permission={permissions.organizations.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: title },
      ]}
      actions={
        org && canWrite ? (
          <EntityActions>
            <Button asChild size="sm" variant="outline">
              <Link href={routes.platform.organizations.edit(uuid)}>
                <Pencil className="size-4" />
                {t("organizations.actions.edit")}
              </Link>
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="text-destructive"
              onClick={() => setConfirmDelete(true)}
            >
              <Trash2 className="size-4" />
              {t("organizations.actions.delete")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {org ? (
        <div className="space-y-6">
          {notice?.demo ? (
            <DemoBanner
              demo={notice.demo}
              onDismiss={() => setNotice({ ...notice, demo: null })}
            />
          ) : null}
          {notice &&
          (notice.profileFields.length || notice.failedCredits.length) ? (
            <CreationBanner
              notice={notice}
              onDismiss={() =>
                setNotice({ ...notice, profileFields: [], failedCredits: [] })
              }
            />
          ) : null}
          <EntityHeader
            leading={
              org.logo_url ? (
                <OrganizationAvatar
                  organization={org}
                  className="bg-muted text-foreground ring-border size-14 rounded-xl"
                />
              ) : (
                <span className="bg-muted flex size-14 shrink-0 items-center justify-center rounded-xl border">
                  <Building2 className="text-muted-foreground size-6" />
                </span>
              )
            }
            title={org.name}
            subtitle={<code className="text-xs">{org.slug}</code>}
            badges={
              <>
                <OrganizationStatusChip status={org.status} />
                <OrganizationAccessChip state={org.access_state} />
              </>
            }
          >
            <div className="grid gap-4 border-t pt-4 sm:grid-cols-2 lg:grid-cols-4">
              <QuickFact
                icon={<Building2 className="size-4" />}
                label={t("commerce.org.header.plan")}
              >
                <OrganizationPlanCell org={org} />
              </QuickFact>
              <QuickFact
                icon={<Users className="size-4" />}
                label={t("commerce.org.header.members")}
              >
                <span className="font-medium tabular-nums">
                  {formatCount(org.member_count, locale)}
                </span>
                {org.pending_invitation_count > 0 ? (
                  <span className="text-muted-foreground block text-xs">
                    {t("commerce.org.header.pending_invites", {
                      count: org.pending_invitation_count,
                    })}
                  </span>
                ) : null}
              </QuickFact>
              <QuickFact
                icon={<Bot className="size-4" />}
                label={t("commerce.org.units.ai_credits")}
              >
                <span className="font-medium tabular-nums">
                  {formatCount(org.ai_credits, locale)}
                </span>
              </QuickFact>
              <QuickFact
                icon={<Video className="size-4" />}
                label={t("commerce.org.units.video_minutes")}
              >
                <span className="font-medium tabular-nums">
                  {formatCount(org.video_minutes, locale)}
                </span>
              </QuickFact>
            </div>
          </EntityHeader>
          <Tabs value={tab} onValueChange={openTab} className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1 pb-1">
              <TabsList className="h-auto w-max">
                {TABS.filter(visible).map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(TAB_LABEL_KEYS[value])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
            <TabsContent value="general">
              <OrganizationGeneralTab org={org} />
            </TabsContent>
            {canSubscriptions ? (
              <>
                <TabsContent value="subscription">
                  <OrganizationSubscriptionTab organizationUuid={uuid} />
                </TabsContent>
                <TabsContent value="wallet">
                  <OrganizationWalletTab organizationUuid={uuid} />
                </TabsContent>
                <TabsContent value="invoices">
                  <OrganizationInvoicesTab />
                </TabsContent>
              </>
            ) : null}
            {canUsage ? (
              <TabsContent value="usage">
                <OrganizationUsageTab organizationUuid={uuid} />
              </TabsContent>
            ) : null}
            <TabsContent value="members">
              <OrganizationMembersTab organizationUuid={uuid} />
            </TabsContent>
            <TabsContent value="audit">
              <OrganizationAuditTab uuid={uuid} />
            </TabsContent>
            {canAIRuns ? (
              <TabsContent value="ai">
                <p className="text-muted-foreground mb-3 text-sm">
                  {t("ai_settings.runs.description")}
                </p>
                <RunLog
                  queryKey={["platform", "organizations", uuid, "ai", "runs"]}
                  fetchPage={(offset, limit) =>
                    platformAIRunsService.runs(uuid, offset, limit)
                  }
                  fetchRun={(run) => platformAIRunsService.run(uuid, run)}
                />
              </TabsContent>
            ) : null}
          </Tabs>
        </div>
      ) : null}

      <EntityDeleteDialog
        open={confirmDelete}
        entityLabel={org?.name ?? ""}
        isPending={remove.isPending}
        onCancel={() => setConfirmDelete(false)}
        onConfirm={() =>
          remove.mutate(uuid, {
            onSuccess: () => router.push(routes.platform.organizations.root),
            onSettled: () => setConfirmDelete(false),
          })
        }
      />
    </EntityPage>
  );
}
