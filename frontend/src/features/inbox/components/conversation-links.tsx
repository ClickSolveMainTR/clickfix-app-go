"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { GitMerge, Link2, Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { inboxKeys, useInboxScope } from "../hooks/use-inbox";
import {
  useConversationLinks,
  useLinkConversation,
  useMergeConversation,
  useUnlinkConversation,
} from "../hooks/use-links";
import { inboxService } from "../services/inbox.service";
import type { Conversation, ConversationDetail, LinkRelation } from "../types";
import { ChannelGlyph } from "./channel-glyph";

/** Linked, parent / child and merged conversations (CF-98). */
export function ConversationLinksPanel({
  conversation: c,
  onOpen,
}: {
  conversation: ConversationDetail;
  onOpen: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const { canManage } = useInboxScope();
  const links = useConversationLinks(c.uuid);
  const unlink = useUnlinkConversation(c.uuid);
  const [dialog, setDialog] = useState<"merge" | "link" | null>(null);
  const items = links.data?.items ?? [];
  const mergedInto = links.data?.merged_into ?? null;

  return (
    <section>
      <div className="mb-2 flex items-center gap-1">
        <h3 className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {t("inbox.links.title")}
        </h3>
        {canManage && !mergedInto && (
          <div className="ml-auto flex gap-0.5">
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              title={t("inbox.links.link")}
              aria-label={t("inbox.links.link")}
              onClick={() => setDialog("link")}
            >
              <Link2 className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              title={t("inbox.links.merge")}
              aria-label={t("inbox.links.merge")}
              onClick={() => setDialog("merge")}
            >
              <GitMerge className="size-3.5" />
            </Button>
          </div>
        )}
      </div>
      {mergedInto && (
        <button
          type="button"
          onClick={() => onOpen(mergedInto.uuid)}
          className="mb-2 w-full rounded-md border border-amber-200 bg-amber-50 px-2 py-1.5 text-left text-xs text-amber-950 dark:border-amber-900/60 dark:bg-amber-950/40 dark:text-amber-100"
        >
          {t("inbox.links.merged_banner", { number: mergedInto.number })}
        </button>
      )}
      {items.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("inbox.links.empty")}
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.map((l) => (
            <li
              key={`${l.relation}-${l.uuid}`}
              className="group flex items-center"
            >
              <button
                type="button"
                onClick={() => onOpen(l.uuid)}
                className="hover:bg-muted flex min-w-0 flex-1 flex-col rounded-md px-2 py-1.5 text-left"
              >
                <span className="flex items-center gap-1.5 text-xs font-medium">
                  <ChannelGlyph type={l.channel_type} className="size-3" />#
                  {l.number}
                  <span className="bg-muted text-muted-foreground rounded px-1 text-[10px] font-normal">
                    {t(`inbox.links.relation.${l.relation}`)}
                  </span>
                  <span className="text-muted-foreground ml-auto font-normal">
                    {t(`inbox.status.${l.status}`)}
                  </span>
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {l.subject || "—"}
                </span>
              </button>
              {canManage && l.relation !== "merged" && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-6 opacity-0 group-hover:opacity-100 focus:opacity-100"
                  aria-label={t("inbox.links.unlink")}
                  title={t("inbox.links.unlink")}
                  disabled={unlink.isPending}
                  onClick={() => unlink.mutate(l.uuid)}
                >
                  <X className="size-3.5" />
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
      {dialog === "merge" && (
        <MergeDialog conversation={c} onClose={() => setDialog(null)} />
      )}
      {dialog === "link" && (
        <LinkDialog conversation={c} onClose={() => setDialog(null)} />
      )}
    </section>
  );
}

function MergeDialog({
  conversation: c,
  onClose,
}: {
  conversation: ConversationDetail;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const merge = useMergeConversation(c.uuid);
  const [picked, setPicked] = useState<Conversation | null>(null);
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t("inbox.links.merge_title", { number: c.number })}
          </DialogTitle>
          <DialogDescription>{t("inbox.links.merge_hint")}</DialogDescription>
        </DialogHeader>
        <ConversationPicker
          exclude={c.uuid}
          picked={picked}
          onPick={setPicked}
        />
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!picked || merge.isPending}
            onClick={() =>
              picked &&
              merge.mutate(picked.uuid, { onSuccess: () => onClose() })
            }
          >
            <GitMerge className="size-4" />
            {picked
              ? t("inbox.links.merge_confirm", { number: picked.number })
              : t("inbox.links.merge")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function LinkDialog({
  conversation: c,
  onClose,
}: {
  conversation: ConversationDetail;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const link = useLinkConversation(c.uuid);
  const [picked, setPicked] = useState<Conversation | null>(null);
  const [relation, setRelation] = useState<LinkRelation>("related");
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("inbox.links.link_title")}</DialogTitle>
          <DialogDescription>{t("inbox.links.link_hint")}</DialogDescription>
        </DialogHeader>
        <Select
          value={relation}
          onValueChange={(v) => setRelation(v as LinkRelation)}
        >
          <SelectTrigger aria-label={t("inbox.links.relation_label")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(["related", "parent", "child"] as const).map((r) => (
              <SelectItem key={r} value={r}>
                {t(`inbox.links.as.${r}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <ConversationPicker
          exclude={c.uuid}
          picked={picked}
          onPick={setPicked}
        />
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!picked || link.isPending}
            onClick={() =>
              picked &&
              link.mutate(
                { other: picked.uuid, relation },
                { onSuccess: () => onClose() },
              )
            }
          >
            <Link2 className="size-4" />
            {t("inbox.links.link")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Searches the inbox (number, subject, contact, message text). */
function ConversationPicker({
  exclude,
  picked,
  onPick,
}: {
  exclude: string;
  picked: Conversation | null;
  onPick: (c: Conversation) => void;
}) {
  const { t } = useLocale();
  const { org } = useInboxScope();
  const [q, setQ] = useState("");
  const [debounced, setDebounced] = useState("");
  useEffect(() => {
    const id = setTimeout(() => setDebounced(q.trim()), 250);
    return () => clearTimeout(id);
  }, [q]);
  const results = useQuery({
    queryKey: [...inboxKeys.all(org), "picker", debounced],
    queryFn: () =>
      inboxService.list({
        view: "all",
        status: "all",
        q: debounced,
        unread: false,
        sort: "-last_message_at",
        limit: 15,
        offset: 0,
      }),
    enabled: Boolean(org),
  });
  const items = (results.data?.items ?? []).filter((x) => x.uuid !== exclude);
  return (
    <div className="flex flex-col gap-2">
      <div className="relative">
        <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
        <Input
          autoFocus
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("inbox.links.search")}
          className="pl-8"
        />
      </div>
      <ul
        className="flex max-h-64 flex-col gap-0.5 overflow-y-auto"
        role="listbox"
      >
        {items.length === 0 && !results.isLoading && (
          <li className="text-muted-foreground px-2 py-3 text-xs">
            {t("inbox.links.no_results")}
          </li>
        )}
        {items.map((x) => (
          <li key={x.uuid}>
            <button
              type="button"
              role="option"
              aria-selected={picked?.uuid === x.uuid}
              onClick={() => onPick(x)}
              className={cn(
                "hover:bg-muted flex w-full flex-col rounded-md px-2 py-1.5 text-left",
                picked?.uuid === x.uuid && "bg-muted ring-ring ring-1",
              )}
            >
              <span className="flex items-center gap-1.5 text-xs font-medium">
                <ChannelGlyph type={x.channel_type} className="size-3" />#
                {x.number} · {x.contact.name || x.contact.email || "—"}
                <span className="text-muted-foreground ml-auto font-normal">
                  {t(`inbox.status.${x.status}`)}
                </span>
              </span>
              <span className="text-muted-foreground truncate text-xs">
                {x.subject || x.last_message_preview}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
