"use client";

import { Cable } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { ConnectedApps } from "./connected-apps";

/**
 * MCP connection settings (CF-173): the MCP clients the member approved, with
 * the way to end one's access. Shown to members who may connect
 * (`tenant.settings.read`, the API's MCP connect permission).
 */
export function McpAppsPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canConnect = permissions.includes(Permission.TenantSettingsRead);
  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Cable className="size-7" />}
        title={t("mcp.title")}
        description={t("mcp.description")}
      />
      {!slug ? null : !canConnect ? (
        <p className="text-muted-foreground text-sm">{t("mcp.forbidden")}</p>
      ) : (
        <ConnectedApps />
      )}
    </div>
  );
}
