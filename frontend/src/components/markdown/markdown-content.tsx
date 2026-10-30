import Markdown, { defaultUrlTransform, type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import { cn } from "@/lib/utils";

type MarkdownContentProps = {
  /** Markdown written with EditorX (knowledge articles). */
  markdown: string;
  className?: string;
  componentsOverride?: Components;
};

/** http(s), mailto and tel links plus relative paths; everything else is dropped. */
function safeUrl(url: string): string {
  const out = defaultUrlTransform(url);
  if (!out) return "";
  if (/^(https?:|mailto:|tel:|\/|#)/i.test(out)) return out;
  return /^[a-z][a-z0-9+.-]*:/i.test(out) ? "" : out;
}

function isExternal(href: string | undefined) {
  return Boolean(href && /^https?:\/\//i.test(href));
}

const components: Components = {
  h1: ({ node: _node, ...props }) => (
    <h2
      className="font-display mt-10 scroll-m-20 text-2xl font-semibold tracking-tight first:mt-0"
      {...props}
    />
  ),
  h2: ({ node: _node, ...props }) => (
    <h2
      className="font-display mt-10 scroll-m-20 border-b pb-2 text-xl font-semibold tracking-tight first:mt-0"
      {...props}
    />
  ),
  h3: ({ node: _node, ...props }) => (
    <h3
      className="mt-8 scroll-m-20 text-lg font-semibold tracking-tight"
      {...props}
    />
  ),
  h4: ({ node: _node, ...props }) => (
    <h4 className="mt-6 text-base font-semibold" {...props} />
  ),
  p: ({ node: _node, ...props }) => (
    <p className="leading-7 [&:not(:first-child)]:mt-5" {...props} />
  ),
  a: ({ node: _node, href, ...props }) => (
    <a
      href={href}
      className="text-signal-text font-medium underline underline-offset-4 hover:no-underline"
      {...(isExternal(href)
        ? { target: "_blank", rel: "noopener noreferrer nofollow" }
        : {})}
      {...props}
    />
  ),
  ul: ({ node: _node, ...props }) => (
    <ul className="mt-5 ml-6 list-disc [&>li]:mt-2" {...props} />
  ),
  ol: ({ node: _node, ...props }) => (
    <ol className="mt-5 ml-6 list-decimal [&>li]:mt-2" {...props} />
  ),
  blockquote: ({ node: _node, ...props }) => (
    <blockquote
      className="border-signal text-muted-foreground mt-5 border-l-2 pl-5 italic"
      {...props}
    />
  ),
  hr: ({ node: _node, ...props }) => (
    <hr className="border-border my-8" {...props} />
  ),
  code: ({ node: _node, className, ...props }) => (
    <code
      className={cn(
        "bg-muted rounded-sm px-1 py-0.5 font-mono text-[0.9em]",
        className,
      )}
      {...props}
    />
  ),
  pre: ({ node: _node, ...props }) => (
    <pre
      className="bg-muted mt-5 overflow-x-auto rounded-lg p-4 text-sm [&_code]:bg-transparent [&_code]:p-0"
      {...props}
    />
  ),
  table: ({ node: _node, ...props }) => (
    <div className="mt-5 w-full overflow-x-auto">
      <table className="w-full border-collapse text-sm" {...props} />
    </div>
  ),
  th: ({ node: _node, ...props }) => (
    <th
      className="border-border bg-muted/50 border px-3 py-2 text-left font-semibold"
      {...props}
    />
  ),
  td: ({ node: _node, ...props }) => (
    <td className="border-border border px-3 py-2 align-top" {...props} />
  ),
  img: ({ node: _node, alt, ...props }) => (
    // Article images are arbitrary hosts chosen by the author; next/image
    // would need every host allow-listed.
    // eslint-disable-next-line @next/next/no-img-element
    <img
      alt={alt ?? ""}
      loading="lazy"
      decoding="async"
      className="mt-5 h-auto max-w-full rounded-lg border"
      {...props}
    />
  ),
};

/**
 * Safe markdown rendering (CF-74 help center, CF-73 version preview): raw
 * HTML in the source is never rendered and link/image URLs are limited to
 * http(s), mailto, tel and relative paths. Works in Server Components.
 */
export function MarkdownContent({
  markdown,
  className,
  componentsOverride,
}: MarkdownContentProps) {
  return (
    <div className={cn("text-foreground text-[15px] break-words", className)}>
      <Markdown
        remarkPlugins={[remarkGfm]}
        skipHtml
        urlTransform={safeUrl}
        components={
          componentsOverride
            ? { ...components, ...componentsOverride }
            : components
        }
      >
        {markdown}
      </Markdown>
    </div>
  );
}
