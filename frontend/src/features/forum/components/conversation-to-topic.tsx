"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { MessagesSquare } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import { forumService } from "../services/forum.service";

/**
 * "Share to the community forum" for a conversation (CF-78). The agent
 * writes the public question and redacted text; the transcript is never
 * copied. The new topic waits in the moderation queue.
 */
export function ConversationToTopic({
  conversationUuid,
  subject,
}: {
  conversationUuid: string;
  subject: string | null | undefined;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const canShare = (org?.permissions ?? []).includes(
    Permission.TenantFeedbackManage,
  );
  const [open, setOpen] = useState(false);
  if (!canShare || !org) return null;
  return (
    <section>
      <Button
        variant="outline"
        size="sm"
        className="w-full"
        onClick={() => setOpen(true)}
      >
        <MessagesSquare className="size-4" />
        {t("forum.share.button")}
      </Button>
      {open && (
        <ShareDialog
          org={org.slug}
          conversationUuid={conversationUuid}
          subject={subject ?? ""}
          onClose={() => setOpen(false)}
        />
      )}
    </section>
  );
}

function ShareDialog({
  org,
  conversationUuid,
  subject,
  onClose,
}: {
  org: string;
  conversationUuid: string;
  subject: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const categories = useQuery({
    queryKey: forumKeys.categories(org),
    queryFn: forumService.categories,
  });
  const [category, setCategory] = useState("");
  const [title, setTitle] = useState(subject.slice(0, 200));
  const [body, setBody] = useState("");
  const share = useMutation({
    mutationFn: () =>
      forumService.fromConversation({
        conversation_uuid: conversationUuid,
        category,
        title: title.trim(),
        body: body.trim(),
      }),
    onSuccess: () => {
      appToast.success(t("forum.toast.shared"));
      onClose();
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("forum.toast.already_shared")
          : isApiError(error)
            ? error.message
            : t("forum.toast.failed"),
      ),
  });
  const invalid = !category || !title.trim() || !body.trim();
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("forum.share.title")}
      description={t("forum.share.hint")}
      className="sm:max-w-lg"
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!invalid) share.mutate();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="share-category">{t("forum.topics.category")}</Label>
          <Select value={category} onValueChange={setCategory}>
            <SelectTrigger id="share-category" className="w-full">
              <SelectValue placeholder={t("forum.share.pick_category")} />
            </SelectTrigger>
            <SelectContent>
              {(categories.data ?? []).map((c) => (
                <SelectItem key={c.uuid} value={c.slug}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {categories.isSuccess && categories.data.length === 0 && (
            <p className="text-muted-foreground text-xs">
              {t("forum.share.no_category")}
            </p>
          )}
        </div>
        <div className="space-y-2">
          <Label htmlFor="share-title">{t("forum.share.question")}</Label>
          <Input
            id="share-title"
            value={title}
            maxLength={200}
            onChange={(e) => setTitle(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="share-body">{t("forum.share.body")}</Label>
          <Textarea
            id="share-body"
            rows={6}
            value={body}
            maxLength={16000}
            onChange={(e) => setBody(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("forum.share.body_hint")}
          </p>
        </div>
        <FormActions sticky={false}>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={invalid || share.isPending}>
            {t("forum.share.submit")}
          </Button>
        </FormActions>
      </form>
    </AppDialog>
  );
}
