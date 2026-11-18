"use client";

import { PenLine } from "lucide-react";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";

import type { ClosingTextOptions } from "../services/closing-texts.service";

/** Value of the picker: a closing text uuid or "none". */
export const NO_CLOSING_TEXT = "none";

type Props = {
  options: ClosingTextOptions;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
};

/**
 * Composer picker of the closing text appended to the reply (CF-250). The
 * server remembers the agent's pick on send and offers it again next time.
 */
export function ClosingTextPicker({
  options,
  value,
  onChange,
  disabled,
}: Props) {
  const { t } = useLocale();
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger
        size="sm"
        className="h-8 w-auto max-w-48 gap-1.5 border-none px-2 text-xs shadow-none"
        aria-label={t("closing_texts.picker.label")}
      >
        <PenLine className="size-3.5 shrink-0" />
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NO_CLOSING_TEXT}>
          {t("closing_texts.picker.none")}
        </SelectItem>
        {options.items.map((o) => (
          <SelectItem key={o.uuid} value={o.uuid}>
            {o.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
