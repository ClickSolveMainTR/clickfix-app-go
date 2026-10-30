"use client";

import { useState } from "react";
import { Split } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useLocale } from "@/providers/locale-provider";

import { useSplitConversation } from "../hooks/use-links";
import type { ConversationDetail, OptimisticMessage } from "../types";

/** Moves a message (and optionally the later ones) into a new child conversation (CF-98). */
export function SplitDialog({
  conversation: c,
  message: m,
  onClose,
  onSplit,
}: {
  conversation: ConversationDetail;
  message: OptimisticMessage;
  onClose: () => void;
  onSplit: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const split = useSplitConversation(c.uuid);
  const [subject, setSubject] = useState(c.subject);
  const [following, setFollowing] = useState(false);
  const preview = (m.content || "").slice(0, 160);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("inbox.split.title")}</DialogTitle>
          <DialogDescription>{t("inbox.split.hint")}</DialogDescription>
        </DialogHeader>
        {preview && (
          <blockquote className="bg-muted/50 text-muted-foreground rounded-md border-l-2 px-3 py-2 text-xs whitespace-pre-wrap">
            {preview}
          </blockquote>
        )}
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="split-subject">{t("inbox.split.subject")}</Label>
          <Input
            id="split-subject"
            value={subject}
            maxLength={500}
            onChange={(e) => setSubject(e.target.value)}
          />
        </div>
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={following}
            onCheckedChange={(v) => setFollowing(v === true)}
          />
          {t("inbox.split.following")}
        </label>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={split.isPending}
            onClick={() =>
              split.mutate(
                {
                  message_uuid: m.uuid,
                  following,
                  subject: subject.trim(),
                },
                { onSuccess: (child) => onSplit(child.uuid) },
              )
            }
          >
            <Split className="size-4" />
            {t("inbox.split.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
