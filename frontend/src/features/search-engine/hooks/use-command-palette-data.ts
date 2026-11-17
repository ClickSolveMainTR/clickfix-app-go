"use client";

import { useQuery } from "@tanstack/react-query";
import { createElement, useMemo, useState } from "react";

import type { AppLayoutVariant } from "@/components/layout/app-layout";
import { useShellNav } from "@/components/layout/use-shell-nav";
import { routes } from "@/config/routes";
import { buildNavPageItems } from "@/features/search-engine/lib/nav-pages";
import { parsePaletteQuery } from "@/features/search-engine/lib/parse-query";
import { readRecentItems } from "@/features/search-engine/lib/recent";
import {
  buildSpecPrefixMap,
  type SpecPrefixEntry,
} from "@/features/search-engine/lib/spec-prefixes";
import { resolveSearchIcon } from "@/features/search-engine/lib/icons";
import {
  fetchSearchHits,
  fetchSearchSpecs,
  fetchTenantSearchHits,
  fetchTenantSearchSpecs,
} from "@/features/search-engine/services/search.service";
import type { PaletteItem, SearchHit } from "@/features/search-engine/types";
import { useActiveOrganization } from "@/features/organizations/lib/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export function useCommandPaletteData(variant: AppLayoutVariant) {
  const { t } = useLocale();
  const { can, canAny } = usePermission();
  const [query, setQuery] = useState("");
  const [activeSpec, setActiveSpec] = useState<string | undefined>();
  const { paletteCatalogs } = useShellNav(variant);
  const { active } = useActiveOrganization();
  const org = active?.slug ?? "";

  const recentQuery = useQuery({
    queryKey: ["search", "recent-items"],
    queryFn: () => Promise.resolve(readRecentItems()),
    staleTime: Infinity,
  });
  const access = useMemo(() => ({ can, canAny }), [can, canAny]);

  const specsQuery = useQuery({
    queryKey: ["search", "specs"],
    // A failed query reaches the global QueryCache toast even when the
    // request is silent: organization staff (no platform role) got "no
    // permission" on every page. Treat a refusal as "no platform index".
    queryFn: () =>
      fetchSearchSpecs().catch(() => ({ items: [], enabled: false })),
    staleTime: 60_000,
    // Organization staff have no platform role: the platform index answers
    // 403 for them, which is not worth retrying.
    retry: false,
  });

  // Organization records (CF-45): the API filters by organization and by
  // the member's tenant permissions, so no client-side permission check.
  const tenantSpecsQuery = useQuery({
    queryKey: ["search", "tenant-specs", org],
    queryFn: () =>
      fetchTenantSearchSpecs(org).catch(() => ({ items: [], enabled: false })),
    enabled: Boolean(org),
    staleTime: 60_000,
    retry: false,
  });

  const platformSpecs = useMemo(() => {
    const items = specsQuery.data?.items ?? [];
    return items.filter((spec) => !spec.permission || can(spec.permission));
  }, [can, specsQuery.data?.items]);

  const tenantSpecs = useMemo(
    () =>
      tenantSpecsQuery.data?.enabled ? (tenantSpecsQuery.data.items ?? []) : [],
    [tenantSpecsQuery.data],
  );

  const remoteSpecs = useMemo(
    () => [...tenantSpecs, ...platformSpecs],
    [platformSpecs, tenantSpecs],
  );

  const prefixMap = useMemo(() => {
    const entries: SpecPrefixEntry[] = [
      {
        id: "pages",
        aliases: ["pages", t("search.prefix_pages")],
      },
    ];
    for (const spec of remoteSpecs) {
      entries.push({
        id: spec.id,
        aliases: [spec.id, t(`search.prefix_${spec.id}`)],
      });
    }
    return buildSpecPrefixMap(entries);
  }, [remoteSpecs, t]);

  const parsed = useMemo(
    () => parsePaletteQuery(query, prefixMap),
    [prefixMap, query],
  );

  const effectiveSpec = activeSpec ?? parsed.spec;
  const searchText = parsed.text;

  const pageItems = useMemo(
    () => buildNavPageItems(paletteCatalogs, access, t),
    [access, paletteCatalogs, t],
  );

  const filteredPages = useMemo(() => {
    if (effectiveSpec && effectiveSpec !== "pages") return [];
    if (!searchText) return pageItems.slice(0, 12);
    const q = searchText.toLowerCase();
    return pageItems.filter(
      (item) =>
        item.label.toLowerCase().includes(q) ||
        item.description?.toLowerCase().includes(q) ||
        item.keywords?.toLowerCase().includes(q) ||
        item.href.toLowerCase().includes(q),
    );
  }, [effectiveSpec, pageItems, searchText]);

  // Only search the platform index once its specs loaded: a 403 there (an
  // organization member) must not fail the whole palette query.
  const platformEnabled =
    specsQuery.isSuccess && specsQuery.data?.enabled !== false;
  const tenantEnabled = tenantSpecs.length > 0;
  const tenantSpecSelected = tenantSpecs.some((s) => s.id === effectiveSpec);
  const wantsTenant = tenantEnabled && (!effectiveSpec || tenantSpecSelected);
  const wantsPlatform =
    platformEnabled && (!effectiveSpec || !tenantSpecSelected);

  const remoteQuery = useQuery({
    queryKey: [
      "search",
      "hits",
      org,
      searchText,
      effectiveSpec,
      wantsTenant,
      wantsPlatform,
    ],
    queryFn: async (): Promise<SearchHit[]> => {
      const [tenantHits, platformHits] = await Promise.all([
        wantsTenant
          ? fetchTenantSearchHits(org, searchText, effectiveSpec).catch(
              (): SearchHit[] => [],
            )
          : Promise.resolve<SearchHit[]>([]),
        wantsPlatform
          ? fetchSearchHits(searchText, effectiveSpec).catch(
              (): SearchHit[] => [],
            )
          : Promise.resolve<SearchHit[]>([]),
      ]);
      return [...tenantHits, ...platformHits];
    },
    enabled: Boolean(searchText) && (wantsTenant || wantsPlatform),
    staleTime: 15_000,
  });

  const remoteItems = useMemo(() => {
    const hits = remoteQuery.data ?? [];
    return hits.map<PaletteItem>((hit) => {
      const text = hit.snippet || hit.subtitle || undefined;
      return {
        id: `${hit.spec}:${hit.id}`,
        spec: hit.spec,
        label: hit.title,
        description:
          hit.private && text ? `${t("search.private_note")} · ${text}` : text,
        // CF-221: contacts open their detail page, also for documents
        // indexed before the index stored that link.
        href:
          hit.spec === "contacts"
            ? routes.cms.contacts.detail(hit.id)
            : hit.href,
        iconKey: hit.icon,
        group: t(`search.specs_${hit.spec}`),
      };
    });
  }, [remoteQuery.data, t]);

  const specOptions = useMemo(() => {
    const options = [
      {
        id: "pages",
        label: t("search.specs_pages"),
        icon: resolveSearchIcon("pages"),
      },
    ];
    for (const spec of remoteSpecs) {
      options.push({
        id: spec.id,
        label: t(spec.label_key),
        icon: resolveSearchIcon(spec.icon ?? spec.id),
      });
    }
    return options;
  }, [remoteSpecs, t]);

  const groupedItems = useMemo(() => {
    const groups = new Map<string, PaletteItem[]>();
    const push = (items: PaletteItem[]) => {
      for (const item of items) {
        const key = item.group;
        groups.set(key, [...(groups.get(key) ?? []), item]);
      }
    };

    const recent = recentQuery.data ?? [];
    if (!searchText && !effectiveSpec && recent.length > 0) {
      push(
        recent.map((item) => ({ ...item, group: t("search.group_recent") })),
      );
    }
    push(filteredPages);
    push(remoteItems);
    return groups;
  }, [
    effectiveSpec,
    filteredPages,
    recentQuery.data,
    remoteItems,
    searchText,
    t,
  ]);

  if (!query && activeSpec !== undefined) {
    setActiveSpec(undefined);
  }

  return {
    query,
    setQuery,
    activeSpec,
    setActiveSpec,
    effectiveSpec,
    searchText,
    specOptions,
    groupedItems,
    loading: remoteQuery.isFetching,
    remoteEnabled: platformEnabled || tenantEnabled,
    placeholder: t("search.placeholder"),
    emptyText: t("search.empty"),
    footerHint: t("search.footer_hint", {
      prefix: t("search.prefix_users"),
      example: t("search.footer_example"),
    }),
    recentEnabled: !searchText && !effectiveSpec,
    refreshRecent: () => {
      void recentQuery.refetch();
    },
  };
}

export function paletteItemIcon(item: PaletteItem) {
  if (item.icon) return item.icon;
  return createElement(resolveSearchIcon(item.iconKey ?? item.spec), {
    className: "size-4 shrink-0 opacity-80",
  });
}
