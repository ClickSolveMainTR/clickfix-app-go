import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";

import type { AppLocale } from "@/config/i18n";

import { headingId, plainText } from "./text";

/**
 * Customer documentation (`/docs`). Pages are Markdown files under
 * `frontend/content/docs/<locale>/<slug>.md` with a small `key: value`
 * front matter; Turkish is the primary language and an English page falls
 * back to Turkish when it is missing. The folder is traced into the
 * standalone build (`outputFileTracingIncludes` in next.config.ts).
 */

export const DOC_SECTIONS = [
  "start",
  "channels",
  "product",
  "developers",
  "security",
] as const;
export type DocSection = (typeof DOC_SECTIONS)[number];

export type DocHeading = { id: string; text: string; depth: 2 | 3 };

export type DocPage = {
  slug: string;
  title: string;
  description: string;
  section: DocSection;
  order: number;
  body: string;
  headings: DocHeading[];
  /** True when the page is shown in Turkish for another language. */
  fallback: boolean;
};

export type DocSearchEntry = {
  slug: string;
  title: string;
  description: string;
  section: DocSection;
  headings: string[];
  text: string;
};

const ROOT = path.join(process.cwd(), "content", "docs");
const SLUG_RE = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function parse(slug: string, raw: string): Omit<DocPage, "fallback"> | null {
  const match = /^---\r?\n([\s\S]*?)\r?\n---\r?\n?/.exec(raw);
  if (!match) return null;
  const meta: Record<string, string> = {};
  for (const line of match[1].split(/\r?\n/)) {
    const i = line.indexOf(":");
    if (i > 0) meta[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  const section = DOC_SECTIONS.find((s) => s === meta.section);
  if (!meta.title || !section) return null;
  const body = raw.slice(match[0].length).trim();

  const headings: DocHeading[] = [];
  let inFence = false;
  for (const line of body.split(/\r?\n/)) {
    if (/^\s*(```|~~~)/.test(line)) inFence = !inFence;
    if (inFence) continue;
    const h = /^(#{2,3})\s+(.+?)\s*#*\s*$/.exec(line);
    if (h) {
      const text = plainText(h[2].replace(/\s*\[#.*?\]$/, ""));
      headings.push({
        id: headingId(text),
        text,
        depth: h[1].length as 2 | 3,
      });
    }
  }

  return {
    slug,
    title: meta.title,
    description: meta.description ?? "",
    section,
    order: Number(meta.order) || 0,
    body,
    headings,
  };
}

function readLocale(locale: AppLocale): Map<string, Omit<DocPage, "fallback">> {
  const pages = new Map<string, Omit<DocPage, "fallback">>();
  let files: string[] = [];
  try {
    files = readdirSync(path.join(ROOT, locale));
  } catch {
    return pages;
  }
  for (const file of files) {
    if (!file.endsWith(".mdx")) continue;
    const slug = file.slice(0, -4);
    if (!SLUG_RE.test(slug)) continue;
    const page = parse(
      slug,
      readFileSync(path.join(ROOT, locale, file), "utf8"),
    );
    if (page) pages.set(slug, page);
  }
  return pages;
}

const memo = new Map<AppLocale, DocPage[]>();

/** Every page in the language, ordered by section then `order`. */
export function listDocs(locale: AppLocale): DocPage[] {
  const cached = memo.get(locale);
  if (cached) return cached;

  const primary = readLocale("tr");
  const own = locale === "tr" ? primary : readLocale(locale);
  const pages: DocPage[] = [...primary.keys()].map((slug) => {
    const page = own.get(slug);
    return page
      ? { ...page, fallback: false }
      : { ...primary.get(slug)!, fallback: locale !== "tr" };
  });
  pages.sort(
    (a, b) =>
      DOC_SECTIONS.indexOf(a.section) - DOC_SECTIONS.indexOf(b.section) ||
      a.order - b.order ||
      a.title.localeCompare(b.title, locale),
  );
  // Re-read on every request in development so edits show up.
  if (process.env.NODE_ENV === "production") memo.set(locale, pages);
  return pages;
}

export function getDoc(locale: AppLocale, slug: string) {
  return listDocs(locale).find((page) => page.slug === slug);
}

/** Compact client-side search index (title, headings, plain text). */
export function docsSearchIndex(locale: AppLocale): DocSearchEntry[] {
  return listDocs(locale).map((page) => ({
    slug: page.slug,
    title: page.title,
    description: page.description,
    section: page.section,
    headings: page.headings.map((h) => h.text),
    text: plainText(page.body).slice(0, 4000),
  }));
}
