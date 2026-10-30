"use client";

import { useMutation } from "@tanstack/react-query";
import { ImagePlus, X } from "lucide-react";
import { usePathname } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  guidesService,
  type ProductFeedbackInput,
} from "../services/guides.service";

const KINDS = ["bug", "idea", "other"] as const;
const MAX_FILE = 5 * 1024 * 1024;

/**
 * "Geri bildirim": feedback about ClickFix itself, with an optional
 * screenshot uploaded through the attachments flow first (CF-141).
 */
export function ProductFeedbackDialog({
  open,
  onOpenChange,
  slug,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  slug: string;
}) {
  const { t } = useLocale();
  const pathname = usePathname() || "/";
  const [kind, setKind] = useState<ProductFeedbackInput["kind"]>("idea");
  const [message, setMessage] = useState("");
  const [file, setFile] = useState<File | null>(null);

  const reset = () => {
    setKind("idea");
    setMessage("");
    setFile(null);
  };

  const send = useMutation({
    mutationFn: async () => {
      const attachment_uuids: string[] = [];
      if (file) {
        const up = await guidesService.uploadScreenshot(slug, file, file.name);
        attachment_uuids.push(up.uuid);
      }
      return guidesService.sendFeedback(slug, {
        kind,
        message: message.trim(),
        route: pathname,
        surface: "web",
        app_version: process.env.NEXT_PUBLIC_APP_VERSION ?? "",
        device:
          typeof navigator !== "undefined"
            ? navigator.userAgent.slice(0, 200)
            : "",
        attachment_uuids,
      });
    },
    onSuccess: () => {
      appToast.success(t("guides.feedback.sent"));
      reset();
      onOpenChange(false);
    },
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) reset();
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("guides.feedback.title")}</DialogTitle>
          <DialogDescription>
            {t("guides.feedback.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <RadioGroup
            value={kind}
            onValueChange={(v) => setKind(v as ProductFeedbackInput["kind"])}
            className="flex flex-wrap gap-4"
          >
            {KINDS.map((k) => (
              <Label
                key={k}
                className="flex cursor-pointer items-center gap-2 font-normal"
              >
                <RadioGroupItem value={k} />
                {t(`guides.feedback.kind.${k}`)}
              </Label>
            ))}
          </RadioGroup>
          <div className="space-y-1.5">
            <Label htmlFor="product-feedback-message">
              {t("guides.feedback.message")}
            </Label>
            <Textarea
              id="product-feedback-message"
              rows={5}
              maxLength={5000}
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              placeholder={t("guides.feedback.placeholder")}
            />
          </div>
          <div className="flex items-center gap-2">
            {file ? (
              <span className="bg-muted inline-flex max-w-full items-center gap-2 rounded-md px-2 py-1 text-sm">
                <span className="truncate">{file.name}</span>
                <button
                  type="button"
                  onClick={() => setFile(null)}
                  aria-label={t("guides.feedback.remove_screenshot")}
                >
                  <X className="size-3.5" />
                </button>
              </span>
            ) : (
              <Button variant="outline" size="sm" asChild>
                <label className="cursor-pointer">
                  <ImagePlus className="size-4" />
                  {t("guides.feedback.add_screenshot")}
                  <input
                    type="file"
                    accept="image/png,image/jpeg,image/webp"
                    className="sr-only"
                    onChange={(e) => {
                      const f = e.target.files?.[0] ?? null;
                      if (f && f.size > MAX_FILE) {
                        appToast.error(t("guides.feedback.too_large"));
                        return;
                      }
                      setFile(f);
                    }}
                  />
                </label>
              </Button>
            )}
            <span className="text-muted-foreground text-xs">
              {t("guides.feedback.privacy")}
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={() => send.mutate()}
            disabled={!message.trim() || send.isPending}
          >
            {t("guides.feedback.send")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
