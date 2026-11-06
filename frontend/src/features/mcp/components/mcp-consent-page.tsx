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

import { mcpKeys, mcpService } from "../services/mcp.service";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/**
 * OAuth consent screen of the MCP server (CF-173). An MCP client sends the
 * member here (`/oauth/authorize` redirects to `/oauth/consent?request=`);
 * the member, signed in to the panel, sees who asks, picks the organization
 * the client may work in and whether it may change data, then approves or
 * denies. The decision is made by the API; this page only sends it and
 * follows the redirect the API answers with.
 */
export function McpConsentPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const request = params.get("request") ?? "";
  const valid = UUID.test(request);
  const { bootstrapped, isAuthenticated } = useAuth();

  // Signed out: sign in first and come back to this request.
  useEffect(() => {
    if (!bootstrapped || isAuthenticated || !valid) return;
    const next = `/oauth/consent?request=${encodeURIComponent(request)}`;
    router.replace(`${routes.guest.login}?next=${encodeURIComponent(next)}`);
  }, [bootstrapped, isAuthenticated, request, router, valid]);

  const consent = useQuery({
    queryKey: mcpKeys.consent(request),
    queryFn: () => mcpService.consent(request),
    enabled: bootstrapped && isAuthenticated && valid,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
  });
  const [organization, setOrganization] = useState("");
  const [allowWrite, setAllowWrite] = useState(false);
  const data = consent.data;
  const organizations = data?.organizations ?? [];
  const selected =
    organizations.find((o) => o.uuid === organization) ??
    (organizations.length === 1 ? organizations[0] : undefined);
  // Write is granted only when the client asked for it and the member may give it here.
  const writeOffered = Boolean(data?.write_requested && selected?.can_write);
  const decide = useMutation({
    mutationFn: (decision: "approve" | "deny") =>
      mcpService.decide(request, {
        decision,
        ...(decision === "approve"
          ? {
              organization_uuid: selected?.uuid,
              allow_write: writeOffered && allowWrite,
            }
          : {}),
      }),
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
          {data ? (
            <CardDescription>
              {t("mcp.consent.asks", {
                client: data.client_name || data.client_id,
              })}
            </CardDescription>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-5 text-sm">
          {!valid ? (
            <p className="text-destructive">{t("mcp.consent.invalid")}</p>
          ) : !bootstrapped || !isAuthenticated || consent.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : consent.isError || !data ? (
            <p className="text-destructive">
              {isApiError(consent.error) && consent.error.status === 404
                ? t("mcp.consent.expired")
                : t("mcp.consent.failed")}
            </p>
          ) : (
            <>
              <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5">
                <dt className="text-muted-foreground">
                  {t("mcp.consent.returns_to")}
                </dt>
                <dd className="font-mono text-xs break-all">
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

              {organizations.length === 0 ? (
                <p className="text-destructive">
                  {t("mcp.consent.no_organization")}
                </p>
              ) : (
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
              )}

              <div className="space-y-2">
                <p className="flex items-center gap-2">
                  <ShieldCheck className="text-muted-foreground size-4" />
                  {t("mcp.consent.scope_read")}
                </p>
                {writeOffered ? (
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
                <p className="text-destructive">
                  {isApiError(decide.error)
                    ? decide.error.message
                    : t("mcp.consent.failed")}
                </p>
              ) : null}

              <div className="flex flex-wrap justify-end gap-2">
                <Button
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
