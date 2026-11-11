"use client";

import { RootProvider } from "fumadocs-ui/provider/next";
import type { ReactNode } from "react";

import { DocsAnchorAliases } from "./anchor-aliases";

/**
 * fumadocs context for the docs pages. Theme is the app-wide next-themes
 * provider (`AppProviders`), so fumadocs' own is off; search uses the static
 * per-language index at `/docs/search`.
 */
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
                "Search(search trigger)": "Ara",
                "Search(search dialog)": "Ara",
                "Open Search(search trigger)(aria-label)": "Aramayı aç",
                "Close Search(search dialog)(aria-label)": "Aramayı kapat",
                "No results found(search dialog)": "Sonuç bulunamadı",
                "Search failed(search dialog)": "Arama başarısız",
                "On this page(table of contents)": "Bu sayfada",
                "No Headings(table of contents)": "Başlık yok",
                "Table of Contents(inline table of contents)": "İçindekiler",
                "Last updated on(page footer)": "Son güncelleme",
                "Next Page(pagination)": "Sonraki sayfa",
                "Previous Page(pagination)": "Önceki sayfa",
                "Edit on GitHub(edit page)": "GitHub'da düzenle",
                "Open Sidebar(sidebar)(aria-label)": "Menüyü aç",
                "Close Sidebar(sidebar)(aria-label)": "Menüyü kapat",
                "Collapse Sidebar(sidebar)(aria-label)": "Menüyü daralt",
                "Hide Sidebar(sidebar)": "Menüyü gizle",
                "Show Sidebar(sidebar)": "Menüyü göster",
                "Toggle Menu(mobile menu)(aria-label)": "Menü",
                "Copy Text(code block)(aria-label)": "Kodu kopyala",
                "Copied Text(code block)(aria-label)": "Kopyalandı",
                "Copy Anchor Link(heading anchor)(aria-label)":
                  "Bağlantıyı kopyala",
                "Copied Anchor Link(heading anchor)(aria-label)":
                  "Bağlantı kopyalandı",
                "Choose a language(language switcher)": "Dil seçin",
                "Toggle Theme(theme switcher)(aria-label)": "Tema",
                "Page Not Found(404 page)": "Sayfa bulunamadı",
                "Back to Home(404 page)": "Ana sayfaya dön",
                "Example Request(operation page)": "Örnek istek",
                "Example Requests(operation page)": "Örnek istekler",
                "Example Response(operation page)": "Örnek yanıt",
                "Request Body(operation page)": "İstek gövdesi",
                "Path Parameters(operation page)": "Yol parametreleri",
                "Query Parameters(operation page)": "Sorgu parametreleri",
                "Header Parameters(operation page)": "Başlık parametreleri",
                "Authorization(operation page)": "Yetkilendirme",
                "Response(operation page)": "Yanıt",
                "Responses(operation page)": "Yanıtlar",
                "Code language(operation page)": "Kod dili",
                "Media type(operation page)": "İçerik türü",
                "Deprecated(operation page)": "Kullanımdan kalktı",
              }
            : undefined,
      }}
      search={{
        options: {
          type: "static",
          api: `/docs/search${locale === "en" ? "?lang=en" : ""}`,
        },
      }}
    >
      <DocsAnchorAliases />
      {children}
    </RootProvider>
  );
}
