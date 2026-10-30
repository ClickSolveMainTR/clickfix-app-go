"use client";

import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { ConversationPage } from "@/features/inbox/types";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";

import {
  coachingService,
  type CalibrationInput,
} from "../services/coaching.service";

const PICK = "__pick__";

/**
 * New calibration session: a resolved conversation every reviewer scores
 * blind against a snapshot of one CF-108 scorecard.
 */
export function CalibrationCreateDialog({
  slug,
  pending,
  onOpenChange,
  onSubmit,
}: {
  slug: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: CalibrationInput) => void;
}) {
  const { t } = useLocale();
  const [name, setName] = useState("");
  const [scorecard, setScorecard] = useState("");
  const [conversation, setConversation] = useState("");

  const cards = useQuery({
    queryKey: ["tenant", slug, "quality", "scorecards"],
    queryFn: () => coachingService.scorecards(slug),
    staleTime: 60_000,
  });

  // Only resolved conversations can be calibrated.
  const loadConversations = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await tenantRequest<ConversationPage>(
        slug,
        "GET",
        "/v1/tenant/conversations",
        {
          query: {
            status: "resolved",
            q: q || undefined,
            limit: 20,
            offset: 0,
          },
          silent: true,
        },
      );
      return page.items.map((c) => ({
        value: c.uuid,
        label: `#${c.number} · ${c.subject || c.contact.name}`,
        description: c.contact.name,
      }));
    },
    [slug],
  );

  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 200 &&
    Boolean(scorecard) &&
    Boolean(conversation);

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t("coaching.calibrations.new")}
      description={t("coaching.calibrations.form_hint")}
      className="sm:max-w-lg"
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="calibration-name">
            {t("coaching.calibrations.name")}
          </Label>
          <Input
            id="calibration-name"
            value={name}
            maxLength={200}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="calibration-scorecard">
            {t("coaching.calibrations.scorecard")}
          </Label>
          <Select
            value={scorecard || PICK}
            onValueChange={(v) => setScorecard(v === PICK ? "" : v)}
          >
            <SelectTrigger id="calibration-scorecard" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={PICK} disabled>
                {t("coaching.calibrations.pick_scorecard")}
              </SelectItem>
              {(cards.data ?? []).map((c) => (
                <SelectItem key={c.uuid} value={c.uuid}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {cards.isError ? (
            <p className="text-destructive text-xs">
              {t("coaching.calibrations.scorecards_error")}
            </p>
          ) : cards.data?.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              {t("coaching.calibrations.no_scorecards")}
            </p>
          ) : null}
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="calibration-conversation">
            {t("coaching.calibrations.conversation")}
          </Label>
          <AsyncCombobox
            id="calibration-conversation"
            value={conversation}
            onValueChange={setConversation}
            loadOptions={loadConversations}
            placeholder={t("coaching.calibrations.pick_conversation")}
            searchPlaceholder={t("coaching.calibrations.search_conversation")}
            emptyText={t("coaching.calibrations.no_conversations")}
            className="w-full"
          />
          <p className="text-muted-foreground text-xs">
            {t("coaching.calibrations.conversation_hint")}
          </p>
        </div>
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button
          disabled={pending || !valid}
          onClick={() =>
            onSubmit({
              name: name.trim(),
              scorecard_uuid: scorecard,
              conversation_uuid: conversation,
            })
          }
        >
          {t("coaching.calibrations.create")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
