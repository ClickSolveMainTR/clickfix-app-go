"use client";

import { useMemo, useState } from "react";

import type { MultiComboboxOption } from "@/components/forms";
import { permissions } from "@/config/permissions";
import { InvitationsTable } from "@/features/invitations/components/invitations-table";
import { InviteDrawer } from "@/features/invitations/components/invite-drawer";
import type { InvitationScope } from "@/features/invitations/types";
import { useAllTenantRoles } from "@/features/tenant-roles/hooks/use-tenant-roles";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** "Davetler" tab of the members page (`/v1/tenant/invitations`). */
export function TenantInvitationsTab({ active }: { active: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.members.write);
  const [inviting, setInviting] = useState(false);
  const scope = useMemo<InvitationScope>(
    () => ({ kind: "tenant", slug }),
    [slug],
  );
  const rolesQuery = useAllTenantRoles(inviting);

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

  return (
    <>
      <InvitationsTable
        scope={scope}
        canWrite={canWrite}
        onInvite={() => setInviting(true)}
        persistKey="tenant-invitations-v1"
        enabled={active}
      />
      <InviteDrawer
        open={inviting}
        onOpenChange={setInviting}
        scope={scope}
        roleOptions={roleOptions}
      />
    </>
  );
}
