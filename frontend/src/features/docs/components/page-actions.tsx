"use client";

import { Check, Copy, ExternalLink, MessageSquareText } from "lucide-react";
import { useState } from "react";

import type { AppLocale } from "@/config/i18n";
import { cn } from "@/lib/utils";

/**
 * Under the page title (CF-208): copy the page as Markdown, open it in
 * ChatGPT or Claude with a prompt that points at the page, and a feedback
 * link. `markdownUrl` is the `/docs/<slug>.md` route; `pageUrl` is the
 * canonical absolute URL the prompts ask the assistant to read.
 */
export function DocsPageActions({
  locale,
  title,
  markdownUrl,
  pageUrl,
  feedbackEmail,
}: {
  locale: AppLocale;
  title: string;
  markdownUrl: string;
  pageUrl: string;
  feedbackEmail: string;
}) {
  const [copied, setCopied] = useState<"idle" | "done" | "failed">("idle");
  const label = (tr: string, en: string) => (locale === "tr" ? tr : en);
  const prompt = label(
    `${pageUrl} sayfasını oku; bu sayfayla ilgili sorular soracağım.`,
    `Read ${pageUrl}, I want to ask questions about it.`,
  );
  const links = [
    {
      key: "chatgpt",
      label: label("ChatGPT'de aç", "Open in ChatGPT"),
      href: `https://chatgpt.com/?${new URLSearchParams({ q: prompt })}`,
    },
    {
      key: "claude",
      label: label("Claude'da aç", "Open in Claude"),
      href: `https://claude.ai/new?${new URLSearchParams({ q: prompt })}`,
    },
  ];
  const feedback = `mailto:${feedbackEmail}?${new URLSearchParams({
    subject: label(
      `Doküman geri bildirimi: ${title}`,
      `Docs feedback: ${title}`,
    ),
    body: `${pageUrl}\n\n`,
  })
    .toString()
    .replace(/\+/g, "%20")}`;

  async function copy() {
    try {
      const res = await fetch(markdownUrl);
      if (!res.ok) throw new Error(String(res.status));
      await navigator.clipboard.writeText(await res.text());
      setCopied("done");
    } catch {
      setCopied("failed");
    }
    window.setTimeout(() => setCopied("idle"), 2000);
  }

  const chip =
    "inline-flex h-8 items-center gap-1.5 rounded-full border border-fd-border bg-fd-card px-3 text-xs font-medium text-fd-muted-foreground transition-colors hover:border-fd-primary/50 hover:text-fd-foreground";

  return (
    <div className="not-prose -mt-2 mb-6 flex flex-wrap items-center gap-2">
      <button type="button" onClick={copy} className={chip}>
        {copied === "done" ? (
          <Check aria-hidden className="size-3.5" />
        ) : (
          <Copy aria-hidden className="size-3.5" />
        )}
        {copied === "done"
          ? label("Kopyalandı", "Copied")
          : copied === "failed"
            ? label("Kopyalanamadı", "Copy failed")
            : label("Kopyala (Markdown)", "Copy as Markdown")}
      </button>
      <a href={markdownUrl} className={chip} target="_blank" rel="noreferrer">
        {label("Markdown olarak gör", "View as Markdown")}
      </a>
      {links.map((link) => (
        <a
          key={link.key}
          href={link.href}
          target="_blank"
          rel="noreferrer noopener"
          className={chip}
        >
          {link.label}
          <ExternalLink aria-hidden className="size-3" />
        </a>
      ))}
      <a href={feedback} className={cn(chip, "ms-auto")}>
        <MessageSquareText aria-hidden className="size-3.5" />
        {label("Geri bildirim", "Feedback")}
      </a>
    </div>
  );
}
