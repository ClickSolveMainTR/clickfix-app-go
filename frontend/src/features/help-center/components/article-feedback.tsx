"use client";

import { ThumbsDown, ThumbsUp } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";

type Labels = {
  question: string;
  yes: string;
  no: string;
  thanks: string;
  failed: string;
};

/**
 * "Bu makale işe yaradı mı?" (CF-74). Posts through the same-origin BFF;
 * labels come from the server page so they follow the help center language,
 * not the visitor's app language.
 */
export function ArticleFeedback({
  org,
  slug,
  locale,
  labels,
}: {
  org: string;
  slug: string;
  locale: "tr" | "en";
  labels: Labels;
}) {
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">(
    "idle",
  );
  const vote = async (helpful: boolean) => {
    setState("sending");
    try {
      const response = await fetch(
        `/api/v1/public/help/${encodeURIComponent(org)}/articles/${encodeURIComponent(slug)}/feedback`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ helpful, locale }),
        },
      );
      // A rate-limited repeat vote was already counted: thank the visitor anyway.
      setState(response.ok || response.status === 429 ? "done" : "error");
    } catch {
      setState("error");
    }
  };

  return (
    <section
      aria-labelledby="help-feedback"
      className="border-border bg-card mt-12 flex flex-col items-start gap-3 rounded-xl border p-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <h2 id="help-feedback" className="font-medium">
        {labels.question}
      </h2>
      <div aria-live="polite" className="flex items-center gap-2">
        {state === "done" ? (
          <p className="text-sm">{labels.thanks}</p>
        ) : (
          <>
            {state === "error" && (
              <p className="text-destructive text-sm">{labels.failed}</p>
            )}
            <Button
              variant="outline"
              size="sm"
              disabled={state === "sending"}
              onClick={() => void vote(true)}
            >
              <ThumbsUp className="size-4" />
              {labels.yes}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={state === "sending"}
              onClick={() => void vote(false)}
            >
              <ThumbsDown className="size-4" />
              {labels.no}
            </Button>
          </>
        )}
      </div>
    </section>
  );
}
