"use client";

import { useQuery } from "@tanstack/react-query";
import { Cable, Check, Lock, Pencil } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { type GuideTool, mcpClientConfigs, splitTools } from "../lib/clients";
import { mcpKeys, mcpService } from "../services/mcp.service";
import { ConnectedApps } from "./connected-apps";
import { CopyBlock } from "./copy-block";

/**
 * "Connect with MCP" (CF-173): where an MCP client connects, how it is
 * authorized and which tools this member gets in the active organization.
 * Shown to members who may connect (`tenant.settings.read`, the MCP connect
 * permission of the API). Nothing secret is on this page: clients register
 * themselves and the member approves them in the browser.
 */
export function McpGuidePage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canConnect = permissions.includes(Permission.TenantSettingsRead);
  const guide = useQuery({
    queryKey: mcpKeys.guide(slug),
    queryFn: () => mcpService.guide(slug),
    enabled: Boolean(slug) && canConnect,
    staleTime: 60_000,
  });

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Cable className="size-7" />}
        title={t("mcp.title")}
        description={t("mcp.description")}
      />
      {!slug ? null : !canConnect ? (
        <p className="text-muted-foreground text-sm">{t("mcp.forbidden")}</p>
      ) : guide.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : guide.isError || !guide.data ? (
        <ErrorState
          title={t("mcp.error.title")}
          description={t("mcp.error.description")}
          onRetry={() => void guide.refetch()}
        />
      ) : (
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_26rem]">
          <div className="min-w-0 space-y-6">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {t("mcp.step1.title")}
                </CardTitle>
                <CardDescription>{t("mcp.step1.description")}</CardDescription>
              </CardHeader>
              <CardContent>
                <CopyBlock
                  value={guide.data.resource_url}
                  label={t("mcp.step1.address")}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {t("mcp.step2.title")}
                </CardTitle>
                <CardDescription>{t("mcp.step2.description")}</CardDescription>
              </CardHeader>
              <CardContent>
                <ClientTabs url={guide.data.resource_url} />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {t("mcp.step3.title")}
                </CardTitle>
                <CardDescription>{t("mcp.step3.description")}</CardDescription>
              </CardHeader>
              <CardContent className="space-y-2 text-sm">
                <ol className="list-decimal space-y-1.5 pl-5">
                  <li>{t("mcp.step3.sign_in")}</li>
                  <li>{t("mcp.step3.organization")}</li>
                  <li>
                    {guide.data.can_write
                      ? t("mcp.step3.write_allowed")
                      : t("mcp.step3.write_denied")}
                  </li>
                </ol>
                <p className="text-muted-foreground text-xs">
                  {t("mcp.step3.no_secret")}
                </p>
              </CardContent>
            </Card>

            <ConnectedApps />
          </div>
          <ToolList tools={guide.data.tools} canWrite={guide.data.can_write} />
        </div>
      )}
    </div>
  );
}

function ClientTabs({ url }: { url: string }) {
  const { t } = useLocale();
  const clients = mcpClientConfigs(url);
  return (
    <Tabs defaultValue={clients[0]!.id}>
      <TabsList className="max-w-full overflow-x-auto">
        {clients.map((c) => (
          <TabsTrigger key={c.id} value={c.id}>
            {c.id === "other" ? t("mcp.clients.other.tab") : c.name}
          </TabsTrigger>
        ))}
      </TabsList>
      {clients.map((c) => (
        <TabsContent key={c.id} value={c.id} className="mt-4 space-y-3">
          <p className="text-sm">{t(`mcp.clients.${c.id}.steps`)}</p>
          {c.tested ? null : (
            <p className="text-muted-foreground text-xs">
              {t("mcp.clients.untested")}
            </p>
          )}
          {c.file ? (
            <p className="text-muted-foreground text-xs">
              {t("mcp.clients.file")}{" "}
              <code className="bg-muted rounded px-1">{c.file}</code>
            </p>
          ) : null}
          <CopyBlock value={c.value} label={c.name} />
        </TabsContent>
      ))}
    </Tabs>
  );
}

function ToolList({
  tools,
  canWrite,
}: {
  tools: GuideTool[];
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { available, unavailable } = splitTools(tools);
  return (
    <Card className="h-fit">
      <CardHeader>
        <CardTitle className="text-base">{t("mcp.tools.title")}</CardTitle>
        <CardDescription>{t("mcp.tools.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4 text-sm">
        {available.length === 0 ? (
          <p className="text-muted-foreground">{t("mcp.tools.none")}</p>
        ) : (
          <ul className="space-y-3">
            {available.map((tool) => (
              <ToolRow key={tool.name} tool={tool} />
            ))}
          </ul>
        )}
        {unavailable.length > 0 ? (
          <div className="space-y-3 border-t pt-4">
            <p className="text-muted-foreground flex items-center gap-1.5 text-xs font-medium">
              <Lock className="size-3.5" />
              {t("mcp.tools.unavailable")}
            </p>
            <ul className="space-y-3 opacity-70">
              {unavailable.map((tool) => (
                <ToolRow key={tool.name} tool={tool} />
              ))}
            </ul>
          </div>
        ) : null}
        <p className="text-muted-foreground text-xs">
          {canWrite ? t("mcp.tools.write_note") : t("mcp.tools.read_only_note")}
        </p>
      </CardContent>
    </Card>
  );
}

function ToolRow({ tool }: { tool: GuideTool }) {
  const { t } = useLocale();
  return (
    <li className="space-y-1">
      <div className="flex flex-wrap items-center gap-2">
        <code className="bg-muted rounded px-1.5 py-0.5 text-xs">
          {tool.name}
        </code>
        <Badge variant={tool.write ? "warning" : "secondary"}>
          {tool.write ? (
            <Pencil className="size-3" />
          ) : (
            <Check className="size-3" />
          )}
          {tool.write ? t("mcp.tools.write") : t("mcp.tools.read")}
        </Badge>
      </div>
      {/* The API's own description of the tool (English, as MCP clients see it). */}
      <p className="text-muted-foreground text-xs">{tool.description}</p>
    </li>
  );
}
