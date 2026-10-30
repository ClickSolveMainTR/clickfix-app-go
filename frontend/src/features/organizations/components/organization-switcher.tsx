"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { Check, ChevronsUpDown, Plus, ShieldCheck } from "lucide-react";
import { useEffect } from "react";

import { AppSidebarLogo } from "@/components/brand";
import { Badge } from "@/components/ui/badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { routes } from "@/config/routes";
import { hasPlatformPermission } from "@/lib/auth/types";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { cn } from "@/lib/utils";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { markWorkspaceEntered } from "../lib/active-organization-store";
import { useActiveOrganization } from "../lib/use-active-organization";

function orgInitials(name: string) {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const a = parts[0]?.[0] ?? "";
  const b = parts.length > 1 ? (parts[1]?.[0] ?? "") : (parts[0]?.[1] ?? "");
  return `${a}${b}`.toLocaleUpperCase("tr-TR") || "?";
}

/** Square organization avatar: the uploaded logo (API URL) or initials. */
export function OrganizationAvatar({
  organization,
  className,
}: {
  organization: { name: string; logo_url?: string | null };
  className?: string;
}) {
  return (
    <span
      className={cn(
        "font-display flex aspect-square size-8 shrink-0 items-center justify-center overflow-hidden rounded-lg bg-white/[0.07] text-[13px] font-semibold tracking-tight text-white ring-1 ring-white/12",
        className,
      )}
    >
      {organization.logo_url ? (
        // eslint-disable-next-line @next/next/no-img-element -- API-owned media URL
        <img
          src={resolveLogoSrc(organization.logo_url)}
          alt=""
          className="size-full object-cover"
        />
      ) : (
        orgInitials(organization.name)
      )}
    </span>
  );
}

/**
 * Sidebar header: the current workspace and a menu to switch between the
 * user's organizations (and, for platform admins, the platform console).
 * In the workspace shell the active organization is current; in the platform
 * console the console is, and picking an organization opens its workspace.
 * Without memberships it shows the ClickFix lockup.
 */
export function OrganizationSwitcher({
  homeHref,
  shell = "cms",
}: {
  homeHref: string;
  shell?: "cms" | "platform";
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { user } = useAuth();
  const { organizations, active, select } = useActiveOrganization();
  const { isMobile } = useSidebar();
  const showConsole = hasPlatformPermission(user);
  const inConsole = shell === "platform";

  // A platform admin already in a workspace keeps it when opening `/`.
  useEffect(() => {
    if (!inConsole && showConsole && active && user)
      markWorkspaceEntered(user.uuid);
  }, [inConsole, showConsole, active, user]);

  if (!active) {
    return (
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton size="lg" asChild>
            <Link href={homeHref}>
              <AppSidebarLogo
                subtitle={inConsole ? t("layout.console_platform") : undefined}
              />
            </Link>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    );
  }

  const enterOrganization = (slug: string) => {
    select(slug);
    if (inConsole) {
      if (user) markWorkspaceEntered(user.uuid);
      router.push(routes.cms.home);
    }
  };

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <SidebarMenuButton
              size="lg"
              className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
              aria-label={t("layout.org_switcher_label")}
            >
              {inConsole ? (
                <AppSidebarLogo subtitle={t("layout.console_platform")} />
              ) : (
                <>
                  <OrganizationAvatar organization={active} />
                  <span className="grid flex-1 text-left leading-tight">
                    <span className="truncate text-sm font-semibold">
                      {active.name}
                    </span>
                    <span className="truncate text-xs opacity-60">
                      {active.support
                        ? t("layout.org_support")
                        : tenantRoleLabel(t, active.role)}
                    </span>
                  </span>
                </>
              )}
              <ChevronsUpDown className="ms-auto size-4 opacity-60" />
            </SidebarMenuButton>
          </DropdownMenuTrigger>
          <DropdownMenuContent
            className="w-(--radix-dropdown-menu-trigger-width) min-w-64 rounded-xl"
            align="start"
            side={isMobile ? "bottom" : "right"}
            sideOffset={8}
          >
            <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
              {t("layout.org_switcher_title")}
            </DropdownMenuLabel>
            {organizations.map((org) => {
              const current = !inConsole && org.slug === active.slug;
              return (
                <DropdownMenuItem
                  key={org.uuid}
                  onSelect={() => enterOrganization(org.slug)}
                  className="gap-2.5 p-2"
                  disabled={org.status === "suspended"}
                >
                  <OrganizationAvatar
                    organization={org}
                    className="bg-ink size-7 text-xs"
                  />
                  <span className="grid min-w-0 flex-1 leading-tight">
                    <span className="truncate text-sm font-medium">
                      {org.name}
                    </span>
                    <span className="text-muted-foreground truncate text-xs">
                      {org.status === "suspended"
                        ? t("layout.org_suspended")
                        : tenantRoleLabel(t, org.role)}
                    </span>
                  </span>
                  {org.support ? (
                    <Badge variant="warning" className="shrink-0">
                      {t("layout.org_support")}
                    </Badge>
                  ) : null}
                  {current ? (
                    <Check className="text-signal-text size-4" aria-hidden />
                  ) : null}
                </DropdownMenuItem>
              );
            })}
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild className="gap-2.5 p-2">
              <Link href={`${routes.cms.onboarding}?yeni=1`}>
                <span className="flex size-7 items-center justify-center rounded-lg border border-dashed">
                  <Plus className="size-4" aria-hidden />
                </span>
                {t("layout.org_create")}
              </Link>
            </DropdownMenuItem>
            {showConsole ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild className="gap-2.5 p-2">
                  <Link href={routes.platform.home}>
                    <span className="bg-muted flex size-7 items-center justify-center rounded-lg">
                      <ShieldCheck className="size-4" aria-hidden />
                    </span>
                    <span className="flex-1">
                      {inConsole
                        ? t("layout.console_platform")
                        : t("layout.org_platform_console")}
                    </span>
                    {inConsole ? (
                      <Check className="text-signal-text size-4" aria-hidden />
                    ) : null}
                  </Link>
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
