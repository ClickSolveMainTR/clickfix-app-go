"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { signIn, signOut, useSession } from "next-auth/react";
import { useQuery } from "@tanstack/react-query";
import { ArrowRight, Check, Loader2, RotateCw } from "lucide-react";
import { useEffect, useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";

import { BrandLogo } from "@/components/brand-logos";
import { AppForm, AppInput, AppPassword } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { routes } from "@/config/routes";
import {
  createSignupCodeSchema,
  createSignupSchema,
  type SignupCodeValues,
  type SignupFormValues,
} from "@/features/auth/schemas";
import { SelectedPlanNotice } from "@/features/onboarding/components/selected-plan-notice";
import {
  openOrganizationErrorKey,
  useOpenOrganization,
} from "@/features/onboarding/hooks/use-open-organization";
import { writeSignupCompany } from "@/features/onboarding/lib/signup-company";
import { isApiError } from "@/lib/api";
import { defaultHomeForUser, isPlatformUser } from "@/lib/auth/types";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";
import { fetchAppPublicConfig } from "@/services/app-config.service";

import { AuthCard } from "./auth-card";
import { SignupClosed } from "./signup-closed";

type OAuthId = "github" | "google" | "facebook" | "apple";
type Step = "account" | "verify" | "organization";
type Account = { email: string; company: string; signedIn: boolean };

const STEPS: Step[] = ["account", "verify", "organization"];

function Stepper({ current }: { current: Step }) {
  const { t } = useLocale();
  const index = STEPS.indexOf(current);
  return (
    <ol
      className="mb-5 flex items-center gap-2"
      aria-label={t("signup.steps.label")}
    >
      {STEPS.map((step, i) => (
        <li
          key={step}
          aria-current={i === index ? "step" : undefined}
          className="flex flex-1 items-center gap-2"
        >
          <span
            className={cn(
              "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold tabular-nums",
              i < index && "border-primary bg-primary text-primary-foreground",
              i === index && "border-primary text-primary",
              i > index && "text-muted-foreground",
            )}
          >
            {i < index ? <Check className="size-3.5" aria-hidden /> : i + 1}
          </span>
          <span
            className={cn(
              "truncate text-xs",
              i === index ? "font-medium" : "text-muted-foreground",
            )}
          >
            {t(`signup.steps.${step}`)}
          </span>
          {i < STEPS.length - 1 ? (
            <span className="bg-border hidden h-px flex-1 sm:block" />
          ) : null}
        </li>
      ))}
    </ol>
  );
}

function Consent() {
  const { t } = useLocale();
  const link = "underline underline-offset-4 hover:text-foreground";
  return (
    <p className="text-muted-foreground text-center text-xs leading-relaxed">
      {t("signup.form.consent_prefix")}
      <Link
        href={routes.public.legal("terms")}
        target="_blank"
        className={link}
      >
        {t("signup.form.consent_terms")}
      </Link>
      {t("signup.form.consent_middle")}
      <Link
        href={routes.public.legal("data-protection")}
        target="_blank"
        className={link}
      >
        {t("signup.form.consent_kvkk")}
      </Link>
      {t("signup.form.consent_suffix")}
    </p>
  );
}

/**
 * Self-serve sign-up at `/register` (CF-143), used while the platform sign-up
 * mode is `open`: account (with the company name) → e-mail code →
 * the organization opens with the trial on the plan picked on the pricing
 * page → AI-assisted set-up (`/getting-started`, CF-140). Interrupted sign-ups
 * continue on `/setup`, where the company name is filled in again.
 */
export function RegisterForm() {
  const { t, locale } = useLocale();
  const router = useRouter();
  const { bootstrapped, isAuthenticated, user, markAuthenticated } = useAuth();
  const { status: sessionStatus } = useSession();
  const plan = useOpenOrganization(useSearchParams().get("plan"));
  const [step, setStep] = useState<Step>("account");
  const [account, setAccount] = useState<Account | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [oauthPending, setOauthPending] = useState<OAuthId | null>(null);
  const [resend, setResend] = useState<"idle" | "sending" | "sent">("idle");
  const [company, setCompany] = useState("");
  const [orgError, setOrgError] = useState<string | null>(null);
  const schema = useMemo(() => createSignupSchema(t), [t]);
  const codeSchema = useMemo(() => createSignupCodeSchema(t), [t]);
  const started = account !== null || pending;

  const { data: appConfig, isLoading } = useQuery({
    queryKey: ["app", "config", locale],
    queryFn: () => fetchAppPublicConfig(locale),
  });

  // Someone already signed in does not need a new account.
  const signedInVisitor = !started && bootstrapped && isAuthenticated && user;
  useEffect(() => {
    if (!signedInVisitor) return;
    router.replace(
      signedInVisitor.organizations.length > 0 ||
        isPlatformUser(signedInVisitor)
        ? defaultHomeForUser(signedInVisitor)
        : routes.cms.onboarding,
    );
  }, [router, signedInVisitor]);

  const methods = appConfig?.auth_methods;
  const passwordRegister = Boolean(methods?.password.register);
  const oauthRegister = [
    {
      id: "github" as const,
      enabled: Boolean(methods?.github.register),
      icon: <BrandLogo slug="github" className="size-4" decorative />,
      label: t("auth.github.sign_up"),
      pendingLabel: t("auth.github.signing_in"),
    },
    {
      id: "google" as const,
      enabled: Boolean(methods?.google.register),
      icon: <BrandLogo slug="google" className="size-4" decorative />,
      label: t("auth.google.sign_up"),
      pendingLabel: t("auth.google.signing_in"),
    },
    {
      id: "facebook" as const,
      enabled: Boolean(methods?.facebook.register),
      icon: <BrandLogo slug="facebook" className="size-4" decorative />,
      label: t("auth.facebook.sign_up"),
      pendingLabel: t("auth.facebook.signing_in"),
    },
    {
      id: "apple" as const,
      enabled: Boolean(methods?.apple.register),
      icon: <BrandLogo slug="apple" className="size-4" decorative />,
      label: t("auth.apple.sign_up"),
      pendingLabel: t("auth.apple.signing_in"),
    },
  ];
  const anyOAuth = oauthRegister.some((button) => button.enabled);

  if (signedInVisitor) {
    return (
      <div className="text-muted-foreground text-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (!isLoading && step === "account" && !appConfig?.registration_enabled) {
    return <SignupClosed />;
  }

  if (!isLoading && step === "account" && !passwordRegister && !anyOAuth) {
    return (
      <AuthCard
        title={t("auth.register.disabled_title")}
        description={t("auth.register.disabled_description")}
      >
        <Button asChild className="w-full">
          <Link href={routes.guest.login}>{t("auth.back_to_login")}</Link>
        </Button>
      </AuthCard>
    );
  }

  const openOrganization = async (name: string) => {
    setOrgError(null);
    setStep("organization");
    try {
      const { payFirst } = await plan.open(name);
      router.replace(
        payFirst ? `${routes.cms.billing}?pay=trial` : routes.cms.smartSetup,
      );
    } catch (err) {
      setCompany(name);
      setOrgError(t(openOrganizationErrorKey(err)));
    }
  };

  /** After the code (or "later"): open the organization while signed in. */
  const continueAfterVerify = async () => {
    if (!account) return;
    if (!account.signedIn) {
      toast.message(t("signup.verify.sign_in_to_continue"));
      router.replace(
        `${routes.guest.login}?next=${encodeURIComponent(routes.cms.onboarding)}`,
      );
      return;
    }
    await openOrganization(account.company);
  };

  const onSubmit = async (values: SignupFormValues) => {
    setFormError(null);
    setPending(true);
    writeSignupCompany(values.company);
    try {
      await authService.register({
        email: values.email,
        password: values.password,
        name: values.name,
        surname: values.surname,
      });
    } catch (error) {
      setPending(false);
      if (isApiError(error) && error.status === 409) {
        setFormError(t("signup.form.email_taken"));
      } else if (isApiError(error)) {
        setFormError(error.message || t("auth.register.error"));
      } else {
        setFormError(t("auth.register.error"));
      }
      return;
    }
    // Sign in right away so the code can be re-sent and the organization
    // opened under this account.
    const result = await signIn("credentials", {
      email: values.email,
      password: values.password,
      redirect: false,
    });
    const signedIn = !result?.error;
    if (signedIn) markAuthenticated();
    setAccount({ email: values.email, company: values.company, signedIn });
    setStep("verify");
    setPending(false);
  };

  const onVerify = async (values: SignupCodeValues) => {
    if (!account) return;
    setFormError(null);
    setPending(true);
    try {
      await authService.verifyEmail({
        email: account.email,
        code: values.code.trim(),
      });
    } catch {
      setFormError(t("signup.verify.invalid"));
      setPending(false);
      return;
    }
    toast.success(t("auth.verify.success"));
    setPending(false);
    await continueAfterVerify();
  };

  const onResend = async () => {
    setResend("sending");
    try {
      await authService.requestEmailVerify();
      setResend("sent");
    } catch {
      setResend("idle");
      toast.error(t("signup.verify.resend_failed"));
    }
  };

  const onRetryOrganization = async (event: FormEvent) => {
    event.preventDefault();
    if (company.trim().length < 2) {
      setOrgError(t("signup.validation.company"));
      return;
    }
    await openOrganization(company.trim());
  };

  const onOAuth = async (provider: OAuthId) => {
    setFormError(null);
    setOauthPending(provider);
    try {
      if (sessionStatus === "authenticated") {
        await signOut({ redirect: false });
      }
      // New OAuth accounts open their organization on the set-up page.
      await signIn(provider, {
        redirect: true,
        callbackUrl: routes.cms.onboarding,
      });
    } catch {
      setFormError(t("auth.oauth.sign_in_error"));
      setOauthPending(null);
    }
  };

  const errorField = formError ? (
    <Field data-invalid={true}>
      <FieldError>{formError}</FieldError>
    </Field>
  ) : null;

  if (step === "verify" && account) {
    return (
      <AuthCard
        title={t("signup.verify.title")}
        description={t("signup.verify.description", { email: account.email })}
      >
        <Stepper current="verify" />
        <AppForm
          schema={codeSchema}
          defaultValues={{ code: "" }}
          onSubmit={onVerify}
        >
          <FieldGroup>
            {errorField}
            <AppInput
              name="code"
              label={t("signup.verify.code")}
              autoComplete="one-time-code"
              inputMode="numeric"
              maxLength={6}
              autoFocus
              placeholder="000000"
              className="[&_input]:text-center [&_input]:font-mono [&_input]:text-lg [&_input]:tracking-[0.4em]"
            />
            <Field>
              <Button type="submit" disabled={pending}>
                {pending ? <Loader2 className="animate-spin" /> : null}
                {pending
                  ? t("signup.verify.submitting")
                  : t("signup.verify.submit")}
              </Button>
              <FieldDescription className="text-center">
                {t("signup.verify.spam_hint")}
              </FieldDescription>
            </Field>
            <div className="flex flex-col items-center gap-2 text-sm">
              {account.signedIn ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={resend !== "idle"}
                  onClick={onResend}
                >
                  {resend === "sending" ? (
                    <Loader2 className="animate-spin" />
                  ) : resend === "sent" ? (
                    <Check />
                  ) : (
                    <RotateCw />
                  )}
                  {resend === "sent"
                    ? t("signup.verify.resent")
                    : t("signup.verify.resend")}
                </Button>
              ) : null}
              <button
                type="button"
                className="text-muted-foreground hover:text-foreground text-xs underline-offset-4 hover:underline"
                disabled={pending}
                onClick={() => {
                  setFormError(null);
                  void continueAfterVerify();
                }}
              >
                {t("signup.verify.later")}
              </button>
            </div>
          </FieldGroup>
        </AppForm>
      </AuthCard>
    );
  }

  if (step === "organization" && account) {
    return (
      <AuthCard
        title={
          orgError
            ? t("signup.organization.retry_title")
            : t("signup.organization.title")
        }
        description={
          orgError
            ? undefined
            : t("signup.organization.description", { company: account.company })
        }
      >
        <Stepper current="organization" />
        {orgError ? (
          <form onSubmit={onRetryOrganization} className="space-y-4" noValidate>
            <div className="grid gap-2">
              <Label htmlFor="signup-company-retry">
                {t("signup.form.company")}
              </Label>
              <Input
                id="signup-company-retry"
                value={company}
                maxLength={120}
                autoComplete="organization"
                onChange={(e) => setCompany(e.target.value)}
                aria-invalid
                aria-describedby="signup-company-error"
              />
              <p
                id="signup-company-error"
                role="alert"
                className="text-destructive text-sm"
              >
                {orgError}
              </p>
            </div>
            <Button type="submit" className="w-full">
              {t("signup.organization.retry")}
              <ArrowRight />
            </Button>
          </form>
        ) : (
          <div className="flex flex-col items-center gap-3 py-6" role="status">
            <Loader2 className="text-primary size-8 animate-spin" aria-hidden />
            <p className="text-muted-foreground text-sm">
              {t("signup.organization.wait")}
            </p>
          </div>
        )}
      </AuthCard>
    );
  }

  const authPending = pending || oauthPending !== null;

  return (
    <AuthCard
      title={t("signup.form.title")}
      description={t("signup.form.description")}
    >
      <Stepper current="account" />
      <div className="space-y-4">
        {plan.selected ? (
          <SelectedPlanNotice
            selected={plan.selected}
            noTrial={plan.noTrial}
            payNow={plan.payNow}
            onPayNowChange={plan.setPayNow}
            onClear={plan.clearPlan}
          />
        ) : null}

        {oauthRegister
          .filter((button) => button.enabled)
          .map((button) => (
            <Button
              key={button.id}
              type="button"
              variant="outline"
              className="w-full"
              disabled={authPending}
              onClick={() => onOAuth(button.id)}
            >
              {button.icon}
              {oauthPending === button.id ? button.pendingLabel : button.label}
            </Button>
          ))}

        {passwordRegister ? (
          <>
            {anyOAuth ? (
              <div className="flex items-center gap-3">
                <Separator className="flex-1" />
                <span className="text-muted-foreground text-xs uppercase">
                  {t("auth.register.or_email")}
                </span>
                <Separator className="flex-1" />
              </div>
            ) : null}

            <AppForm
              schema={schema}
              defaultValues={{
                email: "",
                name: "",
                surname: "",
                password: "",
                company: "",
              }}
              onSubmit={onSubmit}
            >
              <FieldGroup>
                {errorField}
                <div className="grid gap-4 sm:grid-cols-2">
                  <AppInput
                    name="name"
                    label={t("auth.register.name")}
                    autoComplete="given-name"
                    placeholder={t("auth.placeholders.name")}
                  />
                  <AppInput
                    name="surname"
                    label={t("auth.register.surname")}
                    autoComplete="family-name"
                    placeholder={t("auth.placeholders.surname")}
                  />
                </div>
                <AppInput
                  name="email"
                  label={t("signup.form.work_email")}
                  type="email"
                  autoComplete="email"
                  placeholder={t("signup.form.work_email_placeholder")}
                />
                <AppInput
                  name="company"
                  label={t("signup.form.company")}
                  autoComplete="organization"
                  maxLength={120}
                  placeholder={t("signup.form.company_placeholder")}
                  description={t("signup.form.company_hint")}
                />
                <AppPassword
                  name="password"
                  label={t("auth.register.password")}
                  autoComplete="new-password"
                  placeholder={t("auth.placeholders.new_password")}
                />
                <Field>
                  <Button type="submit" disabled={authPending}>
                    {pending ? <Loader2 className="animate-spin" /> : null}
                    {pending
                      ? t("signup.form.submitting")
                      : t("signup.form.submit")}
                    {pending ? null : <ArrowRight />}
                  </Button>
                </Field>
                <Consent />
              </FieldGroup>
            </AppForm>
          </>
        ) : isLoading ? (
          <div
            className="text-muted-foreground flex justify-center py-10"
            role="status"
            aria-label={t("common.loading")}
          >
            <Loader2 className="size-5 animate-spin" aria-hidden />
          </div>
        ) : (
          errorField
        )}

        <p className="text-muted-foreground text-center text-sm">
          {t("auth.register.have_account")}{" "}
          <Link
            href={routes.guest.login}
            className="underline-offset-4 hover:underline"
          >
            {t("auth.register.login_link")}
          </Link>
        </p>
      </div>
    </AuthCard>
  );
}
