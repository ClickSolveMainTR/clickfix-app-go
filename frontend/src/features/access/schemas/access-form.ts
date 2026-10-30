import { z } from "zod";

type Translate = (key: string) => string;

export const createAccessFormSchema = (t: Translate) =>
  z
    .object({
      ttl_hours: z.coerce.number().int().min(1).max(168),
      password_enabled: z.boolean(),
      passkey_enabled: z.boolean(),
      totp_enabled: z.boolean(),
      password_login_totp_required: z.boolean(),
    })
    .refine((v) => v.password_enabled || v.passkey_enabled || v.totp_enabled, {
      message: t("access.form.method_required"),
      path: ["password_enabled"],
    });

export type AccessFormValues = z.infer<
  ReturnType<typeof createAccessFormSchema>
>;
