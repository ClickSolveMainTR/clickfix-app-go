"use client";

import {
  Bot,
  Building2,
  CalendarClock,
  CircleDollarSign,
  Database,
  Globe,
  Hash,
  Landmark,
  Link2,
  Mail,
  MailPlus,
  MapPin,
  Phone,
  Sparkles,
  UserRound,
  Video,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Controller, useFormContext, useWatch } from "react-hook-form";

import {
  AppCombobox,
  AppInput,
  AppSelect,
  AppTextarea,
  FormFieldShell,
  type ComboboxOption,
} from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  ChoiceCard,
  ChoiceCards,
} from "@/features/billing/components/platform/org/choice-cards";
import { usePriceLabel } from "@/features/billing/components/platform/org/change-plan-dialog";
import {
  findPrice,
  type PlanOption,
} from "@/features/billing/components/platform/org/plan-options";
import { formatCount } from "@/features/billing/lib/money";
import { slugFromName } from "@/features/platform-organizations/lib/slug";
import {
  COMMON_COUNTRIES,
  COMMON_TIMEZONES,
  countryName,
  localeName,
  timezoneCity,
  timezoneOffset,
} from "@/features/platform-organizations/lib/region";
import type {
  OrganizationWizardValues,
  WizardStep,
} from "@/features/platform-organizations/schemas/organization-wizard";
import { userFullName } from "@/features/users/lib/user-display";
import { usersService } from "@/features/users/services/users.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type OwnerChoice = { name: string; email: string };

function StepIntro({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <div className="space-y-1">
      <h2
        className="font-display text-lg font-semibold tracking-tight outline-none"
        tabIndex={-1}
        data-wizard-heading
      >
        {title}
      </h2>
      <p className="text-muted-foreground text-sm">{description}</p>
    </div>
  );
}

function SubHeading({ children }: { children: ReactNode }) {
  return (
    <h3 className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
      {children}
    </h3>
  );
}

/** Keeps the slug in sync with the name until the operator edits the slug. */
function SlugAutofill() {
  const { setValue, getFieldState, formState } =
    useFormContext<OrganizationWizardValues>();
  const name = useWatch<OrganizationWizardValues, "name">({ name: "name" });
  const touched = useRef(false);
  const slugDirty = getFieldState("slug", formState).isDirty;
  useEffect(() => {
    if (slugDirty) touched.current = true;
  }, [slugDirty]);
  useEffect(() => {
    if (touched.current) return;
    setValue("slug", slugFromName(name ?? ""), { shouldDirty: false });
  }, [name, setValue]);
  return null;
}

export function CompanyStep() {
  const { t } = useLocale();
  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.company.title")}
        description={t("commerce.wizard.company.description")}
      />
      <SlugAutofill />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="name"
          label={t("commerce.wizard.company.name")}
          placeholder={t("commerce.wizard.company.name_placeholder")}
          startIcon={Building2}
          autoFocus
        />
        <AppInput
          name="slug"
          label={t("commerce.wizard.company.slug")}
          description={t("commerce.wizard.company.slug_hint")}
          startIcon={Link2}
        />
      </div>
      <div className="space-y-4 border-t pt-5">
        <div className="flex flex-wrap items-center gap-2">
          <SubHeading>{t("commerce.wizard.company.legal_title")}</SubHeading>
          <Badge variant="outline" className="font-normal">
            {t("commerce.org.common.optional")}
          </Badge>
        </div>
        <p className="text-muted-foreground -mt-2 text-sm">
          {t("commerce.wizard.company.legal_hint")}
        </p>
        <div className="grid gap-4 sm:grid-cols-2">
          <AppInput
            name="company.legal_name"
            label={t("commerce.org.company.legal_name")}
            placeholder={t("commerce.wizard.company.legal_name_placeholder")}
            startIcon={Landmark}
            className="sm:col-span-2"
          />
          <AppInput
            name="company.tax_office"
            label={t("commerce.org.company.tax_office")}
            placeholder={t("commerce.wizard.company.tax_office_placeholder")}
          />
          <AppInput
            name="company.tax_number"
            label={t("commerce.org.company.tax_number")}
            description={t("commerce.wizard.company.tax_number_hint")}
            inputMode="numeric"
            startIcon={Hash}
          />
          <AppTextarea
            name="company.address"
            label={t("commerce.org.company.address")}
            className="sm:col-span-2"
          />
          <AppInput
            name="company.city"
            label={t("commerce.org.company.city")}
            startIcon={MapPin}
          />
          <AppInput
            name="company.phone"
            label={t("commerce.org.company.phone")}
            type="tel"
            placeholder="+90 212 000 00 00"
            startIcon={Phone}
          />
          <AppInput
            name="company.email"
            label={t("commerce.org.company.email")}
            type="email"
            placeholder="muhasebe@firma.com"
            startIcon={Mail}
          />
          <AppInput
            name="company.website"
            label={t("commerce.org.company.website")}
            placeholder="www.firma.com"
            startIcon={Globe}
          />
        </div>
      </div>
    </div>
  );
}

