"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import {
  Building2,
  Check,
  Copy,
  Download,
  Globe,
  Hash,
  Mail,
  MessageSquarePlus,
  MoreHorizontal,
  Pencil,
  Phone,
  ShieldCheck,
  StickyNote,
  Trash2,
  UserCog,
  UserX,
  GitMerge,
  Ban,
} from "lucide-react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ErasureDialog } from "@/features/compliance/privacy-page";
import { useCreateDataSubjectRequest } from "@/features/compliance/compliance";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { initials } from "@/features/inbox/lib/format";
import { CallButton } from "@/features/softphone/softphone";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useContactMutations,
  useContactsScope,
} from "../../hooks/use-contacts";
import type { ContactOverview } from "../../services/profile.service";
import { CompanyHealthBadge } from "../company-health-badge";
import { CompanyVerifiedBadge } from "../company-verified-badge";
import { MergeDialog } from "../contacts-page";
import { IdentityBadge } from "../identity-badge";
import {
  EditContactSheet,
  NewConversationDialog,
  OwnerDialog,
} from "./contact-dialogs";

type Dialog =
  "edit" | "merge" | "owner" | "conversation" | "erase" | "delete" | null;

/** Identity card and quick actions of the contact page (CF-221). */
export function ContactHeader({
  data,
  onAddNote,
}: {
  data: ContactOverview;
  onAddNote: () => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const scope = useContactsScope();
  const { can } = usePermission();
  const m = useContactMutations();
  const dsr = useCreateDataSubjectRequest();
  const [dialog, setDialog] = useState<Dialog>(null);
  const { contact, company, tags, sections } = data;
  const name = contact.name || contact.email || contact.phone || "—";
  const canStart =
    sections.conversations &&
    can(permissions.tenant.conversations.manage) &&
    can(permissions.tenant.channels.read);
  const owner = company?.account_owner?.name;

  return (
    <header className="bg-card relative overflow-hidden rounded-2xl border">
      <div
        aria-hidden
        className="from-signal-soft/70 pointer-events-none absolute inset-x-0 top-0 h-24 bg-gradient-to-b to-transparent"
      />
      <div className="relative flex flex-col gap-4 p-4 sm:p-5 lg:flex-row lg:items-start">
        <div className="flex min-w-0 flex-1 gap-4">
          <div
            aria-hidden
            className="bg-ink text-paper ring-background flex size-14 shrink-0 items-center justify-center rounded-2xl text-lg font-semibold ring-4 sm:size-16 sm:text-xl"
          >
            {initials(name)}
          </div>
          <div className="flex min-w-0 flex-1 flex-col gap-2">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <h1 className="font-display truncate text-xl font-semibold sm:text-2xl">
                {name}
              </h1>
              <IdentityBadge state={contact.identity_state} />
              {contact.blocked && (
                <Badge variant="danger" className="gap-1">
                  <Ban className="size-3" />
                  {t("contacts.profile.blocked")}
                </Badge>
              )}
            </div>
            <div className="text-muted-foreground flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm">
              {contact.job_title && <span>{contact.job_title}</span>}
              {company ? (
                <Link
                  href={routes.cms.companies.detail(company.uuid)}
                  className="text-foreground hover:text-signal-text inline-flex min-w-0 items-center gap-1.5 font-medium underline-offset-4 hover:underline"
                >
                  <Building2 className="size-4 shrink-0" />
                  <span className="truncate">{company.name}</span>
                </Link>
              ) : (
                <span className="inline-flex items-center gap-1.5">
                  <Building2 className="size-4" />
                  {t("contacts.no_company")}
                </span>
              )}
              {company && (
                <>
                  <CompanyVerifiedBadge verifiedAt={company.verified_at} />
                  <CompanyHealthBadge health={company.health} />
                </>
              )}
            </div>
            <ul className="flex flex-wrap gap-1.5 text-xs">
              {contact.email && (
                <CopyChip
                  icon={Mail}
                  value={contact.email}
                  href={`mailto:${contact.email}`}
                />
              )}
              {contact.phone && (
                <CopyChip
                  icon={Phone}
                  value={contact.phone}
                  href={`tel:${contact.phone}`}
                />
              )}
              {contact.external_id && (
                <CopyChip icon={Hash} value={contact.external_id} mono />
              )}
              {(contact.timezone || contact.locale) && (
                <li className="bg-muted/60 text-muted-foreground inline-flex items-center gap-1.5 rounded-full px-2.5 py-1">
                  <Globe className="size-3.5" />
                  {[contact.locale, contact.timezone]
                    .filter(Boolean)
                    .join(" · ")}
                </li>
              )}
              {contact.channel_types.length > 0 && (
                <li className="bg-muted/60 inline-flex items-center gap-1 rounded-full px-2.5 py-1">
                  {contact.channel_types.map((ch) => (
                    <ChannelGlyph
                      key={ch}
                      type={ch}
                      className="size-3.5"
                      title={t(`channels.types.${ch}`)}
                    />
                  ))}
                </li>
              )}
            </ul>
            <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs">
              <span className="text-muted-foreground">
                {t("contacts.profile.owner")}:{" "}
                <span className="text-foreground font-medium">
                  {owner ?? t("contacts.account.no_owner")}
                </span>
              </span>
              {tags.length > 0 && (
                <ul
                  className="flex flex-wrap gap-1"
                  aria-label={t("contacts.profile.tags")}
                >
                  {tags.slice(0, 8).map((tag) => (
                    <li key={tag.uuid}>
                      <Badge
                        variant="outline"
                        className="gap-1.5 font-normal"
                        title={t("contacts.profile.tag_count", {
                          count: tag.conversations,
                        })}
                      >
                        <span
                          aria-hidden
                          className="bg-muted-foreground size-2 rounded-full"
                          style={tag.color ? { background: tag.color } : {}}
                        />
                        {tag.name}
                      </Badge>
                    </li>
                  ))}
                  {tags.length > 8 && (
                    <li className="text-muted-foreground self-center">
                      +{tags.length - 8}
                    </li>
                  )}
                </ul>
              )}
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 lg:justify-end">
          {canStart && (
            <Button size="sm" onClick={() => setDialog("conversation")}>
              <MessageSquarePlus className="size-4" />
              {t("contacts.profile.action.new_conversation")}
            </Button>
          )}
          {scope.canWrite && (
            <Button size="sm" variant="outline" onClick={onAddNote}>
              <StickyNote className="size-4" />
              {t("contacts.profile.action.add_note")}
            </Button>
          )}
          {scope.canWrite && (
            <Button
              size="sm"
              variant="outline"
              onClick={() => setDialog("edit")}
            >
              <Pencil className="size-4" />
              {t("contacts.profile.action.edit")}
            </Button>
          )}
          <CallButton phone={contact.phone} />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                size="icon"
                variant="outline"
                className="size-8"
                aria-label={t("contacts.profile.action.more")}
              >
                <MoreHorizontal className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-60">
              {scope.canWrite && (
                <>
                  <DropdownMenuItem onSelect={() => setDialog("merge")}>
                    <GitMerge className="size-4" />
                    {t("contacts.profile.action.merge")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    disabled={!company}
                    onSelect={() => setDialog("owner")}
                  >
                    <UserCog className="size-4" />
                    {t("contacts.profile.action.owner")}
                  </DropdownMenuItem>
                </>
              )}
              {sections.compliance && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
                    {t("contacts.profile.action.kvkk")}
                  </DropdownMenuLabel>
                  <DropdownMenuItem
                    disabled={dsr.isPending}
                    onSelect={() =>
                      dsr.mutate({ contact: contact.uuid, kind: "export" })
                    }
                  >
                    <Download className="size-4" />
                    {t("contacts.profile.action.export")}
                  </DropdownMenuItem>
                  <DropdownMenuItem asChild>
                    <Link
                      href={`${routes.cms.settings.privacy}?tab=consents&contact=${contact.uuid}`}
                    >
                      <ShieldCheck className="size-4" />
                      {t("contacts.profile.action.consents")}
                    </Link>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onSelect={() => setDialog("erase")}
                  >
                    <UserX className="size-4" />
                    {t("contacts.profile.action.erase")}
                  </DropdownMenuItem>
                </>
              )}
              {scope.canDelete && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    className="text-destructive focus:text-destructive"
                    onSelect={() => setDialog("delete")}
                  >
                    <Trash2 className="size-4" />
                    {t("contacts.delete")}
                  </DropdownMenuItem>
                </>
              )}
              {!scope.canWrite && !sections.compliance && !scope.canDelete && (
                <DropdownMenuItem disabled>
                  {t("contacts.profile.action.none")}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <EditContactSheet
        contact={contact}
        open={dialog === "edit"}
        onOpenChange={(o) => setDialog(o ? "edit" : null)}
      />
      <MergeDialog
        open={dialog === "merge"}
        onOpenChange={(o) => setDialog(o ? "merge" : null)}
        into={contact}
      />
      {company && (
        <OwnerDialog
          company={company}
          open={dialog === "owner"}
          onOpenChange={(o) => setDialog(o ? "owner" : null)}
        />
      )}
      {canStart && (
        <NewConversationDialog
          contactUuid={contact.uuid}
          contactName={name}
          open={dialog === "conversation"}
          onOpenChange={(o) => setDialog(o ? "conversation" : null)}
        />
      )}
      {sections.compliance && (
        <ErasureDialog
          contact={{ uuid: contact.uuid, label: name }}
          open={dialog === "erase"}
          onOpenChange={(o) => setDialog(o ? "erase" : null)}
        />
      )}
      <ConfirmDialog
        open={dialog === "delete"}
        title={t("contacts.delete_title")}
        description={t("contacts.delete_description")}
        variant="destructive"
        isPending={m.remove.isPending}
        onCancel={() => setDialog(null)}
        onConfirm={() =>
          m.remove.mutate(contact.uuid, {
            onSuccess: () => router.push(routes.cms.contacts.root),
          })
        }
      />
    </header>
  );
}

function CopyChip({
  icon: Icon,
  value,
  href,
  mono,
}: {
  icon: typeof Mail;
  value: string;
  href?: string;
  mono?: boolean;
}) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <li className="bg-muted/60 group inline-flex max-w-full items-center rounded-full">
      {href ? (
        <a
          href={href}
          className="hover:text-foreground text-muted-foreground focus-visible:ring-ring inline-flex min-w-0 items-center gap-1.5 rounded-full py-1 pr-1 pl-2.5 outline-none focus-visible:ring-2"
        >
          <Icon className="size-3.5 shrink-0" />
          <span className={mono ? "truncate font-mono" : "truncate"}>
            {value}
          </span>
        </a>
      ) : (
        <span className="text-muted-foreground inline-flex min-w-0 items-center gap-1.5 py-1 pr-1 pl-2.5">
          <Icon className="size-3.5 shrink-0" />
          <span className={mono ? "truncate font-mono" : "truncate"}>
            {value}
          </span>
        </span>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("contacts.profile.copy", { value })}
            className="text-muted-foreground hover:text-foreground focus-visible:ring-ring mr-1 rounded-full p-1 outline-none focus-visible:ring-2"
            onClick={() => {
              void navigator.clipboard?.writeText(value).then(() => {
                setCopied(true);
                window.setTimeout(() => setCopied(false), 1500);
              });
            }}
          >
            {copied ? (
              <Check className="text-success size-3" />
            ) : (
              <Copy className="size-3" />
            )}
          </button>
        </TooltipTrigger>
        <TooltipContent>
          {copied
            ? t("contacts.profile.copied")
            : t("contacts.profile.copy_short")}
        </TooltipContent>
      </Tooltip>
    </li>
  );
}
