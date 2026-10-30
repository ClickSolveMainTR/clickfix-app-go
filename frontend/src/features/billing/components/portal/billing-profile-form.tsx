"use client";

import { Plus, X } from "lucide-react";
import { useMemo } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppForm,
  AppInput,
  AppRadioGroup,
  AppSwitch,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import type { BillingProfile } from "@/features/billing/services/tenant-billing.service";
import { useSaveBillingProfile } from "@/features/billing/hooks/use-tenant-billing";
import {
  isValidTCKN,
  isValidVKN,
} from "@/features/tenant-settings/lib/tax-number";
import { useLocale } from "@/providers/locale-provider";

function createSchema(t: (key: string) => string) {
  return z
    .object({
      kind: z.enum(["company", "individual"]),
      legal_name: z
        .string()
        .trim()
        .min(1, t("billing.validation.required"))
        .max(240),
      tax_id: z.string().trim(),
      tax_office: z.string().trim().max(120),
      country_code: z
        .string()
        .trim()
        .regex(/^[A-Za-z]{2}$/, t("billing.portal.profile.country_invalid")),
      street: z.string().trim().min(1, t("billing.validation.required")),
      district: z.string().trim().max(120),
      city: z.string().trim().min(1, t("billing.validation.required")),
      postal_code: z.string().trim().max(16),
      emails: z
        .array(
          z.object({
            value: z
              .string()
              .trim()
              .email(t("billing.portal.profile.email_invalid")),
          }),
        )
        .min(1)
        .max(5),
      export_exemption: z.boolean(),
    })
    .superRefine((v, ctx) => {
      const tr = v.country_code.toUpperCase() === "TR";
      if (!tr) {
        return;
      }
      if (v.kind === "company" && !isValidVKN(v.tax_id)) {
        ctx.addIssue({
          code: "custom",
          path: ["tax_id"],
          message: t("billing.portal.profile.vkn_invalid"),
        });
      }
      if (v.kind === "individual" && !isValidTCKN(v.tax_id)) {
        ctx.addIssue({
          code: "custom",
          path: ["tax_id"],
          message: t("billing.portal.profile.tckn_invalid"),
        });
      }
      if (v.kind === "company" && !v.tax_office) {
        ctx.addIssue({
          code: "custom",
          path: ["tax_office"],
          message: t("billing.validation.required"),
        });
      }
    });
}
type Values = z.infer<ReturnType<typeof createSchema>>;

function toForm(p: BillingProfile | null | undefined): Values {
  return {
    kind: p?.kind ?? "company",
    legal_name: p?.legal_name ?? "",
    tax_id: p?.tax_id ?? "",
    tax_office: p?.tax_office ?? "",
    country_code: p?.country_code ?? "TR",
    street: p?.address.street ?? "",
    district: p?.address.district ?? "",
    city: p?.address.city ?? "",
    postal_code: p?.address.postal_code ?? "",
    emails: (p?.emails?.length ? p.emails : [""]).map((value) => ({ value })),
    export_exemption: p?.export_exemption ?? false,
  };
}

