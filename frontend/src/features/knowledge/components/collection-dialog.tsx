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
import { useLocale } from "@/providers/locale-provider";

import { isValidSlug, slugify } from "../lib/slugify";
import type {
  KnowledgeCollection,
  KnowledgeCollectionInput,
} from "../services/knowledge.service";

type CollectionDialogProps = {
  /** Existing row to edit; null creates. */
  collection: KnowledgeCollection | null;
  /** Kind and parent of a new row. */
  kind: "collection" | "category";
  parentUuid?: string | null;
  /** Top-level collections a new category can be placed in. */
  parents: KnowledgeCollection[];
  pending: boolean;
  onSubmit: (body: KnowledgeCollectionInput) => void;
  onOpenChange: (open: boolean) => void;
};

/**
 * Create / rename a collection or category. Kind and parent are fixed after
 * creation (the API keeps collection URLs and article placement stable).
 */
export function CollectionDialog({
  collection,
  kind: initialKind,
  parentUuid,
  parents,
  pending,
  onSubmit,
  onOpenChange,
}: CollectionDialogProps) {
  const { t } = useLocale();
  const editing = collection !== null;
  const kind = (collection?.kind ?? initialKind) as "collection" | "category";
  const [name, setName] = useState(collection?.name ?? "");
  const [slug, setSlug] = useState(collection?.slug ?? "");
  const [slugTouched, setSlugTouched] = useState(editing);
  const [parent, setParent] = useState<string>(
    parentUuid ?? parents[0]?.uuid ?? "",
  );
  const effectiveSlug = slugTouched ? slug : slugify(name);
  const slugOk = isValidSlug(effectiveSlug);
  const valid =
    name.trim() !== "" &&
    name.length <= 200 &&
    slugOk &&
    (kind === "collection" || editing || parent !== "");

  const title = editing
    ? t(`knowledge.collections.edit_${kind}`)
    : t(`knowledge.collections.new_${kind}`);

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!valid || pending) return;
            onSubmit({
              name: name.trim(),
              slug: effectiveSlug,
              kind,
              parent_uuid:
                kind === "category"
                  ? editing
                    ? (parents.find((p) => p.id === collection?.parent_id)
                        ?.uuid ?? null)
                    : parent
                  : null,
            });
          }}
        >
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>
              {t(`knowledge.collections.${kind}_hint`)}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="kb-collection-name">
              {t("knowledge.fields.name")}
            </Label>
            <Input
              id="kb-collection-name"
              value={name}
              maxLength={200}
              autoFocus
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="kb-collection-slug">
              {t("knowledge.fields.slug")}
            </Label>
            <Input
              id="kb-collection-slug"
              value={effectiveSlug}
              maxLength={200}
              aria-invalid={effectiveSlug !== "" && !slugOk}
              onChange={(e) => {
                setSlugTouched(true);
                setSlug(e.target.value.toLowerCase());
              }}
            />
            <p className="text-muted-foreground text-xs">
              {effectiveSlug !== "" && !slugOk
                ? t("knowledge.slug_invalid")
                : t("knowledge.slug_hint")}
            </p>
          </div>
          {kind === "category" && !editing && (
            <div className="space-y-2">
              <Label htmlFor="kb-collection-parent">
                {t("knowledge.collections.parent")}
              </Label>
              <Select value={parent} onValueChange={setParent}>
                <SelectTrigger id="kb-collection-parent" className="w-full">
                  <SelectValue
                    placeholder={t("knowledge.collections.pick_parent")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {parents.map((p) => (
                    <SelectItem key={p.uuid} value={p.uuid}>
                      {p.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || pending}>
              {t("knowledge.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
