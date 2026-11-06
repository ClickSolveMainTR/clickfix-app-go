"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Cable } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  consentLoginPath,
  consentRequestId,
  consentView,
  decideBody,
  decideErrorKey,
  decisionGate,
  safeClientRedirect,
  selectedOrganization,
} from "../lib/consent";
import { mcpKeys, mcpService } from "../services/mcp.service";
import { ConsentCard } from "./consent-card";

/**
 * OAuth consent screen of the MCP server (CF-173). An MCP client sends the
 * member here (`/oauth/authorize` redirects to `/oauth/consent?request=`);
 * the member, signed in to the panel, sees who asks and where the answer
 * goes, picks the organization the client may work in and whether it may
 * change data, then denies or allows.
 *
 * The decision is made by the API (`/v1/mcp/consent/{uuid}/decide`, through
 * the panel BFF and its CSRF check); this page only sends it and follows the
 * redirect the API answers with, after checking it is an address a client
 * can have. The request id is never logged or sent anywhere but that API
 * call.
 */
export function McpConsentPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const requestId = consentRequestId(params.get("request"));
  const { bootstrapped, isAuthenticated, user } = useAuth();

  const consent = useQuery({
    queryKey: mcpKeys.consent(requestId ?? ""),
    queryFn: () => mcpService.consent(requestId ?? ""),
    enabled: bootstrapped && isAuthenticated && requestId !== null,
    retry: false,
    staleTime: Infinity,
    // Never kept after the screen closes.
    gcTime: 0,
  });
  const [organization, setOrganization] = useState("");
  const [allowWrite, setAllowWrite] = useState(false);
  const data = consent.data;
  const organizations = data?.organizations ?? [];
  const selected = selectedOrganization(organizations, organization);

  const view = consentView({
    requestId,
    bootstrapped,
    authenticated: isAuthenticated,
    loading: consent.isLoading,
    errorStatus: consent.isError
      ? isApiError(consent.error)
        ? consent.error.status
        : 0
      : null,
    organizations: data ? organizations : null,
  });

  // Signed out: sign in first and come back to this same request.
  useEffect(() => {
    if (view === "sign_in" && requestId)
      router.replace(consentLoginPath(routes.guest.login, requestId));
  }, [requestId, router, view]);

  // One decision per request, also for two clicks inside the same frame.
  const gate = useRef(decisionGate());
  const [unsafeRedirect, setUnsafeRedirect] = useState(false);
  const decide = useMutation({
    mutationFn: async (decision: "approve" | "deny") => {
      const body = decideBody({
        decision,
        organization: selected,
        writeRequested: Boolean(data?.write_requested),
        allowWrite,
      });
      if (!body || !requestId) throw new Error("consent: nothing to decide");
      return mcpService.decide(requestId, body);
    },
    onSuccess: (out) => {
      // Back to the client: its registered redirect URI, built by the API and
      // checked here once more (https, or http on this computer).
      const to = safeClientRedirect(out.redirect_url);
      if (to) window.location.assign(to);
      else setUnsafeRedirect(true);
    },
    onError: () => gate.current.reset(),
  });
  const onDecide = (decision: "approve" | "deny") => {
    if (gate.current.take()) decide.mutate(decision);
  };
  const busy = decide.isPending || decide.isSuccess;
  const errorKey = unsafeRedirect
    ? "mcp.consent.unsafe_redirect"
    : decide.isError
      ? decideErrorKey(isApiError(decide.error) ? decide.error.status : null)
      : null;

  return (
    <main className="flex min-h-svh items-center justify-center p-4">
      {view === "ready" && data ? (
        <ConsentCard
          consent={data}
          account={user?.email ?? ""}
          selected={selected?.uuid}
          allowWrite={allowWrite}
          busy={busy}
          errorKey={errorKey}
          onSelect={(uuid) => {
            setOrganization(uuid);
            setAllowWrite(false);
          }}
          onAllowWrite={setAllowWrite}
          onDecide={onDecide}
        />
      ) : (
        <Card className="w-full max-w-lg">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cable className="size-5" />
              {t("mcp.consent.title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="text-sm">
            {view === "loading" || view === "sign_in" ? (
              <Loading label={t("common.loading")} />
            ) : (
              <p role="alert" className="text-destructive">
                {t(`mcp.consent.${view === "ready" ? "failed" : view}`)}
              </p>
            )}
          </CardContent>
        </Card>
      )}
    </main>
  );
}
