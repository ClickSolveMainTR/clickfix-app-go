"use client";

import { useRef, useState, type ComponentProps } from "react";
import { Copy } from "lucide-react";
import { MarkdownContent } from "@/components/markdown/markdown-content";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

function CodeBlock({ children, ...props }: ComponentProps<"pre">) {
  const ref = useRef<HTMLPreElement>(null);
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="bg-background my-3 overflow-hidden rounded-xl border">
      <div className="flex justify-end border-b p-1">
        <Button
          size="sm"
          variant="ghost"
          onClick={() => {
            void navigator.clipboard
              .writeText(ref.current?.textContent ?? "")
              .then(() => setCopied(true))
              .catch(() => undefined);
          }}
        >
          <Copy className="size-3" />
          {copied ? t("assistant.copied") : t("assistant.copy")}
        </Button>
      </div>
      <pre
        {...props}
        ref={ref}
        className="overflow-x-auto p-3 text-xs [&_code]:bg-transparent [&_code]:p-0"
      >
        {children}
      </pre>
    </div>
  );
}

const components = {
  pre: ({
    node: _node,
    ...props
  }: ComponentProps<"pre"> & { node?: unknown }) => <CodeBlock {...props} />,
};
export function AssistantMarkdown({ text }: { text: string }) {
  return (
    <MarkdownContent
      markdown={text}
      componentsOverride={components}
      className="text-sm [&_p]:my-1 [&_ul]:my-1"
    />
  );
}
