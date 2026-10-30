"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Ban,
  CalendarX2,
  KeyRound,
  Network,
  ShieldAlert,
  TriangleAlert,
} from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** API error codes the guard turns into dedicated cards. */
export const TENANT_ACCESS_CODES = {
  notMember: "NO_TENANT_MEMBERSHIP",
  suspended: "ORGANIZATION_SUSPENDED",
  expired: "ORGANIZATION_ACCESS_EXPIRED",
  ipBlocked: "ORGANIZATION_IP_NOT_ALLOWED",
  ssoRequired: "ORGANIZATION_SSO_REQUIRED",
} as const;

type AccessIssue =
  | "not_member"
  | "suspended"
  | "expired"
  | "ip_blocked"
  | "sso_required"
  | "unavailable";

function issueFor(code: string | undefined): AccessIssue {
  switch (code) {
    case TENANT_ACCESS_CODES.notMember:
      return "not_member";
    case TENANT_ACCESS_CODES.suspended:
      return "suspended";
    case TENANT_ACCESS_CODES.expired:
      return "expired";
    case TENANT_ACCESS_CODES.ipBlocked:
      return "ip_blocked";
    case TENANT_ACCESS_CODES.ssoRequired:
      return "sso_required";
    default:
      return "unavailable";
  }
}

const ISSUE_VIEW = {
  not_member: {
    icon: ShieldAlert,
    title: "tenant.access.not_member_title",
    body: "tenant.access.not_member_body",
  },
  suspended: {
    icon: Ban,
    title: "tenant.access.suspended_title",
    body: "tenant.access.suspended_body",
  },
  expired: {
    icon: CalendarX2,
    title: "tenant.access.expired_title",
    body: "tenant.access.expired_body",
  },
  ip_blocked: {
    icon: Network,
    title: "tenant.access.ip_blocked_title",
    body: "tenant.access.ip_blocked_body",
  },
  sso_required: {
    icon: KeyRound,
    title: "tenant.access.sso_required_title",
    body: "tenant.access.sso_required_body",
  },
  unavailable: {
    icon: TriangleAlert,
    title: "tenant.access.unavailable_title",
    body: "tenant.access.unavailable_body",
  },
} as const;

/** Organization SSO entry (CF-128) that returns to `next` afterwards. */
export function organizationSSOHref(organization: string, next: string) {
  const query = new URLSearchParams({ organization, next });
  return `${routes.guest.loginSSO}?${query.toString()}`;
}

function TenantAccessCard({
  issue,
  slug,
  organization,
  onRetry,
}: {
  issue: AccessIssue;
  slug: string;
  /** Organization uuid, for the SSO sign-in. */
  organization?: string;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  const { user } = useAuth();
  const pathname = usePathname();
  const view = ISSUE_VIEW[issue];
  const Icon = view.icon;

  return (
    <div className="flex items-center justify-center py-12">
      <Card className="w-full max-w-md" data-tenant-access={issue}>
        <CardHeader>
          <div className="bg-muted text-muted-foreground mb-2 flex size-10 items-center justify-center rounded-full">
            <Icon className="size-5" />
          </div>
          <CardTitle>{t(view.title)}</CardTitle>
          <CardDescription>{t(view.body, { slug })}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {user?.email ? (
            <p className="text-muted-foreground text-sm">
              {t("tenant.access.signed_in_as", { email: user.email })}
            </p>
          ) : null}
          {issue === "sso_required" && organization ? (
            <Button asChild>
              <Link href={organizationSSOHref(organization, pathname)}>
                <KeyRound className="size-4" />
                {t("tenant.access.sso_sign_in")}
              </Link>
            </Button>
          ) : null}
          {issue === "unavailable" || issue === "ip_blocked" ? (
            <Button variant="outline" onClick={onRetry}>
              {t("tenant.access.retry")}
            </Button>
          ) : null}
          <Button
            asChild
            variant={issue === "sso_required" ? "outline" : "default"}
          >
            <Link href={routes.cms.home}>{t("tenant.access.go_home")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * Gate of the organization settings pages: an active organization (sidebar
 * switcher) whose membership, status and access window the API confirms.
 * Renders right away from the cached `/me` membership when it is usable;
 * the API answer replaces it. Sign-in is the shell's `RouteGuard`.
 */
export function ActiveOrganizationGate({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  const tenant = useTenant();

  if (!tenant.slug) {
    return (
      <EmptyState
        title={t("tenant.no_organization.title")}
        description={t("tenant.no_organization.body")}
      />
    );
  }

  if (tenant.failed) {
    return (
      <TenantAccessCard
        issue={issueFor(tenant.error?.code)}
        slug={tenant.organization?.name ?? tenant.slug}
        organization={tenant.organization?.uuid}
        onRetry={tenant.refetch}
      />
    );
  }

  if (!tenant.verified && !tenant.optimistic) {
    return <Loading label={t("common.loading")} />;
  }

  return <>{children}</>;
}

/**
 * Workspace-wide gate for the organization access policy (CF-128): when the
 * active organization enforces SSO or its IP allow-list excludes the caller,
 * every tenant page would fail, so the shell shows the card instead (the
 * sidebar stays usable to switch organizations). Other errors are left to
 * the pages.
 */
export function OrganizationPolicyGate({ children }: { children: ReactNode }) {
  const tenant = useTenant();
  const pathname = usePathname();
  const issue = tenant.failed ? issueFor(tenant.error?.code) : null;
  // The personal profile is not organization data.
  const personal = pathname.startsWith(routes.cms.profile.root);
  if (personal || (issue !== "sso_required" && issue !== "ip_blocked")) {
    return <>{children}</>;
  }
  return (
    <TenantAccessCard
      issue={issue}
      slug={tenant.organization?.name ?? tenant.slug}
      organization={tenant.organization?.uuid}
      onRetry={tenant.refetch}
    />
  );
}
