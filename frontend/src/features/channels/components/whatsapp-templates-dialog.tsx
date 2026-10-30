"use client";

import { Trash2 } from "lucide-react";
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
import { Textarea } from "@/components/ui/textarea";
import {
  useCreateWhatsAppTemplate,
  useDeleteWhatsAppTemplate,
  useWhatsAppTemplates,
} from "@/features/channels/hooks/use-channels";
import type {
  ChannelInstance,
  WhatsAppTemplateCreate,
} from "@/features/channels/types";
import { useLocale } from "@/providers/locale-provider";

const statusTone: Record<string, "success" | "warning" | "danger"> = {
  APPROVED: "success",
  PENDING: "warning",
  REJECTED: "danger",
  PAUSED: "warning",
  DISABLED: "danger",
};

/** Number of {{n}} placeholders in a template body. */
function placeholders(body: string) {
  return new Set(body.match(/\{\{\d+\}\}/g) ?? []).size;
}

/**
 * Message templates of a WhatsApp number's business account: approval
 * state, submit a new one (body with {{1}}..{{n}} and an example for each),
 * delete. Templates are what starts a conversation outside the 24 hour
 * window.
 */
export function WhatsAppTemplatesDialog({
  channel,
  onOpenChange,
}: {
  channel: ChannelInstance;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const list = useWhatsAppTemplates(channel.uuid);
  const create = useCreateWhatsAppTemplate(channel.uuid);
  const remove = useDeleteWhatsAppTemplate(channel.uuid);
  const empty: WhatsAppTemplateCreate = {
    name: "",
    category: "UTILITY",
    language: "tr",
    body: "",
    example: [],
  };
  const [form, setForm] = useState<WhatsAppTemplateCreate>(empty);
  const vars = placeholders(form.body);
  const examples = Array.from(
    { length: vars },
    (_, i) => form.example?.[i] ?? "",
  );

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("channels.templates.title")}</DialogTitle>
          <DialogDescription>{t("channels.templates.hint")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          {list.isLoading ? (
            <p className="text-muted-foreground text-sm">
              {t("channels.loading")}
            </p>
          ) : (list.data ?? []).length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("channels.templates.empty")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border">
              {(list.data ?? []).map((tpl) => (
                <li
                  key={tpl.id}
                  className="flex items-center justify-between gap-3 p-2 text-sm"
                >
                  <div className="min-w-0">
                    <p className="truncate font-medium">
                      {tpl.name}{" "}
                      <span className="text-muted-foreground">
                        · {tpl.language} · {tpl.category}
                      </span>
                    </p>
                    {tpl.rejected_reason && tpl.rejected_reason !== "NONE" ? (
                      <p className="text-destructive text-xs">
                        {tpl.rejected_reason}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2">
                    <Badge variant={statusTone[tpl.status] ?? "secondary"}>
                      {tpl.status}
                    </Badge>
                    <Button
                      size="icon"
                      variant="ghost"
                      aria-label={t("channels.templates.delete")}
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(tpl.name)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>

        <form
          className="space-y-3 border-t pt-4"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate(
              { ...form, example: examples },
              { onSuccess: () => setForm(empty) },
            );
          }}
        >
          <p className="font-medium">{t("channels.templates.new")}</p>
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="space-y-1 sm:col-span-1">
              <Label htmlFor="tpl-name">{t("channels.templates.name")}</Label>
              <Input
                id="tpl-name"
                value={form.name}
                placeholder="siparis_durumu"
                onChange={(e) =>
                  setForm({
                    ...form,
                    name: e.target.value
                      .toLowerCase()
                      .replace(/[^a-z0-9_]/g, "_"),
                  })
                }
              />
            </div>
            <div className="space-y-1">
              <Label>{t("channels.templates.category")}</Label>
              <Select
                value={form.category}
                onValueChange={(v) =>
                  setForm({ ...form, category: v as typeof form.category })
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {["UTILITY", "MARKETING", "AUTHENTICATION"].map((c) => (
                    <SelectItem key={c} value={c}>
                      {t(`channels.templates.categories.${c}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="tpl-lang">
                {t("channels.templates.language")}
              </Label>
              <Input
                id="tpl-lang"
                value={form.language}
                onChange={(e) => setForm({ ...form, language: e.target.value })}
              />
            </div>
          </div>
          <div className="space-y-1">
            <Label htmlFor="tpl-body">{t("channels.templates.body")}</Label>
            <Textarea
              id="tpl-body"
              value={form.body}
              maxLength={1024}
              placeholder={t("channels.templates.body_placeholder")}
              onChange={(e) => setForm({ ...form, body: e.target.value })}
            />
          </div>
          {examples.map((value, i) => (
            <div key={i} className="space-y-1">
              <Label htmlFor={`tpl-ex-${i}`}>
                {t("channels.templates.example", { n: i + 1 })}
              </Label>
              <Input
                id={`tpl-ex-${i}`}
                value={value}
                onChange={(e) => {
                  const next = [...examples];
                  next[i] = e.target.value;
                  setForm({ ...form, example: next });
                }}
              />
            </div>
          ))}
          <DialogFooter>
            <Button
              type="submit"
              disabled={
                create.isPending ||
                !form.name ||
                !form.body ||
                examples.some((x) => x.trim() === "")
              }
            >
              {t("channels.templates.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
