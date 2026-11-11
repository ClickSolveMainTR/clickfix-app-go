"use client";

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
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import {
  dialogCopy,
  REASON_MAX,
  reasonTooLong,
  type TakedownRequest,
} from "../lib/takedown";

/**
 * Asks before a post is taken down or published again (CF-182). A takedown
 * takes an optional reason, stored with who removed the post and when.
 * Nothing is sent until the moderator confirms.
 */
export function VisibilityDialog({
  request,
  isPending,
  onConfirm,
  onCancel,
}: {
  request: TakedownRequest | null;
  isPending: boolean;
  onConfirm: (reason: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const [reason, setReason] = useState("");
  const copy = request ? dialogCopy(request) : null;
  const close = () => {
    setReason("");
    onCancel();
  };
  return (
    <Dialog
      open={request !== null}
      onOpenChange={(next) => {
        if (!next && !isPending) close();
      }}
    >
      <DialogContent>
        {copy && (
          <>
            <DialogHeader>
              <DialogTitle>{t(copy.title)}</DialogTitle>
              <DialogDescription>{t(copy.body)}</DialogDescription>
            </DialogHeader>
            {copy.destructive && (
              <div className="space-y-2">
                <Label htmlFor="forum-takedown-reason">
                  {t("forum.visibility.reason")}
                </Label>
                <Textarea
                  id="forum-takedown-reason"
                  value={reason}
                  maxLength={REASON_MAX + 1}
                  rows={3}
                  placeholder={t("forum.visibility.reason_hint")}
                  onChange={(e) => setReason(e.target.value)}
                />
                {reasonTooLong(reason) && (
                  <p className="text-destructive text-xs">
                    {t("forum.visibility.reason_too_long", {
                      max: REASON_MAX,
                    })}
                  </p>
                )}
              </div>
            )}
            <DialogFooter>
              <Button variant="outline" onClick={close} disabled={isPending}>
                {t("common.cancel")}
              </Button>
              <Button
                variant={copy.destructive ? "destructive" : "default"}
                disabled={isPending || reasonTooLong(reason)}
                onClick={() => {
                  onConfirm(reason);
                  setReason("");
                }}
              >
                {t(copy.action)}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
