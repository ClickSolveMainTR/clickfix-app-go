"use client";

import { useQuery } from "@tanstack/react-query";
import { Workflow } from "lucide-react";
import { useState } from "react";

import { PageHeader } from "@/components/layout/page-header";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { connectKeys, connectService } from "../services/connect.service";
import { ActionsCard } from "./actions-card";
import { CallsCard } from "./calls-card";
import { ConceptsCard } from "./concepts-card";
import { ConnectionCard } from "./connection-card";
import { PlaybooksCard } from "./playbooks-card";
import { PreviewCard } from "./preview-card";
import { SourceCard } from "./source-card";

/**
 * "Sistemini bağla" (CF-197): the organization's own system on one screen,
 * in one flow: connection → concepts → where each concept is read from →
 * actions → how the AI should work → try as a customer.
 */
export function ConnectSystemPage() {
  const { t } = useLocale();
  const { slug, permissions } = useTenant();
  const canManage = permissions.includes(Permission.TenantIntegrationsManage);
  const canCreateConcept = permissions.includes(
    Permission.TenantCustomFieldsManage,
  );
  const overview = useQuery({
    queryKey: connectKeys.overview(slug ?? ""),
    queryFn: () => connectService.overview(slug ?? ""),
    enabled: Boolean(slug),
  });
  const [selected, setSelected] = useState<string | null>(null);
  const data = overview.data;
  const concept =
    data?.concepts.find((c) => c.object_type_uuid === selected) ??
    data?.concepts[0] ??
    null;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<Workflow className="size-7" />}
        title={t("integrations_connect.title")}
        description={t("integrations_connect.description")}
      />
      {!slug ? (
        <p className="text-muted-foreground text-sm">
          {t("integrations_connect.no_organization")}
        </p>
      ) : overview.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : !data ? (
        <p className="text-destructive text-sm">
          {t("integrations_connect.failed")}
        </p>
      ) : (
        <div className="space-y-6">
          <ConnectionCard
            key={data.connection?.uuid ?? "new"}
            slug={slug}
            connection={data.connection ?? null}
            canManage={canManage}
          />
          <ConceptsCard
            slug={slug}
            concepts={data.concepts}
            selected={concept?.object_type_uuid ?? null}
            onSelect={setSelected}
            canCreate={canCreateConcept}
          />
          {concept && (
            <SourceCard
              key={`${concept.object_type_uuid}-${concept.source?.uuid ?? "none"}`}
              slug={slug}
              concept={concept}
              connected={Boolean(data.connection)}
              canManage={canManage}
            />
          )}
          <ActionsCard
            slug={slug}
            actions={data.actions}
            concepts={data.concepts}
            canManage={canManage}
          />
          <PlaybooksCard
            slug={slug}
            playbooks={data.playbooks}
            concepts={data.concepts}
            actions={data.actions}
            canManage={canManage}
          />
          {canManage && <PreviewCard slug={slug} playbooks={data.playbooks} />}
          <CallsCard calls={data.calls} />
        </div>
      )}
    </div>
  );
}
