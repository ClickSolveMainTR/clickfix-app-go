import type { ReactNode } from "react";

import { parse, type Inline } from "@widget/shared/markdown";

/**
 * An outbound message as the customer sees it: the widget's Markdown subset
 * (bold, italic, code, links, lists) rendered with the widget's own parser,
 * so the agent no longer reads "**" in the timeline (CF-177). The API
 * translates the same source text for each channel when it sends it. Only
 * React nodes are built; nothing is set as HTML.
 */
export function MessageContent({ text }: { text: string }) {
  return (
    <div className="space-y-1.5">
      {parse(text).map((block, i) => {
        if (block.t === "pre")
          return (
            <pre
              key={i}
              className="overflow-x-auto rounded-md bg-current/10 px-2 py-1.5 font-mono text-xs whitespace-pre-wrap"
            >
              {block.v}
            </pre>
          );
        if (block.t === "list") {
          const Tag = block.ordered ? "ol" : "ul";
          return (
            <Tag
              key={i}
              className={block.ordered ? "list-decimal pl-5" : "list-disc pl-5"}
            >
              {block.items.map((item, n) => (
                <li key={n}>{inline(item)}</li>
              ))}
            </Tag>
          );
        }
        return <p key={i}>{inline(block.c)}</p>;
      })}
    </div>
  );
}

function inline(nodes: Inline[]): ReactNode[] {
  return nodes.map((node, i) => {
    switch (node.t) {
      case "text":
        return node.v;
      case "br":
        return <br key={i} />;
      case "strong":
        return <strong key={i}>{inline(node.c)}</strong>;
      case "em":
        return <em key={i}>{inline(node.c)}</em>;
      case "code":
        return (
          <code
            key={i}
            className="rounded bg-current/10 px-1 py-0.5 font-mono text-[0.85em]"
          >
            {node.v}
          </code>
        );
      case "link":
        return (
          <a
            key={i}
            href={node.href}
            target="_blank"
            rel="noopener noreferrer"
            className="underline underline-offset-2"
          >
            {inline(node.c)}
          </a>
        );
    }
  });
}