/**
 * Account search for the owner: name with the e-mail underneath (the shared
 * user picker shows internal ids, which operators do not need here).
 */
function OwnerPicker({ owners }: { owners: Map<string, OwnerChoice> }) {
  const { t } = useLocale();
  const [options, setOptions] = useState<ComboboxOption[]>(() =>
    [...owners.entries()].map(([value, o]) => ({
      value,
      label: o.name,
      description: o.email,
    })),
  );
  const loadOptions = useCallback(
    async (query: string) => {
      const result = await usersService.list({
        limit: 20,
        offset: 0,
        q: query.trim() || undefined,
        status: "active",
      });
      const next = result.items.map((user): ComboboxOption => {
        const name = userFullName(user) || user.email;
        owners.set(user.uuid, { name, email: user.email });
        return { value: user.uuid, label: name, description: user.email };
      });
      setOptions((prev) => {
        const byValue = new Map(prev.map((o) => [o.value, o]));
        for (const o of next) byValue.set(o.value, o);
        return [...byValue.values()];
      });
      return next;
    },
    [owners],
  );
  return (
    <AppCombobox
      name="owner_user_uuid"
      label={t("commerce.wizard.owner.user")}
      description={t("commerce.wizard.owner.user_hint")}
      loadOptions={loadOptions}
      options={options}
      placeholder={t("commerce.wizard.owner.user_placeholder")}
      searchPlaceholder={t("commerce.wizard.owner.user_search")}
      emptyText={t("commerce.wizard.owner.user_empty")}
    />
  );
}

export function OwnerStep({ owners }: { owners: Map<string, OwnerChoice> }) {
  const { t } = useLocale();
  const { control } = useFormContext<OrganizationWizardValues>();
  const mode = useWatch<OrganizationWizardValues, "owner_mode">({
    name: "owner_mode",
  });
  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.owner.title")}
        description={t("commerce.wizard.owner.description")}
      />
      <Controller
        control={control}
        name="owner_mode"
        render={({ field }) => (
          <ChoiceCards
            value={field.value}
            onValueChange={field.onChange}
            className="sm:grid-cols-2"
            aria-label={t("commerce.wizard.owner.title")}
          >
            <ChoiceCard
              value="existing"
              icon={<UserRound className="size-4" />}
              title={t("commerce.wizard.owner.existing")}
              description={t("commerce.wizard.owner.existing_hint")}
            />
            <ChoiceCard
              value="invite"
              icon={<MailPlus className="size-4" />}
              title={t("commerce.wizard.owner.invite")}
              description={t("commerce.wizard.owner.invite_hint")}
            />
          </ChoiceCards>
        )}
      />
      {mode === "existing" ? (
        <OwnerPicker owners={owners} />
      ) : (
        <div className="space-y-4">
          <AppInput
            name="owner_email"
            type="email"
            autoComplete="off"
            label={t("commerce.wizard.owner.email")}
            placeholder="yetkili@firma.com"
            startIcon={Mail}
          />
          <div className="bg-muted/50 space-y-1 rounded-lg px-4 py-3 text-sm">
            <p className="font-medium">
              {t("commerce.wizard.owner.invite_info_title")}
            </p>
            <p className="text-muted-foreground">
              {t("commerce.wizard.owner.invite_info")}
            </p>
          </div>
        </div>
      )}
    </div>
  );
}

/** What happens to the subscription with the chosen plan. */
export function PlanOutcome({
  option,
}: {
  option: (PlanOption & { price: { uuid: string } }) | null;
}) {
  const { t } = useLocale();
  const days = option?.version.trial_days ?? 0;
  const text = !option
    ? t("commerce.wizard.plan.outcome_default")
    : days > 0
      ? t("commerce.wizard.plan.outcome_trial", { days })
      : t("commerce.wizard.plan.outcome_pending");
  return (
    <div
      className={cn(
        "flex items-start gap-3 rounded-lg px-4 py-3 text-sm",
        option && days === 0
          ? "border border-amber-500/30 bg-amber-500/10"
          : "bg-primary/5 border-primary/20 border",
      )}
      aria-live="polite"
    >
      <CalendarClock className="mt-0.5 size-4 shrink-0" />
      <p>{text}</p>
    </div>
  );
}

