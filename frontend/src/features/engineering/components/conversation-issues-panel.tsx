"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { Bug, Link2, Plus, TriangleAlert } from "lucide-react";

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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import {
  useConversationEngineering,
  useCreateIssue,
  useEngineeringScope,
  useIssues,
  useLinkIssue,
} from "@/features/engineering/hooks/use-engineering";
import { ISSUE_PRIORITIES } from "@/features/engineering/lib/issue-display";
import type { IssuePriority } from "@/features/engineering/services/engineering.service";
import { useDebounce } from "@/hooks/use-debounce";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * Inbox right pane: escalations of the conversation, suggested product
 * requests (link one), "new request" and "link to a request". Ported from
 * the mobile `EngineeringCard` and `IssueSheet`.
 */
export function ConversationIssuesPanel({
  conversationUuid,
}: {
  conversationUuid: string;
}) {
  const { t, locale } = useLocale();
  const { canWrite } = useEngineeringScope();
  const { enabled, suggestions, escalations } =
    useConversationEngineering(conversationUuid);
  const link = useLinkIssue();
  const [creating, setCreating] = useState(false);
  const [picking, setPicking] = useState(false);
  if (!enabled) return null;

  const linkTo = (issue: string) =>
    link.mutate({
      issue,
      conversation: conversationUuid,
      notifyCustomer: false,
    });
  const suggested = suggestions.data?.items ?? [];
  const escalated = escalations.data?.items ?? [];
  if (!canWrite && suggested.length === 0 && escalated.length === 0) {
    return null;
  }

  return (
    <section>
      <div className="mb-2 flex items-center">
        <h3 className="text-muted-foreground text-[11px] font-semibold tracking-wide uppercase">
          {t("engineering.panel.title")}
        </h3>
        {canWrite && (
          <span className="ml-auto flex items-center">
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              title={t("engineering.panel.link")}
              aria-label={t("engineering.panel.link")}
              onClick={() => setPicking(true)}
            >
              <Link2 className="size-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="size-6"
              title={t("engineering.panel.new")}
              aria-label={t("engineering.panel.new")}
              onClick={() => setCreating(true)}
            >
              <Plus className="size-3.5" />
            </Button>
          </span>
        )}
      </div>

      {escalated.length > 0 && (
        <ul className="mb-2 flex flex-col gap-1">
          {escalated.map((e) => (
            <li
              key={e.uuid}
              className="bg-destructive/10 flex items-start gap-2 rounded-md px-2 py-1.5 text-xs"
            >
              <TriangleAlert className="text-destructive mt-0.5 size-3.5 shrink-0" />
              <span className="min-w-0 break-words">
                {t("engineering.panel.escalation", {
                  tier: e.tier,
                  due: datetime(e.due_at, "dd.MM HH:mm", locale),
                })}
                {e.reason ? ` · ${e.reason}` : ""}
              </span>
            </li>
          ))}
        </ul>
      )}

      {suggested.length > 0 ? (
        <>
          <p className="text-muted-foreground mb-1 text-xs">
            {t("engineering.panel.suggested")}
          </p>
          <ul className="flex flex-col gap-1">
            {suggested.map((issue) => (
              <li key={issue.uuid} className="flex items-center gap-1">
                <Link
                  href={routes.cms.engineering.detail(issue.uuid)}
                  className="hover:bg-muted flex min-w-0 flex-1 flex-col rounded-md px-2 py-1.5"
                >
                  <span className="flex items-center gap-1.5 text-xs font-medium">
                    <Bug className="size-3 shrink-0" />
                    <span className="truncate">{issue.title}</span>
                  </span>
                  <span className="text-muted-foreground truncate text-xs">
                    {t(`engineering.status.${issue.status}`)} ·{" "}
                    {t(`inbox.priority.${issue.priority}`)}
                  </span>
                </Link>
                {canWrite && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6 shrink-0"
                    title={t("engineering.panel.link")}
                    aria-label={`${t("engineering.panel.link")}: ${issue.title}`}
                    disabled={link.isPending}
                    onClick={() => linkTo(issue.uuid)}
                  >
                    <Link2 className="size-3.5" />
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </>
      ) : (
        escalated.length === 0 && (
          <p className="text-muted-foreground text-xs">
            {t("engineering.panel.empty")}
          </p>
        )
      )}

      {creating && (
        <CreateIssueDialog
          conversationUuid={conversationUuid}
          onClose={() => setCreating(false)}
        />
      )}
      {picking && (
        <LinkIssueDialog
          pending={link.isPending}
          onClose={() => setPicking(false)}
          onPick={(issue) => {
            setPicking(false);
            linkTo(issue);
          }}
        />
      )}
    </section>
  );
}

/** New request linked to this conversation; "AI summary" lets the API draft it. */
function CreateIssueDialog({
  conversationUuid,
  onClose,
}: {
  conversationUuid: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { canSummarize } = useEngineeringScope();
  const create = useCreateIssue();
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [ai, setAi] = useState(canSummarize);
  const [priority, setPriority] = useState<IssuePriority>("normal");

  const submit = () =>
    create.mutate(
      {
        conversation_uuid: conversationUuid,
        ai_summary: ai,
        priority,
        ...(title.trim() ? { title: title.trim() } : {}),
        ...(description.trim() ? { description: description.trim() } : {}),
      },
      {
        onSuccess: (issue) => {
          onClose();
          router.push(routes.cms.engineering.detail(issue.uuid));
        },
      },
    );

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("engineering.panel.new")}</DialogTitle>
          <DialogDescription>
            {t("engineering.create.description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <div className="flex items-start justify-between gap-3">
            <div className="flex flex-col gap-0.5">
              <Label htmlFor="issue-ai">{t("engineering.create.ai")}</Label>
              <p className="text-muted-foreground text-xs">
                {canSummarize
                  ? t("engineering.create.ai_hint")
                  : t("engineering.create.ai_forbidden")}
              </p>
            </div>
            <Switch
              id="issue-ai"
              checked={ai}
              disabled={!canSummarize}
              onCheckedChange={setAi}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="issue-title">{t("engineering.fields.title")}</Label>
            <Input
              id="issue-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder={
                ai ? t("engineering.create.ai_placeholder") : undefined
              }
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="issue-description">
              {t("engineering.fields.description")}
            </Label>
            <Textarea
              id="issue-description"
              rows={4}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="issue-new-priority">
              {t("engineering.fields.priority")}
            </Label>
            <Select
              value={priority}
              onValueChange={(value) => setPriority(value as IssuePriority)}
            >
              <SelectTrigger id="issue-new-priority" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {ISSUE_PRIORITIES.map((value) => (
                  <SelectItem key={value} value={value}>
                    {t(`inbox.priority.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={create.isPending || (!ai && !title.trim())}
            >
              {t("engineering.create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Searchable picker of existing requests. */
function LinkIssueDialog({
  pending,
  onClose,
  onPick,
}: {
  pending: boolean;
  onClose: () => void;
  onPick: (issue: string) => void;
}) {
  const { t } = useLocale();
  const [text, setText] = useState("");
  const q = useDebounce(text, 300).trim();
  const issues = useIssues({ limit: 20, offset: 0, q: q || undefined });
  const items = issues.data?.items ?? [];

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("engineering.panel.link")}</DialogTitle>
          <DialogDescription>
            {t("engineering.link.description")}
          </DialogDescription>
        </DialogHeader>
        <Input
          autoFocus
          value={text}
          onChange={(event) => setText(event.target.value)}
          placeholder={t("engineering.link.search")}
          aria-label={t("engineering.link.search")}
        />
        <div className="max-h-72 overflow-y-auto" aria-busy={issues.isFetching}>
          {issues.isLoading ? (
            <p className="text-muted-foreground px-2 py-6 text-center text-sm">
              {t("common.loading")}
            </p>
          ) : issues.isError ? (
            <p className="text-destructive px-2 py-6 text-center text-sm">
              {t("engineering.load_failed")}
            </p>
          ) : items.length === 0 ? (
            <p className="text-muted-foreground px-2 py-6 text-center text-sm">
              {t("engineering.link.empty")}
            </p>
          ) : (
            <ul className="flex flex-col gap-0.5">
              {items.map((issue) => (
                <li key={issue.uuid}>
                  <button
                    type="button"
                    disabled={pending}
                    onClick={() => onPick(issue.uuid)}
                    className="hover:bg-muted flex w-full flex-col rounded-md px-2 py-1.5 text-left disabled:opacity-50"
                  >
                    <span className="truncate text-sm font-medium">
                      {issue.title}
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {t(`engineering.status.${issue.status}`)} ·{" "}
                      {t(`inbox.priority.${issue.priority}`)}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
