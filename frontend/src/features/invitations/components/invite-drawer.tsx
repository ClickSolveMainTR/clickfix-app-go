"use client";

import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  type MultiComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useCreateInvitation } from "@/features/invitations/hooks/use-invitations";
import {
  createInviteSchema,
  inviteDefaults,
  inviteRequest,
  type InviteFormValues,
} from "@/features/invitations/schemas/invite-form";
import type { InvitationScope } from "@/features/invitations/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "invite-member";

/**
 * Invite a staff member by e-mail with one or more roles. The API applies the
 * same "only grant what you hold" rules as member updates.
 */
export function InviteDrawer({
  open,
  onOpenChange,
  scope,
  roleOptions,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: InvitationScope;
  roleOptions: MultiComboboxOption[];
}) {
  const { t } = useLocale();
  const create = useCreateInvitation(scope);
  const schema = useMemo(() => createInviteSchema(t), [t]);

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={t("invitations.create.title")}
      description={t("invitations.create.description")}
      size="md"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("invitations.actions.send")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<InviteFormValues>
          id={FORM_ID}
          className="space-y-5 px-4"
          schema={schema}
          defaultValues={inviteDefaults()}
          onSubmit={async (values) => {
            await create.mutateAsync(inviteRequest(values));
            onOpenChange(false);
          }}
        >
          <AppInput
            name="email"
            type="email"
            autoComplete="off"
            label={t("invitations.fields.email")}
            placeholder={t("invitations.fields.email_placeholder")}
          />
          <AppMultiCombobox
            name="role_uuids"
            label={t("invitations.fields.roles")}
            description={t("invitations.fields.roles_hint")}
            options={roleOptions}
            placeholder={t("invitations.fields.roles_placeholder")}
            emptyText={t("invitations.fields.roles_empty")}
          />
          <p className="text-muted-foreground text-sm">
            {t("invitations.create.expiry_hint")}
          </p>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
