"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { CircleX, KeyRound, LogIn } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { safeNextPath } from "@/lib/auth/next-path";
import { startOrganizationSSO } from "@/lib/auth/organization-sso";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { AuthCard } from "./auth-card";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** `?sso=` outcomes the BFF callback sends back here (static keys). */
const OUTCOME_KEYS: Record<string, string> = {
  failed: "auth.sso.outcome_failed",
  mismatch: "auth.sso.outcome_mismatch",
  session: "auth.sso.outcome_session",
};

/**
 * Organization SSO sign-in (CF-128). The IdP verifies the panel user for one
 * organization; the global sign-in stays the user's own (the API's SSO token
 * is tenant-only), so a signed-out visitor signs in first and comes back.
 */
export function OrganizationSSOSignIn() {
  const { t } = useLocale();
  const params = useSearchParams();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);

  const organization = params.get("organization") ?? "";
  const next = safeNextPath(params.get("next"), routes.cms.home);
  const outcome = OUTCOME_KEYS[params.get("sso") ?? ""];
  const membership = user?.organizations.find(
    (o) => o.uuid.toLowerCase() === organization.toLowerCase(),
  );
  const self = `${routes.guest.loginSSO}?${new URLSearchParams({
    organization,
    next,
  }).toString()}`;

  if (!UUID.test(organization)) {
    return (
      <AuthCard
        title={t("auth.sso.title")}
        description={t("auth.sso.invalid_link")}
      >
        <Button asChild className="w-full">
          <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
        </Button>
      </AuthCard>
    );
  }

  const start = async () => {
    setPending(true);
    setFailed(false);
    try {
      window.location.assign(await startOrganizationSSO(organization, next));
    } catch {
      setFailed(true);
      setPending(false);
    }
  };

  return (
    <AuthCard
      title={
        membership
          ? t("auth.sso.title_org", { name: membership.name })
          : t("auth.sso.title")
      }
      description={t(
        isAuthenticated
          ? "auth.sso.description"
          : "auth.sso.description_signed_out",
      )}
    >
      <div className="space-y-4">
        {outcome || failed ? (
          <Alert variant="destructive">
            <CircleX className="size-4" />
            <AlertDescription>
              {t(failed ? "auth.sso.start_failed" : outcome!)}
            </AlertDescription>
          </Alert>
        ) : null}
        {!bootstrapped ? (
          <p className="text-muted-foreground text-center text-sm">
            {t("common.loading")}
          </p>
        ) : isAuthenticated ? (
          <>
            {user?.email ? (
              <p className="text-muted-foreground text-center text-sm">
                {t("auth.sso.signed_in_as", { email: user.email })}
              </p>
            ) : null}
            <Button
              type="button"
              className="w-full"
              disabled={pending}
              onClick={() => void start()}
            >
              <KeyRound className="size-4" />
              {pending ? t("auth.sso.redirecting") : t("auth.sso.continue")}
            </Button>
            <Button asChild variant="ghost" className="w-full">
              <Link href={routes.cms.home}>{t("auth.sso.cancel")}</Link>
            </Button>
          </>
        ) : (
          <Button asChild className="w-full">
            <Link
              href={`${routes.guest.login}?${new URLSearchParams({ next: self }).toString()}`}
            >
              <LogIn className="size-4" />
              {t("auth.sso.sign_in_first")}
            </Link>
          </Button>
        )}
      </div>
    </AuthCard>
  );
}
