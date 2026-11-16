"use client";

import { MonitorSmartphone } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useMemo } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useLocale } from "@/providers/locale-provider";

import { desktopAppLink, desktopOpenPath } from "../lib/desktop-open";

/**
 * "Masaüstünde aç" target of notification e-mails (CF-236):
 * `/desktop/open?path=<in-app path>`. Hands the path to ClickFix Masaüstü
 * (`clickfix://`) once and keeps a "continue in the browser" link. Nothing
 * is changed by opening this page (no GET side effects).
 */
export function DesktopOpenPage() {
  const { t } = useLocale();
  const search = useSearchParams();
  const path = desktopOpenPath(search.get("path"));
  const appLink = useMemo(() => (path ? desktopAppLink(path) : null), [path]);

  useEffect(() => {
    if (appLink) window.location.assign(appLink);
  }, [appLink]);

  return (
    <main className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <MonitorSmartphone className="size-5" aria-hidden />
            {t("auth.desktop_open.title")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          {path ? (
            <>
              <p>{t("auth.desktop_open.body")}</p>
              <p className="text-muted-foreground">
                {t("auth.desktop_open.fallback")}
              </p>
            </>
          ) : (
            <p role="alert" className="text-destructive">
              {t("auth.desktop_open.invalid")}
            </p>
          )}
        </CardContent>
        {path && appLink ? (
          <CardFooter className="flex flex-col gap-2 sm:flex-row sm:justify-end">
            <Button asChild variant="outline">
              <a href={path}>{t("auth.desktop_open.browser")}</a>
            </Button>
            <Button asChild>
              <a href={appLink}>{t("auth.desktop_open.again")}</a>
            </Button>
          </CardFooter>
        ) : null}
      </Card>
    </main>
  );
}
