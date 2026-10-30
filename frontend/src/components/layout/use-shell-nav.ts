"use client";

import { useMemo } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { cmsNav, platformNav, settingsNav } from "@/config/nav";
import { routes } from "@/config/routes";
import type { NavCatalog } from "@/features/nav-engine";

export type ShellNav = {
  /** Sidebar catalog of the shell. */
  catalog: NavCatalog;
  /** Catalogs the command palette indexes as pages. */
  paletteCatalogs: NavCatalog[];
  homeHref: string;
};

/** Sidebar + palette catalogs of the current shell. */
export function useShellNav(variant: AppLayoutVariant): ShellNav {
  return useMemo<ShellNav>(() => {
    if (variant === "cms") {
      return {
        catalog: cmsNav,
        paletteCatalogs: [cmsNav],
        homeHref: routes.cms.home,
      };
    }
    // Platform palette also indexes the System Settings sub-pages.
    return {
      catalog: platformNav,
      paletteCatalogs: [platformNav, settingsNav],
      homeHref: routes.platform.home,
    };
  }, [variant]);
}
