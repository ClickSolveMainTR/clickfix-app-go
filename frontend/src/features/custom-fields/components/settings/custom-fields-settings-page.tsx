"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import { useCustomObjectTypes } from "../../hooks/use-custom-fields";
import type { CustomFieldScope } from "../../services/custom-fields.service";
import { BUILTIN_ENTITY_TYPES } from "../../types";
import { FieldList } from "./field-list";

const OBJECT_TAB_PREFIX = "object:";

/** Tab value of a custom object type on the custom fields screen. */
export function objectFieldsTab(typeUuid: string) {
  return `${OBJECT_TAB_PREFIX}${typeUuid}`;
}

function scopeOf(tab: string): CustomFieldScope {
  if (tab.startsWith(OBJECT_TAB_PREFIX)) {
    return {
      entityType: "object",
      objectTypeUuid: tab.slice(OBJECT_TAB_PREFIX.length),
    };
  }
  return { entityType: tab as CustomFieldScope["entityType"] };
}

/** `/settings/custom-fields`: fields per entity and per custom object. */
export function CustomFieldsSettingsPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const canManage = can(permissions.tenant.customFields.manage);
  const canReadObjects = can(permissions.tenant.customObjects.read);
  const objectTypes = useCustomObjectTypes({ enabled: canReadObjects });

  const tabs = useMemo(() => {
    const builtin = BUILTIN_ENTITY_TYPES.map((entity) => ({
      value: entity as string,
      label: t(`custom_fields.entities.${entity}`),
    }));
    const objects = (objectTypes.data ?? [])
      .filter((o) => !o.archived)
      .map((o) => ({ value: objectFieldsTab(o.uuid), label: o.plural_label }));
    return [...builtin, ...objects];
  }, [objectTypes.data, t]);

  const requested = searchParams.get("tab") ?? "";
  const tab = tabs.some((x) => x.value === requested)
    ? requested
    : requested.startsWith(OBJECT_TAB_PREFIX) && objectTypes.isLoading
      ? requested
      : BUILTIN_ENTITY_TYPES[0];

  const selectTab = (value: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", value);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };

  return (
    <EntityPage
      title={t("custom_fields.title")}
      description={t("custom_fields.subtitle")}
      permission={permissions.tenant.customFields.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("custom_fields.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.cms.home },
        { label: t("layout.section_settings") },
        { label: t("custom_fields.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={selectTab} className="gap-4">
        <div className="-mx-1 overflow-x-auto px-1 pb-1">
          <TabsList>
            {tabs.map((x) => (
              <TabsTrigger key={x.value} value={x.value} className="flex-none">
                {x.label}
              </TabsTrigger>
            ))}
          </TabsList>
        </div>
        <TabsContent value={tab}>
          <FieldList key={tab} scope={scopeOf(tab)} canManage={canManage} />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
