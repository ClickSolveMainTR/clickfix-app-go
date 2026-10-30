"use client";

import Link from "next/link";
import { Hash, Mail, Phone } from "lucide-react";

import { ConversationToTopic } from "@/features/forum/components/conversation-to-topic";
import { ConversationFormPanel } from "@/features/request-forms/components/conversation-form-panel";
import { SidebarAppsPanel } from "@/features/sidebar-apps/components/sidebar-apps-panel";
import { SlaPanel } from "@/features/sla/components/sla-panel";
import { datetime } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useContactHistory } from "../hooks/use-inbox";
import {
  contactLabel,
  initials,
  isWaitingStatus,
  shortAgo,
} from "../lib/format";
import type { ConversationDetail } from "../types";
import { ChannelGlyph } from "./channel-glyph";
import { LinkedAppointmentsCard } from "@/features/calendar";

import { ConversationLinksPanel } from "./conversation-links";
import { DiagnosticsPanel } from "./diagnostics-panel";
import { SessionReplayPanel } from "./session-replay-panel";
import { SideConversationsPanel } from "./side-conversations";

/** Right pane: who the contact is and what happened before. */
export function ContextPanel({
  conversation: c,
  onOpen,
}: {
  conversation: ConversationDetail;
  onOpen: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const history = useContactHistory(c.contact.uuid);
  const others = (history.data?.items ?? []).filter((x) => x.uuid !== c.uuid);
  const name = contactLabel(c);
  return (
    <aside
      className="flex h-full flex-col gap-5 overflow-y-auto p-4"
      aria-label={t("inbox.context.label")}
    >
      <section className="flex items-center gap-3">
        <div className="bg-ink text-paper flex size-11 items-center justify-center rounded-full text-sm font-semibold">
          {initials(name)}
        </div>
        <div className="min-w-0">
          <Link
            href={`/contacts?c=${c.contact.uuid}`}
            className="block truncate font-semibold hover:underline"
          >
            {name}
          </Link>
          {c.company && (
            <p className="text-muted-foreground truncate text-xs">
              {c.company.name}
            </p>
          )}
        </div>
      </section>

      <section className="flex flex-col gap-1.5 text-sm">
        {c.contact.email && (
          <a
            href={`mailto:${c.contact.email}`}
            className="hover:text-foreground text-muted-foreground flex items-center gap-2 truncate"
          >
            <Mail className="size-3.5 shrink-0" />
            <span className="truncate">{c.contact.email}</span>
          </a>
        )}
        {c.contact.phone && (
          <span className="text-muted-foreground flex items-center gap-2">
            <Phone className="size-3.5 shrink-0" />
            {c.contact.phone}
          </span>
        )}
        <span className="text-muted-foreground flex items-center gap-2">
          <ChannelGlyph type={c.channel_type} className="size-3.5 shrink-0" />
          {t(`channels.types.${c.channel_type}`)} · {c.inbox?.name ?? "—"}
        </span>
      </section>

      <section>
        <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
          {t("inbox.context.details")}
        </h3>
        <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs">
          <Row label={t("inbox.context.status")}>
            {t(`inbox.status.${c.status}`)}
          </Row>
          <Row label={t("inbox.context.created")}>
            {datetime(c.created_at, "dd.MM.yyyy HH:mm")}
          </Row>
          <Row label={t("inbox.context.first_response")}>
            {c.first_response_at
              ? datetime(c.first_response_at, "dd.MM HH:mm")
              : "—"}
          </Row>
          {c.waiting_since && isWaitingStatus(c.status) && (
            <Row label={t("inbox.context.waiting")}>
              <span className="text-amber-600 dark:text-amber-400">
                {shortAgo(c.waiting_since, locale)}
              </span>
            </Row>
          )}
          <Row label={t("inbox.context.messages")}>{c.message_count}</Row>
        </dl>
      </section>

      <SidebarAppsPanel
        placement="conversation_sidebar"
        conversationUuid={c.uuid}
        contactUuid={c.contact.uuid}
      />
      <DiagnosticsPanel conversationUuid={c.uuid} />
      <SessionReplayPanel
        conversationUuid={c.uuid}
        channelType={c.channel_type}
      />
      <SlaPanel conversation={c} />
      <ConversationLinksPanel conversation={c} onOpen={onOpen} />
      <SideConversationsPanel conversation={c} />
      <LinkedAppointmentsCard
        compact
        context={{
          conversationUuid: c.uuid,
          contactUuid: c.contact.uuid,
          label: name,
          inviteeName: name,
          inviteeEmail: c.contact.email ?? undefined,
        }}
      />
      <ConversationFormPanel conversationUuid={c.uuid} />
      <ConversationToTopic conversationUuid={c.uuid} subject={c.subject} />

      {c.tags.length > 0 && (
        <section>
          <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
            {t("inbox.context.tags")}
          </h3>
          <div className="flex flex-wrap gap-1.5">
            {c.tags.map((tag) => (
              <span
                key={tag.uuid}
                className="bg-muted flex items-center gap-1 rounded-md px-2 py-0.5 text-xs"
              >
                <Hash className="text-muted-foreground size-3" />
                {tag.name}
              </span>
            ))}
          </div>
        </section>
      )}

      {c.participants.length > 0 && (
        <section>
          <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
            {t("inbox.context.participants")}
          </h3>
          <ul className="flex flex-col gap-1.5">
            {c.participants.map((p) => (
              <li key={p.uuid} className="flex items-center gap-2 text-xs">
                <span className="bg-muted flex size-6 items-center justify-center rounded-full text-[9px] font-semibold">
                  {initials(p.name)}
                </span>
                <span className="truncate">{p.name}</span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section>
        <h3 className="text-muted-foreground mb-2 text-[11px] font-semibold tracking-wide uppercase">
          {t("inbox.context.history")}
        </h3>
        {others.length === 0 ? (
          <p className="text-muted-foreground text-xs">
            {t("inbox.context.no_history")}
          </p>
        ) : (
          <ul className="flex flex-col gap-1">
            {others.map((o) => (
              <li key={o.uuid}>
                <button
                  type="button"
                  onClick={() => onOpen(o.uuid)}
                  className="hover:bg-muted flex w-full flex-col rounded-md px-2 py-1.5 text-left"
                >
                  <span className="flex items-center gap-1.5 text-xs font-medium">
                    <ChannelGlyph type={o.channel_type} className="size-3" />#
                    {o.number}
                    <span className="text-muted-foreground ml-auto font-normal">
                      {t(`inbox.status.${o.status}`)}
                    </span>
                  </span>
                  <span className="text-muted-foreground truncate text-xs">
                    {o.last_message_preview || o.subject}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </aside>
  );
}

function Row({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="truncate">{children}</dd>
    </>
  );
}
