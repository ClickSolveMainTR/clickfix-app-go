"use client";

import { useMemo, useState } from "react";

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
import { useSaveCategory } from "@/features/categories/hooks/use-categories";
import { flatten, parseExamples, subtree } from "@/features/categories/lib";
import type { CategoryNode } from "@/features/categories/types";
import { useLocale } from "@/providers/locale-provider";

const ROOT = "__root";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tree: CategoryNode[];
  /** Node being edited; undefined creates one. */
  category?: CategoryNode;
  /** Parent preselected for "add child". */
  parentUuid?: string;
};

/**
 * Create or edit a category: name, slug, description, examples (one per
 * line, the embedding classifier compares records with them) and parent.
 * Department scoping arrives with departments (CF-215).
 */
export function CategoryFormDialog(props: Props) {
  const { open, onOpenChange, category, parentUuid } = props;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {open ? (
          <CategoryForm
            key={`${category?.uuid ?? "new"}:${parentUuid ?? ""}`}
            {...props}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function CategoryForm({ onOpenChange, tree, category, parentUuid }: Props) {
  const { t } = useLocale();
  const save = useSaveCategory();
  const [name, setName] = useState(category?.name ?? "");
  const [slug, setSlug] = useState(category?.slug ?? "");
  const [description, setDescription] = useState(category?.description ?? "");
  const [examples, setExamples] = useState(
    (category?.examples ?? []).join("\n"),
  );
  const [parent, setParent] = useState(
    category?.parent_uuid ?? parentUuid ?? ROOT,
  );

  const parents = useMemo(() => {
    const blocked = category ? subtree(category) : new Set<string>();
    return flatten(tree).filter((n) => !blocked.has(n.uuid) && !n.archived);
  }, [tree, category]);

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    save.mutate(
      {
        uuid: category?.uuid,
        body: {
          name: name.trim(),
          slug: slug.trim() || undefined,
          description: description.trim(),
          examples: parseExamples(examples),
          parent_uuid: parent === ROOT ? "" : parent,
        },
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <form onSubmit={submit} className="space-y-4">
      <DialogHeader>
        <DialogTitle>
          {category
            ? t("categories.form.edit_title")
            : t("categories.form.create_title")}
        </DialogTitle>
        <DialogDescription>
          {t("categories.form.description")}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <Label htmlFor="category-name">{t("categories.form.name")}</Label>
        <Input
          id="category-name"
          value={name}
          maxLength={120}
          required
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="category-slug">{t("categories.form.slug")}</Label>
        <Input
          id="category-slug"
          value={slug}
          maxLength={80}
          placeholder={t("categories.form.slug_placeholder")}
          onChange={(e) => setSlug(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label>{t("categories.form.parent")}</Label>
        <Select value={parent} onValueChange={setParent}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ROOT}>{t("categories.form.root")}</SelectItem>
            {parents.map((n) => (
              <SelectItem key={n.uuid} value={n.uuid}>
                {"— ".repeat(n.depth)}
                {n.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="category-description">
          {t("categories.form.description_label")}
        </Label>
        <Textarea
          id="category-description"
          value={description}
          maxLength={1000}
          rows={2}
          onChange={(e) => setDescription(e.target.value)}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="category-examples">
          {t("categories.form.examples")}
        </Label>
        <Textarea
          id="category-examples"
          value={examples}
          rows={4}
          placeholder={t("categories.form.examples_placeholder")}
          onChange={(e) => setExamples(e.target.value)}
        />
        <p className="text-muted-foreground text-xs">
          {t("categories.form.examples_help")}
        </p>
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
