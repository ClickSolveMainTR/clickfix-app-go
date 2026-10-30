"use client";

import { Check, Loader2, Plug } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useChannelTypes, useOnboardingActions } from "../hooks/use-onboarding";
import {
  onboardingService,
  type ChannelType,
} from "../services/onboarding.service";

/** Label with a readable fallback when the catalog has no translation. */
function useLabel() {
  const { t } = useLocale();
  return (key: string, fallback: string) => {
    const value = t(key);
    return value === key ? fallback : value;
  };
}

function humanize(value: string) {
  const s = value.replace(/[_.-]+/g, " ").trim();
  return s.charAt(0).toLocaleUpperCase("tr-TR") + s.slice(1);
}

/**
 * Connect a first channel from the wizard: the connectable types come from
 * the API (`/v1/tenant/channels/types`), and each type's credential fields
 * render as a form, so a new adapter shows up here without UI work.
 */
export function ChannelStep({ slug, done }: { slug: string; done: boolean }) {
  const { t } = useLocale();
  const label = useLabel();
  const types = useChannelTypes(slug);
  const { refresh } = useOnboardingActions(slug);
  const [selected, setSelected] = useState<ChannelType | null>(null);
  const [name, setName] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const typeLabel = (type: string) =>
    label(`onboarding.channel.types.${type}`, humanize(type));

  const choose = (type: ChannelType) => {
    setSelected(type);
    setName(typeLabel(type.type));
    setValues({});
    setError(null);
  };

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (!selected) return;
    const missing = selected.fields.find(
      (f) => f.required && !values[f.key]?.trim(),
    );
    if (missing || !name.trim()) {
      setError(t("onboarding.channel.error_required"));
      return;
    }
    setPending(true);
    setError(null);
    try {
      await onboardingService.connectChannel(slug, {
        type: selected.type,
        name: name.trim(),
        credentials: Object.fromEntries(
          Object.entries(values).map(([k, v]) => [k, v.trim()]),
        ),
      });
      toast.success(t("onboarding.channel.connected"));
      setSelected(null);
      await refresh();
    } catch (err) {
      setError(
        isApiError(err) && err.code === "CHANNEL_CREDENTIALS_REJECTED"
          ? t("onboarding.channel.error_rejected")
          : isApiError(err) && err.code === "CHANNEL_ALREADY_CONNECTED"
            ? t("onboarding.channel.error_linked")
            : t("onboarding.channel.error_generic"),
      );
    } finally {
      setPending(false);
    }
  };

  if (types.isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2">
        <Skeleton className="h-20" />
        <Skeleton className="h-20" />
      </div>
    );
  }

  const items = types.data ?? [];

  return (
    <div className="flex flex-col gap-5">
      {done ? (
        <p className="bg-signal-soft text-signal-text flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium">
          <Check className="size-4" aria-hidden />
          {t("onboarding.channel.done")}
        </p>
      ) : null}

      {items.length === 0 ? (
        <p className="text-muted-foreground rounded-lg border border-dashed p-4 text-sm">
          {types.isError
            ? t("onboarding.channel.error_types")
            : t("onboarding.channel.empty")}
        </p>
      ) : (
        <div
          role="radiogroup"
          aria-label={t("onboarding.channel.pick")}
          className="grid gap-3 sm:grid-cols-2"
        >
          {items.map((type) => {
            const active = selected?.type === type.type;
            return (
              <button
                key={type.type}
                type="button"
                role="radio"
                aria-checked={active}
                onClick={() => choose(type)}
                className={cn(
                  "hover:border-foreground/25 focus-visible:ring-ring/50 flex items-start gap-3 rounded-xl border p-4 text-left transition-colors outline-none focus-visible:ring-[3px]",
                  active && "border-signal-text bg-signal-soft/40",
                )}
              >
                <span className="bg-ink flex size-9 shrink-0 items-center justify-center rounded-lg">
                  <Plug className="text-signal size-4" aria-hidden />
                </span>
                <span className="min-w-0">
                  <span className="block font-medium">
                    {typeLabel(type.type)}
                  </span>
                  <span className="text-muted-foreground block text-xs">
                    {label(
                      `onboarding.channel.types.${type.type}_hint`,
                      t("onboarding.channel.generic_hint"),
                    )}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      )}

      {selected ? (
        <form
          onSubmit={onSubmit}
          className="bg-muted/40 flex flex-col gap-4 rounded-xl border p-4"
          noValidate
        >
          <div className="grid gap-2">
            <Label htmlFor="channel-name">
              {t("onboarding.channel.name_label")}
            </Label>
            <Input
              id="channel-name"
              value={name}
              maxLength={120}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          {selected.fields.map((field) => (
            <div key={field.key} className="grid gap-2">
              <Label htmlFor={`channel-${field.key}`}>
                {label(field.label_key, humanize(field.key))}
                {field.required ? (
                  <span aria-hidden className="text-destructive">
                    {" "}
                    *
                  </span>
                ) : null}
              </Label>
              <Input
                id={`channel-${field.key}`}
                type={field.secret ? "password" : "text"}
                autoComplete="off"
                required={field.required}
                value={values[field.key] ?? ""}
                onChange={(e) =>
                  setValues((v) => ({ ...v, [field.key]: e.target.value }))
                }
              />
            </div>
          ))}
          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={pending}>
              {pending ? <Loader2 className="animate-spin" /> : null}
              {t("onboarding.channel.connect")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setSelected(null)}
            >
              {t("common.cancel")}
            </Button>
          </div>
        </form>
      ) : null}
    </div>
  );
}
