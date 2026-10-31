"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import {
  ArrowLeft,
  ArrowRight,
  Building2,
  Check,
  Coins,
  Globe,
  ListChecks,
  Loader2,
  Package,
  UserRound,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { FormProvider, useForm } from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { publishedPlanOptions } from "@/features/billing/components/platform/org/plan-options";
import type { BillingWalletUnit } from "@/features/billing/types";
import {
  CompanyStep,
  CreditsStep,
  OwnerStep,
  PlanStep,
  RegionStep,
  ReviewStep,
  type OwnerChoice,
} from "@/features/platform-organizations/components/wizard/wizard-steps";
import { usePlanCatalog } from "@/features/platform-organizations/hooks/use-commerce";
import { useCreateOrganization } from "@/features/platform-organizations/hooks/use-organizations";
import { saveCreationNotice } from "@/features/platform-organizations/lib/creation-notice";
import {
  PROFILE_FIELD_KEYS,
  STEP_FIELDS,
  WIZARD_DEFAULTS,
  WIZARD_STEPS,
  createOrganizationWizardSchema,
  normalizeWebsite,
  stepOfApiField,
  type OrganizationWizardValues,
  type WizardStep,
} from "@/features/platform-organizations/schemas/organization-wizard";
import { organizationCommerceService } from "@/features/platform-organizations/services/commerce.service";
import type { CreateOrganizationRequest } from "@/features/platform-organizations/types";
import { isApiError } from "@/lib/api/client";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const STEP_ICONS: Record<WizardStep, typeof Building2> = {
  company: Building2,
  owner: UserRound,
  plan: Package,
  credits: Coins,
  region: Globe,
  review: ListChecks,
};

const STEP_LABEL_KEYS: Record<WizardStep, string> = {
  company: "commerce.wizard.steps.company",
  owner: "commerce.wizard.steps.owner",
  plan: "commerce.wizard.steps.plan",
  credits: "commerce.wizard.steps.credits",
  region: "commerce.wizard.steps.region",
  review: "commerce.wizard.steps.review",
};

const STEP_HINT_KEYS: Record<WizardStep, string> = {
  company: "commerce.wizard.hints.company",
  owner: "commerce.wizard.hints.owner",
  plan: "commerce.wizard.hints.plan",
  credits: "commerce.wizard.hints.credits",
  region: "commerce.wizard.hints.region",
  review: "commerce.wizard.hints.review",
};

/** Server field error codes → wizard copy (server texts are English). */
const FIELD_CODE_KEYS: Record<string, string> = {
  too_long: "commerce.wizard.validation.too_long",
  taken: "commerce.wizard.validation.taken",
  already_member: "commerce.wizard.validation.already_member",
};

const OPENING_UNITS: {
  unit: Extract<BillingWalletUnit, "ai_credits" | "video_minutes">;
  field: "ai_credits" | "video_minutes";
}[] = [
  { unit: "ai_credits", field: "ai_credits" },
  { unit: "video_minutes", field: "video_minutes" },
];

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("organizations.forbidden")}
    />
  );
}

