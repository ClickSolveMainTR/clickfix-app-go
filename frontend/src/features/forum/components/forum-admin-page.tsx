"use client";

import { MessagesSquare } from "lucide-react";

import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";

import { CategoriesTab } from "./categories-tab";
import { ModerationTab } from "./moderation-tab";
import { TopicsTab } from "./topics-tab";

/**
 * Community forum admin (CF-78): moderation queue, published topics with
 * accepted answers and conversion to conversations, and categories. Uses
 * the existing feedback permission like the backend (no new slug).
 */
export function ForumAdminPage() {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canManage = perms.includes(Permission.TenantFeedbackManage);
  const canConvert = perms.includes(Permission.TenantConversationsManage);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<MessagesSquare className="size-7" />}
        title={t("forum.title")}
        description={t("forum.description")}
      />
      {org && !canManage ? (
        <p className="text-muted-foreground text-sm">{t("forum.forbidden")}</p>
      ) : null}
      {org && canManage ? (
        <Tabs defaultValue="moderation">
          <TabsList className="flex h-auto w-full flex-wrap justify-start sm:w-auto">
            <TabsTrigger value="moderation">
              {t("forum.tabs.moderation")}
            </TabsTrigger>
            <TabsTrigger value="topics">{t("forum.tabs.topics")}</TabsTrigger>
            <TabsTrigger value="categories">
              {t("forum.tabs.categories")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="moderation" className="mt-4">
            <ModerationTab org={org.slug} />
          </TabsContent>
          <TabsContent value="topics" className="mt-4">
            <TopicsTab org={org.slug} canConvert={canConvert} />
          </TabsContent>
          <TabsContent value="categories" className="mt-4">
            <CategoriesTab org={org.slug} />
          </TabsContent>
        </Tabs>
      ) : null}
    </div>
  );
}
