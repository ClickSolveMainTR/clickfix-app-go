"use client";

import { useState } from "react";

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
import { useSaveConversationStatus } from "@/features/conversation-statuses/hooks/use-conversation-statuses";
import {
  CUSTOM_STATUS_CATEGORIES,
  STATUS_COLORS,
  type CustomStatus,
  type CustomStatusCategory,
} from "@/features/conversation-statuses/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Status being edited; undefined creates one. */
  status?: CustomStatus;
};

/** Create or edit a custom status: name, color and base category. */
export function StatusFormDialog(props: Props) {
  const { open, onOpenChange, status } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        {open ? <StatusForm key={status?.uuid ?? "new"} {...props} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function StatusForm({ onOpenChange, status }: Props) {
  const { t } = useLocale();
  const save = useSaveConversationStatus();
  const [name, setName] = useState(status?.name ?? "");
  const [color, setColor] = useState(status?.color || STATUS_COLORS[0]);
  const [category, setCategory] = useState<CustomStatusCategory>(
    status?.category ?? "open",
  );

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    save.mutate(
      {
        uuid: status?.uuid,
        body: { name: name.trim(), color: color.trim(), category },
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>
          {status
            ? t("conversation_statuses.form.edit_title")
            : t("conversation_statuses.form.create_title")}
        </DialogTitle>
        <DialogDescription>
          {t("conversation_statuses.form.description")}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <Label htmlFor="status-name">
          {t("conversation_statuses.form.name")}
        </Label>
        <Input
          id="status-name"
          value={name}
          maxLength={64}
          required
          placeholder={t("conversation_statuses.form.name_placeholder")}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label>{t("conversation_statuses.form.category")}</Label>
        <Select
          value={category}
          onValueChange={(v) => setCategory(v as CustomStatusCategory)}
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CUSTOM_STATUS_CATEGORIES.map((c) => (
              <SelectItem key={c} value={c}>
                {t(`inbox.status.${c}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-xs">
          {t("conversation_statuses.form.category_help")}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="status-color">
          {t("conversation_statuses.form.color")}
        </Label>
        <div className="flex flex-wrap items-center gap-2">
          {STATUS_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              aria-label={c}
              aria-pressed={color === c}
              onClick={() => setColor(c)}
              className={cn(
                "size-6 rounded-full border-2",
                color === c ? "border-foreground" : "border-transparent",
              )}
              style={{ backgroundColor: c }}
            />
          ))}
          <Input
            id="status-color"
            value={color}
            maxLength={32}
            className="h-8 w-28 font-mono text-xs"
            onChange={(e) => setColor(e.target.value)}
          />
        </div>
      </div>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={save.isPending || !name.trim()}>
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
