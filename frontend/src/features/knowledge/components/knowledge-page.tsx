"use client";

import { BookOpen, ExternalLink, Plus } from "lucide-react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";

import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";

import { ArticlesTab } from "./articles-tab";
import { CollectionsTab } from "./collections-tab";
import { HistoryTab } from "./history-tab";

const TABS = ["articles", "collections", "history"] as const;
type Tab = (typeof TABS)[number];

/**
 * Knowledge base admin (CF-73): articles (drafts, publication, versions),
 * the collection / category tree that structures the public help center and
 * the history pipeline that turns past records into suggestions (CF-217).
 */
export function KnowledgePage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantSettingsRead);
  const canWrite = perms.includes(Permission.TenantSettingsWrite);
  const canHistory = perms.includes(Permission.TenantHistoryRead);
  const raw = params.get("tab");
  const tab: Tab =
    TABS.includes(raw as Tab) && (raw !== "history" || canHistory)
      ? (raw as Tab)
      : "articles";

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<BookOpen className="size-7" />}
        title={t("knowledge.title")}
        description={t("knowledge.description")}
        actions={
          <div className="flex flex-wrap gap-2">
            {org && (
              <Button variant="outline" size="sm" asChild>
                <a
                  href={routes.help.home(org.slug)}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <ExternalLink className="size-4" />
                  {t("knowledge.open_help_center")}
                </a>
              </Button>
            )}
            {canWrite && (
              <Button size="sm" asChild>
                <Link href={routes.cms.settings.knowledge.create}>
                  <Plus className="size-4" />
                  {t("knowledge.articles.new")}
                </Link>
              </Button>
            )}
          </div>
        }
      />
      {org && !canRead ? (
        <p className="text-muted-foreground text-sm">
          {t("knowledge.forbidden")}
        </p>
      ) : (
        <Tabs
          value={tab}
          onValueChange={(next) =>
            router.replace(routes.cms.settings.knowledge.tab(next), {
              scroll: false,
            })
          }
        >
          <TabsList>
            <TabsTrigger value="articles">
              {t("knowledge.tabs.articles")}
            </TabsTrigger>
            <TabsTrigger value="collections">
              {t("knowledge.tabs.collections")}
            </TabsTrigger>
            {canHistory && (
              <TabsTrigger value="history">
                {t("knowledge.tabs.history")}
              </TabsTrigger>
            )}
          </TabsList>
          <TabsContent value="articles" className="mt-4">
            {org && <ArticlesTab />}
          </TabsContent>
          <TabsContent value="collections" className="mt-4">
            {org && <CollectionsTab canWrite={canWrite} />}
          </TabsContent>
          {canHistory && (
            <TabsContent value="history" className="mt-4">
              <HistoryTab
                canManage={perms.includes(Permission.TenantHistoryManage)}
                canReview={perms.includes(Permission.TenantHistoryReview)}
              />
            </TabsContent>
          )}
        </Tabs>
      )}
    </div>
  );
}
