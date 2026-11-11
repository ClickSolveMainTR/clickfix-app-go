import type * as PageTree from "fumadocs-core/page-tree";

import type { AppLocale } from "@/config/i18n";
import { pageMarkdown } from "@/features/docs/lib/markdown";
import { siteBase } from "@/features/landing/lib/seo";
import { source } from "@/lib/docs/source";

function text(node: PageTree.Node | PageTree.Root): string {
  return typeof node.name === "string" ? node.name : String(node.name ?? "");
}

function pageOf(url: string, locale: AppLocale) {
  const path = url.replace(/\?lang=en$/, "");
  const slugs = path
    .replace(/^\/docs\/?/, "")
    .split("/")
    .filter(Boolean);
  return source.getPage(slugs, locale);
}

/**
 * `llms.txt`: one Markdown index of the docs in the given language, grouped
 * by the sidebar sections. Every entry links the raw Markdown of the page
 * (`/docs/<slug>.md`), and the HTML page is one `.md` away.
 */
export function llmsIndex(locale: AppLocale) {
  const base = siteBase();
  const tree = source.getPageTree(locale);
  const lines: string[] = [];
  const intro =
    locale === "tr"
      ? "> ClickFix, B2B müşteri destek platformudur: çok kanallı gelen kutusu, AI ajanı, bilgi bankası, otomasyon, randevu ve görüntülü görüşme, REST API, webhook'lar, SDK'lar ve mobil webview. Bu dosya dokümantasyonun dizinidir; her bağlantı sayfanın ham Markdown hâlidir, HTML sayfası için `.md` uzantısını kaldırın."
      : "> ClickFix is a B2B customer support platform: multichannel inbox, AI agent, knowledge base, automation, appointments and video rooms, REST API, webhooks, SDKs and a mobile webview. This file is the index of the documentation; every link is the raw Markdown of the page, drop the `.md` suffix for the HTML page.";
  lines.push(
    `# ClickFix ${locale === "tr" ? "dokümantasyonu" : "documentation"}`,
    "",
    intro,
    "",
  );
  lines.push(
    `- ${locale === "tr" ? "Tam metin" : "Full text"}: ${base}${locale === "en" ? "/en" : ""}/llms-full.txt`,
    `- ${locale === "tr" ? "İngilizce" : "Turkish"}: ${base}${locale === "en" ? "" : "/en"}/llms.txt`,
    "",
  );
  const item = (node: PageTree.Item, indent = "") => {
    const page = pageOf(node.url, locale);
    const md = page
      ? `${base}/docs/${page.slugs.length ? page.slugs.join("/") : "index"}.md${locale === "en" ? "?lang=en" : ""}`
      : `${base}${node.url}`;
    const description = node.description ? `: ${String(node.description)}` : "";
    lines.push(`${indent}- [${text(node)}](${md})${description}`);
  };
  const walk = (nodes: PageTree.Node[], indent = "") => {
    for (const node of nodes) {
      if (node.type === "separator") {
        lines.push("", `## ${text(node)}`, "");
      } else if (node.type === "folder") {
        if (node.index) item(node.index, indent);
        walk(node.children, `${indent}  `);
      } else item(node, indent);
    }
  };
  walk(tree.children);
  return lines.join("\n") + "\n";
}

/** `llms-full.txt`: every page of the language as Markdown, in sidebar order. */
export async function llmsFull(locale: AppLocale) {
  const tree = source.getPageTree(locale);
  const urls: string[] = [];
  const walk = (nodes: PageTree.Node[]) => {
    for (const node of nodes) {
      if (node.type === "folder") {
        if (node.index) urls.push(node.index.url);
        walk(node.children);
      } else if (node.type === "page") urls.push(node.url);
    }
  };
  walk(tree.children);
  const parts: string[] = [];
  for (const url of urls) {
    const page = pageOf(url, locale);
    if (page) parts.push(await pageMarkdown(page, locale));
  }
  return parts.join("\n---\n\n");
}
