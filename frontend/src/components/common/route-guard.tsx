"use client";

import { useEffect, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { hasEnteredWorkspace } from "@/features/organizations";
import {
  type AuthUser,
  defaultHomeForUser,
  isCmsUser,
  isPlatformUser,
} from "@/lib/auth/types";
import { nextFromLocation } from "@/lib/auth/next-path";
import { useAuthStore } from "@/lib/auth/session-store";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

type RouteGuardProps = {
  children: ReactNode;
  /**
   * `guest`: signed-out pages; `platform`: platform shell (needs a platform
   * permission); `cms`: workspace shell (no platform permission, or a
   * platform admin with an organization membership).
   */
  mode: "guest" | "platform" | "cms";
};

function PageLoader({ label }: { label: string }) {
  return (
    <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
      {label}
    </div>
  );
}

function RoleMismatch({ expected, home }: { expected: string; home: string }) {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t("errors.forbidden_title")}</CardTitle>
          <CardDescription>{t("errors.role_mismatch")}</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <p className="text-muted-foreground text-sm">
            {t("errors.area_requires", { area: expected })}
          </p>
          <Button asChild>
            <Link href={home}>{t("errors.continue")}</Link>
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

function dispatchesFromCmsRoot(pathname: string, user: AuthUser) {
  if (pathname !== routes.cms.root) return false;
  if (defaultHomeForUser(user) === routes.cms.root) return false;
  // A platform admin who switched into one of their organizations stays.
  return !(isCmsUser(user) && hasEnteredWorkspace(user.uuid));
}

export function RouteGuard({ children, mode }: RouteGuardProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { bootstrapped, isAuthenticated, user } = useAuth();
  const { t } = useLocale();
  const cachedUser = useAuthStore((s) => s.user);

  useEffect(() => {
    if (!bootstrapped) return;

    // A signed-in visitor on a guest page (the login form right after a
    // successful sign-in) continues to `?next=` when it is a safe path.
    if (mode === "guest" && isAuthenticated && user) {
      router.replace(nextFromLocation(defaultHomeForUser(user)));
      return;
    }

    // Keep where the visitor was going so login can send them back.
    if (mode !== "guest" && !isAuthenticated) {
      const here = `${window.location.pathname}${window.location.search}`;
      router.replace(
        here && here !== "/"
          ? `${routes.guest.login}?next=${encodeURIComponent(here)}`
          : routes.guest.login,
      );
      return;
    }

    // `/dashboard` is the post-login landing (OAuth / passkey callbacks): send
    // platform users and organization members to their own home.
    if (mode === "cms" && user && dispatchesFromCmsRoot(pathname, user)) {
      router.replace(defaultHomeForUser(user));
    }
  }, [bootstrapped, mode, isAuthenticated, user, router, pathname]);

  // Show the loader only when there is no cached user to render optimistically.
  // With a cached user, render immediately and let session verification update
  // state in the background.
  if (!bootstrapped && !cachedUser) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (mode !== "guest" && !isAuthenticated) {
    return null;
  }

  if (mode === "cms" && user && dispatchesFromCmsRoot(pathname, user)) {
    return <PageLoader label={t("common.loading")} />;
  }

  if (mode === "platform" && user && !isPlatformUser(user)) {
    return (
      <RoleMismatch
        expected={t("layout.nav_platform")}
        home={defaultHomeForUser(user)}
      />
    );
  }

  if (mode === "cms" && user && !isCmsUser(user)) {
    return (
      <RoleMismatch
        expected={t("layout.section_cms")}
        home={
          isPlatformUser(user) ? routes.platform.home : routes.errors.forbidden
        }
      />
    );
  }

  return <>{children}</>;
}
