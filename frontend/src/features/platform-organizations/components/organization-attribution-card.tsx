"use client";

import { useQuery } from "@tanstack/react-query";

import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Loading } from "@/components/common/loading";
import {
  marketingService,
  type OrganizationAttribution,
} from "@/features/settings/services/marketing.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Touch = OrganizationAttribution["first_touch"];

/**
 * Where the organization signed up from (CF-144): first and last touch of
 * the marketing site visit (UTM, ad click id, referrer, landing page).
 */
export function OrganizationAttributionCard({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const query = useQuery({
    queryKey: ["platform", "organizations", uuid, "attribution"],
    queryFn: () => marketingService.organizationAttribution(uuid),
  });

  const fields = (id: string, touch: Touch) => {
    const clickId = touch.gclid
      ? `gclid ${touch.gclid}`
      : touch.msclkid
        ? `msclkid ${touch.msclkid}`
        : touch.fbclid
          ? `fbclid ${touch.fbclid}`
          : "";
    const rows: [string, string | undefined][] = [
      ["source", touch.source],
      ["medium", touch.medium],
      ["campaign", touch.campaign],
      ["term", touch.term],
      ["content", touch.content],
      ["click_id", clickId],
      ["referrer", touch.referrer],
      ["landing", touch.landing],
      ["at", touch.at ? datetime(touch.at, undefined, locale) : ""],
    ];
    return {
      id,
      title: t(`organizations.attribution.${id}`),
      fields: rows
        .filter(([, value]) => value)
        .map(([key, value]) => ({
          key,
          label: t(`organizations.attribution.${key}`),
          value: <span className="break-all">{value}</span>,
        })),
    };
  };

  return (
    <EntitySectionCard
      title={t("organizations.attribution.title")}
      description={t("organizations.attribution.description")}
    >
      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError ? (
        <p role="alert" className="text-destructive text-sm">
          {t("common.error_server")}
        </p>
      ) : query.data ? (
        <EntityDetail
          sections={[
            fields("first_touch", query.data.first_touch),
            fields("last_touch", query.data.last_touch),
          ]}
        />
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("organizations.attribution.none")}
        </p>
      )}
    </EntitySectionCard>
  );
}
