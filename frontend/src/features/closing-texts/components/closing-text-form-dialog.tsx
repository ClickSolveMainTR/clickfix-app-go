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
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { Brand } from "@/features/brands/services/brands.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { useClosingTextMutations } from "../hooks/use-closing-texts";
import type { ClosingText } from "../services/closing-texts.service";

const ORG = "__organization";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Text being edited; undefined creates one. */
  item?: ClosingText;
  brands: Brand[];
};

/** Create or edit a closing text: name, Turkish/English body, brand, default. */
export function ClosingTextFormDialog(props: Props) {
  const { open, onOpenChange, item } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open ? <ClosingTextForm key={item?.uuid ?? "new"} {...props} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function ClosingTextForm({ onOpenChange, item, brands }: Props) {
  const { t } = useLocale();
  const { save } = useClosingTextMutations();
  const [name, setName] = useState(item?.name ?? "");
  const [bodyTr, setBodyTr] = useState(item?.body_tr ?? "");
  const [bodyEn, setBodyEn] = useState(item?.body_en ?? "");
  const [brand, setBrand] = useState(item?.brand_uuid ?? ORG);
  const [isDefault, setIsDefault] = useState(item?.is_default ?? false);
  const [position, setPosition] = useState(String(item?.position ?? 0));

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    save.mutate(
      {
        uuid: item?.uuid,
        body: {
          name: name.trim(),
          body_tr: bodyTr.trim(),
          body_en: bodyEn.trim(),
          brand_uuid: brand === ORG ? null : brand,
          is_default: isDefault,
          position: Math.max(0, Math.min(10000, Number(position) || 0)),
        },
      },
      {
        onSuccess: () => {
          appToast.success(t("closing_texts.toast.saved"));
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>
          {item
            ? t("closing_texts.form.edit_title")
            : t("closing_texts.form.create_title")}
        </DialogTitle>
        <DialogDescription>
          {t("closing_texts.form.description")}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <Label htmlFor="closing-name">{t("closing_texts.form.name")}</Label>
        <Input
          id="closing-name"
          value={name}
          maxLength={120}
          required
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="closing-body-tr">
          {t("closing_texts.form.body_tr")}
        </Label>
        <Textarea
          id="closing-body-tr"
          value={bodyTr}
          maxLength={2000}
          rows={3}
          required
          onChange={(e) => setBodyTr(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="closing-body-en">
          {t("closing_texts.form.body_en")}
        </Label>
        <Textarea
          id="closing-body-en"
          value={bodyEn}
          maxLength={2000}
          rows={3}
          onChange={(e) => setBodyEn(e.target.value)}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
        <div className="space-y-2">
          <Label>{t("closing_texts.form.brand")}</Label>
          <Select value={brand} onValueChange={setBrand}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ORG}>
                {t("closing_texts.organization_wide")}
              </SelectItem>
              {brands.map((b) => (
                <SelectItem key={b.uuid} value={b.uuid}>
                  {b.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-2">
          <Label htmlFor="closing-position">
            {t("closing_texts.form.position")}
          </Label>
          <Input
            id="closing-position"
            type="number"
            min={0}
            max={10000}
            value={position}
            onChange={(e) => setPosition(e.target.value)}
          />
        </div>
      </div>
      <label className="flex items-start gap-3 text-sm">
        <Switch checked={isDefault} onCheckedChange={setIsDefault} />
        <span>
          <span className="font-medium">
            {t("closing_texts.form.is_default")}
          </span>
          <span className="text-muted-foreground block text-xs">
            {t("closing_texts.form.is_default_help")}
          </span>
        </span>
      </label>
      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          onClick={() => onOpenChange(false)}
        >
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={save.isPending || !name.trim() || !bodyTr.trim()}
        >
          {t("common.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
