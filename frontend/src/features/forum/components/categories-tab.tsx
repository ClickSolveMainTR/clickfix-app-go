"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, FolderOpen, Plus } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { FormActions } from "@/components/forms/form-actions";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { forumKeys } from "../lib/keys";
import { forumService, slugify } from "../services/forum.service";

const SLUG_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/**
 * Forum categories (CF-78). The API can create categories; renaming and
 * deleting are not part of it yet, so the list is read-only.
 */
export function CategoriesTab({ org }: { org: string }) {
  const { t } = useLocale();
  const [creating, setCreating] = useState(false);
  const list = useQuery({
    queryKey: forumKeys.categories(org),
    queryFn: forumService.categories,
  });

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("forum.categories.hint")}
        </p>
        <div className="flex gap-2">
          <Button size="sm" variant="outline" asChild>
            <a
              href={routes.help.forum(org)}
              target="_blank"
              rel="noopener noreferrer"
            >
              <ExternalLink className="size-4" />
              {t("forum.view_public")}
            </a>
          </Button>
          <Button size="sm" onClick={() => setCreating(true)}>
            <Plus className="size-4" />
            {t("forum.categories.new")}
          </Button>
        </div>
      </div>
      {list.isLoading && <Loading label={t("common.loading")} />}
      {list.isError && (
        <ErrorState
          title={t("forum.error.title")}
          description={t("forum.error.description")}
          onRetry={() => void list.refetch()}
        />
      )}
      {list.data?.length === 0 && (
        <Card>
          <CardContent className="text-muted-foreground py-8 text-center text-sm">
            {t("forum.categories.empty")}
          </CardContent>
        </Card>
      )}
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {(list.data ?? []).map((c) => (
          <Card key={c.uuid}>
            <CardContent className="flex items-center gap-3 py-4">
              <FolderOpen className="text-muted-foreground size-5 shrink-0" />
              <div className="min-w-0">
                <p className="truncate font-medium">{c.name}</p>
                <p className="text-muted-foreground truncate font-mono text-xs">
                  {c.slug}
                </p>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>
      {creating && (
        <CategoryDialog org={org} onClose={() => setCreating(false)} />
      )}
    </div>
  );
}

function CategoryDialog({
  org,
  onClose,
}: {
  org: string;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugTouched, setSlugTouched] = useState(false);
  const effectiveSlug = slugTouched ? slug : slugify(name);
  const create = useMutation({
    mutationFn: () =>
      forumService.createCategory({ name: name.trim(), slug: effectiveSlug }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: forumKeys.categories(org) });
      appToast.success(t("forum.toast.category_created"));
      onClose();
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) && error.status === 409
          ? t("forum.toast.category_exists")
          : isApiError(error)
            ? error.message
            : t("forum.toast.failed"),
      ),
  });
  const invalid =
    !name.trim() || name.length > 120 || !SLUG_RE.test(effectiveSlug);
  return (
    <AppDialog
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("forum.categories.new")}
      description={t("forum.categories.dialog_hint")}
    >
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          if (!invalid) create.mutate();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="forum-cat-name">{t("forum.categories.name")}</Label>
          <Input
            id="forum-cat-name"
            value={name}
            maxLength={120}
            autoFocus
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="forum-cat-slug">{t("forum.categories.slug")}</Label>
          <Input
            id="forum-cat-slug"
            value={effectiveSlug}
            maxLength={160}
            aria-invalid={!!effectiveSlug && !SLUG_RE.test(effectiveSlug)}
            onChange={(e) => {
              setSlugTouched(true);
              setSlug(e.target.value.toLowerCase().trim());
            }}
          />
          <p className="text-muted-foreground text-xs">
            {t("forum.categories.slug_hint")}
          </p>
        </div>
        <FormActions sticky={false}>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={invalid || create.isPending}>
            {t("forum.categories.create")}
          </Button>
        </FormActions>
      </form>
    </AppDialog>
  );
}
