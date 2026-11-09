"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { KEY_PATTERN, PARAM_PATTERN, toKey } from "../lib/keys";
import {
  connectKeys,
  connectService,
  type SystemAction,
  type SystemChannel,
  type SystemConcept,
  type SystemParam,
} from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

type Method = NonNullable<SystemAction["method"]>;
const METHODS: Method[] = ["POST", "PUT", "PATCH", "DELETE"];
export const CHANNELS: SystemChannel[] = ["ai", "agent", "workflow"];
const NONE = "__none__";

/** Create or edit an action of the organization's system. */
export function ActionDialog({
  slug,
  action,
  concepts,
  onClose,
}: {
  slug: string;
  action: SystemAction | null;
  concepts: SystemConcept[];
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [title, setTitle] = useState(action?.title ?? "");
  const [key, setKey] = useState(action?.key ?? "");
  const [keyTouched, setKeyTouched] = useState(Boolean(action));
  const [description, setDescription] = useState(action?.description ?? "");
  const [concept, setConcept] = useState(action?.concept_key || NONE);
  const [method, setMethod] = useState<Method>(action?.method ?? "POST");
  const [path, setPath] = useState(action?.path_template ?? "");
  const [params, setParams] = useState<SystemParam[]>(action?.params ?? []);
  const [approval, setApproval] = useState(action?.requires_approval ?? true);
  const [allowed, setAllowed] = useState<SystemChannel[]>(
    action?.allowed_for ?? ["agent"],
  );
  const [enabled, setEnabled] = useState(action?.enabled ?? true);
  const effectiveKey = keyTouched ? key : toKey(title);

  const save = useMutation({
    mutationFn: () =>
      connectService.saveAction(slug, effectiveKey, {
        title,
        description,
        concept_key: concept === NONE ? "" : concept,
        method,
        path_template: path,
        params,
        requires_approval: approval,
        allowed_for: allowed,
        enabled,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.actions.saved"));
      onClose();
    },
    onError,
  });
  const valid =
    KEY_PATTERN.test(effectiveKey) &&
    title.trim() !== "" &&
    path.startsWith("/") &&
    allowed.length > 0 &&
    params.every((p) => PARAM_PATTERN.test(p.key));

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {t(
              action
                ? "integrations_connect.actions.edit"
                : "integrations_connect.actions.new",
            )}
          </DialogTitle>
          <DialogDescription>
            {t("integrations_connect.actions.dialog_help")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="act-title">
              {t("integrations_connect.actions.title_label")}
            </Label>
            <Input
              id="act-title"
              value={title}
              maxLength={120}
              placeholder={t("integrations_connect.actions.title_placeholder")}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="act-key">{t("integrations_connect.key")}</Label>
            <Input
              id="act-key"
              className="font-mono"
              value={effectiveKey}
              maxLength={50}
              disabled={Boolean(action)}
              onChange={(e) => {
                setKeyTouched(true);
                setKey(e.target.value);
              }}
            />
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label htmlFor="act-desc">
              {t("integrations_connect.actions.description_label")}
            </Label>
            <Textarea
              id="act-desc"
              rows={2}
              maxLength={1000}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>{t("integrations_connect.actions.concept")}</Label>
            <Select value={concept} onValueChange={setConcept}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE}>
                  {t("integrations_connect.actions.no_concept")}
                </SelectItem>
                {concepts.map((c) => (
                  <SelectItem key={c.key} value={c.key}>
                    {c.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t("integrations_connect.actions.endpoint")}</Label>
            <div className="flex gap-2">
              <Select
                value={method}
                onValueChange={(v) => setMethod(v as Method)}
              >
                <SelectTrigger className="w-28">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {METHODS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Input
                className="font-mono"
                value={path}
                maxLength={500}
                placeholder="/faturalar/{fatura_no}/tekrar-gonder"
                aria-label={t("integrations_connect.actions.endpoint")}
                onChange={(e) => setPath(e.target.value)}
              />
            </div>
          </div>
          <div className="space-y-2 md:col-span-2">
            <div className="flex items-center justify-between">
              <Label>{t("integrations_connect.actions.params")}</Label>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() =>
                  setParams((p) => [
                    ...p,
                    { key: "", label: "", type: "string", required: true },
                  ])
                }
              >
                <Plus className="size-4" />
                {t("integrations_connect.actions.add_param")}
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              {t("integrations_connect.actions.params_help")}
            </p>
            {params.map((p, i) => {
              const set = (patch: Partial<SystemParam>) =>
                setParams((cur) =>
                  cur.map((x, j) => (j === i ? { ...x, ...patch } : x)),
                );
              return (
                <div key={i} className="flex flex-wrap items-center gap-2">
                  <Input
                    className="w-40 font-mono"
                    value={p.key}
                    maxLength={40}
                    placeholder="fatura_no"
                    aria-label={t("integrations_connect.key")}
                    onChange={(e) => set({ key: e.target.value })}
                  />
                  <Input
                    className="w-40"
                    value={p.label}
                    maxLength={80}
                    placeholder={t("integrations_connect.actions.param_label")}
                    aria-label={t("integrations_connect.actions.param_label")}
                    onChange={(e) => set({ label: e.target.value })}
                  />
                  <Select
                    value={p.type}
                    onValueChange={(v) =>
                      set({ type: v as SystemParam["type"] })
                    }
                  >
                    <SelectTrigger className="w-32">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {(["string", "number", "boolean"] as const).map((ty) => (
                        <SelectItem key={ty} value={ty}>
                          {t(`integrations_connect.actions.type.${ty}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <label className="flex items-center gap-1 text-xs">
                    <Checkbox
                      checked={p.required}
                      onCheckedChange={(v) => set({ required: v === true })}
                    />
                    {t("integrations_connect.actions.required")}
                  </label>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    aria-label={t("integrations_connect.remove")}
                    onClick={() =>
                      setParams((cur) => cur.filter((_, j) => j !== i))
                    }
                  >
                    <X className="size-4" />
                  </Button>
                </div>
              );
            })}
          </div>
          <div className="space-y-2 md:col-span-2">
            <Label>{t("integrations_connect.actions.allowed_for")}</Label>
            <div className="flex flex-wrap gap-4">
              {CHANNELS.map((c) => (
                <label key={c} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={allowed.includes(c)}
                    onCheckedChange={(v) =>
                      setAllowed((cur) =>
                        v === true
                          ? [...cur.filter((x) => x !== c), c]
                          : cur.filter((x) => x !== c),
                      )
                    }
                  />
                  {t(`integrations_connect.channel.${c}`)}
                </label>
              ))}
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm md:col-span-2">
            <Switch checked={approval} onCheckedChange={setApproval} />
            {t("integrations_connect.actions.requires_approval")}
          </label>
          {approval && allowed.some((c) => c !== "agent") && (
            <p className="text-xs text-amber-600 md:col-span-2">
              {t("integrations_connect.actions.approval_note")}
            </p>
          )}
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
