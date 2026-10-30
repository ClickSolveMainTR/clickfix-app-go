"use client";

import { useEffect, useMemo, useRef } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSwitch,
  AppTextarea,
  FormActions,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  CREDIT_UNITS,
  PRODUCT_CODE_RE,
  formatUnitPrice,
  productCodeFromName,
} from "@/features/billing/components/platform/catalog/catalog-lib";
import { useSaveCreditProduct } from "@/features/billing/hooks/use-credit-products";
import { usePlans } from "@/features/billing/hooks/use-platform-billing";
import { minorToInput, parseMajorToMinor } from "@/features/billing/lib/money";
import type {
  CreditProduct,
  CreditProductInput,
  CreditProductUnit,
} from "@/features/billing/services/credit-products.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

function createSchema(t: (key: string) => string, create: boolean) {
  const whole = (min: number, max: number) =>
    z.coerce
      .number({ message: t("credit_products.validation.number") })
      .int(t("credit_products.validation.number"))
      .min(min, t("credit_products.validation.number"))
      .max(max, t("credit_products.validation.number"));
  return z
    .object({
      unit: z.enum(CREDIT_UNITS),
      code: create
        ? z
            .string()
            .trim()
            .toUpperCase()
            .regex(PRODUCT_CODE_RE, t("credit_products.validation.code"))
        : z.string(),
      name: z
        .string()
        .trim()
        .min(1, t("credit_products.validation.required"))
        .max(200),
      description: z.string().max(500),
      credits: whole(1, 1_000_000_000),
      price: z
        .string()
        .refine(
          (v) => (parseMajorToMinor(v) ?? 0) >= 1,
          t("credit_products.validation.price"),
        ),
      unlimited: z.boolean(),
      expires_days: z.coerce.number(),
      plan_uuids: z.array(z.string()).max(50),
      sort_order: whole(0, 10000),
      active: z.boolean(),
    })
    .superRefine((v, ctx) => {
      if (
        !v.unlimited &&
        (!Number.isInteger(v.expires_days) ||
          v.expires_days < 1 ||
          v.expires_days > 3650)
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["expires_days"],
          message: t("credit_products.validation.days"),
        });
      }
    });
}
type Values = z.infer<ReturnType<typeof createSchema>>;

/** Fills the code from the name until the admin types a code of their own. */
function CodeSuggestion() {
  const { setValue, getValues } = useFormContext<Values>();
  const name = useWatch<Values, "name">({ name: "name" });
  const last = useRef("");
  useEffect(() => {
    const next = productCodeFromName(name ?? "");
    if (getValues("code") === last.current) {
      setValue("code", next, { shouldValidate: next.length >= 3 });
    }
    last.current = next;
  }, [name, getValues, setValue]);
  return null;
}