/** "Fill with demo data": seeds every module on the all-features plan. */
function DemoDataToggle() {
  const { t } = useLocale();
  const { control, setValue } = useFormContext<OrganizationWizardValues>();
  return (
    <Controller
      control={control}
      name="demo_data"
      render={({ field }) => (
        <label
          htmlFor="wizard-demo-data"
          className={cn(
            "flex cursor-pointer items-start gap-3 rounded-xl border p-4 transition-colors",
            field.value ? "border-primary bg-primary/5" : "hover:bg-muted/40",
          )}
        >
          <Database className="mt-0.5 size-4 shrink-0" />
          <span className="min-w-0 flex-1 space-y-1">
            <span className="block text-sm font-medium">
              {t("commerce.wizard.demo.title")}
            </span>
            <span className="text-muted-foreground block text-sm">
              {t("commerce.wizard.demo.hint")}
            </span>
          </span>
          <Switch
            id="wizard-demo-data"
            checked={field.value}
            onCheckedChange={(on) => {
              field.onChange(on);
              if (on) {
                setValue("plan_mode", "default");
                setValue("price_uuid", "", { shouldValidate: true });
              }
            }}
          />
        </label>
      )}
    />
  );
}

/** Outcome line when the demo plan is used. */
function DemoPlanOutcome() {
  const { t } = useLocale();
  return (
    <div
      className="bg-primary/5 border-primary/20 flex items-start gap-3 rounded-lg border px-4 py-3 text-sm"
      aria-live="polite"
    >
      <CalendarClock className="mt-0.5 size-4 shrink-0" />
      <p>{t("commerce.wizard.demo.plan_outcome")}</p>
    </div>
  );
}

export function PlanStep({
  options,
  isLoading,
  canReadPlans,
}: {
  options: PlanOption[];
  isLoading: boolean;
  canReadPlans: boolean;
}) {
  const { t } = useLocale();
  const priceLabel = usePriceLabel();
  const { control, setValue, formState } =
    useFormContext<OrganizationWizardValues>();
  const mode = useWatch<OrganizationWizardValues, "plan_mode">({
    name: "plan_mode",
  });
  const priceUuid = useWatch<OrganizationWizardValues, "price_uuid">({
    name: "price_uuid",
  });
  const demo = useWatch<OrganizationWizardValues, "demo_data">({
    name: "demo_data",
  });
  const selected =
    mode === "choose" && priceUuid ? findPrice(options, priceUuid) : null;
  const value = mode === "default" ? "default" : priceUuid;
  const error = formState.errors.price_uuid?.message;

  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.plan.title")}
        description={t("commerce.wizard.plan.description")}
      />
      <DemoDataToggle />
      {demo ? (
        <DemoPlanOutcome />
      ) : isLoading ? (
        <div className="grid gap-3">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : (
        <Controller
          control={control}
          name="price_uuid"
          render={() => (
            <FormFieldShell name="price_uuid" error={error}>
              <ChoiceCards
                value={value}
                onValueChange={(v) => {
                  if (v === "default") {
                    setValue("plan_mode", "default");
                    setValue("price_uuid", "", { shouldValidate: true });
                  } else {
                    setValue("plan_mode", "choose");
                    setValue("price_uuid", v, { shouldValidate: true });
                  }
                }}
                aria-label={t("commerce.wizard.plan.title")}
                aria-invalid={Boolean(error)}
              >
                <ChoiceCard
                  value="default"
                  icon={<Sparkles className="size-4" />}
                  title={t("commerce.wizard.plan.default")}
                  description={t("commerce.wizard.plan.default_hint")}
                />
                {options.map((option) => (
                  <div
                    key={option.plan.uuid}
                    className="space-y-2 rounded-xl border border-dashed p-3"
                  >
                    <div className="flex flex-wrap items-baseline justify-between gap-2 px-1">
                      <span className="font-medium">{option.plan.name}</span>
                      <span className="text-muted-foreground text-xs">
                        {option.version.trial_days
                          ? t("commerce.org.plan.trial_days", {
                              days: option.version.trial_days,
                            })
                          : t("commerce.org.plan.no_trial")}
                      </span>
                    </div>
                    {option.plan.description ? (
                      <p className="text-muted-foreground px-1 text-sm">
                        {option.plan.description}
                      </p>
                    ) : null}
                    <div className="grid gap-2 sm:grid-cols-2">
                      {option.prices.map((price) => (
                        <ChoiceCard
                          key={price.uuid}
                          value={price.uuid}
                          className="p-3"
                          icon={<CircleDollarSign className="size-4" />}
                          title={t(
                            price.interval === "yearly"
                              ? "commerce.org.price.yearly"
                              : "commerce.org.price.monthly",
                          )}
                          description={priceLabel(price)}
                        />
                      ))}
                    </div>
                  </div>
                ))}
              </ChoiceCards>
            </FormFieldShell>
          )}
        />
      )}
      {!demo && !isLoading && options.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {canReadPlans
            ? t("commerce.wizard.plan.none")
            : t("commerce.wizard.plan.no_access")}
        </p>
      ) : null}
      {demo ? null : <PlanOutcome option={selected} />}
    </div>
  );
}

