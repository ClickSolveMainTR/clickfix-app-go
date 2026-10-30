"use client";

import { ArrowRight, Loader2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import {
  openOrganizationErrorKey,
  useOpenOrganization,
} from "../hooks/use-open-organization";
import { readSignupCompany } from "../lib/signup-company";
import { SelectedPlanNotice } from "./selected-plan-notice";

/**
 * First step for an account without an organization: name it. The caller
 * becomes its owner; the profile is reloaded so the switcher shows it and
 * it becomes the active organization. A plan picked on the pricing page
 * (CF-42) runs the trial on that plan; "pay now" (or a plan without a
 * trial) continues to the billing portal's payment step. The company name
 * typed on `/register` (CF-143) is filled in.
 */
export function CreateOrganizationForm({
  onCreated,
}: {
  onCreated?: (slug: string) => void;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { selected, noTrial, payNow, setPayNow, clearPlan, open } =
    useOpenOrganization();
  const [name, setName] = useState(() =>
    typeof window === "undefined" ? "" : readSignupCompany(),
  );
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (name.trim().length < 2) {
      setError(t("onboarding.create.error_name"));
      return;
    }
    setPending(true);
    setError(null);
    try {
      const { slug, payFirst } = await open(name);
      if (payFirst) {
        router.replace(`${routes.cms.billing}?pay=trial`);
        return;
      }
      onCreated?.(slug);
    } catch (err) {
      setError(t(openOrganizationErrorKey(err)));
    } finally {
      setPending(false);
    }
  };

  return (
    <form onSubmit={onSubmit} className="flex flex-col gap-4" noValidate>
      {selected ? (
        <SelectedPlanNotice
          selected={selected}
          noTrial={noTrial}
          payNow={payNow}
          onPayNowChange={setPayNow}
          onClear={clearPlan}
        />
      ) : null}
      <div className="grid gap-2">
        <Label htmlFor="org-name">{t("onboarding.create.name_label")}</Label>
        <Input
          id="org-name"
          autoFocus
          autoComplete="organization"
          value={name}
          maxLength={120}
          placeholder={t("onboarding.create.name_placeholder")}
          onChange={(e) => setName(e.target.value)}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? "org-name-error" : "org-name-hint"}
        />
        {error ? (
          <p
            id="org-name-error"
            role="alert"
            className="text-destructive text-sm"
          >
            {error}
          </p>
        ) : (
          <p id="org-name-hint" className="text-muted-foreground text-sm">
            {t("onboarding.create.name_hint")}
          </p>
        )}
      </div>
      <Button type="submit" disabled={pending} className="self-start">
        {pending ? <Loader2 className="animate-spin" /> : null}
        {t("onboarding.create.submit")}
        {pending ? null : <ArrowRight />}
      </Button>
    </form>
  );
}
