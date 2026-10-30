"use client";

import { Suspense } from "react";

import { AuthShell } from "@/features/auth/components/auth-shell";
import { OrganizationSSOSignIn } from "@/features/auth/components/organization-sso-sign-in";
import { useLocale } from "@/providers/locale-provider";

/**
 * Organization SSO entry (CF-128). No guest guard: an organization that
 * enforces SSO is entered by a signed-in user who still has to verify with
 * the organization's IdP.
 */
export default function OrganizationSSOPage() {
  const { t } = useLocale();
  return (
    <AuthShell>
      <Suspense
        fallback={
          <div className="text-muted-foreground text-center text-sm">
            {t("common.loading")}
          </div>
        }
      >
        <OrganizationSSOSignIn />
      </Suspense>
    </AuthShell>
  );
}
