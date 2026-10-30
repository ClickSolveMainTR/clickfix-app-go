"use client";

import { Suspense } from "react";

import { routes } from "@/config/routes";
import { SignupPlanCapture } from "@/features/landing/components/signup-plan-capture";
import type { SignupMode } from "@/features/landing/lib/signup";
import { useLocale } from "@/providers/locale-provider";

import { AuthShell } from "./auth-shell";
import { RegisterForm } from "./register-form";
import { SignupClosed } from "./signup-closed";
import { SignupPitch } from "./signup-pitch";

/**
 * `/register` (CF-143). Sign-up mode `open`: the self-serve sign-up flow with
 * the plan from `?plan=`; `invite` / `closed`: a demo request and sign-in.
 */
export function SignupPage({ mode }: { mode: SignupMode }) {
  const { t } = useLocale();
  const open = mode === "open";
  return (
    <AuthShell
      homeHref={routes.public.landing}
      pitch={<SignupPitch open={open} />}
      wide={open}
    >
      {open ? (
        <Suspense
          fallback={
            <div className="text-muted-foreground text-center text-sm">
              {t("common.loading")}
            </div>
          }
        >
          <SignupPlanCapture />
          <RegisterForm />
        </Suspense>
      ) : (
        <SignupClosed />
      )}
    </AuthShell>
  );
}
