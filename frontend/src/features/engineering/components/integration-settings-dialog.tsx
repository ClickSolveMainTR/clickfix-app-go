"use client";

import { RefreshCw } from "lucide-react";
import { useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { generateSecret } from "@/features/channels/lib/channel-setup";
import { useSaveIntegration } from "@/features/engineering/hooks/use-engineering-integrations";
import {
  draftOf,
  hasStateMapping,
  hasTransitionMapping,
  inputOf,
  integrationErrorKey,
  MAPPED_STATUSES,
  PROVIDER_FIELDS,
  suggestedRedirectUri,
  validateDraft,
  type IntegrationDraft,
} from "@/features/engineering/lib/integration-setup";
import type {
  EngineeringIntegration,
  EngineeringProvider,
} from "@/features/engineering/services/engineering.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Setup form of one provider: where requests go (project / repository),
 * status mapping, and the OAuth app of the organization. Stored secrets are
 * never read back; an empty secret field keeps the stored value.
 */
export function IntegrationSettingsDialog({
  provider,
  integration,
  onClose,
}: {
  provider: EngineeringProvider;
  integration: EngineeringIntegration | undefined;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveIntegration(provider);
  const [draft, setDraft] = useState<IntegrationDraft>(() =>
    draftOf(provider, integration),
  );
  const [problems, setProblems] = useState<string[]>([]);
  const [errorKey, setErrorKey] = useState("");
  const name = t(`engineering.source.${provider}`);
  const k = (key: string) => `engineering.integrations.${key}`;
  const suggested = suggestedRedirectUri(provider);

  const setConfig = (key: keyof IntegrationDraft["config"], value: string) =>
    setDraft((prev) => ({ ...prev, config: { ...prev.config, [key]: value } }));

  const submit = () => {
    const found = validateDraft(provider, draft);
    setProblems(found);
    setErrorKey("");
    if (found.length > 0) return;
    save.mutate(
      // A new integration starts switched on; editing keeps the switch.
      inputOf(provider, draft, integration?.enabled ?? true),
      {
        onSuccess: () => {
          appToast.success(t(k("toast.saved"), { provider: name }));
          onClose();
        },
        onError: (error) => setErrorKey(integrationErrorKey(error)),
      },
    );
  };

  const invalid = (key: string) =>
    problems.includes(key) ? (
      <p className="text-destructive text-xs">{t(k(`invalid.${key}`))}</p>
    ) : null;

  const stored = (configured: boolean | undefined) =>
    configured ? t(k("fields.secret_stored")) : undefined;

  const mapping = (
    kind: "states" | "transitions",
    values: Record<string, string>,
  ) => (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t(k(`${kind}.title`))}</legend>
      <p className="text-muted-foreground text-xs">
        {t(k(`${kind}.hint.${provider}`))}
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {MAPPED_STATUSES.map((status) => (
          <div key={status} className="space-y-1">
            <Label htmlFor={`eng-${kind}-${status}`}>
              {t(`engineering.status.${status}`)}
            </Label>
            <Input
              id={`eng-${kind}-${status}`}
              autoComplete="off"
              className="font-mono text-xs"
              value={values[status] ?? ""}
              onChange={(e) =>
                setDraft((prev) => ({
                  ...prev,
                  [kind]: { ...prev[kind], [status]: e.target.value },
                }))
              }
            />
          </div>
        ))}
      </div>
    </fieldset>
  );

  return (
    <Dialog
      open
      onOpenChange={(next) => {
        if (!next && !save.isPending) onClose();
      }}
    >
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t(k("dialog.title"), { provider: name })}</DialogTitle>
          <DialogDescription>{t(k("dialog.description"))}</DialogDescription>
        </DialogHeader>
        <form
          className="space-y-6"
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">
              {t(k("target.title"))}
            </legend>
            <div className="grid gap-3 sm:grid-cols-2">
              {PROVIDER_FIELDS[provider].map((field) => (
                <div key={field.key} className="space-y-1">
                  <Label htmlFor={`eng-${field.key}`}>
                    {t(k(`fields.${provider}.${field.key}`))}
                    {field.required ? null : (
                      <span className="text-muted-foreground font-normal">
                        {" "}
                        {t(k("fields.optional"))}
                      </span>
                    )}
                  </Label>
                  <Input
                    id={`eng-${field.key}`}
                    autoComplete="off"
                    value={draft.config[field.key]}
                    aria-invalid={problems.includes(field.key)}
                    onChange={(e) => setConfig(field.key, e.target.value)}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t(k(`hints.${provider}.${field.key}`))}
                  </p>
                  {invalid(field.key)}
                </div>
              ))}
            </div>
          </fieldset>

          {hasStateMapping(provider) ? mapping("states", draft.states) : null}
          {hasTransitionMapping(provider)
            ? mapping("transitions", draft.transitions)
            : null}

          <fieldset className="space-y-3">
            <legend className="text-sm font-medium">
              {t(k("oauth.title"))}
            </legend>
            <p className="text-muted-foreground text-xs">
              {t(k(`oauth.hint.${provider}`))}
            </p>
            <div className="space-y-1">
              <Label htmlFor="eng-redirect-uri">
                {t(k("fields.redirect_uri"))}
              </Label>
              <Input
                id="eng-redirect-uri"
                autoComplete="off"
                className="font-mono text-xs"
                value={draft.config.redirect_uri}
                aria-invalid={problems.includes("redirect_uri")}
                onChange={(e) => setConfig("redirect_uri", e.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                {t(k("hints.redirect_uri"))}
                {suggested && draft.config.redirect_uri !== suggested ? (
                  <>
                    {" "}
                    <button
                      type="button"
                      className="text-primary underline-offset-4 hover:underline"
                      onClick={() => setConfig("redirect_uri", suggested)}
                    >
                      {t(k("fields.redirect_uri_reset"))}
                    </button>
                  </>
                ) : null}
              </p>
              {invalid("redirect_uri")}
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="eng-client-id">
                  {t(k("fields.client_id"))}
                </Label>
                <Input
                  id="eng-client-id"
                  autoComplete="off"
                  value={draft.clientId}
                  placeholder={stored(integration?.oauth_configured)}
                  onChange={(e) =>
                    setDraft({ ...draft, clientId: e.target.value })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="eng-client-secret">
                  {t(k("fields.client_secret"))}
                </Label>
                <Input
                  id="eng-client-secret"
                  type="password"
                  autoComplete="new-password"
                  value={draft.clientSecret}
                  placeholder={stored(integration?.oauth_configured)}
                  onChange={(e) =>
                    setDraft({ ...draft, clientSecret: e.target.value })
                  }
                />
              </div>
            </div>
          </fieldset>

          <fieldset className="space-y-2">
            <legend className="text-sm font-medium">
              {t(k("webhook.title"))}
            </legend>
            <div className="space-y-1">
              <Label htmlFor="eng-webhook-secret">
                {t(k("fields.webhook_secret"))}
              </Label>
              <div className="flex gap-2">
                <Input
                  id="eng-webhook-secret"
                  autoComplete="off"
                  className="font-mono text-xs"
                  value={draft.webhookSecret}
                  placeholder={stored(integration?.webhook_configured)}
                  aria-invalid={problems.includes("webhook_secret")}
                  onChange={(e) =>
                    setDraft({ ...draft, webhookSecret: e.target.value })
                  }
                />
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  aria-label={t(k("fields.webhook_secret_generate"))}
                  title={t(k("fields.webhook_secret_generate"))}
                  onClick={() =>
                    setDraft({ ...draft, webhookSecret: generateSecret() })
                  }
                >
                  <RefreshCw className="size-4" />
                </Button>
              </div>
              <p className="text-muted-foreground text-xs">
                {t(k(`webhook.hint.${provider}`))}
              </p>
              {invalid("webhook_secret")}
            </div>
          </fieldset>

          {errorKey ? (
            <Alert variant="destructive">
              <AlertDescription>{t(k(`errors.${errorKey}`))}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              onClick={onClose}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t(k("dialog.save"))}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
