"use client";

import type { ReactNode } from "react";

import { SoftphoneProvider } from "@/features/softphone/softphone";
import { RouteGuard } from "@/components/common/route-guard";
import { AppLayout } from "@/components/layout";
import { OrganizationPolicyGate, TenantProvider } from "@/features/tenant";

/** Workspace shell — `/dashboard`, scoped to the active organization. */
export default function CmsLayout({ children }: { children: ReactNode }) {
  return (
    <RouteGuard mode="cms">
      <TenantProvider>
        <SoftphoneProvider>
          <AppLayout variant="cms">
            <OrganizationPolicyGate>{children}</OrganizationPolicyGate>
          </AppLayout>
        </SoftphoneProvider>
      </TenantProvider>
    </RouteGuard>
  );
}
