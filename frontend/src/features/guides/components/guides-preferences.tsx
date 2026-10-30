"use client";

import { RotateCcw } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useOptionalGuides } from "./guides-provider";

/** Profile → Preferences card: auto-start switch and "reset tours". */
export function GuidesPreferences() {
  const { t } = useLocale();
  const guides = useOptionalGuides();
  const [busy, setBusy] = useState(false);
  if (!guides) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("guides.prefs.title")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="guides-auto" className="font-normal">
            {t("guides.prefs.auto")}
          </Label>
          <Switch
            id="guides-auto"
            checked={guides.autoStart}
            onCheckedChange={guides.setAutoStart}
          />
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-muted-foreground text-sm">
            {t("guides.prefs.reset_hint")}
          </p>
          <Button
            variant="outline"
            size="sm"
            data-guide="guides-reset"
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              try {
                await guides.resetTours();
                appToast.success(t("guides.prefs.reset_done"));
              } finally {
                setBusy(false);
              }
            }}
          >
            <RotateCcw className="size-4" />
            {t("guides.prefs.reset")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