function Fields({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { control } = useFormContext<Values>();
  const kind = useWatch({ control, name: "kind" });
  const country = useWatch({ control, name: "country_code" });
  const emails = useFieldArray({ control, name: "emails" });
  const foreign = (country ?? "").toUpperCase() !== "TR";
  return (
    <div className="space-y-4">
      <AppRadioGroup
        name="kind"
        label={t("billing.portal.profile.kind")}
        orientation="horizontal"
        options={[
          { value: "company", label: t("billing.portal.profile.kind_company") },
          {
            value: "individual",
            label: t("billing.portal.profile.kind_individual"),
          },
        ]}
      />
      <AppInput
        name="legal_name"
        disabled={disabled}
        label={
          kind === "company"
            ? t("billing.portal.profile.legal_name_company")
            : t("billing.portal.profile.legal_name_individual")
        }
        autoComplete={kind === "company" ? "organization" : "name"}
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <AppInput
          name="country_code"
          disabled={disabled}
          label={t("billing.portal.profile.country")}
          maxLength={2}
          className="uppercase"
        />
        <AppInput
          name="tax_id"
          disabled={disabled}
          inputMode="numeric"
          label={
            foreign
              ? t("billing.portal.profile.tax_id_foreign")
              : kind === "company"
                ? t("billing.portal.profile.vkn")
                : t("billing.portal.profile.tckn")
          }
        />
        <AppInput
          name="tax_office"
          disabled={disabled}
          label={t("billing.portal.profile.tax_office")}
        />
      </div>
      <AppInput
        name="street"
        disabled={disabled}
        label={t("billing.portal.profile.street")}
        autoComplete="street-address"
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <AppInput
          name="district"
          disabled={disabled}
          label={t("billing.portal.profile.district")}
        />
        <AppInput
          name="city"
          disabled={disabled}
          label={t("billing.portal.profile.city")}
          autoComplete="address-level1"
        />
        <AppInput
          name="postal_code"
          disabled={disabled}
          label={t("billing.portal.profile.postal_code")}
          autoComplete="postal-code"
        />
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">
          {t("billing.portal.profile.emails")}
        </legend>
        <p className="text-muted-foreground text-xs">
          {t("billing.portal.profile.emails_hint")}
        </p>
        {emails.fields.map((field, i) => (
          <div key={field.id} className="flex items-start gap-2">
            <AppInput
              name={`emails.${i}.value`}
              type="email"
              disabled={disabled}
              aria-label={t("billing.portal.profile.email_n", { n: i + 1 })}
              className="flex-1"
            />
            {emails.fields.length > 1 && !disabled ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={t("billing.portal.profile.email_remove")}
                onClick={() => emails.remove(i)}
              >
                <X className="size-4" />
              </Button>
            ) : null}
          </div>
        ))}
        {emails.fields.length < 5 && !disabled ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => emails.append({ value: "" })}
          >
            <Plus className="size-4" />
            {t("billing.portal.profile.email_add")}
          </Button>
        ) : null}
      </fieldset>
      {foreign ? (
        <AppSwitch
          name="export_exemption"
          label={t("billing.portal.profile.export_exemption")}
        />
      ) : null}
    </div>
  );
}

/**
 * Billing profile (CF-32): company (VKN + tax office) or individual (TCKN),
 * address, up to five e-Arşiv recipient addresses; foreign buyers may claim
 * the service-export VAT exemption. Checksums mirror the API.
 */
export function BillingProfileForm({
  profile,
  disabled,
  onSaved,
  submitLabel,
}: {
  profile: BillingProfile | null | undefined;
  disabled?: boolean;
  onSaved?: () => void;
  submitLabel?: string;
}) {
  const { t } = useLocale();
  const save = useSaveBillingProfile();
  const schema = useMemo(() => createSchema(t), [t]);
  return (
    <AppForm<Values>
      key={JSON.stringify(profile ?? {})}
      schema={schema}
      defaultValues={toForm(profile)}
      className="space-y-4"
      onSubmit={async (v) => {
        const foreign = v.country_code.toUpperCase() !== "TR";
        await save.mutateAsync({
          kind: v.kind,
          legal_name: v.legal_name.trim(),
          tax_id: v.tax_id.trim(),
          tax_office: v.tax_office.trim(),
          country_code: v.country_code.toUpperCase(),
          address: {
            street: v.street.trim(),
            district: v.district.trim(),
            city: v.city.trim(),
            postal_code: v.postal_code.trim(),
          },
          emails: v.emails.map((e) => e.value.trim()),
          export_exemption: foreign && v.export_exemption,
        });
        onSaved?.();
      }}
    >
      {(form) => (
        <>
          <Fields disabled={Boolean(disabled)} />
          {disabled ? null : (
            <FormActions>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {submitLabel ?? t("billing.common.save")}
              </Button>
            </FormActions>
          )}
        </>
      )}
    </AppForm>
  );
}
