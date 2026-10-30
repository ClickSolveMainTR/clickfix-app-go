import { z } from "zod";

/** Who can open an account (CF-143); the single source of truth for sign-up. */
export const SIGNUP_MODES = ["open", "invite", "closed"] as const;
export type SignupMode = (typeof SIGNUP_MODES)[number];

export const authSettingsFormSchema = z.object({
  signup_mode: z.enum(SIGNUP_MODES),
  default_role_uuid: z.string().uuid().nullable(),
  password_login_enabled: z.boolean(),
  password_register_enabled: z.boolean(),
  passkey_login_enabled: z.boolean(),
});

export type AuthSettingsFormValues = z.infer<typeof authSettingsFormSchema>;
