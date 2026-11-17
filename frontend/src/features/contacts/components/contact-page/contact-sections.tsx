"use client";

import Link from "next/link";
import {
  Building2,
  Download,
  ExternalLink,
  Lightbulb,
  Link2,
  MessageSquare,
  MessagesSquare,
  ShieldCheck,
  ShieldX,
  ThumbsUp,
  UserRound,
} from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { formatMinor } from "@/features/billing/lib/money";
import { LinkedAppointmentsCard } from "@/features/calendar";
import { useCreateDataSubjectRequest } from "@/features/compliance/compliance";
import {
  CustomFieldValue,
  useCustomFieldDefinitions,
} from "@/features/custom-fields";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { shortAgo } from "@/features/inbox/lib/format";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type {
  ContactOverview,
  ContactProfileConversation,
} from "../../services/profile.service";
import { CompanyHealthBadge } from "../company-health-badge";
import { CompanyHealthCard } from "../company-health-card";
import { CompanyRevenueCard } from "../company-revenue-card";
import { CompanyVerifiedBadge } from "../company-verified-badge";
import { dateOf, dateTimeOf, isClosed, relative } from "./lib";

function Card({
  title,
  count,
  action,
  children,
  className,
}: {
  title: string;
  count?: number;
  action?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("bg-card rounded-2xl border p-4", className)}>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="font-medium">{title}</h2>
        {count != null && (
          <span className="text-muted-foreground text-xs tabular-nums">
            {count}
          </span>
        )}
        {action && <div className="ml-auto">{action}</div>}
      </div>
      {children}
    </section>
  );
}

/** Open conversations first, then recently closed ones. */
export function ConversationsSection({ data }: { data: ContactOverview }) {
  const { t } = useLocale();
  const open = data.conversations.filter((c) => !isClosed(c.status));
  const closed = data.conversations.filter((c) => isClosed(c.status));
  if (data.conversations.length === 0) {
    return (
      <EmptyState
        icon={<MessagesSquare className="size-6" />}
        title={t("contacts.account.no_conversations")}
        description={t("contacts.profile.conversations_empty_hint")}
      />
    );
  }
  return (
    <div className="grid gap-4">
      <Card
        title={t("contacts.profile.open_conversations")}
        count={open.length}
      >
        {open.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("contacts.profile.no_open")}
          </p>
        ) : (
          <ConversationList rows={open} />
        )}
      </Card>
      {closed.length > 0 && (
        <Card
          title={t("contacts.profile.closed_conversations")}
          count={data.stats.closed_conversations}
        >
          <ConversationList rows={closed} />
        </Card>
      )}
    </div>
  );
}

