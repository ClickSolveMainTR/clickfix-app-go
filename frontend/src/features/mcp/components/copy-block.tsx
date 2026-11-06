"use client";

import { Check, Copy } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/** A value to paste into an MCP client, with a copy button. Holds no secret. */
export function CopyBlock({ value, label }: { value: string; label: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);
  return (
    <div className="bg-muted/50 relative rounded-md border">
      <pre
        aria-label={label}
        className="overflow-x-auto p-3 pr-12 font-mono text-xs break-all whitespace-pre-wrap"
      >
        {value}
      </pre>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="absolute top-1.5 right-1.5 size-8"
        aria-label={`${t("common.copy")}: ${label}`}
        onClick={() => {
          void navigator.clipboard.writeText(value).then(() => {
            setCopied(true);
            window.setTimeout(() => setCopied(false), 2000);
          });
        }}
      >
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      </Button>
    </div>
  );
}