function UnitPicker({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { setValue } = useFormContext<Values>();
  const unit = useWatch<Values, "unit">({ name: "unit" });
  return (
    <div className="space-y-2">
      <Label>{t("credit_products.fields.unit")}</Label>
      <RadioGroup
        value={unit}
        disabled={disabled}
        onValueChange={(v) =>
          setValue("unit", v as CreditProductUnit, { shouldDirty: true })
        }
        className="grid gap-2 sm:grid-cols-2"
      >
        {CREDIT_UNITS.map((u) => (
          <Label
            key={u}
            htmlFor={`unit-${u}`}
            className={cn(
              "flex cursor-pointer items-start gap-3 rounded-lg border p-3 font-normal transition-colors",
              unit === u && "border-primary bg-primary/5",
              disabled && "cursor-not-allowed opacity-70",
            )}
          >
            <RadioGroupItem id={`unit-${u}`} value={u} className="mt-0.5" />
            <span className="flex flex-col gap-0.5">
              <span className="text-sm font-medium">
                {t(`credit_products.unit.${u}`)}
              </span>
              <span className="text-muted-foreground text-xs">
                {t(`credit_products.unit_hint.${u}`)}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
    </div>
  );
}

/** Live "₺0,25 / kredi" line under the price. */
function UnitPriceHint() {
  const { t, locale } = useLocale();
  const [unit, credits, price] = useWatch<Values, ["unit", "credits", "price"]>(
    { name: ["unit", "credits", "price"] },
  );
  const minor = parseMajorToMinor(price ?? "");
  const qty = Number(credits);
  if (!minor || !Number.isFinite(qty) || qty < 1) return null;
  return (
    <p className="text-muted-foreground text-xs tabular-nums">
      {t(`credit_products.unit_price.${unit}`, {
        price: formatUnitPrice(minor, qty, locale),
      })}
    </p>
  );
}

function ValidityField() {
  const { t } = useLocale();
  const unlimited = useWatch<Values, "unlimited">({ name: "unlimited" });
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <AppSwitch
        name="unlimited"
        label={t("credit_products.fields.no_expiry")}
        description={t("credit_products.fields.no_expiry_hint")}
      />
      {unlimited ? null : (
        <AppInput
          name="expires_days"
          type="number"
          min={1}
          max={3650}
          inputMode="numeric"
          label={t("credit_products.fields.expires_days")}
          description={t("credit_products.fields.expires_days_hint")}
          className="max-w-48"
        />
      )}
    </div>
  );
}

function QuantityField() {
  const { t } = useLocale();
  const unit = useWatch<Values, "unit">({ name: "unit" });
  return (
    <AppInput
      name="credits"
      type="number"
      min={1}
      inputMode="numeric"
      label={t(`credit_products.fields.quantity.${unit}`)}
    />
  );
}

/**
 * Create or edit a credit product. Code and unit are fixed after creation;
 * past purchases keep the terms they were invoiced with.
 */
export function CreditProductDialog({
  open,
  product,
  defaultUnit,
  onClose,
}: {
  open: boolean;
  /** Present when editing. */
  product: CreditProduct | null;
  defaultUnit: CreditProductUnit;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const save = useSaveCreditProduct();
  const create = !product;
  const schema = useMemo(() => createSchema(t, create), [t, create]);
  const plans = usePlans({ limit: 100, offset: 0 });
  const planOptions = useMemo(
    () =>
      (plans.data?.items ?? [])
        .filter((p) => !p.archived_at || product?.plan_uuids.includes(p.uuid))
        .map((p) => ({ value: p.uuid, label: p.name, hint: p.code })),
    [plans.data, product],
  );

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="max-h-[92dvh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {create
              ? t("credit_products.create")
              : t("credit_products.edit_title", { name: product.name })}
          </DialogTitle>
          <DialogDescription>
            {create
              ? t("credit_products.create_hint")
              : t("credit_products.edit_hint")}
          </DialogDescription>
        </DialogHeader>
        <AppForm<Values>
          key={product?.uuid ?? `new-${defaultUnit}`}
          className="space-y-4"
          schema={schema}
          defaultValues={{
            unit: product?.unit ?? defaultUnit,
            code: product?.code ?? "",
            name: product?.name ?? "",
            description: product?.description ?? "",
            credits: product?.credits ?? 1000,
            price: product ? minorToInput(product.amount_minor, locale) : "",
            unlimited: product ? product.expires_days == null : false,
            expires_days: product?.expires_days ?? 365,
            plan_uuids: product?.plan_uuids ?? [],
            sort_order: product?.sort_order ?? 0,
            active: product?.active ?? true,
          }}
          onSubmit={async (values) => {
            const body: CreditProductInput = {
              unit: values.unit,
              code: create ? values.code.trim() : product.code,
              name: values.name.trim(),
              description: values.description.trim(),
              credits: values.credits,
              amount_minor: parseMajorToMinor(values.price) ?? 0,
              plan_uuids: values.plan_uuids,
              sort_order: values.sort_order,
              ...(values.unlimited
                ? {}
                : { expires_days: values.expires_days }),
              ...(create ? {} : { active: values.active }),
            };
            await save.mutateAsync({ uuid: product?.uuid, body });
            onClose();
          }}
        >
          {(form) => (
            <>
              {create ? <CodeSuggestion /> : null}
              <UnitPicker disabled={!create} />
              <div className="grid gap-4 sm:grid-cols-2">
                <AppInput
                  name="name"
                  label={t("credit_products.fields.name")}
                  placeholder={t("credit_products.fields.name_placeholder")}
                />
                <AppInput
                  name="code"
                  label={t("credit_products.fields.code")}
                  description={
                    create
                      ? t("credit_products.fields.code_hint")
                      : t("credit_products.fields.code_fixed")
                  }
                  disabled={!create}
                  autoComplete="off"
                  spellCheck={false}
                />
              </div>
              <AppTextarea
                name="description"
                label={t("credit_products.fields.description")}
                description={t("credit_products.fields.description_hint")}
                rows={2}
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <QuantityField />
                <div className="space-y-1.5">
                  <AppInput
                    name="price"
                    inputMode="decimal"
                    label={t("credit_products.fields.price")}
                    description={t("credit_products.fields.price_hint")}
                    placeholder="0,00"
                  />
                  <UnitPriceHint />
                </div>
              </div>
              <ValidityField />
              <AppMultiCombobox
                name="plan_uuids"
                label={t("credit_products.fields.plans")}
                description={t("credit_products.fields.plans_hint")}
                options={planOptions}
                placeholder={t("credit_products.fields.plans_placeholder")}
                emptyText={t("credit_products.fields.plans_empty")}
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <AppInput
                  name="sort_order"
                  type="number"
                  min={0}
                  inputMode="numeric"
                  label={t("credit_products.fields.sort_order")}
                  description={t("credit_products.fields.sort_order_hint")}
                />
                {create ? null : (
                  <AppSwitch
                    name="active"
                    label={t("credit_products.fields.active")}
                    description={t("credit_products.fields.active_hint")}
                    className="sm:pt-6"
                  />
                )}
              </div>
              {create ? null : (
                <p className="text-muted-foreground bg-muted/50 rounded-md px-3 py-2 text-xs">
                  {t("credit_products.fixed_note")}
                </p>
              )}
              <FormActions>
                <Button type="button" variant="outline" onClick={onClose}>
                  {t("common.cancel")}
                </Button>
                <Button type="submit" disabled={form.formState.isSubmitting}>
                  {create
                    ? t("credit_products.create_submit")
                    : t("credit_products.save")}
                </Button>
              </FormActions>
            </>
          )}
        </AppForm>
      </DialogContent>
    </Dialog>
  );
}
