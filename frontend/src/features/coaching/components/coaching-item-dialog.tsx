"use client";

import { format, parseISO } from "date-fns";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useLocale } from "@/providers/locale-provider";

import type {
  CoachingInput,
  CoachingItem,
  CoachingKind,
} from "../services/coaching.service";

const KINDS: CoachingKind[] = ["note", "goal", "task"];
const PICK = "__pick__";

/** End of the picked local day, so "due on" includes that whole day. */
function dueAt(day: string) {
  const d = parseISO(day);
  d.setHours(23, 59, 0, 0);
  return d.toISOString();
}

/**
 * New coaching entry for one agent: a note, a goal (optional due date) or a
 * follow-up task under a goal (due date required).
 */
export function CoachingItemDialog({
  agent,
  goals,
  defaultKind = "note",
  defaultParent = "",
  pending,
  onOpenChange,
  onSubmit,
}: {
  agent: string;
  goals: CoachingItem[];
  defaultKind?: CoachingKind;
  defaultParent?: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: CoachingInput) => void;
}) {
  const { t } = useLocale();
  const [kind, setKind] = useState<CoachingKind>(defaultKind);
  const [body, setBody] = useState("");
  const [due, setDue] = useState(
    defaultKind === "task" ? format(new Date(), "yyyy-MM-dd") : "",
  );
  const [parent, setParent] = useState(defaultParent);

  const valid =
    body.trim().length > 0 &&
    body.length <= 5000 &&
    (kind !== "task" || (Boolean(due) && Boolean(parent)));

  const submit = () =>
    onSubmit({
      agent_uuid: agent,
      kind,
      body: body.trim(),
      due_at: kind !== "note" && due ? dueAt(due) : null,
      parent_uuid: kind === "task" ? parent : null,
    });

  return (
    <AppDialog
      open
      onOpenChange={onOpenChange}
      title={t("coaching.items.new")}
      description={t(`coaching.items.kind_hint.${kind}`)}
      className="sm:max-w-lg"
    >
      <div className="grid gap-4">
        <ToggleGroup
          type="single"
          variant="outline"
          value={kind}
          onValueChange={(v) => v && setKind(v as CoachingKind)}
          className="w-full"
        >
          {KINDS.map((k) => (
            <ToggleGroupItem
              key={k}
              value={k}
              disabled={k === "task" && goals.length === 0}
              className="flex-1"
            >
              {t(`coaching.kind.${k}`)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        {kind === "task" && (
          <div className="grid gap-1.5">
            <Label htmlFor="coaching-parent">{t("coaching.items.goal")}</Label>
            <Select
              value={parent || PICK}
              onValueChange={(v) => setParent(v === PICK ? "" : v)}
            >
              <SelectTrigger id="coaching-parent" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={PICK} disabled>
                  {t("coaching.items.pick_goal")}
                </SelectItem>
                {goals.map((g) => (
                  <SelectItem key={g.uuid} value={g.uuid}>
                    <span className="block max-w-[22rem] truncate">
                      {g.body}
                    </span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="grid gap-1.5">
          <Label htmlFor="coaching-body">{t("coaching.items.body")}</Label>
          <Textarea
            id="coaching-body"
            rows={4}
            maxLength={5000}
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
        </div>
        {kind !== "note" && (
          <div className="grid gap-1.5">
            <Label>
              {kind === "task"
                ? t("coaching.items.due_required")
                : t("coaching.items.due_optional")}
            </Label>
            <DatePicker
              value={due}
              onChange={setDue}
              className="w-full sm:w-48"
            />
          </div>
        )}
      </div>
      <FormActions>
        <Button variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button disabled={pending || !valid} onClick={submit}>
          {t("common.save")}
        </Button>
      </FormActions>
    </AppDialog>
  );
}
