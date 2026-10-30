import { z } from "zod";

import type { CreateInvitationRequest } from "@/features/invitations/types";

type Translate = (key: string) => string;

export function createInviteSchema(t: Translate) {
  return z.object({
    email: z
      .string()
      .trim()
      .min(1, t("invitations.validation.email_required"))
      .email(t("invitations.validation.email_invalid")),
    role_uuids: z
      .array(z.string())
      .min(1, t("invitations.validation.roles_required")),
  });
}

export type InviteFormValues = z.infer<ReturnType<typeof createInviteSchema>>;

export function inviteDefaults(): InviteFormValues {
  return { email: "", role_uuids: [] };
}

export function inviteRequest(
  values: InviteFormValues,
): CreateInvitationRequest {
  return {
    email: values.email.trim(),
    role_uuids: values.role_uuids,
  };
}
