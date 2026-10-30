"use client";

import { MemberSIPSettings } from "@/features/softphone/member-sip";
import { useMemo } from "react";

import { EntityDrawer } from "@/components/entity";
import {
  AppForm,
  AppMultiCombobox,
  type MultiComboboxOption,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useUpdateTenantMember } from "@/features/tenant-members/hooks/use-tenant-members";
import {
  createMemberAccessSchema,
  memberAccessDefaults,
  memberAccessRequest,
  type MemberAccessValues,
} from "@/features/tenant-members/schemas/member-access";
import type { TenantMember } from "@/features/tenant-members/types";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "tenant-member-access";
/** Drawer to change a member's roles (several; permissions are their union). */
export function MemberAccessDrawer({
  member,
  onOpenChange,
}: {
  member: TenantMember | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const update = useUpdateTenantMember();
  const schema = useMemo(() => createMemberAccessSchema(t), [t]);
  const rolesQuery = useAllTenantRoles(Boolean(member));

  const roleOptions = useMemo<MultiComboboxOption[]>(
    () =>
      (rolesQuery.data?.items ?? []).map((role) => ({
        value: role.uuid,
        label: tenantRoleLabel(t, role),
        hint: role.is_template
          ? t("tenant_roles.template")
          : t("tenant_roles.custom"),
      })),
    [rolesQuery.data?.items, t],
  );
  const fullName = member
    ? `${member.user.name} ${member.user.surname}`.trim() || member.user.email
    : "";

  return (
    <EntityDrawer
      open={Boolean(member)}
      onOpenChange={onOpenChange}
      title={t("tenant_members.access.title")}
      description={
        member
          ? t("tenant_members.access.description", {
              name: fullName,
              email: member.user.email,
            })
          : undefined
      }
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
          <Button type="submit" form={FORM_ID} disabled={update.isPending}>
            {t("tenant_members.actions.save")}
          </Button>
        </div>
      }
    >
      {member ? (
        <AppForm<MemberAccessValues>
          key={member.user.uuid}
          id={FORM_ID}
          className="space-y-5 px-4"
          schema={schema}
          defaultValues={memberAccessDefaults(member)}
          onSubmit={async (values) => {
            await update.mutateAsync({
              userUuid: member.user.uuid,
              body: memberAccessRequest(values),
            });
            onOpenChange(false);
          }}
        >
          <AppMultiCombobox
            name="role_uuids"
            label={t("tenant_members.fields.roles")}
            description={t("tenant_members.fields.roles_hint")}
            options={roleOptions}
            placeholder={t("tenant_members.fields.roles_placeholder")}
            emptyText={t("tenant_members.fields.roles_empty")}
          />
        </AppForm>
      ) : null}
      {member && (
        <MemberSIPSettings key={member.user.uuid} user={member.user.uuid} />
      )}
    </EntityDrawer>
  );
}
