"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";
import { TenantProvider } from "@/features/tenant";

/**
 * Full-screen workspace pages without the sidebar shell (CF-140 smart
 * set-up). Same guard and organization scope as the `(cms)` shell.
 */
export default function SetupLayout({ children }: { children: ReactNode }) {
  return (
    <RouteGuard mode="cms">
      <TenantProvider>{children}</TenantProvider>
    </RouteGuard>
  );
}
