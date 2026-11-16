"use client";

import { useMutation } from "@tanstack/react-query";
import { MonitorSmartphone } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

import {
  desktopLoginParams,
  desktopLoginSignInPath,
  desktopLoginView,
  safeDesktopCallback,
} from "../lib/desktop-login";

/** How long a code is valid (API: 60 s); the "open again" link stops then. */
const CODE_TTL_MS = 60_000;

/**
 * Desktop app sign-in in the system browser (CF-234,
 * docs/design/masaustu.md §6). The app opens
 * `/desktop/login?state=...&challenge=...`; a signed-out visitor signs in
 * the normal way (password, Google, Apple, passkey, SSO) and comes back here.
 * After confirmation the page asks the API for a one-time code and sends the
 * browser to `clickfix://auth/callback?code=...&state=...`, then shows a
 * fallback in case the app did not open. No token is handled here; the code
 * is useless without the app's PKCE verifier.
 */
export function DesktopLoginPage() {
  const { t } = useLocale();
  const router = useRouter();
  const search = useSearchParams();
  const params = desktopLoginParams(
    search.get("state"),
    search.get("challenge"),
  );
  const { bootstrapped, isAuthenticated, user, logout } = useAuth();
  const [callback, setCallback] = useState<string | null>(null);
  const [expired, setExpired] = useState(false);
  const [cancelled, setCancelled] = useState(false);
  const busy = useRef(false);

  const create = useMutation({
    mutationFn: async () => {
      if (!params) throw new Error("desktop login: invalid request");
      const out = await authService.createDesktopLoginCode(
        params.state,
        params.challenge,
      );
      const to = safeDesktopCallback(out.redirect_url, params.state);
      if (!to) throw new Error("desktop login: unexpected redirect");
      return to;
    },
    onSuccess: (to) => {
      setCallback(to);
      setExpired(false);
      window.location.assign(to);
    },
    onSettled: () => {
      busy.current = false;
    },
  });

  const view = desktopLoginView({
    params,
    bootstrapped,
    authenticated: isAuthenticated,
    sent: callback !== null,
    errorStatus: create.isError
      ? isApiError(create.error)
        ? create.error.status
        : 0
      : null,
  });

  // Signed out: sign in first and come back to this same request.
  useEffect(() => {
    if (view === "sign_in" && params)
      router.replace(desktopLoginSignInPath(routes.guest.login, params));
  }, [params, router, view]);

  // The code dies after 60 s; offer a new one instead of a dead link.
  useEffect(() => {
    if (!callback) return;
    const timer = window.setTimeout(() => setExpired(true), CODE_TTL_MS);
    return () => window.clearTimeout(timer);
  }, [callback]);

  const start = () => {
    if (busy.current) return;
    busy.current = true;
    create.mutate();
  };

  const switchAccount = async () => {
    if (!params) return;
    await logout();
    router.replace(desktopLoginSignInPath(routes.guest.login, params));
  };

  return (
    <main className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MonitorSmartphone className="size-5" aria-hidden />
            {view === "sent"
              ? t("auth.desktop_login.sent_title")
              : t("auth.desktop_login.title")}
          </CardTitle>
          {view === "confirm" && !cancelled ? (
            <CardDescription>
              {t("auth.desktop_login.description")}
            </CardDescription>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          {view === "loading" || view === "sign_in" ? (
            <Loading label={t("common.loading")} />
          ) : view === "invalid" ||
            view === "forbidden" ||
            view === "failed" ? (
            <p role="alert" className="text-destructive">
              {t(`auth.desktop_login.${view}`)}
            </p>
          ) : view === "confirm" ? (
            cancelled ? (
              <p>{t("auth.desktop_login.cancelled")}</p>
            ) : (
              <div className="bg-muted rounded-lg px-3 py-2">
                <p className="text-muted-foreground text-xs">
                  {t("auth.desktop_login.account")}
                </p>
                <p className="font-medium break-all">{user?.email ?? ""}</p>
              </div>
            )
          ) : (
            <>
              <p>{t("auth.desktop_login.sent_body")}</p>
              <div className="border-border space-y-1 rounded-lg border border-dashed p-3">
                <p className="font-medium">
                  {t("auth.desktop_login.fallback_title")}
                </p>
                <p className="text-muted-foreground">
                  {expired
                    ? t("auth.desktop_login.expired")
                    : t("auth.desktop_login.fallback_body")}
                </p>
              </div>
            </>
          )}
        </CardContent>
        {view === "confirm" && !cancelled ? (
          <CardFooter className="flex flex-col gap-2 sm:flex-row sm:justify-between">
            <Button
              type="button"
              variant="ghost"
              onClick={() => void switchAccount()}
              disabled={create.isPending}
            >
              {t("auth.desktop_login.switch_account")}
            </Button>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setCancelled(true)}
                disabled={create.isPending}
              >
                {t("auth.desktop_login.cancel")}
              </Button>
              <Button type="button" onClick={start} disabled={create.isPending}>
                {t("auth.desktop_login.continue")}
              </Button>
            </div>
          </CardFooter>
        ) : view === "sent" ? (
          <CardFooter className="flex flex-col gap-2 sm:flex-row sm:justify-end">
            {expired ? (
              <Button type="button" onClick={start} disabled={create.isPending}>
                {t("auth.desktop_login.retry")}
              </Button>
            ) : callback ? (
              <Button asChild>
                <a href={callback}>{t("auth.desktop_login.open_again")}</a>
              </Button>
            ) : null}
          </CardFooter>
        ) : view === "failed" ? (
          <CardFooter className="justify-end">
            <Button
              type="button"
              onClick={() => {
                create.reset();
                start();
              }}
            >
              {t("auth.desktop_login.retry")}
            </Button>
          </CardFooter>
        ) : null}
      </Card>
    </main>
  );
}
