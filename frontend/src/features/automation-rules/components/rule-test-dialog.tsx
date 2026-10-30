"use client";

import { useMutation } from "@tanstack/react-query";
import { CheckCircle2, FlaskConical, Search, XCircle } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useActiveOrganization } from "@/features/organizations";
import { useSampleConversations } from "@/features/workflows/hooks/use-workflows";
import { isUuid } from "@/features/workflows/lib/graph";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { rulesService, type DryRunResult } from "../services/rules.service";

/**
 * "Test et": evaluates the saved rule's conditions against a conversation.
 * No action runs and no run-log row is written.
 */
export function RuleTestDialog({
  uuid,
  onClose,
}: {
  uuid: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const [q, setQ] = useState("");
  const debounced = useDebounce(q, 300);
  const [conversation, setConversation] = useState("");
  const [result, setResult] = useState<DryRunResult | null>(null);
  const samples = useSampleConversations(isUuid(debounced) ? "" : debounced);
  const run = useMutation({
    mutationFn: (c: string) => rulesService.test(org?.slug ?? "", uuid, c),
    onSuccess: setResult,
  });
  const selected = isUuid(q.trim()) ? q.trim() : conversation;

  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("rules.test.title")}
      description={t("rules.test.description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
    >
      <div className="space-y-4">
        <div className="relative">
          <Search className="text-muted-foreground absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            value={q}
            className="pl-8"
            placeholder={t("workflows.test.search")}
            aria-label={t("workflows.test.search")}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        {!isUuid(q.trim()) && (
          <ul
            className="max-h-56 space-y-1 overflow-y-auto"
            aria-label={t("workflows.test.samples")}
          >
            {(samples.data ?? []).map((c) => (
              <li key={c.uuid}>
                <button
                  type="button"
                  onClick={() => {
                    setConversation(c.uuid);
                    setResult(null);
                  }}
                  aria-pressed={conversation === c.uuid}
                  className={cn(
                    "hover:bg-muted flex w-full items-center gap-2 rounded-md border px-2.5 py-1.5 text-left text-sm",
                    conversation === c.uuid && "border-primary bg-primary/5",
                  )}
                >
                  <span className="text-muted-foreground text-xs tabular-nums">
                    #{c.number}
                  </span>
                  <span className="truncate">
                    {c.subject ||
                      c.contact?.name ||
                      c.contact?.email ||
                      t("workflows.test.no_subject")}
                  </span>
                  <span className="text-muted-foreground ml-auto text-xs">
                    {t(`workflows.statuses.${c.status}`)}
                  </span>
                </button>
              </li>
            ))}
            {samples.isSuccess && (samples.data ?? []).length === 0 && (
              <li className="text-muted-foreground py-4 text-center text-sm">
                {t("workflows.test.no_samples")}
              </li>
            )}
          </ul>
        )}
        {run.isError && (
          <p className="text-destructive text-sm" role="alert">
            {isApiError(run.error)
              ? run.error.message
              : t("rules.toast.failed")}
          </p>
        )}
        {result && (
          <div
            className={cn(
              "flex items-start gap-2 rounded-md border p-3 text-sm",
              result.matched
                ? "border-emerald-500/40 bg-emerald-500/5"
                : "bg-muted/40",
            )}
            role="status"
          >
            {result.matched ? (
              <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
            ) : (
              <XCircle className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            )}
            <div>
              <p className="font-medium">
                {result.matched
                  ? t("rules.test.matched")
                  : t("rules.test.not_matched")}
              </p>
              <p className="text-muted-foreground text-xs">
                {t("rules.test.hypothetical")}
              </p>
            </div>
          </div>
        )}
      </div>
      <FormActions sticky={false}>
        <Button type="button" variant="outline" onClick={onClose}>
          {t("common.close")}
        </Button>
        <Button
          type="button"
          disabled={!selected || run.isPending}
          onClick={() => run.mutate(selected)}
        >
          <FlaskConical className="size-4" />
          {run.isPending ? t("rules.test.running") : t("rules.test.run")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
