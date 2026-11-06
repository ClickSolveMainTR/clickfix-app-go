"use client";

import Link from "next/link";
import { useState } from "react";
import { Inbox } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useAddIssueComment,
  useEngineeringScope,
  useIssue,
  useIssueComments,
  useIssueConversations,
  useUpdateIssue,
} from "@/features/engineering/hooks/use-engineering";
import {
  humanLabel,
  ISSUE_PRIORITIES,
  ISSUE_STATUSES,
  issuePriorityTone,
  issueStatusTone,
} from "@/features/engineering/lib/issue-display";
import type {
  IssuePriority,
  IssueStatus,
} from "@/features/engineering/services/engineering.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/**
 * One product request: status and priority (changeable with write access),
 * description, linked conversations and comments. Ported from the mobile
 * `IssueScreen`.
 */
export function IssueDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { canWrite } = useEngineeringScope();
  const query = useIssue(uuid);
  const links = useIssueConversations(uuid);
  const comments = useIssueComments(uuid);
  const update = useUpdateIssue(uuid);
  const addComment = useAddIssueComment(uuid);
  const [comment, setComment] = useState("");
  const issue = query.data;
  const title = issue?.title ?? t("engineering.detail_title");
  const linked = links.data?.items ?? [];
  const commentItems = comments.data?.items ?? [];

  return (
    <EntityPage
      title={title}
      permission={permissions.tenant.tickets.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("engineering.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("engineering.title"), href: routes.cms.engineering.root },
        { label: title },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("engineering.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {issue ? (
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
          <div className="flex min-w-0 flex-col gap-4">
            <EntitySectionCard title={t("engineering.fields.description")}>
              {issue.description ? (
                <MarkdownContent markdown={issue.description} />
              ) : (
                <p className="text-muted-foreground text-sm">
                  {t("engineering.no_description")}
                </p>
              )}
            </EntitySectionCard>

            <EntitySectionCard
              title={t("engineering.comments.title")}
              badge={commentItems.length || undefined}
            >
              <div className="flex flex-col gap-4">
                {commentItems.length === 0 ? (
                  <p className="text-muted-foreground text-sm">
                    {t("engineering.comments.empty")}
                  </p>
                ) : (
                  <ul className="flex flex-col divide-y">
                    {commentItems.map((c) => (
                      <li key={c.uuid} className="py-2 first:pt-0 last:pb-0">
                        <p className="text-sm break-words whitespace-pre-wrap">
                          {c.body}
                        </p>
                        <p className="text-muted-foreground mt-0.5 text-xs">
                          {[
                            c.source,
                            datetime(c.created_at, "dd.MM.yyyy HH:mm", locale),
                          ]
                            .filter(Boolean)
                            .join(" · ")}
                        </p>
                      </li>
                    ))}
                  </ul>
                )}
                {canWrite ? (
                  <form
                    className="flex flex-col gap-2"
                    onSubmit={(event) => {
                      event.preventDefault();
                      const body = comment.trim();
                      if (!body) return;
                      addComment.mutate(body, {
                        onSuccess: () => setComment(""),
                      });
                    }}
                  >
                    <Label htmlFor="issue-comment" className="sr-only">
                      {t("engineering.comments.add")}
                    </Label>
                    <Textarea
                      id="issue-comment"
                      rows={3}
                      value={comment}
                      onChange={(event) => setComment(event.target.value)}
                      placeholder={t("engineering.comments.add")}
                    />
                    <Button
                      type="submit"
                      size="sm"
                      className="self-end"
                      disabled={!comment.trim() || addComment.isPending}
                    >
                      {t("engineering.comments.send")}
                    </Button>
                  </form>
                ) : null}
              </div>
            </EntitySectionCard>
          </div>

          <div className="flex min-w-0 flex-col gap-4">
            <EntitySectionCard title={t("engineering.detail_title")}>
              <div className="flex flex-col gap-4">
                {canWrite ? (
                  <>
                    <div className="flex flex-col gap-1.5">
                      <Label htmlFor="issue-status">
                        {t("engineering.fields.status")}
                      </Label>
                      <Select
                        value={issue.status}
                        disabled={update.isPending}
                        onValueChange={(status) =>
                          update.mutate({
                            status: status as IssueStatus,
                            revision: issue.revision,
                          })
                        }
                      >
                        <SelectTrigger id="issue-status" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ISSUE_STATUSES.map((status) => (
                            <SelectItem key={status} value={status}>
                              {t(`engineering.status.${status}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="flex flex-col gap-1.5">
                      <Label htmlFor="issue-priority">
                        {t("engineering.fields.priority")}
                      </Label>
                      <Select
                        value={issue.priority}
                        disabled={update.isPending}
                        onValueChange={(priority) =>
                          update.mutate({
                            priority: priority as IssuePriority,
                            revision: issue.revision,
                          })
                        }
                      >
                        <SelectTrigger id="issue-priority" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ISSUE_PRIORITIES.map((priority) => (
                            <SelectItem key={priority} value={priority}>
                              {t(`inbox.priority.${priority}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </>
                ) : (
                  <div className="flex flex-wrap gap-2">
                    <StatusChip
                      label={t(`engineering.status.${issue.status}`)}
                      tone={issueStatusTone(issue.status)}
                    />
                    <StatusChip
                      label={t(`inbox.priority.${issue.priority}`)}
                      tone={issuePriorityTone(issue.priority)}
                    />
                  </div>
                )}
                {issue.labels.length ? (
                  <div className="flex flex-wrap gap-1">
                    {issue.labels.map((label) => (
                      <Badge
                        key={label}
                        variant="secondary"
                        className="font-normal"
                      >
                        {humanLabel(label, locale)}
                      </Badge>
                    ))}
                  </div>
                ) : null}
                <p className="text-muted-foreground text-xs">
                  {t("engineering.updated", {
                    at: datetime(issue.updated_at, "dd.MM.yyyy HH:mm", locale),
                  })}
                </p>
              </div>
            </EntitySectionCard>

            {linked.length ? (
              <EntitySectionCard
                title={t("engineering.conversations.title")}
                badge={linked.length}
              >
                <ul className="flex flex-col gap-1">
                  {linked.map((l) => (
                    <li key={l.conversation_uuid}>
                      <Link
                        href={`${routes.cms.inbox}?c=${l.conversation_uuid}`}
                        className="hover:bg-muted flex items-start gap-2 rounded-md px-2 py-1.5"
                      >
                        <Inbox className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                        <span className="min-w-0">
                          <span className="block truncate text-sm font-medium">
                            {l.subject || l.conversation_uuid.slice(0, 8)}
                          </span>
                          <span className="text-muted-foreground block text-xs">
                            {[
                              t(`channels.types.${l.channel}`),
                              l.notify_customer
                                ? t("engineering.conversations.notifies")
                                : null,
                            ]
                              .filter(Boolean)
                              .join(" · ")}
                          </span>
                        </span>
                      </Link>
                    </li>
                  ))}
                </ul>
              </EntitySectionCard>
            ) : null}
          </div>
        </div>
      ) : null}
    </EntityPage>
  );
}
