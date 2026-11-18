"use client";

import { useState } from "react";
import { AlertTriangle, Pencil } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { useEditMessage } from "../hooks/use-inbox";
import { deliveredToContact } from "../lib/message-edit";
import type { ConversationDetail, OptimisticMessage } from "../types";

const MAX_LENGTH = 10000;

/**
 * Corrects the agent's own reply (CF-248). When the contact already has
 * the reply and the channel cannot update it, the agent is told the
 * contact keeps the first text.
 */
export function EditMessageDialog({
  conversation: c,
  message: m,
  onClose,
}: {
  conversation: ConversationDetail;
  message: OptimisticMessage;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const edit = useEditMessage(c.uuid);
  const [content, setContent] = useState(m.content);
  const [error, setError] = useState("");
  const delivered = deliveredToContact(m);
  const unsupported = delivered && !c.channel_edits;
  const text = content.trim();
  const unchanged = text === m.content.trim();

  const save = () => {
    setError("");
    edit.mutate(
      { message: m.uuid, content: text },
      {
        onSuccess: (out) => {
          toast.success(t(`inbox.edit.done.${out.channel_edit}`));
          onClose();
        },
        onError: (err) => {
          if (isApiError(err) && err.code === "MESSAGE_EDIT_WINDOW_CLOSED") {
            setError(t("inbox.edit.error.window"));
          } else if (isApiError(err) && err.code === "MESSAGE_NOT_EDITABLE") {
            setError(t("inbox.edit.error.not_editable"));
          } else {
            setError(t("inbox.edit.error.generic"));
          }
        },
      },
    );
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("inbox.edit.title")}</DialogTitle>
          <DialogDescription>
            {delivered && c.channel_edits
              ? t("inbox.edit.hint_notified")
              : delivered
                ? t("inbox.edit.hint")
                : t("inbox.edit.hint_not_sent")}
          </DialogDescription>
        </DialogHeader>
        {unsupported && (
          <Alert>
            <AlertTriangle className="size-4" />
            <AlertDescription>{t("inbox.edit.unsupported")}</AlertDescription>
          </Alert>
        )}
        <Textarea
          aria-label={t("inbox.edit.label")}
          value={content}
          maxLength={MAX_LENGTH}
          rows={5}
          autoFocus
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && text) {
              e.preventDefault();
              if (!unchanged && !edit.isPending) save();
            }
          }}
        />
        {error && (
          <p role="alert" className="text-destructive text-sm">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!text || unchanged || edit.isPending}
            onClick={save}
          >
            <Pencil className="size-4" />
            {t("inbox.edit.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
