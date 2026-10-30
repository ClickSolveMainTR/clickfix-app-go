"use client";

import { Plug } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { ApiKeysTab } from "./api-keys-tab";
import { ConnectHelp } from "./connect-help";
import { HooksTab } from "./hooks-tab";

/**
 * Workflow integrations (CF-97): API keys with scopes, REST hook
 * subscriptions and how to connect Zapier / Make / n8n. Keys need
 * tenant.settings.write; subscriptions tenant.channels.read (delete:
 * tenant.channels.manage).
 */
export function AutomationIntegrationsPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canKeys = permissions.includes(Permission.TenantSettingsWrite);
  const canHooks = permissions.includes(Permission.TenantChannelsRead);
  const canDeleteHooks = permissions.includes(Permission.TenantChannelsManage);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Plug className="size-7" />}
        title={t("automation_integrations.title")}
        description={t("automation_integrations.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("automation_integrations.no_organization")}
        </p>
      ) : (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
          <Tabs defaultValue={canKeys ? "keys" : "hooks"} className="min-w-0">
            <TabsList className="max-w-full overflow-x-auto">
              {canKeys && (
                <TabsTrigger value="keys">
                  {t("automation_integrations.tabs.keys")}
                </TabsTrigger>
              )}
              {canHooks && (
                <TabsTrigger value="hooks">
                  {t("automation_integrations.tabs.hooks")}
                </TabsTrigger>
              )}
            </TabsList>
            {canKeys && (
              <TabsContent value="keys" className="mt-4">
                <ApiKeysTab slug={slug} />
              </TabsContent>
            )}
            {canHooks && (
              <TabsContent value="hooks" className="mt-4">
                <HooksTab slug={slug} canDelete={canDeleteHooks} />
              </TabsContent>
            )}
            {!canKeys && !canHooks && (
              <p className="text-muted-foreground mt-4 text-sm">
                {t("automation_integrations.forbidden")}
              </p>
            )}
          </Tabs>
          <ConnectHelp />
        </div>
      )}
    </div>
  );
}
