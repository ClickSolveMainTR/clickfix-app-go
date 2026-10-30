"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Eye, EyeOff, Loader2, RotateCcw } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { widgetConfigService } from "@/features/widget/services/widget.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";
import { useDialogs } from "@/providers/dialog-provider";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  widgetId: string;
  current: number;
  canPublish: boolean;
  /** Shows a version in the live preview (null settings: back to the draft). */
  onPreview: (
    version: number,
    settings: Record<string, unknown> | null,
  ) => void;
  onRolledBack: () => Promise<void>;
};

/** Published versions with preview and one-click rollback (CF-150). */
export function VersionHistory({
  open,
  onOpenChange,
  widgetId,
  current,
  canPublish,
  onPreview,
  onRolledBack,
}: Props) {
  const { confirm } = useDialogs();
  const { t, locale } = useLocale();
  const [previewing, setPreviewing] = useState<number | null>(null);
  const versions = useQuery({
    queryKey: ["widget-versions", widgetId],
    queryFn: () => widgetConfigService.versions(widgetId),
    enabled: open,
  });
  const preview = useMutation({
    mutationFn: (version: number) =>
      widgetConfigService.version(widgetId, version),
    onSuccess: (v) => {
      setPreviewing(v.version);
      onPreview(v.version, v.settings as Record<string, unknown>);
    },
  });
  const rollback = useMutation({
    mutationFn: (version: number) =>
      widgetConfigService.rollback(widgetId, version, current),
    onSuccess: async (v) => {
      appToast.success(t("widget.toast.rolled_back", { version: v.version }));
      setPreviewing(null);
      onOpenChange(false);
      await versions.refetch();
      await onRolledBack();
    },
    onError: (err) =>
      appToast.error(isApiError(err) ? err.message : t("widget.toast.failed")),
  });
  const fmt = (iso: string) =>
    new Date(iso).toLocaleString(locale === "tr" ? "tr-TR" : "en-US");
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t("widget.history.title")}</SheetTitle>
          <SheetDescription>{t("widget.history.description")}</SheetDescription>
        </SheetHeader>
        <div className="space-y-2 p-4">
          {versions.isLoading ? (
            <Loader2 className="size-4 animate-spin" />
          ) : null}
          {versions.data?.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("widget.history.empty")}
            </p>
          ) : null}
          {(versions.data ?? []).map((v) => (
            <div key={v.uuid} className="space-y-2 rounded-lg border p-3">
              <div className="flex items-center gap-2">
                <span className="font-semibold">v{v.version}</span>
                {v.version === current ? (
                  <Badge>{t("widget.history.live")}</Badge>
                ) : null}
                {v.rollback_of ? (
                  <Badge variant="secondary">
                    {t("widget.history.rollback_of", {
                      version: v.rollback_of,
                    })}
                  </Badge>
                ) : null}
                <span className="text-muted-foreground ms-auto text-xs">
                  {fmt(v.created_at)}
                </span>
              </div>
              {v.note ? <p className="text-sm">{v.note}</p> : null}
              {v.created_by_name ? (
                <p className="text-muted-foreground text-xs">
                  {v.created_by_name}
                </p>
              ) : null}
              <div className="flex gap-2">
                {previewing === v.version ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                      setPreviewing(null);
                      onPreview(v.version, null);
                    }}
                  >
                    <EyeOff className="size-4" />
                    {t("widget.history.back_to_draft")}
                  </Button>
                ) : (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={preview.isPending}
                    onClick={() => preview.mutate(v.version)}
                  >
                    <Eye className="size-4" />
                    {t("widget.history.preview")}
                  </Button>
                )}
                {canPublish && v.version !== current ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={rollback.isPending}
                    onClick={async () => {
                      if (
                        await confirm({
                          title: t("widget.history.rollback_confirm", {
                            version: v.version,
                          }),
                        })
                      )
                        rollback.mutate(v.version);
                    }}
                  >
                    <RotateCcw className="size-4" />
                    {t("widget.history.rollback")}
                  </Button>
                ) : null}
              </div>
            </div>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  );
}
