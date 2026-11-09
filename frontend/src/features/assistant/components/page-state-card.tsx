"use client";

import { AlertTriangle, CircleDashed, FlaskConical } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import type { AssistantPageState } from "../services/assistant.service";

import { RouteLink } from "./action-card";

/**
 * "What can I do here, what is missing" (CF-187): the page's missing
 * steps, warnings and tests on the welcome screen. Every button only sends
 * a message; anything that changes data still goes through a confirmation
 * card.
 */
export function PageStateCard({
  state,
  disabled,
  onAsk,
}: {
  state: AssistantPageState;
  disabled: boolean;
  onAsk: (prompt: string) => void;
}) {
  const { t } = useLocale();
  const empty =
    state.missing.length === 0 &&
    state.warnings.length === 0 &&
    state.tests.length === 0;
  if (empty) return null;
  return (
    <section
      aria-label={t("assistant.page_state.title", { page: state.title })}
      className="bg-card space-y-3 rounded-2xl border p-4 text-sm shadow-xs"
    >
      <p className="font-medium">
        {t("assistant.page_state.title", { page: state.title })}
      </p>
      {[
        ...state.missing.map((s) => ({ ...s, warn: false })),
        ...state.warnings.map((s) => ({ ...s, warn: true })),
      ].map((s) => (
        <div key={s.key} className="flex items-start gap-2">
          {s.warn ? (
            <AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600" />
          ) : (
            <CircleDashed className="text-muted-foreground mt-0.5 size-4 shrink-0" />
          )}
          <div className="min-w-0 flex-1">
            <p>{s.title}</p>
            {s.detail ? (
              <p className="text-muted-foreground text-xs break-words">
                {s.detail}
              </p>
            ) : null}
            <div className="flex flex-wrap items-center gap-2">
              {s.prompt ? (
                <Button
                  size="sm"
                  variant="link"
                  className="h-auto px-0"
                  disabled={disabled}
                  onClick={() => onAsk(s.prompt ?? "")}
                >
                  {t("assistant.page_state.help")}
                </Button>
              ) : null}
              {s.focus ? (
                <RouteLink route={s.focus.route} target={s.focus.target} />
              ) : null}
            </div>
          </div>
        </div>
      ))}
      {state.tests.length ? (
        <div className="space-y-1">
          <p className="text-muted-foreground text-xs">
            {t("assistant.page_state.tests")}
          </p>
          <div className="flex flex-wrap gap-2">
            {state.tests.map((test) => (
              <Button
                key={test.title}
                size="sm"
                variant="outline"
                disabled={disabled}
                onClick={() => onAsk(test.prompt)}
              >
                <FlaskConical className="size-3" />
                {test.title}
              </Button>
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}
