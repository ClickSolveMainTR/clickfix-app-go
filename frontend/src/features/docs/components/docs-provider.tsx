"use client";

import { RootProvider } from "fumadocs-ui/provider/next";
import type { ReactNode } from "react";

import { DocsAnchorAliases } from "./anchor-aliases";

export function DocsProvider({
  children,
  locale,
}: {
  children: ReactNode;
  locale: "tr" | "en";
}) {
  return (
    <RootProvider
      theme={{ enabled: false }}
      i18n={{
        locale,
        translations:
          locale === "tr"
            ? {
                search: "Ara",
                searchNoResult: "Sonuç bulunamadı",
                toc: "Bu sayfada",
                tocNoHeadings: "Başlık yok",
                lastUpdate: "Son güncelleme",
                chooseLanguage: "Dil seçin",
                nextPage: "Sonraki sayfa",
                previousPage: "Önceki sayfa",
              }
            : undefined,
      }}
      search={{ options: { type: "static", api: "/docs/search" } }}
    >
      <DocsAnchorAliases />
      {children}
    </RootProvider>
  );
}
