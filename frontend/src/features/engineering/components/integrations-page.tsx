"use client";

import { useState } from "react";

import { BrandLogo } from "@/components/brand-logos";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage } from "@/components/entity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CopyField } from "@/features/channels/components/setup-fields";
import { IntegrationSettingsDialog } from "@/features/engineering/components/integration-settings-dialog";
import {
  useIntegrationScope,
  useIntegrations,
  useSaveIntegration,
  useStartOAuth,
} from "@/features/engineering/hooks/use-engineering-integrations";
import {
  ENGINEERING_PROVIDERS,
  integrationErrorKey,
  integrationState,
  integrationStateTone,
  PROVIDER_FIELDS,
  webhookPath,
} from "@/features/engineering/lib/integration-setup";
import type {
  EngineeringIntegration,
  EngineeringProvider,
} from "@/features/engineering/services/engineering.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const k = (key: string) => `engineering.integrations.${key}`;

/**
 * Jira, Linear and GitHub connections of product requests
 * (`/v1/tenant/engineering/integrations`). Settings readers see each
 * provider's status; setup, OAuth and switching off need
 * `tenant.settings.write`. Secrets are write-only: the API reports only
 * whether one is stored.
 */
export function EngineeringIntegrationsPage() {
  const { t } = useLocale();
  const { canWrite } = useIntegrationScope();
  const query = useIntegrations();
  const items = query.data?.items ?? [];

  return (
    <EntityPage
      title={t(k("title"))}
      description={t(k("description"))}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t(k("forbidden"))}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t(k("title")) },
      ]}
    >
      {query.isError ? (
        <ErrorState
          title={t(k("load_failed"))}
          description={t(k(`errors.${integrationErrorKey(query.error)}`))}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : query.isPending ? (
        <div className="grid gap-4 xl:grid-cols-3">
          {ENGINEERING_PROVIDERS.map((provider) => (
            <Skeleton key={provider} className="h-56" />
          ))}
        </div>
      ) : (
        <div className="space-y-4">
          {canWrite ? null : (
            <Alert>
              <AlertDescription>{t(k("read_only"))}</AlertDescription>
            </Alert>
          )}
          <div className="grid items-start gap-4 xl:grid-cols-3">
            {ENGINEERING_PROVIDERS.map((provider) => (
              <ProviderCard
                key={provider}
                provider={provider}
                integration={items.find((i) => i.provider === provider)}
                canWrite={canWrite}
              />
            ))}
          </div>
        </div>
      )}
    </EntityPage>
  );
}

