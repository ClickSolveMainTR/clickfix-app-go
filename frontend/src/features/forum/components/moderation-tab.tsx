"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ban, Check, ShieldAlert, Sparkles, X } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import {
  forumService,
  type ForumModerationItem,
} from "../services/forum.service";
import { Pager } from "./pager";

const PAGE = 20;
const KNOWN_FLAGS = new Set([
  "unsafe_instructions",
  "link_spam",
  "awaiting_review",
  "ai_reviewed",
]);

/**
 * Pending posts (CF-78): every visitor topic and reply waits here. Approve
 * publishes; reject and "spam" both hide it (the API has two outcomes:
 * published / rejected). AI triage is explicit and only adds a suggestion.
 */
export function ModerationTab({ org }: { org: string }) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const [offset, setOffset] = useState(0);
  const key = forumKeys.moderation(org, offset);
  const list = useQuery({
    queryKey: key,
    queryFn: () => forumService.moderation(PAGE, offset),
  });
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: forumKeys.all(org) });
  const onError = (error: unknown) =>
    appToast.error(isApiError(error) ? error.message : t("forum.toast.failed"));

  const moderate = useMutation({
    mutationFn: (v: {
      uuid: string;
      status: "published" | "rejected";
      spam?: boolean;
    }) => forumService.moderate(v.uuid, v.status),
    onSuccess: async (_d, v) => {
      await invalidate();
      appToast.success(
        t(
          v.status === "published"
            ? "forum.toast.approved"
            : v.spam
              ? "forum.toast.spam"
              : "forum.toast.rejected",
        ),
      );
    },
    onError,
  });
  const assist = useMutation({
    mutationFn: (uuid: string) => forumService.assist(uuid),
    onSuccess: async (r) => {
      await invalidate();
      appToast.info(
        r.flagged
          ? t("forum.toast.ai_flagged", { reason: r.reason })
          : t("forum.toast.ai_clean"),
      );
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 429
          ? t("forum.toast.ai_limited")
          : isApiError(error)
            ? error.message
            : t("forum.toast.failed"),
      ),
  });

  const flagLabel = (reason: string) => {
    if (!reason) return null;
    if (KNOWN_FLAGS.has(reason)) return t(`forum.flag.${reason}`);
    return reason.startsWith("ai: ")
      ? t("forum.flag.ai", { reason: reason.slice(4) })
      : reason;
  };
  const busy = (uuid: string) =>
    (moderate.isPending && moderate.variables?.uuid === uuid) ||
    (assist.isPending && assist.variables === uuid);

  if (list.isLoading) return <Loading label={t("common.loading")} />;
  if (list.isError)
    return (
      <ErrorState
        title={t("forum.error.title")}
        description={t("forum.error.description")}
        onRetry={() => void list.refetch()}
      />
    );
  const page = list.data;
  if (!page || page.items.length === 0)
    return (
      <Card>
        <CardContent className="text-muted-foreground py-8 text-center text-sm">
          {t("forum.moderation.empty")}
        </CardContent>
      </Card>
    );

  return (
    <div className="space-y-3">
      <p className="text-muted-foreground text-sm">
        {t("forum.moderation.count", { count: page.total })}
      </p>
      {page.items.map((p: ForumModerationItem) => {
        const isReply = p.topic_id != null;
        const flag = flagLabel(p.flag_reason);
        const risky =
          p.flag_reason === "unsafe_instructions" ||
          p.flag_reason === "link_spam" ||
          p.flag_reason.startsWith("ai: ");
        return (
          <Card key={p.uuid}>
            <CardContent className="space-y-3 py-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge variant={isReply ? "secondary" : "default"}>
                  {isReply ? t("forum.kind.reply") : t("forum.kind.topic")}
                </Badge>
                {flag && (
                  <Badge
                    variant="outline"
                    className={
                      risky
                        ? "border-red-500/40 text-red-700 dark:text-red-300"
                        : undefined
                    }
                  >
                    {risky && <ShieldAlert className="size-3" />}
                    {flag}
                  </Badge>
                )}
                <span className="text-muted-foreground ml-auto text-xs">
                  {datetime(p.created_at, "dd.MM.yyyy HH:mm", locale)}
                </span>
              </div>
              {p.title && <p className="font-medium break-words">{p.title}</p>}
              <p className="text-muted-foreground line-clamp-6 text-sm break-words whitespace-pre-wrap">
                {p.body}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  disabled={busy(p.uuid)}
                  onClick={() =>
                    moderate.mutate({ uuid: p.uuid, status: "published" })
                  }
                >
                  <Check className="size-4" />
                  {t("forum.moderation.approve")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy(p.uuid)}
                  onClick={() =>
                    moderate.mutate({ uuid: p.uuid, status: "rejected" })
                  }
                >
                  <X className="size-4" />
                  {t("forum.moderation.reject")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  disabled={busy(p.uuid)}
                  onClick={() =>
                    moderate.mutate({
                      uuid: p.uuid,
                      status: "rejected",
                      spam: true,
                    })
                  }
                >
                  <Ban className="size-4" />
                  {t("forum.moderation.spam")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={busy(p.uuid)}
                  title={t("forum.moderation.assist_hint")}
                  onClick={() => assist.mutate(p.uuid)}
                >
                  <Sparkles className="size-4" />
                  {t("forum.moderation.assist")}
                </Button>
              </div>
            </CardContent>
          </Card>
        );
      })}
      <Pager
        total={page.total}
        limit={PAGE}
        offset={offset}
        onChange={setOffset}
      />
    </div>
  );
}
