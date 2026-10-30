"use client";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { SidebarUserCard } from "@/components/layout/sidebar-user-card";
import {
  Sidebar,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from "@/components/ui/sidebar";
import { cmsNav, platformNav } from "@/config/nav";
import { routes } from "@/config/routes";
import { NavEngine } from "@/features/nav-engine";
import { OrganizationSwitcher } from "@/features/organizations";

/**
 * Floating ink sidebar (docs/brand.md) in both colour modes. Both shells open
 * with the organization switcher: the workspace shell (`cms`) shows the active
 * organization, the platform console the ClickFix lockup (with the switcher
 * when the admin also belongs to an organization). The footer shows the
 * signed-in user's presence.
 */
export function AppSidebar({
  variant = "platform",
}: {
  variant?: AppLayoutVariant;
}) {
  const catalog = variant === "cms" ? cmsNav : platformNav;
  const homeHref =
    variant === "platform" ? routes.platform.home : routes.cms.home;

  return (
    <Sidebar
      data-guide="sidebar"
      collapsible="icon"
      variant="floating"
      className="sidebar-ink dark text-sidebar-foreground"
    >
      <SidebarHeader>
        <OrganizationSwitcher
          homeHref={homeHref}
          shell={variant === "cms" ? "cms" : "platform"}
        />
      </SidebarHeader>

      <NavEngine catalog={catalog} homeHref={homeHref} />

      <SidebarFooter className="border-sidebar-border border-t">
        <SidebarUserCard agentStatus={variant === "cms"} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}
