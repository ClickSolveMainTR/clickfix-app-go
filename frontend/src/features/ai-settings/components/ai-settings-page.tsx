"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity/entity-page";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { AI_SETTINGS_TABS } from "./tab-registry";

/**
 * Organization "Yapay zeka" settings (`/settings/ai?tab=`): one page whose
 * tabs come from `AI_SETTINGS_TABS`. Reading needs `tenant.settings.read`,
 * changes `tenant.settings.write`.
 */
export function AISettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const { active } = useActiveOrganization();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const requested = searchParams.get("tab") ?? "";
  const tab = AI_SETTINGS_TABS.some((x) => x.id === requested)
    ? requested
    : AI_SETTINGS_TABS[0].id;
  const selectTab = (value: string) => {
    const params = new URLSearchParams();
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };
  const props = {
    slug: active?.slug ?? "",
    canWrite: can(permissions.tenant.settings.write),
    canRun: can(permissions.tenant.conversations.reply),
  };

  return (
    <EntityPage
      title={t("ai_settings.title")}
      description={t("ai_settings.description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("ai_settings.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("layout.section_settings") },
        { label: t("ai_settings.title") },
      ]}
    >
      {!props.canWrite && (
        <p className="text-muted-foreground mb-4 text-sm">
          {t("ai_settings.read_only")}
        </p>
      )}
      <Tabs value={tab} onValueChange={selectTab} className="gap-4">
        <div className="-mx-1 overflow-x-auto px-1 pb-1">
          <TabsList className="w-max">
            {AI_SETTINGS_TABS.map(({ id, labelKey, icon: Icon }) => (
              <TabsTrigger key={id} value={id} className="flex-none gap-1.5">
                <Icon className="size-4" />
                {t(labelKey)}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        {props.slug &&
          AI_SETTINGS_TABS.map(({ id, component: Tab }) => (
            <TabsContent key={id} value={id}>
              {tab === id && <Tab key={props.slug} {...props} />}
            </TabsContent>
          ))}
      </Tabs>
    </EntityPage>
  );
}
