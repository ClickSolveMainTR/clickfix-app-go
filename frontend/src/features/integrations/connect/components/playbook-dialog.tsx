"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { aiSettingsService } from "@/features/ai-settings/services/ai-settings.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { INTENT_PATTERN, toKey } from "../lib/keys";
import {
  connectKeys,
  connectService,
  type SystemAction,
  type SystemConcept,
  type SystemPlaybook,
} from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

const NONE = "__none__";

function Chips({
  items,
  selected,
  onToggle,
}: {
  items: { key: string; label: string }[];
  selected: string[];
  onToggle: (key: string) => void;
}) {
  return (
    <div className="flex flex-wrap gap-2">
      {items.map((i) => (
        <button
          key={i.key}
          type="button"
          onClick={() => onToggle(i.key)}
          className={cn(
            "hover:bg-muted rounded-md border px-2 py-1 text-xs",
            selected.includes(i.key) && "border-primary bg-primary/5",
          )}
        >
          {i.label}
        </button>
      ))}
    </div>
  );
}

/** A plain-language playbook: what the AI should do for one intent. */
export function PlaybookDialog({
  slug,
  playbook,
  concepts,
  actions,
  onClose,
}: {
  slug: string;
  playbook: SystemPlaybook | null;
  concepts: SystemConcept[];
  actions: SystemAction[];
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [title, setTitle] = useState(playbook?.title ?? "");
  const [intent, setIntent] = useState(playbook?.intent ?? "");
  const [intentTouched, setIntentTouched] = useState(Boolean(playbook));
  const [instructions, setInstructions] = useState(
    playbook?.instructions ?? "",
  );
  const [conceptKeys, setConceptKeys] = useState<string[]>(
    playbook?.concept_keys ?? [],
  );
  const [actionKeys, setActionKeys] = useState<string[]>(
    playbook?.action_keys ?? [],
  );
  const [procedure, setProcedure] = useState(playbook?.procedure_uuid ?? NONE);
  const [enabled, setEnabled] = useState(playbook?.enabled ?? true);
  const effectiveIntent = intentTouched ? intent : toKey(title, 64);
  const procedures = useQuery({
    queryKey: ["tenant", slug, "ai", "procedures", "connect"],
    queryFn: () => aiSettingsService.procedures(slug),
  });
  const toggle = (set: typeof setConceptKeys) => (key: string) =>
    set((cur) =>
      cur.includes(key) ? cur.filter((k) => k !== key) : [...cur, key],
    );

  const save = useMutation({
    mutationFn: () =>
      connectService.savePlaybook(slug, effectiveIntent, {
        title,
        instructions,
        concept_keys: conceptKeys,
        action_keys: actionKeys,
        procedure_uuid: procedure === NONE ? null : procedure,
        enabled,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.playbooks.saved"));
      onClose();
    },
    onError,
  });
  const valid =
    INTENT_PATTERN.test(effectiveIntent) &&
    title.trim() !== "" &&
    instructions.trim() !== "";

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(
              playbook
                ? "integrations_connect.playbooks.edit"
                : "integrations_connect.playbooks.new",
            )}
          </DialogTitle>
          <DialogDescription>
            {t("integrations_connect.playbooks.dialog_help")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="pb-title">
              {t("integrations_connect.playbooks.title_label")}
            </Label>
            <Input
              id="pb-title"
              value={title}
              maxLength={200}
              placeholder={t(
                "integrations_connect.playbooks.title_placeholder",
              )}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="pb-intent">
              {t("integrations_connect.playbooks.intent")}
            </Label>
            <Input
              id="pb-intent"
              className="font-mono"
              value={effectiveIntent}
              maxLength={64}
              disabled={Boolean(playbook)}
              onChange={(e) => {
                setIntentTouched(true);
                setIntent(e.target.value);
              }}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="pb-text">
              {t("integrations_connect.playbooks.instructions")}
            </Label>
            <Textarea
              id="pb-text"
              rows={5}
              maxLength={3000}
              value={instructions}
              placeholder={t(
                "integrations_connect.playbooks.instructions_placeholder",
              )}
              onChange={(e) => setInstructions(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("integrations_connect.playbooks.matching_help")}
            </p>
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label>{t("integrations_connect.playbooks.concepts")}</Label>
            <Chips
              items={concepts.map((c) => ({ key: c.key, label: c.label }))}
              selected={conceptKeys}
              onToggle={toggle(setConceptKeys)}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label>{t("integrations_connect.playbooks.actions")}</Label>
            <Chips
              items={actions.map((a) => ({ key: a.key, label: a.title }))}
              selected={actionKeys}
              onToggle={toggle(setActionKeys)}
            />
            <p className="text-muted-foreground text-xs">
              {t("integrations_connect.playbooks.actions_help")}
            </p>
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label>{t("integrations_connect.playbooks.procedure")}</Label>
            <Select value={procedure} onValueChange={setProcedure}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>
                  {t("integrations_connect.playbooks.no_procedure")}
                </SelectItem>
                {(procedures.data?.items ?? []).map((p) => (
                  <SelectItem key={p.uuid} value={p.uuid}>
                    {p.title}{" "}
                    <Badge variant="outline" className="ml-1">
                      {p.intent}
                    </Badge>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 text-sm md:col-span-2">
            <Switch checked={enabled} onCheckedChange={setEnabled} />
            {t("integrations_connect.enabled")}
          </label>
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={onClose}>
            {t("integrations_connect.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!valid || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("integrations_connect.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
