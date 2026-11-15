"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { MessageSquarePlus } from "lucide-react";

import { AppDialog } from "@/components/dialogs/app-dialog";
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { routes } from "@/config/routes";
import { useChannels } from "@/features/channels/hooks/use-channels";
import { getActiveOrganizationSlug } from "@/features/organizations/lib/active-organization-store";
import { ChannelGlyph } from "@/features/inbox/components/channel-glyph";
import { apiClient, unwrap } from "@/lib/api";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";

import { useCompanyMutations } from "../../hooks/use-contacts";
import type { Company, Contact } from "../../services/contacts.service";
import { ContactForm } from "../contacts-page";
import { OwnerPicker } from "../owner-picker";

/** Edit form of the contact in a side sheet (the list's form, reused). */
export function EditContactSheet({
  contact,
  open,
  onOpenChange,
}: {
  contact: Contact;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t("contacts.profile.action.edit")}</SheetTitle>
          <SheetDescription>
            {t("contacts.detail_description")}
          </SheetDescription>
        </SheetHeader>
        {open && (
          <ContactForm
            key={contact.uuid + contact.updated_at}
            contact={contact}
            onDeleted={() => router.push(routes.cms.contacts.root)}
          />
        )}
      </SheetContent>
    </Sheet>
  );
}

/** Account owner of the contact's company (owners live on companies). */
export function OwnerDialog({
  company,
  open,
  onOpenChange,
}: {
  company: Company;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const m = useCompanyMutations();
  const [owner, setOwner] = useState<string | null>(
    company.account_owner?.uuid ?? null,
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("contacts.profile.action.owner")}
      description={t("contacts.profile.owner_hint", { company: company.name })}
      footer={
        <>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={
              m.update.isPending ||
              owner === (company.account_owner?.uuid ?? null)
            }
            onClick={() =>
              m.update.mutate(
                { uuid: company.uuid, body: { account_owner: owner } },
                { onSuccess: () => onOpenChange(false) },
              )
            }
          >
            {t("contacts.save")}
          </Button>
        </>
      }
    >
      <div className="grid gap-1.5">
        <Label>{t("contacts.companies.fields.account_owner")}</Label>
        <OwnerPicker value={owner} onChange={setOwner} />
      </div>
    </AppDialog>
  );
}

/** Opens a conversation with the contact on one of the active inboxes. */
export function NewConversationDialog({
  contactUuid,
  contactName,
  open,
  onOpenChange,
}: {
  contactUuid: string;
  contactName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const channels = useChannels();
  const inboxes = (channels.data ?? []).filter((c) => c.status === "active");
  const [inbox, setInbox] = useState("");
  const [subject, setSubject] = useState("");
  const selected = inbox || inboxes[0]?.uuid || "";
  const create = useAppMutation({
    mutationFn: async () =>
      unwrap<{ uuid: string; number: number; status: string }>(
        await apiClient.POST("/v1/tenant/conversations", {
          params: {
            header: { "X-Organization": getActiveOrganizationSlug() ?? "" },
          },
          body: {
            contact_uuid: contactUuid,
            inbox_uuid: selected,
            subject: subject.trim(),
          },
        }),
      ),
    onSuccess: (c) => {
      onOpenChange(false);
      router.push(`${routes.cms.inbox}?c=${c.uuid}`);
    },
  });
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("contacts.profile.action.new_conversation")}
      description={t("contacts.profile.new_conversation_hint", {
        name: contactName,
      })}
      footer={
        <>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="contact-new-conversation"
            disabled={!selected || !subject.trim() || create.isPending}
          >
            <MessageSquarePlus className="size-4" />
            {t("contacts.profile.start")}
          </Button>
        </>
      }
    >
      <form
        id="contact-new-conversation"
        className="grid gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (selected && subject.trim()) create.mutate();
        }}
      >
        <div className="grid gap-1.5">
          <Label htmlFor="new-conversation-inbox">
            {t("contacts.profile.inbox")}
          </Label>
          <Select value={selected} onValueChange={setInbox}>
            <SelectTrigger id="new-conversation-inbox" className="w-full">
              <SelectValue placeholder={t("contacts.profile.inbox_pick")} />
            </SelectTrigger>
            <SelectContent>
              {inboxes.map((c) => (
                <SelectItem key={c.uuid} value={c.uuid}>
                  <span className="flex items-center gap-2">
                    <ChannelGlyph type={c.type} className="size-4" />
                    {c.name}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {!channels.isLoading && inboxes.length === 0 && (
            <p className="text-muted-foreground text-xs">
              {t("contacts.profile.no_inbox")}
            </p>
          )}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="new-conversation-subject">
            {t("contacts.profile.subject")}
          </Label>
          <Input
            id="new-conversation-subject"
            value={subject}
            maxLength={500}
            autoFocus
            onChange={(e) => setSubject(e.target.value)}
          />
        </div>
      </form>
    </AppDialog>
  );
}
