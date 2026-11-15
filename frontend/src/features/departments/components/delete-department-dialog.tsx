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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useDeleteDepartment,
  useDepartmentOptions,
} from "@/features/departments/hooks/use-departments";
import type { Department } from "@/features/departments/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * Deleting a department: its teams and conversations lose it; the knowledge
 * it owns becomes organization-wide or moves to another department (the API
 * refuses to drop it silently).
 */
export function DeleteDepartmentDialog({
  department,
  onClose,
  onDeleted,
}: {
  department: Department | null;
  onClose: () => void;
  onDeleted?: () => void;
}) {
  const { t } = useLocale();
  const remove = useDeleteDepartment();
  const { options } = useDepartmentOptions();
  const [moveTo, setMoveTo] = useState("none");
  const targets = options.filter((o) => o.value !== department?.uuid);
  return (
    <Dialog
      open={Boolean(department)}
      onOpenChange={(open) => !open && onClose()}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t("departments.delete.title", { name: department?.name ?? "" })}
          </DialogTitle>
          <DialogDescription>
            {t("departments.delete.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="department-move-to">
            {t("departments.delete.move_to")}
          </Label>
          <Select value={moveTo} onValueChange={setMoveTo}>
            <SelectTrigger id="department-move-to" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">
                {t("departments.delete.organization_wide")}
              </SelectItem>
              {targets.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={remove.isPending || !department}
            onClick={() => {
              if (!department) return;
              remove.mutate(
                { uuid: department.uuid, moveTo },
                {
                  onSuccess: () => onDeleted?.(),
                  onSettled: onClose,
                },
              );
            }}
          >
            {t("departments.actions.delete")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
