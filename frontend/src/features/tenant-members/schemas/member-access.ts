import { z } from "zod";

import type {
  TenantMember,
  TenantMemberPatchRequest,
} from "@/features/tenant-members/types";

type Translate = (key: string) => string;

export function createMemberAccessSchema(t: Translate) {
  return z.object({
    role_uuids: z
      .array(z.string())
      .min(1, t("tenant_members.validation.roles_required")),
  });
}

export type MemberAccessValues = z.infer<
  ReturnType<typeof createMemberAccessSchema>
>;

export function memberAccessDefaults(member: TenantMember): MemberAccessValues {
  return { role_uuids: member.roles.map((role) => role.uuid) };
}

export function memberAccessRequest(
  values: MemberAccessValues,
): TenantMemberPatchRequest {
  return { role_uuids: values.role_uuids };
}
