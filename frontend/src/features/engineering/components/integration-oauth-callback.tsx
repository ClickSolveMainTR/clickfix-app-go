"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
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
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const k = (key: string) => `engineering.integrations.${key}`;

/** `callback.*` key of the outcome; "" while the code is being exchanged. */
type Outcome = "" | "cancelled" | "provider_error" | "missing" | "failed";

/**
 * Return leg of the provider consent
 * (`/settings/engineering/callback/{provider}?code=&state=`). The code and
 * state go to the API once, in the session that started the flow, and are
 * dropped from the address bar; they are never shown or logged. A cancelled
 * or failed consent explains itself and leads back to the settings screen.
 */
export function IntegrationOAuthCallback({
  provider,
}: {
  provider: EngineeringProvider;
}) {
  const { t } = useLocale();
  const { slug } = useIntegrationScope();
  const router = useRouter();
  const queryClient = useQueryClient();
  const params = useSearchParams();
  // Read once: the query is removed from the URL right after.
  const [returned] = useState(() => ({
    code: params.get("code") ?? "",
    state: params.get("state") ?? "",
    error: params.get("error"),
  }));
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
    if (started.current) return;
    started.current = true;
    window.history.replaceState(null, "", window.location.pathname);
    if (outcome !== "" || !slug) return;
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