function ProviderCard({
  provider,
  integration,
  canWrite,
}: {
  provider: EngineeringProvider;
  integration: EngineeringIntegration | undefined;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [errorKey, setErrorKey] = useState("");
  const save = useSaveIntegration(provider);
  const connect = useStartOAuth(provider);
  const state = integrationState(integration);
  const name = t(`engineering.source.${provider}`);
  const busy = save.isPending || connect.isPending;
  // The API starts OAuth only with a stored client and an HTTPS redirect URI.
  const oauthReady = Boolean(
    integration?.oauth_configured && integration.config.redirect_uri,
  );

  const setEnabled = (enabled: boolean) => {
    if (!integration) return;
    setErrorKey("");
    save.mutate(
      { config: integration.config, enabled },
      {
        onSuccess: () => {
          setConfirming(false);
          appToast.success(
            t(k(enabled ? "toast.enabled" : "toast.disabled"), {
              provider: name,
            }),
          );
        },
        onError: (error) => {
          setConfirming(false);
          setErrorKey(integrationErrorKey(error));
        },
      },
    );
  };

  const startOAuth = () => {
    setErrorKey("");
    connect.mutate(undefined, {
      onError: (error) => setErrorKey(integrationErrorKey(error)),
    });
  };

  const target = integration
    ? PROVIDER_FIELDS[provider]
        .map((field) => ({
          key: field.key,
          value: integration.config[field.key] ?? "",
        }))
        .filter((row) => row.value)
    : [];
  const mapped = Object.keys(integration?.config.states ?? {}).length;
  const yesNo = (value: boolean) =>
    t(k(value ? "summary.stored" : "summary.missing"));

  return (
    <Card data-provider={provider}>
      <CardContent className="space-y-4 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <BrandLogo slug={provider} decorative className="size-7 shrink-0" />
            <h2 className="truncate text-base font-semibold">{name}</h2>
          </div>
          <StatusChip
            label={t(k(`state.${state}`))}
            tone={integrationStateTone[state]}
          />
        </div>
        <p className="text-muted-foreground text-sm">
          {t(k(`state_hint.${state}`), { provider: name })}
        </p>

        {integration ? (
          <dl className="space-y-1.5 text-sm">
            {target.map((row) => (
              <div key={row.key} className="flex justify-between gap-3">
                <dt className="text-muted-foreground shrink-0">
                  {t(k(`fields.${provider}.${row.key}`))}
                </dt>
                <dd className="truncate font-mono text-xs" title={row.value}>
                  {row.value}
                </dd>
              </div>
            ))}
            {provider === "github" ? null : (
              <div className="flex justify-between gap-3">
                <dt className="text-muted-foreground">
                  {t(k("summary.states"))}
                </dt>
                <dd>{t(k("summary.states_count"), { count: mapped })}</dd>
              </div>
            )}
            <div className="flex justify-between gap-3">
              <dt className="text-muted-foreground">{t(k("summary.oauth"))}</dt>
              <dd>{yesNo(integration.oauth_configured)}</dd>
            </div>
            <div className="flex justify-between gap-3">
              <dt className="text-muted-foreground">
                {t(k("summary.webhook"))}
              </dt>
              <dd>{yesNo(integration.webhook_configured)}</dd>
            </div>
          </dl>
        ) : null}

        {integration && canWrite ? (
          <CopyField
            id={`eng-webhook-${provider}`}
            label={t(k("webhook.path"))}
            value={webhookPath(integration)}
            hint={t(k("webhook.path_hint"))}
          />
        ) : null}

        {errorKey ? (
          <Alert variant="destructive">
            <AlertDescription>{t(k(`errors.${errorKey}`))}</AlertDescription>
          </Alert>
        ) : null}

        {canWrite ? (
          <div className="space-y-2">
            <div className="flex flex-wrap gap-2">
              {state === "needs_auth" || state === "connected" ? (
                <Button
                  size="sm"
                  variant={state === "connected" ? "outline" : "default"}
                  disabled={busy || !oauthReady}
                  onClick={startOAuth}
                >
                  {t(k(state === "connected" ? "reconnect" : "connect"), {
                    provider: name,
                  })}
                </Button>
              ) : null}
              <Button
                size="sm"
                variant={state === "not_configured" ? "default" : "outline"}
                disabled={busy}
                onClick={() => setEditing(true)}
              >
                {t(k(state === "not_configured" ? "setup" : "edit"))}
              </Button>
              {state === "disabled" ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setEnabled(true)}
                >
                  {t(k("enable"))}
                </Button>
              ) : null}
              {state === "needs_auth" || state === "connected" ? (
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive"
                  disabled={busy}
                  onClick={() => setConfirming(true)}
                >
                  {t(k("disconnect"))}
                </Button>
              ) : null}
            </div>
            {(state === "needs_auth" || state === "connected") &&
            !oauthReady ? (
              <p className="text-muted-foreground text-xs">
                {t(k("oauth.not_ready"))}
              </p>
            ) : null}
          </div>
        ) : null}
      </CardContent>

      {editing ? (
        <IntegrationSettingsDialog
          provider={provider}
          integration={integration}
          onClose={() => setEditing(false)}
        />
      ) : null}
      <ConfirmDialog
        open={confirming}
        variant="destructive"
        title={t(k("disconnect_confirm.title"), { provider: name })}
        description={t(k("disconnect_confirm.description"))}
        confirmLabel={t(k("disconnect"))}
        isPending={save.isPending}
        onConfirm={() => setEnabled(false)}
        onCancel={() => setConfirming(false)}
      />
    </Card>
  );
}
