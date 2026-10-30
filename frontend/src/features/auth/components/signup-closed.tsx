"use client";

import { CalendarClock, LogIn, MailOpen } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { CONTACT_PATH, contactEmail } from "@/features/landing/constants";
import { useLocale } from "@/providers/locale-provider";

import { AuthCard } from "./auth-card";

/**
 * `/register` while the platform sign-up mode is `invite` or `closed` (CF-143):
 * new accounts come only from invitations, so the visitor can ask for a
 * demo or sign in with the invited address.
 */
export function SignupClosed() {
  const { t } = useLocale();
  const demoHref = contactEmail
    ? `mailto:${contactEmail}?subject=${encodeURIComponent(t("landing.cta.demo_subject"))}`
    : CONTACT_PATH;

  return (
    <AuthCard
      title={t("signup.closed.title")}
      description={t("signup.closed.body")}
    >
      <div className="space-y-4">
        <Button asChild className="w-full">
          <a href={demoHref}>
            <CalendarClock aria-hidden />
            {t("signup.closed.demo")}
          </a>
        </Button>
        <div className="bg-muted/40 flex gap-3 rounded-lg border p-3 text-sm">
          <MailOpen
            className="text-muted-foreground mt-0.5 size-4 shrink-0"
            aria-hidden
          />
          <div className="space-y-1">
            <p className="font-medium">{t("signup.closed.have_invite")}</p>
            <p className="text-muted-foreground">
              {t("signup.closed.invite_hint")}
            </p>
          </div>
        </div>
        <Button asChild variant="outline" className="w-full">
          <Link href={routes.guest.login}>
            <LogIn aria-hidden />
            {t("signup.closed.login")}
          </Link>
        </Button>
      </div>
    </AuthCard>
  );
}
