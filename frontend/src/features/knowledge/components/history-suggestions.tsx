"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Lightbulb } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useActiveOrganization } from "@/features/organizations";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  historyKeys,
  historyService,
  type HistoryDecisionInput,
  type HistorySuggestion,
} from "../services/history.service";

type Decision = HistoryDecisionInput["decision"];

/** Pending suggestions that reached the evidence threshold (CF-217). */
export function HistorySuggestions({ canReview }: { canReview: boolean }) {
  const { t } = useLocale();
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  const list = useQuery({
    queryKey: historyKeys.suggestions(slug),
    queryFn: () => historyService.suggestions(slug),
    enabled: Boolean(slug),
  });
  const [reviewing, setReviewing] = useState<HistorySuggestion | null>(null);
  const items = list.data?.items ?? [];

  return (
    <section className="space-y-3">
      <h3 className="font-medium">{t("knowledge.history.suggestions")}</h3>
      {items.length === 0 ? (
        <EmptyState
          icon={<Lightbulb className="text-signal size-5" />}
          title={t("knowledge.history.suggestions_empty_title")}
          description={t("knowledge.history.suggestions_empty_description")}
        />
      ) : (
        <div className="space-y-2">
          {items.map((s) => (
            <Card key={s.uuid} className="py-0">
              <CardContent className="space-y-2 p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <p className="min-w-0 flex-1 font-medium">{s.title}</p>
                  <Badge variant="secondary">
                    {t("knowledge.history.evidence", {
                      count: s.evidence_count,
                    })}
                  </Badge>
                  {canReview && (
                    <Button size="sm" onClick={() => setReviewing(s)}>
                      {t("knowledge.history.review")}
                    </Button>
                  )}
                </div>
                {s.conflict_article_title && (
                  <p className="flex items-center gap-1 text-xs text-amber-700 dark:text-amber-300">
                    <AlertTriangle className="size-3.5" />
                    {t("knowledge.history.conflict", {
                      title: s.conflict_article_title,
                    })}
                  </p>
                )}
                <p className="text-muted-foreground line-clamp-3 text-sm whitespace-pre-line">
                  {s.body}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
      {reviewing && (
        <ReviewDialog
          suggestion={reviewing}
          onClose={() => setReviewing(null)}
        />
      )}
    </section>
  );
}

function ReviewDialog({
  suggestion,
  onClose,
}: {
  suggestion: HistorySuggestion;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const slug = org?.slug ?? "";
  const [decision, setDecision] = useState<Decision>("approve");
  const [reason, setReason] = useState("");
  const [title, setTitle] = useState(suggestion.title);
  const [body, setBody] = useState(suggestion.body);
  const decide = useMutation({
    mutationFn: () =>
      historyService.decide(slug, suggestion.uuid, {
        decision,
        reason: reason.trim(),
        locale: locale === "en" ? "en" : "tr",
        ...(decision === "correct" ? { title, body } : {}),
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: historyKeys.all(slug) });
      appToast.success(t(`knowledge.history.decided.${decision}`));
      onClose();
    },
  });
  const valid =
    reason.trim().length >= 3 &&
    (decision !== "correct" || (title.trim() !== "" && body.trim() !== ""));

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("knowledge.history.review_title")}</DialogTitle>
          <DialogDescription>
            {t("knowledge.history.review_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            {(["approve", "correct", "reject"] as const).map((d) => (
              <Button
                key={d}
                size="sm"
                variant={decision === d ? "default" : "outline"}
                onClick={() => setDecision(d)}
              >
                {t(`knowledge.history.decision.${d}`)}
              </Button>
            ))}
          </div>
          {decision === "correct" ? (
            <div className="space-y-2">
              <Label htmlFor="history-review-title">
                {t("knowledge.history.title_label")}
              </Label>
              <Input
                id="history-review-title"
                value={title}
                maxLength={300}
                onChange={(e) => setTitle(e.target.value)}
              />
              <Label htmlFor="history-review-body">
                {t("knowledge.history.body_label")}
              </Label>
              <Textarea
                id="history-review-body"
                rows={8}
                value={body}
                onChange={(e) => setBody(e.target.value)}
              />
            </div>
          ) : (
            <div className="bg-muted/50 max-h-64 overflow-y-auto rounded-md p-3 text-sm whitespace-pre-line">
              <p className="mb-1 font-medium">{suggestion.title}</p>
              {suggestion.body}
            </div>
          )}
          <div className="space-y-2">
            <Label htmlFor="history-review-reason">
              {t("knowledge.history.reason")}
            </Label>
            <Textarea
              id="history-review-reason"
              rows={3}
              maxLength={2000}
              value={reason}
              placeholder={t("knowledge.history.reason_placeholder")}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            variant={decision === "reject" ? "destructive" : "default"}
            disabled={!valid || decide.isPending}
            onClick={() => decide.mutate()}
          >
            {t(`knowledge.history.decision.${decision}`)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
