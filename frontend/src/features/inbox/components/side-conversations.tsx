"use client";

import { useState } from "react";
import { Mail, MessagesSquare, Plus, Send } from "lucide-react";

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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn, datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useInboxScope } from "../hooks/use-inbox";
import {
  useReplySideConversation,
  useSideConversations,
  useSideStatus,
  useSideThread,
  useStartSideConversation,
} from "../hooks/use-links";
import type { ConversationDetail } from "../types";

/**
 * Side conversations (CF-98): e-mail threads with people outside the
 * conversation (another team, a supplier) run from inside it.
 */
export function SideConversationsPanel({
  conversation: c,
}: {
  conversation: ConversationDetail;
}) {
  const { t, locale } = useLocale();
  const { canReply } = useInboxScope();
  const list = useSideConversations(c.uuid);
  const [starting, setStarting] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const items = list.data?.items ?? [];
  const inboxes = list.data?.inboxes ?? [];
  return (
    <section>
      <div className="mb-2 flex items-center">
        <h3 className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {t("inbox.side.title")}
        </h3>
        {canReply && (
          <Button
            variant="ghost"
            size="icon"
            className="ml-auto size-6"
            title={t("inbox.side.start")}
            aria-label={t("inbox.side.start")}
            onClick={() => setStarting(true)}
          >
            <Plus className="size-3.5" />
          </Button>
        )}
      </div>
      {items.length === 0 ? (
        <p className="text-muted-foreground text-xs">{t("inbox.side.empty")}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.map((s) => (
            <li key={s.uuid}>
              <button
                type="button"
                onClick={() => setOpen(s.uuid)}
                className="hover:bg-muted flex w-full flex-col rounded-md px-2 py-1.5 text-left"
              >
                <span className="flex items-center gap-1.5 text-xs font-medium">
                  <MessagesSquare className="size-3 shrink-0" />
                  <span className="truncate">{s.subject}</span>
                  <span
                    className={cn(
                      "ml-auto shrink-0 font-normal",
                      s.status === "open"
                        ? "text-emerald-600 dark:text-emerald-400"
                        : "text-muted-foreground",
                    )}
                  >
                    {t(`inbox.side.status.${s.status}`)}
                  </span>
                </span>
                <span className="text-muted-foreground truncate text-xs">
                  {s.participants.join(", ")} ·{" "}
                  {datetime(s.last_message_at, "dd.MM HH:mm", locale)}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {starting && (
        <StartDialog
          conversation={c}
          inboxes={inboxes}
          onClose={() => setStarting(false)}
          onStarted={(uuid) => {
            setStarting(false);
            setOpen(uuid);
          }}
        />
      )}
      {open && (
        <ThreadDialog
          conversation={c.uuid}
          side={open}
          canReply={canReply}
          onClose={() => setOpen(null)}
        />
      )}
    </section>
  );
}

function StartDialog({
  conversation: c,
  inboxes,
  onClose,
  onStarted,
}: {
  conversation: ConversationDetail;
  inboxes: { uuid: string; name: string }[];
  onClose: () => void;
  onStarted: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const start = useStartSideConversation(c.uuid);
  const [to, setTo] = useState("");
  const [subject, setSubject] = useState(c.subject ? c.subject : "");
  const [body, setBody] = useState("");
  const [inbox, setInbox] = useState(inboxes[0]?.uuid ?? "");
  const recipients = to
    .split(/[,;\s]+/)
    .map((x) => x.trim())
    .filter(Boolean);
  const valid =
    recipients.length > 0 && subject.trim() !== "" && body.trim() !== "";
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("inbox.side.start_title")}</DialogTitle>
          <DialogDescription>{t("inbox.side.start_hint")}</DialogDescription>
        </DialogHeader>
        {inboxes.length === 0 ? (
          <p className="text-muted-foreground rounded-md border border-dashed p-3 text-sm">
            {t("inbox.side.no_inbox")}
          </p>
        ) : (
          <form
            id="side-start"
            className="flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (!valid) return;
              start.mutate(
                {
                  to: recipients,
                  subject: subject.trim(),
                  body: body.trim(),
                  ...(inbox ? { inbox_uuid: inbox } : {}),
                },
                { onSuccess: (s) => onStarted(s.uuid) },
              );
            }}
          >
            {inboxes.length > 1 && (
              <div className="flex flex-col gap-1.5">
                <Label>{t("inbox.side.from")}</Label>
                <Select value={inbox} onValueChange={setInbox}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {inboxes.map((i) => (
                      <SelectItem key={i.uuid} value={i.uuid}>
                        {i.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="side-to">{t("inbox.side.to")}</Label>
              <Input
                id="side-to"
                autoFocus
                value={to}
                onChange={(e) => setTo(e.target.value)}
                placeholder="depo@tedarikci.com"
              />
              <p className="text-muted-foreground text-xs">
                {t("inbox.side.to_hint")}
              </p>
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="side-subject">{t("inbox.side.subject")}</Label>
              <Input
                id="side-subject"
                value={subject}
                maxLength={500}
                onChange={(e) => setSubject(e.target.value)}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="side-body">{t("inbox.side.body")}</Label>
              <Textarea
                id="side-body"
                rows={6}
                value={body}
                onChange={(e) => setBody(e.target.value)}
              />
            </div>
          </form>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="side-start"
            disabled={!valid || inboxes.length === 0 || start.isPending}
          >
            <Send className="size-4" />
            {t("inbox.side.send")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ThreadDialog({
  conversation,
  side,
  canReply,
  onClose,
}: {
  conversation: string;
  side: string;
  canReply: boolean;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const thread = useSideThread(conversation, side);
  const reply = useReplySideConversation(conversation, side);
  const status = useSideStatus(conversation, side);
  const [body, setBody] = useState("");
  const s = thread.data;
  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Mail className="size-4" />
            {s?.subject ?? "…"}
          </DialogTitle>
          <DialogDescription>
            {s
              ? t("inbox.side.thread_hint", {
                  people: s.participants.join(", "),
                  inbox: s.inbox?.name ?? "—",
                })
              : ""}
          </DialogDescription>
        </DialogHeader>
        <ol className="-mx-1 flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-1">
          {(s?.messages ?? []).map((m) => (
            <li
              key={m.uuid}
              className={cn(
                "rounded-lg border p-3 text-sm",
                m.direction === "out" ? "bg-muted/50 ml-8" : "bg-card mr-8",
              )}
            >
              <div className="text-muted-foreground mb-1 flex items-center gap-2 text-xs">
                <span className="text-foreground font-medium">
                  {m.direction === "out"
                    ? (m.sender?.name ?? m.from_name)
                    : m.from_name || m.from_address}
                </span>
                {m.direction === "in" && m.from_name && (
                  <span>&lt;{m.from_address}&gt;</span>
                )}
                <span className="ml-auto">
                  {datetime(m.created_at, "dd.MM.yyyy HH:mm", locale)}
                </span>
              </div>
              <p className="break-words whitespace-pre-wrap">{m.content}</p>
              {m.status === "failed" && (
                <p className="text-destructive mt-1 text-xs">
                  {t("inbox.side.failed")}: {m.error}
                </p>
              )}
              {m.status === "pending" && (
                <p className="text-muted-foreground mt-1 text-xs">
                  {t("inbox.side.pending")}
                </p>
              )}
            </li>
          ))}
        </ol>
        {canReply && s && (
          <form
            className="flex flex-col gap-2 border-t pt-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (!body.trim()) return;
              reply.mutate(body.trim(), { onSuccess: () => setBody("") });
            }}
          >
            <Textarea
              rows={3}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder={t("inbox.side.reply_placeholder")}
              aria-label={t("inbox.side.reply_placeholder")}
            />
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={status.isPending}
                onClick={() =>
                  status.mutate(s.status === "open" ? "closed" : "open")
                }
              >
                {s.status === "open"
                  ? t("inbox.side.close")
                  : t("inbox.side.reopen")}
              </Button>
              <Button
                type="submit"
                size="sm"
                className="ml-auto"
                disabled={!body.trim() || reply.isPending}
              >
                <Send className="size-4" />
                {t("inbox.side.send")}
              </Button>
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
