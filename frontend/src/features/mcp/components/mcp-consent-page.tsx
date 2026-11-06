"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { Cable, ShieldCheck } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

import { Loading } from "@/components/common/loading";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
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
  selectedOrganization,
  writeOffered,
} from "../lib/consent";
import { mcpKeys, mcpService } from "../services/mcp.service";

/**
 * OAuth consent screen of the MCP server (CF-173). An MCP client sends the
 * member here (`/oauth/authorize` redirects to `/oauth/consent?request=`);
 * the member, signed in to the panel, sees who asks and where the answer
 * goes, picks the organization the client may work in and whether it may
 * change data, then denies or allows.
 *
 * The decision is made by the API (`/v1/mcp/consent/{uuid}/decide`, through
 * the panel BFF and its CSRF check); this page only sends it and follows the
 * redirect the API answers with. Text that comes from the client (its name,
 * its redirect host) is rendered as text only. The request id is never
 * logged or sent anywhere but that API call.
 */
export function McpConsentPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const requestId = consentRequestId(params.get("request"));
  const { bootstrapped, isAuthenticated } = useAuth();

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
  const canGrantWrite = writeOffered(Boolean(data?.write_requested), selected);

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
    // Back to the client: its registered redirect URI, built by the API.
    onSuccess: (out) => window.location.assign(out.redirect_url),
  });
  const busy = decide.isPending || decide.isSuccess;

  return (
    <main className="flex min-h-svh items-center justify-center p-4">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Cable className="size-5" />
            {t("mcp.consent.title")}
          </CardTitle>
          {view === "ready" && data ? (
            <CardDescription>
              {t("mcp.consent.asks", {
                client: data.client_name || data.client_id,
              })}
            </CardDescription>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-5 text-sm">
          {view === "loading" || view === "sign_in" ? (
            <Loading label={t("common.loading")} />
          ) : view !== "ready" || !data ? (
            <p role="alert" className="text-destructive">
              {t(`mcp.consent.${view === "ready" ? "failed" : view}`)}
            </p>
          ) : (
            <>
              <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2">
                <dt className="text-muted-foreground">
                  {t("mcp.consent.returns_to")}
                </dt>
                <dd className="text-base font-semibold break-all">
                  {data.redirect_host}
                </dd>
                <dt className="text-muted-foreground">
                  {t("mcp.consent.connects_to")}
                </dt>
                <dd className="font-mono text-xs break-all">
                  {data.resource_url}
                </dd>
              </dl>
              <p className="text-muted-foreground text-xs">
                {t("mcp.consent.warning")}
              </p>

              <fieldset className="space-y-2">
                <legend className="font-medium">
                  {t("mcp.consent.organization")}
                </legend>
                {organizations.map((o) => (
                  <label
                    key={o.uuid}
                    className="hover:bg-muted/50 flex cursor-pointer items-center gap-3 rounded-md border p-3"
                  >
                    <input
                      type="radio"
                      name="organization"
                      value={o.uuid}
                      checked={selected?.uuid === o.uuid}
                      disabled={busy}
                      onChange={() => {
                        setOrganization(o.uuid);
                        setAllowWrite(false);
                      }}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block font-medium break-words">
                        {o.name}
                      </span>
                      {o.tool_count != null ? (
                        <span className="text-muted-foreground text-xs">
                          {t("mcp.consent.tool_count", {
                            count: o.tool_count,
                          })}
                        </span>
                      ) : null}
                    </span>
                  </label>
                ))}
              </fieldset>

              <div className="space-y-2">
                <p className="flex items-center gap-2">
                  <ShieldCheck className="text-muted-foreground size-4" />
                  {t("mcp.consent.scope_read")}
                </p>
                {canGrantWrite ? (
                  <div className="flex items-start gap-2">
                    <Checkbox
                      id="mcp-allow-write"
                      checked={allowWrite}
                      disabled={busy}
                      onCheckedChange={(v) => setAllowWrite(v === true)}
                    />
                    <Label htmlFor="mcp-allow-write" className="leading-snug">
                      {t("mcp.consent.scope_write")}
                    </Label>
                  </div>
                ) : data.write_requested && selected ? (
                  <p className="text-muted-foreground text-xs">
                    {t("mcp.consent.write_unavailable")}
                  </p>
                ) : null}
              </div>

              {decide.isError ? (
                <p role="alert" className="text-destructive">
                  {t(
                    decideErrorKey(
                      isApiError(decide.error) ? decide.error.status : null,
                    ),
                  )}
                </p>
              ) : null}

              {/* Deny has the focus; allowing takes a deliberate click. */}
              <div className="flex flex-wrap justify-end gap-2">
                <Button
                  autoFocus
                  variant="outline"
                  disabled={busy}
                  onClick={() => decide.mutate("deny")}
                >
                  {t("mcp.consent.deny")}
                </Button>
                <Button
                  disabled={busy || !selected}
                  onClick={() => decide.mutate("approve")}
                >
                  {t("mcp.consent.approve")}
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>
    </main>
  );
}
