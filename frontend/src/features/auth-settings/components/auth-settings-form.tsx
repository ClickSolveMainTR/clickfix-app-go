"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  DoorOpen,
  Info,
  KeyRound,
  Lock,
  MailPlus,
  UserPlus,
  type LucideIcon,
} from "lucide-react";

import {
  AppForm,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  authSettingsFormSchema,
  SIGNUP_MODES,
  type AuthSettingsFormValues,
  type SignupMode,
} from "@/features/auth-settings/schemas/auth-settings-form";
import type { AuthSettings } from "@/features/auth-settings/services/auth-settings.service";
import { rolesService } from "@/features/roles/services/roles.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type AuthSettingsFormProps = {
  settings: AuthSettings;
  canWrite: boolean;
  isSaving: boolean;
  onSubmit: (values: AuthSettingsFormValues) => Promise<void>;
};

const NONE = "__none__";

const MODE_ICONS: Record<SignupMode, LucideIcon> = {
  open: DoorOpen,
  invite: MailPlus,
  closed: Lock,
};

type SwitchField =
  | "password_login_enabled"
  | "password_register_enabled"
  | "passkey_login_enabled";

export function AuthSettingsForm({
  settings,
  canWrite,
  isSaving,
  onSubmit,
}: AuthSettingsFormProps) {
  const { t } = useLocale();

  const { data: rolesData } = useQuery({
    queryKey: ["platform", "roles", "auth-settings-picker"],
    queryFn: () => rolesService.list({ limit: 100, offset: 0 }),
    enabled: canWrite || Boolean(settings.default_role),
  });

  const defaultValues = useMemo<AuthSettingsFormValues>(
    () => ({
      signup_mode: settings.signup_mode,
      default_role_uuid: settings.default_role?.uuid ?? null,
      password_login_enabled: settings.password_login_enabled,
      password_register_enabled: settings.password_register_enabled,
      passkey_login_enabled: settings.passkey_login_enabled,
    }),
    [settings],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "signup",
          label: t("signup.settings.section_title"),
          description: t("signup.settings.section_hint"),
          icon: UserPlus,
        },
        {
          key: "methods",
          label: t("auth.settings.form.section_title"),
          description: t("auth.settings.form.section_hint"),
          icon: KeyRound,
        },
      ]),
    [t],
  );

  return (
    <AppForm
      schema={authSettingsFormSchema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      {(form) => {
        const disabled = !canWrite || isSaving;
        const switchRow = (
          field: SwitchField,
          labelKey: string,
          hintKey: string,
        ) => (
          <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
            <div className="space-y-1">
              <Label htmlFor={field}>{t(labelKey)}</Label>
              <p className="text-muted-foreground text-sm">{t(hintKey)}</p>
            </div>
            <Switch
              id={field}
              checked={form.watch(field)}
              onCheckedChange={(checked) =>
                form.setValue(field, checked, { shouldDirty: true })
              }
              disabled={disabled}
            />
          </div>
        );
        const mode = form.watch("signup_mode");

        return (
          <FormLayout navItems={sections.navItems}>
            <FormSection
              id={sections.id("signup")}
              title={t("signup.settings.section_title")}
              description={t("signup.settings.section_hint")}
            >
              <RadioGroup
                value={mode}
                onValueChange={(value) =>
                  form.setValue("signup_mode", value as SignupMode, {
                    shouldDirty: true,
                  })
                }
                disabled={disabled}
                aria-label={t("signup.settings.mode_label")}
                className="grid gap-3"
              >
                {SIGNUP_MODES.map((value) => {
                  const Icon = MODE_ICONS[value];
                  const selected = mode === value;
                  return (
                    <Label
                      key={value}
                      htmlFor={`signup-mode-${value}`}
                      className={cn(
                        "flex cursor-pointer items-start gap-4 rounded-xl border p-4 font-normal transition-colors",
                        "hover:bg-muted/40 has-[:focus-visible]:ring-ring/50 has-[:focus-visible]:ring-[3px]",
                        selected && "border-primary bg-primary/5",
                        disabled && "cursor-not-allowed opacity-70",
                      )}
                    >
                      <span
                        className={cn(
                          "bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-lg",
                          selected && "bg-primary text-primary-foreground",
                        )}
                      >
                        <Icon className="size-4" aria-hidden />
                      </span>
                      <div className="min-w-0 flex-1 space-y-1">
                        <p className="text-sm font-semibold">
                          {t(`signup.settings.mode.${value}.title`)}
                        </p>
                        <p className="text-muted-foreground text-sm leading-relaxed">
                          {t(`signup.settings.mode.${value}.body`)}
                        </p>
                      </div>
                      <RadioGroupItem
                        id={`signup-mode-${value}`}
                        value={value}
                        className="mt-1"
                      />
                    </Label>
                  );
                })}
              </RadioGroup>

              <p className="text-muted-foreground flex items-start gap-2 text-sm">
                <Info className="mt-0.5 size-4 shrink-0" aria-hidden />
                {t("signup.settings.admin_note")}
              </p>

              <div className="space-y-2">
                <Label>{t("auth.settings.form.default_role")}</Label>
                <p className="text-muted-foreground text-sm">
                  {t("auth.settings.form.default_role_hint")}
                </p>
                <Select
                  value={form.watch("default_role_uuid") ?? NONE}
                  onValueChange={(value) =>
                    form.setValue(
                      "default_role_uuid",
                      value === NONE ? null : value,
                      { shouldDirty: true },
                    )
                  }
                  disabled={disabled}
                >
                  <SelectTrigger className="sm:max-w-sm">
                    <SelectValue
                      placeholder={t("auth.settings.form.default_role_none")}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>
                      {t("auth.settings.form.default_role_none")}
                    </SelectItem>
                    {(rolesData?.items ?? []).map((role) => (
                      <SelectItem key={role.uuid} value={role.uuid}>
                        {role.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </FormSection>

            <FormSection
              id={sections.id("methods")}
              title={t("auth.settings.form.section_title")}
              description={t("auth.settings.form.section_hint")}
            >
              {switchRow(
                "password_register_enabled",
                "auth.settings.form.password_register",
                "auth.settings.form.password_register_hint",
              )}
              {switchRow(
                "password_login_enabled",
                "auth.settings.form.password_login",
                "auth.settings.form.password_login_hint",
              )}
              {switchRow(
                "passkey_login_enabled",
                "auth.settings.form.passkey_login",
                "auth.settings.form.passkey_login_hint",
              )}
            </FormSection>

            {canWrite ? (
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => form.reset(defaultValues)}
                  disabled={isSaving}
                >
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={isSaving}>
                  {isSaving
                    ? t("common.loading")
                    : t("auth.settings.form.save")}
                </Button>
              </FormActions>
            ) : null}
          </FormLayout>
        );
      }}
    </AppForm>
  );
}
