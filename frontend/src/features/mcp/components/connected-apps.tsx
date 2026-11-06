"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { mcpKeys, mcpService, type McpGrant } from "../services/mcp.service";

/** Clients the member approved; revoking one ends its access at once. */
export function ConnectedApps() {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const { user } = useAuth();
  const key = mcpKeys.grants(user?.uuid ?? "");
  const [target, setTarget] = useState<McpGrant | null>(null);
  const grants = useQuery({
    queryKey: key,
    queryFn: () => mcpService.grants(),
    enabled: Boolean(user),
  });
  const revoke = useMutation({
    mutationFn: (uuid: string) => mcpService.revokeGrant(uuid),
    onSuccess: () => appToast.success(t("mcp.apps.revoked")),
    onError: (error) =>
      appToast.error(isApiError(error) ? error.message : t("mcp.apps.failed")),
    onSettled: () => {
      setTarget(null);
      return qc.invalidateQueries({ queryKey: key });
    },
  });
  const items = grants.data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{t("mcp.apps.title")}</CardTitle>
        <CardDescription>{t("mcp.apps.description")}</CardDescription>
      </CardHeader>
      <CardContent className="text-sm">
        {grants.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : grants.isError ? (
          <ErrorState
            title={t("mcp.apps.error")}
            onRetry={() => void grants.refetch()}
          />
        ) : items.length === 0 ? (
          <p className="text-muted-foreground">{t("mcp.apps.empty")}</p>
        ) : (
          <ul className="divide-y">
            {items.map((g) => (
              <li
                key={g.uuid}
                className="flex flex-wrap items-center gap-3 py-3 first:pt-0 last:pb-0"
              >
                <div className="min-w-0 flex-1 space-y-0.5">
                  <p className="font-medium break-words">
                    {g.client_name || g.client_id}
                  </p>
                  <p className="text-muted-foreground text-xs">
                    {[
                      g.organization_name,
                      g.scopes.includes("mcp:write")
                        ? t("mcp.apps.scope_write")
                        : t("mcp.apps.scope_read"),
                      g.last_used_at
                        ? t("mcp.apps.last_used", {
                            at: datetime(
                              g.last_used_at,
                              "dd.MM.yyyy HH:mm",
                              locale,
                            ),
                          })
                        : t("mcp.apps.never_used"),
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  className="text-destructive"
                  disabled={revoke.isPending}
                  onClick={() => setTarget(g)}
                >
                  {t("mcp.apps.revoke")}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
      <ConfirmDialog
        open={target !== null}
        title={t("mcp.apps.revoke_title", {
          name: target?.client_name || target?.client_id || "",
        })}
        description={t("mcp.apps.revoke_description")}
        confirmLabel={t("mcp.apps.revoke")}
        variant="destructive"
        isPending={revoke.isPending}
        onConfirm={() =>
          target && !revoke.isPending && revoke.mutate(target.uuid)
        }
        onCancel={() => setTarget(null)}
      />
    </Card>
  );
}