export function CreditsStep() {
  const { t } = useLocale();
  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.credits.title")}
        description={t("commerce.wizard.credits.description")}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="ai_credits"
          label={t("commerce.org.units.ai_credits")}
          inputMode="numeric"
          placeholder="0"
          startIcon={Bot}
          description={t("commerce.wizard.credits.ai_hint")}
        />
        <AppInput
          name="video_minutes"
          label={t("commerce.org.units.video_minutes")}
          inputMode="numeric"
          placeholder="0"
          startIcon={Video}
          description={t("commerce.wizard.credits.video_hint")}
        />
        <AppInput
          name="credit_reason"
          label={t("commerce.org.common.reason")}
          description={t("commerce.wizard.credits.reason_hint")}
          className="sm:col-span-2"
        />
      </div>
      <p className="text-muted-foreground text-sm">
        {t("commerce.wizard.credits.later")}
      </p>
    </div>
  );
}

export function RegionStep() {
  const { t, locale } = useLocale();
  const { control } = useFormContext<OrganizationWizardValues>();
  const timezones = useMemo(
    () =>
      COMMON_TIMEZONES.map((zone): ComboboxOption => ({
        value: zone,
        label: `${timezoneCity(zone)} (${timezoneOffset(zone)})`,
        description: zone,
      })),
    [],
  );
  const countries = useMemo(
    () =>
      COMMON_COUNTRIES.map((code) => ({
        value: code,
        label: countryName(code, locale),
      })),
    [locale],
  );
  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.region.title")}
        description={t("commerce.wizard.region.description")}
      />
      <Controller
        control={control}
        name="locale"
        render={({ field }) => (
          <FormFieldShell
            name="locale"
            label={t("commerce.wizard.region.locale")}
            description={t("commerce.wizard.region.locale_hint")}
          >
            <ChoiceCards
              value={field.value}
              onValueChange={field.onChange}
              className="grid-cols-2"
              aria-label={t("commerce.wizard.region.locale")}
            >
              {(["tr", "en"] as const).map((code) => (
                <ChoiceCard
                  key={code}
                  value={code}
                  className="p-3"
                  title={localeName(code, locale)}
                />
              ))}
            </ChoiceCards>
          </FormFieldShell>
        )}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppCombobox
          name="timezone"
          label={t("commerce.wizard.region.timezone")}
          description={t("commerce.wizard.region.timezone_hint")}
          options={timezones}
          searchPlaceholder={t("commerce.wizard.region.timezone_search")}
          emptyText={t("commerce.wizard.region.timezone_empty")}
        />
        <AppSelect
          name="country_code"
          label={t("commerce.wizard.region.country")}
          description={t("commerce.wizard.region.country_hint")}
          options={countries}
        />
      </div>
    </div>
  );
}

function ReviewSection({
  title,
  onEdit,
  children,
}: {
  title: string;
  onEdit: () => void;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <section className="space-y-3 rounded-xl border p-4">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{title}</h3>
        <Button type="button" variant="ghost" size="sm" onClick={onEdit}>
          {t("commerce.org.common.edit")}
        </Button>
      </div>
      <dl className="grid gap-3 sm:grid-cols-2">{children}</dl>
    </section>
  );
}

function ReviewItem({
  label,
  value,
  wide,
}: {
  label: string;
  value: ReactNode;
  wide?: boolean;
}) {
  return (
    <div className={cn("min-w-0 space-y-0.5", wide && "sm:col-span-2")}>
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{value || "—"}</dd>
    </div>
  );
}

