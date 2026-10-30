"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CheckCircle2,
  ExternalLink,
  MessageSquare,
  MessagesSquare,
  Search,
  ThumbsUp,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useChannels } from "@/features/channels/hooks/use-channels";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import {
  forumService,
  type ForumTopicSummary,
} from "../services/forum.service";
import { Pager } from "./pager";

const PAGE = 20;
const ALL = "__all__";

/**
 * Published topics (CF-78) as visitors see them, with agent actions: mark
 * an accepted answer and turn a topic into an inbox conversation.
 */
export function TopicsTab({
  org,
  canConvert,
}: {
  org: string;
  canConvert: boolean;
}) {
  const { t, locale } = useLocale();
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  const [category, setCategory] = useState<string>(ALL);
  const [offset, setOffset] = useState(0);
  const [open, setOpen] = useState<ForumTopicSummary | null>(null);
  useEffect(() => {
    const id = setTimeout(() => {
      setDebounced(q.trim());
      setOffset(0);
    }, 300);
    return () => clearTimeout(id);
  }, [q]);
  const categories = useQuery({
    queryKey: forumKeys.categories(org),
    queryFn: forumService.categories,
  });
  const filter = {
    offset,
    category: category === ALL ? undefined : category,
    q: debounced || undefined,
  };
  const list = useQuery({
    queryKey: forumKeys.topics(org, filter),
    queryFn: () => forumService.topics({ limit: PAGE, ...filter }),
  });
  const catName = (slug: string) =>
    categories.data?.find((c) => c.slug === slug)?.name ?? slug;

  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-2 sm:flex-row">
        <div className="relative flex-1">
          <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
          <Input
            value={q}
            maxLength={200}
            placeholder={t("forum.topics.search")}
            aria-label={t("forum.topics.search")}
            className="pl-9"
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <Select
          value={category}
          onValueChange={(v) => {
            setCategory(v);
            setOffset(0);
          }}
        >
          <SelectTrigger
            className="w-full sm:w-56"
            aria-label={t("forum.topics.category")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t("forum.topics.all_categories")}
            </SelectItem>
            {(categories.data ?? []).map((c) => (
              <SelectItem key={c.uuid} value={c.slug}>
                {c.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <p className="text-muted-foreground text-xs">{t("forum.topics.hint")}</p>
      {list.isLoading && <Loading label={t("common.loading")} />}
      {list.isError && (
        <ErrorState
          title={t("forum.error.title")}
          description={t("forum.error.description")}
          onRetry={() => void list.refetch()}
        />
      )}
      {list.data?.items.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("forum.topics.empty")}
          </CardContent>
        </Card>
      )}
      <div className="space-y-2">
        {(list.data?.items ?? []).map((topic) => (
          <button
            key={topic.uuid}
            type="button"
            onClick={() => setOpen(topic)}
            className="bg-card hover:bg-muted/50 focus-visible:ring-ring flex w-full items-start gap-3 rounded-xl border p-4 text-left outline-none focus-visible:ring-2"
          >
            <MessagesSquare className="text-muted-foreground mt-0.5 size-5 shrink-0" />
            <span className="min-w-0 flex-1 space-y-1">
              <span className="block font-medium break-words">
                {topic.title}
              </span>
              <span className="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
                <span>{catName(topic.category_slug)}</span>
                <span>{datetime(topic.created_at, "dd.MM.yyyy", locale)}</span>
                <span className="flex items-center gap-1">
                  <ThumbsUp className="size-3" />
                  {topic.votes}
                </span>
              </span>
            </span>
            {topic.accepted_reply_uuid && (
              <Badge
                variant="outline"
                className="shrink-0 border-emerald-500/40 text-emerald-700 dark:text-emerald-300"
              >
                <CheckCircle2 className="size-3" />
                {t("forum.topics.answered")}
              </Badge>
            )}
          </button>
        ))}
      </div>
      {list.data && (
        <Pager
          total={list.data.total}
          limit={PAGE}
          offset={offset}
          onChange={setOffset}
        />
      )}
      {open && (
        <TopicDialog
          org={org}
          summary={open}
          canConvert={canConvert}
          onClose={() => setOpen(null)}
        />
      )}
    </div>
  );
}

function TopicDialog({
  org,
  summary,
  canConvert,
  onClose,
}: {
  org: string;
  summary: ForumTopicSummary;
  canConvert: boolean;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const [converting, setConverting] = useState(false);
  const topic = useQuery({
    queryKey: forumKeys.topic(org, summary.slug),
    queryFn: () => forumService.topic(summary.slug),
  });
  const accept = useMutation({
    mutationFn: (reply: string) => forumService.accept(summary.uuid, reply),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: forumKeys.all(org) });
      appToast.success(t("forum.toast.accepted"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("forum.toast.failed"),
      ),
  });
  const data = topic.data;
  const acceptedUuid =
    data?.accepted_answer?.uuid ?? summary.accepted_reply_uuid;

  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={summary.title}
      description={datetime(summary.created_at, "dd.MM.yyyy HH:mm", locale)}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" asChild>
            <a
              href={routes.help.forumTopic(org, summary.slug)}
              target="_blank"
              rel="noopener noreferrer"
            >
              <ExternalLink className="size-4" />
              {t("forum.view_public")}
            </a>
          </Button>
          {canConvert && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setConverting(true)}
            >
              <MessageSquare className="size-4" />
              {t("forum.topics.to_conversation")}
            </Button>
          )}
        </div>
        {topic.isLoading && <Skeleton className="h-32 w-full" />}
        {topic.isError && (
          <p className="text-destructive text-sm">{t("forum.error.title")}</p>
        )}
        {data && (
          <>
            <p className="text-sm break-words whitespace-pre-wrap">
              {data.body}
            </p>
            <h3 className="border-t pt-3 text-sm font-semibold">
              {t("forum.topics.replies", { count: data.replies.total })}
            </h3>
            {data.replies.items.length === 0 && (
              <p className="text-muted-foreground text-sm">
                {t("forum.topics.no_replies")}
              </p>
            )}
            <ul className="space-y-2">
              {data.replies.items.map((r) => {
                const accepted = r.uuid === acceptedUuid;
                return (
                  <li
                    key={r.uuid}
                    className={
                      accepted
                        ? "rounded-lg border border-emerald-500/40 bg-emerald-500/5 p-3"
                        : "rounded-lg border p-3"
                    }
                  >
                    <p className="text-sm break-words whitespace-pre-wrap">
                      {r.body}
                    </p>
                    <div className="text-muted-foreground mt-2 flex flex-wrap items-center gap-3 text-xs">
                      <span>
                        {datetime(r.created_at, "dd.MM.yyyy HH:mm", locale)}
                      </span>
                      <span className="flex items-center gap-1">
                        <ThumbsUp className="size-3" />
                        {r.votes}
                      </span>
                      {accepted ? (
                        <span className="flex items-center gap-1 font-medium text-emerald-700 dark:text-emerald-300">
                          <CheckCircle2 className="size-3.5" />
                          {t("forum.topics.accepted")}
                        </span>
                      ) : (
                        <Button
                          size="sm"
                          variant="ghost"
                          className="ml-auto h-7"
                          disabled={accept.isPending}
                          onClick={() => accept.mutate(r.uuid)}
                        >
                          <CheckCircle2 className="size-3.5" />
                          {t("forum.topics.accept")}
                        </Button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>
            {data.replies.total > data.replies.items.length && (
              <p className="text-muted-foreground text-xs">
                {t("forum.topics.more_replies")}
              </p>
            )}
          </>
        )}
      </div>
      {converting && (
        <ToConversationDialog
          topicUuid={summary.uuid}
          onClose={() => setConverting(false)}
        />
      )}
    </AppDialog>
  );
}

/** Opens an inbox conversation for the topic (or returns the linked one). */
function ToConversationDialog({
  topicUuid,
  onClose,
}: {
  topicUuid: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const channels = useChannels();
  const inboxes = (channels.data ?? []).filter((c) => c.status === "active");
  const [inbox, setInbox] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  const convert = useMutation({
    mutationFn: () => forumService.toConversation(topicUuid, inbox),
    onSuccess: (conv) => {
      appToast.success(t("forum.toast.converted"));
      setCreated(conv.uuid ?? null);
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("forum.toast.failed"),
      ),
  });
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("forum.topics.to_conversation")}
      description={t("forum.topics.to_conversation_hint")}
    >
      {created ? (
        <FormActions sticky={false}>
          <Button variant="outline" onClick={onClose}>
            {t("forum.close")}
          </Button>
          <Button asChild>
            <Link href={`/inbox?c=${created}`}>
              {t("forum.topics.open_conversation")}
            </Link>
          </Button>
        </FormActions>
      ) : (
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (inbox) convert.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="forum-inbox">{t("forum.topics.inbox")}</Label>
            <Select value={inbox} onValueChange={setInbox}>
              <SelectTrigger id="forum-inbox" className="w-full">
                <SelectValue placeholder={t("forum.topics.pick_inbox")} />
              </SelectTrigger>
              <SelectContent>
                {inboxes.map((c) => (
                  <SelectItem key={c.uuid} value={c.uuid}>
                    {c.name} · {t(`channels.types.${c.type}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {channels.isSuccess && inboxes.length === 0 && (
              <p className="text-muted-foreground text-xs">
                {t("forum.topics.no_inbox")}
              </p>
            )}
          </div>
          <FormActions sticky={false}>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!inbox || convert.isPending}>
              {t("forum.topics.convert")}
            </Button>
          </FormActions>
        </form>
      )}
    </AppDialog>
  );
}
