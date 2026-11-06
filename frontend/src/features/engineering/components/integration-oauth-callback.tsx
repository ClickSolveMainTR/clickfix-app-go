"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  integrationKeys,
  useIntegrationScope,
} from "@/features/engineering/hooks/use-engineering-integrations";
import { integrationErrorKey } from "@/features/engineering/lib/integration-setup";
import {
  engineeringService,
  type EngineeringProvider,
} from "@/features/engineering/services/engineering.service";
import { ActiveOrganizationGate } from "@/features/tenant/components/active-organization-gate";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const k = (key: string) => `engineering.integrations.${key}`;

/** `callback.*` key of the outcome; "" while the code is being exchanged. */
type Outcome = "" | "cancelled" | "provider_error" | "missing" | "failed";

type Returned = { code: string; state: string; error: string | null };

/**
 * Return leg of the provider consent
 * (`/settings/engineering/callback/{provider}?code=&state=`). The query is
 * read once and dropped from the address bar before anything else, also
 * when the organization cannot be loaded and nothing is exchanged; it is
 * never shown or logged. The code and state go to the API once, in the
 * session that started the flow. A cancelled or failed consent explains
 * itself and leads back to the settings screen.
 */
export function IntegrationOAuthCallback({
  provider,
}: {
  provider: EngineeringProvider;
}) {
  const { t } = useLocale();
  const tenant = useTenant();
  const params = useSearchParams();
  const [returned] = useState<Returned>(() => ({
    code: params.get("code") ?? "",
    state: params.get("state") ?? "",
    error: params.get("error"),
  }));

  useEffect(() => {
    window.history.replaceState(null, "", window.location.pathname);
  }, []);

  // The gate below shows why the organization is unavailable; the exchange
  // is not mounted then, so say what happened to the consent.
  const interrupted = !tenant.slug || tenant.failed;

  return (
    <div className="space-y-4">
      {interrupted ? (
        <Alert variant="destructive">
          <AlertTitle>{t(k("callback.interrupted_title"))}</AlertTitle>
          <AlertDescription>{t(k("callback.interrupted"))}</AlertDescription>
        </Alert>
      ) : null}
      <ActiveOrganizationGate>
        <OAuthExchange provider={provider} returned={returned} />
      </ActiveOrganizationGate>
    </div>
  );
}

function OAuthExchange({
  provider,
  returned,
}: {
  provider: EngineeringProvider;
  returned: Returned;
}) {
  const { t } = useLocale();
  const { slug, canWrite } = useIntegrationScope();
  const router = useRouter();
  const queryClient = useQueryClient();
  const started = useRef(false);
  const [outcome, setOutcome] = useState<Outcome>(() => {
    // Providers send ?error=access_denied when the user declines.
    if (returned.error === "access_denied") return "cancelled";
    if (returned.error !== null) return "provider_error";
    if (!returned.code || !returned.state) return "missing";
    return "";
  });
  const [errorKey, setErrorKey] = useState("");
  const name = t(`engineering.source.${provider}`);
  const back = routes.cms.settings.engineering.root;

  useEffect(() => {
    if (started.current || outcome !== "" || !canWrite) return;
    started.current = true;
    engineeringService
      .completeOAuth(slug, provider, {
        state: returned.state,
        code: returned.code,
      })
      .then(() => {
        void queryClient.invalidateQueries({
          queryKey: integrationKeys.list(slug),
        });
        appToast.success(t(k("toast.connected"), { provider: name }));
        router.replace(back);
      })
      .catch((error: unknown) => {
        setErrorKey(integrationErrorKey(error));
        setOutcome("failed");
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <EntityPage
      title={t(k("callback.title"), { provider: name })}
      permission={permissions.tenant.settings.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t(k("errors.forbidden"))}
        />
      }
    >
      {outcome === "" ? (
        <Loading className="py-16" label={t(k("callback.working"))} />
      ) : (
        <ErrorState
          title={t(
            k(
              outcome === "cancelled"
                ? "callback.cancelled_title"
                : "callback.failed_title",
            ),
            { provider: name },
          )}
          description={
            outcome === "failed"
              ? t(k(`errors.${errorKey}`))
              : t(k(`callback.${outcome}`), { provider: name })
          }
          action={
            <Button onClick={() => router.replace(back)}>
              {t(k("callback.back"))}
            </Button>
          }
        />
      )}
    </EntityPage>
  );
}