export function ReviewStep({
  options,
  owners,
  goTo,
}: {
  options: PlanOption[];
  owners: Map<string, OwnerChoice>;
  goTo: (step: WizardStep) => void;
}) {
  const { t, locale } = useLocale();
  const priceLabel = usePriceLabel();
  const values = useWatch<OrganizationWizardValues>();
  const company = values.company ?? {};
  const companyLines = [
    company.legal_name,
    [company.tax_office, company.tax_number].filter(Boolean).join(" · "),
    [company.address, company.city].filter(Boolean).join(", "),
    [company.phone, company.email, company.website].filter(Boolean).join(" · "),
  ].filter(Boolean);
  const owner =
    values.owner_mode === "existing"
      ? owners.get(values.owner_user_uuid ?? "")
      : null;
  const selected =
    values.plan_mode === "choose" && values.price_uuid
      ? findPrice(options, values.price_uuid)
      : null;
  const credits = Number(values.ai_credits || 0);
  const minutes = Number(values.video_minutes || 0);

  return (
    <div className="space-y-6">
      <StepIntro
        title={t("commerce.wizard.review.title")}
        description={t("commerce.wizard.review.description")}
      />
      <div className="grid gap-4">
        <ReviewSection
          title={t("commerce.wizard.steps.company")}
          onEdit={() => goTo("company")}
        >
          <ReviewItem
            label={t("commerce.wizard.company.name")}
            value={values.name}
          />
          <ReviewItem
            label={t("commerce.wizard.company.slug")}
            value={
              values.slug ? (
                <code className="text-xs">{values.slug}</code>
              ) : (
                t("commerce.wizard.review.slug_auto")
              )
            }
          />
          <ReviewItem
            wide
            label={t("commerce.wizard.company.legal_title")}
            value={
              companyLines.length ? (
                <span className="flex flex-col">
                  {companyLines.map((line) => (
                    <span key={line}>{line}</span>
                  ))}
                </span>
              ) : (
                t("commerce.wizard.review.company_empty")
              )
            }
          />
        </ReviewSection>
        <ReviewSection
          title={t("commerce.wizard.steps.owner")}
          onEdit={() => goTo("owner")}
        >
          {values.owner_mode === "existing" ? (
            <ReviewItem
              wide
              label={t("commerce.wizard.owner.existing")}
              value={owner ? `${owner.name} · ${owner.email}` : "—"}
            />
          ) : (
            <ReviewItem
              wide
              label={t("commerce.wizard.owner.invite")}
              value={t("commerce.wizard.review.invite_value", {
                email: values.owner_email ?? "",
              })}
            />
          )}
        </ReviewSection>
        <ReviewSection
          title={t("commerce.wizard.steps.plan")}
          onEdit={() => goTo("plan")}
        >
          <ReviewItem
            wide
            label={t("commerce.wizard.steps.plan")}
            value={
              values.demo_data
                ? t("commerce.wizard.demo.plan_name")
                : selected
                  ? `${selected.plan.name} · ${priceLabel(selected.price)}`
                  : t("commerce.wizard.plan.default")
            }
          />
          <ReviewItem
            wide
            label={t("commerce.wizard.demo.title")}
            value={
              values.demo_data
                ? t("commerce.wizard.demo.review_on")
                : t("commerce.wizard.demo.review_off")
            }
          />
          <div className="sm:col-span-2">
            {values.demo_data ? (
              <DemoPlanOutcome />
            ) : (
              <PlanOutcome option={selected} />
            )}
          </div>
        </ReviewSection>
        <ReviewSection
          title={t("commerce.wizard.steps.credits")}
          onEdit={() => goTo("credits")}
        >
          {credits > 0 || minutes > 0 ? (
            <>
              <ReviewItem
                label={t("commerce.org.units.ai_credits")}
                value={formatCount(credits, locale)}
              />
              <ReviewItem
                label={t("commerce.org.units.video_minutes")}
                value={formatCount(minutes, locale)}
              />
              <ReviewItem
                wide
                label={t("commerce.org.common.reason")}
                value={values.credit_reason}
              />
            </>
          ) : (
            <ReviewItem
              wide
              label={t("commerce.wizard.steps.credits")}
              value={t("commerce.wizard.review.no_credits")}
            />
          )}
        </ReviewSection>
        <ReviewSection
          title={t("commerce.wizard.steps.region")}
          onEdit={() => goTo("region")}
        >
          <ReviewItem
            label={t("commerce.wizard.region.locale")}
            value={localeName(values.locale ?? "tr", locale)}
          />
          <ReviewItem
            label={t("commerce.wizard.region.country")}
            value={countryName(values.country_code ?? "", locale)}
          />
          <ReviewItem
            wide
            label={t("commerce.wizard.region.timezone")}
            value={
              values.timezone
                ? `${timezoneCity(values.timezone)} (${timezoneOffset(values.timezone)})`
                : "—"
            }
          />
        </ReviewSection>
      </div>
    </div>
  );
}