function ConversationList({ rows }: { rows: ContactProfileConversation[] }) {
  const { t, locale } = useLocale();
  return (
    <ul className="-mx-2 grid gap-0.5">
      {rows.map((c) => {
        const closed = isClosed(c.status);
        return (
          <li key={c.uuid}>
            <Link
              href={`${routes.cms.inbox}?c=${c.uuid}`}
              className="hover:bg-muted focus-visible:ring-ring flex min-w-0 gap-3 rounded-lg px-2 py-2.5 outline-none focus-visible:ring-2"
            >
              <ChannelGlyph
                type={c.channel_type}
                className="mt-0.5 size-4 shrink-0"
                title={t(`channels.types.${c.channel_type}`)}
              />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <div className="flex min-w-0 items-center gap-2">
                  <p className="min-w-0 flex-1 truncate text-sm font-medium">
                    <span className="text-muted-foreground tabular-nums">
                      #{c.number}
                    </span>{" "}
                    {c.subject || c.last_message_preview || "—"}
                  </p>
                  <span className="text-muted-foreground shrink-0 text-xs tabular-nums">
                    {shortAgo(c.last_message_at, locale)}
                  </span>
                </div>
                {c.subject && c.last_message_preview && (
                  <p className="text-muted-foreground truncate text-xs">
                    {c.last_message_preview}
                  </p>
                )}
                <div className="flex flex-wrap items-center gap-1.5 text-xs">
                  <Badge
                    variant={closed ? "secondary" : "default"}
                    className="h-5 px-1.5 text-[10px]"
                  >
                    {t(`inbox.status.${c.status}`)}
                  </Badge>
                  {c.priority && c.priority !== "normal" && !closed && (
                    <Badge
                      variant={c.priority === "urgent" ? "danger" : "warning"}
                      className="h-5 px-1.5 text-[10px]"
                    >
                      {t(`inbox.priority.${c.priority}`)}
                    </Badge>
                  )}
                  {!closed && c.sla_breached_at && (
                    <Badge variant="danger" className="h-5 px-1.5 text-[10px]">
                      {t("contacts.profile.sla_breached")}
                    </Badge>
                  )}
                  {!closed && !c.sla_breached_at && c.sla_due_at && (
                    <Badge
                      variant="outline"
                      className="h-5 px-1.5 text-[10px]"
                      title={dateTimeOf(c.sla_due_at, locale)}
                    >
                      {t("contacts.profile.sla_due", {
                        when: relative(c.sla_due_at, locale),
                      })}
                    </Badge>
                  )}
                  {(c.assignee || c.team) && (
                    <span className="text-muted-foreground inline-flex items-center gap-1">
                      <UserRound className="size-3" />
                      {[c.assignee, c.team].filter(Boolean).join(" · ")}
                    </span>
                  )}
                  {c.tags.slice(0, 4).map((tag) => (
                    <span
                      key={tag}
                      className="bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[10px]"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              </div>
            </Link>
          </li>
        );
      })}
    </ul>
  );
}

/** The contact's company: account card, health and revenue. */
export function CompanySection({
  data,
  onEdit,
  canWrite,
}: {
  data: ContactOverview;
  onEdit: () => void;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const co = data.company;
  if (!co) {
    return (
      <EmptyState
        icon={<Building2 className="size-6" />}
        title={t("contacts.profile.no_company")}
        description={t("contacts.profile.no_company_hint")}
        action={
          canWrite ? (
            <Button size="sm" variant="outline" onClick={onEdit}>
              <Link2 className="size-4" />
              {t("contacts.profile.link_company")}
            </Button>
          ) : undefined
        }
      />
    );
  }
  const revenue = co.revenue;
  return (
    <div className="grid gap-4">
      <section className="bg-card rounded-2xl border p-4">
        <div className="flex flex-wrap items-start gap-3">
          <span className="bg-muted flex size-11 shrink-0 items-center justify-center rounded-xl">
            <Building2 className="size-5" />
          </span>
          <div className="min-w-0 flex-1">
            <Link
              href={routes.cms.companies.detail(co.uuid)}
              className="hover:text-signal-text inline-flex items-center gap-1.5 font-semibold underline-offset-4 hover:underline"
            >
              {co.name}
              <ExternalLink className="size-3.5" />
            </Link>
            <p className="text-muted-foreground truncate text-sm">
              {[co.domain, co.tier].filter(Boolean).join(" · ") || "—"}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <CompanyVerifiedBadge verifiedAt={co.verified_at} />
            <CompanyHealthBadge health={co.health} />
          </div>
        </div>
        <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          <Fact
            label={t("contacts.account.owner")}
            value={co.account_owner?.name ?? t("contacts.account.no_owner")}
          />
          <Fact
            label={t("contacts.companies.contacts")}
            value={String(co.contact_count)}
          />
          <Fact
            label={t("contacts.account.open")}
            value={String(co.open_conversations)}
          />
          <Fact
            label={t("contacts.account.mrr")}
            value={
              revenue && revenue.mrr_minor > 0
                ? formatMinor(revenue.mrr_minor, revenue.currency, locale)
                : "—"
            }
          />
        </dl>
      </section>
      <div className="grid gap-4 md:grid-cols-2">
        <CompanyHealthCard companyUuid={co.uuid} />
        <CompanyRevenueCard companyUuid={co.uuid} />
      </div>
    </div>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted-foreground truncate text-xs">{label}</dt>
      <dd className="truncate font-medium">{value}</dd>
    </div>
  );
}

/** Custom fields and profile facts of the contact. */
export function FieldsSection({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  const defs = useCustomFieldDefinitions({ entityType: "contact" });
  const active = (defs.data ?? []).filter((d) => !d.archived);
  const values = (data.contact.custom_fields ?? {}) as Record<string, unknown>;
  const c = data.contact;
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card title={t("contacts.custom_fields.contact")} count={active.length}>
        {defs.isLoading ? (
          <div className="grid gap-2">
            {[0, 1, 2].map((i) => (
              <div key={i} className="bg-muted h-5 animate-pulse rounded" />
            ))}
          </div>
        ) : active.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("contacts.profile.fields_empty")}
          </p>
        ) : (
          <dl className="grid gap-2 text-sm">
            {active.map((d) => (
              <div key={d.uuid} className="grid grid-cols-2 gap-2">
                <dt className="text-muted-foreground truncate">{d.label}</dt>
                <dd className="min-w-0">
                  <CustomFieldValue definition={d} value={values[d.key]} />
                </dd>
              </div>
            ))}
          </dl>
        )}
      </Card>
      <Card title={t("contacts.profile.details")}>
        <dl className="grid gap-2 text-sm">
          {[
            [t("contacts.fields.external_id"), c.external_id ?? "—"],
            [
              t("contacts.profile.verified_at"),
              c.verified_at ? dateTimeOf(c.verified_at, locale) : "—",
            ],
            [t("contacts.profile.locale"), c.locale ?? "—"],
            [t("contacts.profile.timezone"), c.timezone ?? "—"],
            [
              t("contacts.columns.last_seen"),
              c.last_seen_at ? dateTimeOf(c.last_seen_at, locale) : "—",
            ],
            [
              t("contacts.profile.created_at"),
              dateTimeOf(c.created_at, locale),
            ],
            [
              t("contacts.profile.updated_at"),
              dateTimeOf(c.updated_at, locale),
            ],
          ].map(([k, v]) => (
            <div key={k} className="grid grid-cols-2 gap-2">
              <dt className="text-muted-foreground truncate">{k}</dt>
              <dd className="min-w-0 truncate">{v}</dd>
            </div>
          ))}
        </dl>
        {c.notes && (
          <div className="mt-3 border-t pt-3">
            <h3 className="text-muted-foreground mb-1 text-xs">
              {t("contacts.fields.notes")}
            </h3>
            <p className="text-sm whitespace-pre-wrap">{c.notes}</p>
          </div>
        )}
      </Card>
    </div>
  );
}

/** Appointments of the contact, with booking (calendar module). */
export function AppointmentsSection({ data }: { data: ContactOverview }) {
  const { t } = useLocale();
  const name = data.contact.name || data.contact.email || "";
  return (
    <div className="grid gap-3">
      <LinkedAppointmentsCard
        context={{
          contactUuid: data.contact.uuid,
          label: name,
          inviteeName: data.contact.name,
          inviteeEmail: data.contact.email ?? undefined,
        }}
      />
      {data.appointments.length === 0 && (
        <p className="text-muted-foreground px-1 text-xs">
          {t("contacts.profile.appointments_hint")}
        </p>
      )}
    </div>
  );
}

/** Roadmap votes / comments and community forum posts. */
export function FeedbackSection({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <Card title={t("contacts.profile.roadmap")} count={data.feedback.length}>
        {data.feedback.length === 0 ? (
          <EmptyState
            className="py-6"
            icon={<Lightbulb className="size-5" />}
            title={t("contacts.profile.roadmap_empty")}
          />
        ) : (
          <ul className="grid gap-2">
            {data.feedback.map((f, i) => (
              <li
                key={`${f.request_uuid}-${f.kind}-${i}`}
                className="flex gap-2.5 text-sm"
              >
                {f.kind === "vote" ? (
                  <ThumbsUp className="text-signal-text mt-0.5 size-4 shrink-0" />
                ) : (
                  <MessageSquare className="text-muted-foreground mt-0.5 size-4 shrink-0" />
                )}
                <div className="min-w-0 flex-1">
                  <p className="font-medium">{f.title}</p>
                  {f.body && (
                    <p className="text-muted-foreground line-clamp-2">
                      {f.body}
                    </p>
                  )}
                  <p className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
                    <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
                      {t(`contacts.profile.feature_status.${f.status}`)}
                    </Badge>
                    {dateOf(f.created_at, locale)}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Card>
      <Card
        title={t("contacts.profile.forum")}
        count={data.forum_posts.length}
        action={
          <Button size="sm" variant="ghost" asChild>
            <Link href={routes.cms.forum}>{t("contacts.profile.open")}</Link>
          </Button>
        }
      >
        {data.forum_posts.length === 0 ? (
          <EmptyState
            className="py-6"
            icon={<MessagesSquare className="size-5" />}
            title={t("contacts.profile.forum_empty")}
          />
        ) : (
          <ul className="grid gap-2">
            {data.forum_posts.map((p) => (
              <li key={p.uuid} className="text-sm">
                <p className="font-medium">
                  {p.is_topic
                    ? p.title
                    : t("contacts.profile.reply_to", { title: p.title })}
                </p>
                <p className="text-muted-foreground line-clamp-2">
                  {p.excerpt}
                </p>
                <p className="text-muted-foreground mt-0.5 flex items-center gap-2 text-xs">
                  {p.status !== "published" && (
                    <Badge variant="warning" className="h-5 px-1.5 text-[10px]">
                      {t(`contacts.profile.forum_status.${p.status}`)}
                    </Badge>
                  )}
                  {dateOf(p.created_at, locale)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

/** KVKK: latest consent per purpose and data subject requests. */
export function ComplianceSection({ data }: { data: ContactOverview }) {
  const { t, locale } = useLocale();
  const dsr = useCreateDataSubjectRequest();
  const privacy = `${routes.cms.settings.privacy}?tab=consents&contact=${data.contact.uuid}`;
  return (
    <div className="grid gap-4">
      <Card
        title={t("contacts.profile.consents")}
        count={data.consents.length}
        action={
          <Button size="sm" variant="ghost" asChild>
            <Link href={privacy}>{t("contacts.profile.manage")}</Link>
          </Button>
        }
      >
        {data.consents.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("contacts.profile.consents_empty")}
          </p>
        ) : (
          <div className="-mx-4 overflow-x-auto">
            <table className="w-full min-w-[480px] text-sm">
              <thead className="text-muted-foreground text-left text-xs">
                <tr>
                  <th scope="col" className="px-4 pb-2 font-normal">
                    {t("contacts.profile.consent.purpose")}
                  </th>
                  <th scope="col" className="px-2 pb-2 font-normal">
                    {t("contacts.profile.consent.state")}
                  </th>
                  <th scope="col" className="px-2 pb-2 font-normal">
                    {t("contacts.profile.consent.version")}
                  </th>
                  <th scope="col" className="px-2 pb-2 font-normal">
                    {t("contacts.profile.consent.source")}
                  </th>
                  <th scope="col" className="px-4 pb-2 text-right font-normal">
                    {t("contacts.profile.consent.date")}
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.consents.map((c) => (
                  <tr key={c.uuid} className="border-t">
                    <td className="px-4 py-2 font-medium">{c.purpose}</td>
                    <td className="px-2 py-2">
                      <Badge variant={c.granted ? "success" : "danger"}>
                        {c.granted ? (
                          <ShieldCheck className="size-3" />
                        ) : (
                          <ShieldX className="size-3" />
                        )}
                        {t(
                          c.granted
                            ? "contacts.profile.consent.granted"
                            : "contacts.profile.consent.revoked",
                        )}
                      </Badge>
                    </td>
                    <td className="text-muted-foreground px-2 py-2">
                      {c.notice_version}
                    </td>
                    <td className="text-muted-foreground px-2 py-2">
                      {c.source}
                    </td>
                    <td className="text-muted-foreground px-4 py-2 text-right whitespace-nowrap">
                      {dateOf(c.created_at, locale)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <Card
        title={t("contacts.profile.requests")}
        count={data.privacy_requests.length}
        action={
          <Button
            size="sm"
            variant="outline"
            disabled={dsr.isPending}
            onClick={() =>
              dsr.mutate({ contact: data.contact.uuid, kind: "export" })
            }
          >
            <Download className="size-4" />
            {t("contacts.profile.action.export")}
          </Button>
        }
      >
        {data.privacy_requests.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("contacts.profile.requests_empty")}
          </p>
        ) : (
          <ul className="grid gap-2">
            {data.privacy_requests.map((r) => (
              <li
                key={r.uuid}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                <Badge variant={r.kind === "erase" ? "danger" : "secondary"}>
                  {t(`contacts.profile.request_kind.${r.kind}`)}
                </Badge>
                <Badge variant="outline">
                  {t(`contacts.profile.request_status.${r.status}`)}
                </Badge>
                <span className="text-muted-foreground min-w-0 flex-1 truncate text-xs">
                  {[r.requested_by, dateTimeOf(r.created_at, locale)]
                    .filter(Boolean)
                    .join(" · ")}
                </span>
                <Link
                  href={`${routes.cms.settings.privacy}?tab=requests`}
                  className="text-muted-foreground hover:text-foreground text-xs underline-offset-2 hover:underline"
                >
                  {t("contacts.profile.open")}
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