function Stepper({
  current,
  reached,
  onSelect,
}: {
  current: number;
  reached: number;
  onSelect: (index: number) => void;
}) {
  const { t } = useLocale();
  return (
    <nav aria-label={t("commerce.wizard.steps_label")}>
      {/* Phones: compact progress line. */}
      <div className="space-y-2 lg:hidden">
        <div className="flex items-baseline justify-between gap-2 text-sm">
          <span className="font-medium">
            {t(STEP_LABEL_KEYS[WIZARD_STEPS[current]])}
          </span>
          <span className="text-muted-foreground tabular-nums">
            {t("commerce.wizard.step_of", {
              current: current + 1,
              total: WIZARD_STEPS.length,
            })}
          </span>
        </div>
        <div className="bg-muted h-1.5 overflow-hidden rounded-full">
          <div
            className="bg-primary h-full rounded-full transition-[width]"
            style={{ width: `${((current + 1) / WIZARD_STEPS.length) * 100}%` }}
          />
        </div>
      </div>
      {/* Desktop: vertical list, done steps are links back. */}
      <ol className="hidden space-y-1 lg:block">
        {WIZARD_STEPS.map((step, index) => {
          const Icon = STEP_ICONS[step];
          const done = index < current;
          const active = index === current;
          const enabled = index <= reached && !active;
          return (
            <li key={step}>
              <button
                type="button"
                disabled={!enabled}
                aria-current={active ? "step" : undefined}
                onClick={() => onSelect(index)}
                className={cn(
                  "flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-start transition-colors",
                  active && "bg-muted",
                  enabled && "hover:bg-muted/60",
                  !enabled && !active && "cursor-default",
                )}
              >
                <span
                  className={cn(
                    "mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border text-xs",
                    active &&
                      "border-primary bg-primary text-primary-foreground",
                    done && "border-primary text-primary",
                    !active && !done && "text-muted-foreground",
                  )}
                >
                  {done ? (
                    <Check className="size-3.5" />
                  ) : (
                    <Icon className="size-3.5" />
                  )}
                </span>
                <span className="min-w-0">
                  <span
                    className={cn(
                      "block text-sm",
                      active ? "font-semibold" : "font-medium",
                      !active && !done && "text-muted-foreground",
                    )}
                  >
                    {t(STEP_LABEL_KEYS[step])}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {t(STEP_HINT_KEYS[step])}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
  );
}

/**
 * `/platform/organizations/create` (CF-143): six steps (company, owner,
 * plan and trial, opening credits, region, review). The organization is
 * created in one request; opening credits are posted right after with
 * stable idempotency keys, profile problems come back as warnings.
 */
export function OrganizationCreateWizard() {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const create = useCreateOrganization();
  const canReadPlans = can(permissions.billing.plansRead);
  const canCredit = can(permissions.billing.subscriptionsWrite);
  const plans = usePlanCatalog(canReadPlans);
  const options = useMemo(
    () => publishedPlanOptions(plans.data?.items),
    [plans.data],
  );
  const schema = useMemo(() => createOrganizationWizardSchema(t), [t]);
  const form = useForm<OrganizationWizardValues>({
    resolver: zodResolver(schema as never),
    defaultValues: {
      ...WIZARD_DEFAULTS,
      credit_reason: t("commerce.wizard.credits.default_reason"),
    },
    mode: "onTouched",
  });
  const [owners] = useState(() => new Map<string, OwnerChoice>());
  const [current, setCurrent] = useState(0);
  const [reached, setReached] = useState(0);
  const [submitting, setSubmitting] = useState(false);
  const cardRef = useRef<HTMLDivElement>(null);
  const step = WIZARD_STEPS[current];
  const last = current === WIZARD_STEPS.length - 1;

  // Move focus to the step heading so keyboard and screen reader users
  // land on the new step.
  const focusPending = useRef(false);
  useEffect(() => {
    if (!focusPending.current) return;
    focusPending.current = false;
    cardRef.current
      ?.querySelector<HTMLElement>("[data-wizard-heading]")
      ?.focus();
  }, [current]);

  const goTo = (index: number) => {
    focusPending.current = true;
    setCurrent(index);
    setReached((r) => Math.max(r, index));
  };
  const goToStep = (target: WizardStep) => goTo(WIZARD_STEPS.indexOf(target));

  const next = async () => {
    const ok = await form.trigger(STEP_FIELDS[step] as never);
    if (ok) goTo(current + 1);
  };

  const applyServerErrors = (
    error: unknown,
    values: OrganizationWizardValues,
  ) => {
    if (!isApiError(error)) return;
    let first: number | null = null;
    const mark = (target: WizardStep, field: string, message: string) => {
      form.setError(field as never, { type: "server", message });
      const index = WIZARD_STEPS.indexOf(target);
      if (first === null || index < first) first = index;
    };
    for (const detail of error.details) {
      if (!detail.field) continue;
      const target = stepOfApiField(detail.field);
      if (!target) continue;
      const key = detail.code ? FIELD_CODE_KEYS[detail.code] : undefined;
      mark(
        target.step,
        target.field,
        key ? t(key) : t("commerce.wizard.validation.server"),
      );
    }
    if (
      first === null &&
      error.isNotFound &&
      values.owner_mode === "existing"
    ) {
      mark(
        "owner",
        "owner_user_uuid",
        t("commerce.wizard.validation.owner_missing"),
      );
    }
    if (first !== null) goTo(first);
  };

  const submit = async (values: OrganizationWizardValues) => {
    const company = {
      ...values.company,
      website: normalizeWebsite(values.company.website),
    };
    const hasCompany = Object.values(company).some(Boolean);
    const body: CreateOrganizationRequest = {
      name: values.name.trim(),
      ...(values.slug ? { slug: values.slug } : {}),
      ...(values.owner_mode === "existing"
        ? { owner_user_uuid: values.owner_user_uuid }
        : { owner_email: values.owner_email.trim() }),
      ...(values.demo_data
        ? { demo_data: true }
        : values.plan_mode === "choose" && values.price_uuid
          ? { trial_price_uuid: values.price_uuid }
          : {}),
      locale: values.locale,
      timezone: values.timezone,
      country_code: values.country_code,
      ...(hasCompany ? { company } : {}),
    };

    setSubmitting(true);
    let result;
    try {
      result = await create.mutateAsync(body);
    } catch (error) {
      applyServerErrors(error, values);
      setSubmitting(false);
      return;
    }
    const uuid = result.organization.uuid;

    // Opening credits: one ledger entry per unit, keyed by organization and
    // unit so a retry (step-up prompt, double click) never doubles them.
    const failedCredits: string[] = [];
    let posted = 0;
    if (canCredit) {
      for (const { unit, field } of OPENING_UNITS) {
        const amount = Number(values[field] || 0);
        if (amount <= 0) continue;
        try {
          await organizationCommerceService.walletEntry(
            uuid,
            {
              unit,
              delta: amount,
              reason: values.credit_reason,
              category: "sales",
              idempotency_key: `opening:${uuid}:${unit}`,
            },
            { silent: true },
          );
          posted += 1;
        } catch {
          failedCredits.push(unit);
        }
      }
    }
    if (posted > 0) appToast.success(t("commerce.wizard.toast.credits_added"));
    if (failedCredits.length) {
      appToast.warning(t("commerce.wizard.notice.credits"));
    }
    const profileFields = (result.profile_errors ?? [])
      .map((e) => e.field ?? "")
      .filter(Boolean);
    if (profileFields.length) {
      appToast.warning(
        t("commerce.wizard.notice.profile", {
          fields: profileFields
            .map((f) => (PROFILE_FIELD_KEYS[f] ? t(PROFILE_FIELD_KEYS[f]) : f))
            .join(", "),
        }),
      );
    }
    if (result.demo) appToast.success(t("commerce.wizard.demo.toast"));
    saveCreationNotice(uuid, {
      profileFields,
      failedCredits,
      demo: result.demo ?? null,
    });
    router.push(routes.platform.organizations.detail(uuid));
  };

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (submitting) return;
    if (!last) {
      await next();
      return;
    }
    const ok = await form.trigger();
    if (!ok) {
      const errors = form.formState.errors as Record<string, unknown>;
      const index = WIZARD_STEPS.findIndex((s) =>
        STEP_FIELDS[s].some((f) => errors[f.split(".")[0]]),
      );
      if (index >= 0) goTo(index);
      return;
    }
    await submit(form.getValues());
  };

  return (
    <EntityPage
      title={t("commerce.wizard.title")}
      description={t("commerce.wizard.description")}
      permission={permissions.organizations.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: t("commerce.wizard.title") },
      ]}
    >
      <FormProvider {...form}>
        <form
          noValidate
          onSubmit={onSubmit}
          className="mx-auto grid w-full max-w-6xl gap-6 lg:grid-cols-[260px_minmax(0,1fr)]"
        >
          <aside className="lg:sticky lg:top-20 lg:self-start">
            <Stepper
              current={current}
              reached={reached}
              onSelect={(index) => goTo(index)}
            />
          </aside>
          <div className="min-w-0 space-y-4">
            <div
              ref={cardRef}
              className="bg-card text-card-foreground rounded-xl border p-5 sm:p-6"
            >
              {step === "company" ? <CompanyStep /> : null}
              {step === "owner" ? <OwnerStep owners={owners} /> : null}
              {step === "plan" ? (
                <PlanStep
                  options={options}
                  isLoading={canReadPlans && plans.isLoading}
                  canReadPlans={canReadPlans}
                />
              ) : null}
              {step === "credits" ? (
                canCredit ? (
                  <CreditsStep />
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("commerce.wizard.credits.no_access")}
                  </p>
                )
              ) : null}
              {step === "region" ? <RegionStep /> : null}
              {step === "review" ? (
                <ReviewStep options={options} owners={owners} goTo={goToStep} />
              ) : null}
            </div>
            <div className="bg-card sticky bottom-3 z-10 flex flex-wrap items-center justify-between gap-2 rounded-xl border px-4 py-3 shadow-sm">
              <Button
                type="button"
                variant="ghost"
                onClick={() => router.push(routes.platform.organizations.root)}
                disabled={submitting}
              >
                {t("form.cancel")}
              </Button>
              <div className="flex items-center gap-2">
                {current > 0 ? (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => goTo(current - 1)}
                    disabled={submitting}
                  >
                    <ArrowLeft className="size-4" />
                    {t("commerce.wizard.back")}
                  </Button>
                ) : null}
                <Button type="submit" disabled={submitting}>
                  {submitting ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : null}
                  {last
                    ? t("commerce.wizard.create")
                    : t("commerce.wizard.next")}
                  {!last ? <ArrowRight className="size-4" /> : null}
                </Button>
              </div>
            </div>
          </div>
        </form>
      </FormProvider>
    </EntityPage>
  );
}
