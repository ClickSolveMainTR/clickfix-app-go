"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  ExternalLink,
  EyeOff,
  FileText,
  Save,
  Send,
  Trash2,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EditorX } from "@/components/editor/editor-x";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ArticleBrandAssign } from "@/features/brands/components/article-brand-assign";
import { useDepartmentOptions } from "@/features/departments/hooks/use-departments";
import { useActiveOrganization } from "@/features/organizations";
import { useUnsavedChangesGuard } from "@/hooks/use-unsaved-changes-guard";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { collectionOptions, collectionTree } from "../lib/collections";
import { isValidSlug, slugify } from "../lib/slugify";
import {
  knowledgeKeys,
  knowledgeService,
  type KnowledgeArticle,
  type KnowledgeArticleInput,
  type KnowledgeArticleVersion,
} from "../services/knowledge.service";
import { ArticleAudience } from "./article-audience";
import { articleState, ArticleStateBadge } from "./article-state-badge";
import { TagsInput } from "./tags-input";
import { VersionHistory } from "./version-history";

const NONE = "none";

type Draft = {
  title: string;
  slug: string;
  /** False while the slug follows the title. */
  slugTouched: boolean;
  locale: "tr" | "en";
  visibility: "public" | "internal";
  collection: string;
  /** Department uuid or NONE (organization-wide, CF-215). */
  department: string;
  tags: string[];
  body: string;
};

function draftOf(a: KnowledgeArticle | null, collectionUuid: string): Draft {
  return {
    title: a?.title ?? "",
    slug: a?.slug ?? "",
    slugTouched: a !== null,
    locale: a?.locale ?? "tr",
    visibility: a?.visibility ?? "public",
    collection: collectionUuid,
    department: a?.department?.uuid ?? NONE,
    tags: a?.tags ?? [],
    body: a?.body ?? "",
  };
}

function sameDraft(a: Draft, b: Draft) {
  return (
    a.title === b.title &&
    (a.slugTouched ? a.slug : slugify(a.title)) ===
      (b.slugTouched ? b.slug : slugify(b.title)) &&
    a.locale === b.locale &&
    a.visibility === b.visibility &&
    a.collection === b.collection &&
    a.department === b.department &&
    a.tags.join("\u0000") === b.tags.join("\u0000") &&
    a.body.trim() === b.body.trim()
  );
}

/** Article editor (CF-73): `uuid` undefined creates a new draft. */
export function ArticleEditorPage({ uuid }: { uuid?: string }) {
  const { t, locale: uiLocale } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const { active: org } = useActiveOrganization();
  const perms = org?.permissions ?? [];
  const canRead = perms.includes(Permission.TenantSettingsRead);
  const canWrite = perms.includes(Permission.TenantSettingsWrite);

  const article = useQuery({
    queryKey: knowledgeKeys.article(org?.slug, uuid ?? "new"),
    queryFn: () => knowledgeService.article(uuid!),
    enabled: Boolean(org && uuid && canRead),
  });
  const collections = useQuery({
    queryKey: knowledgeKeys.collections(org?.slug),
    queryFn: knowledgeService.collections,
    enabled: Boolean(org && canRead),
  });

  if (org && !canRead) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("knowledge.forbidden")}
      </p>
    );
  }
  if ((uuid && article.isLoading) || collections.isLoading) {
    return <Loading label={t("common.loading")} />;
  }
  if ((uuid && article.isError) || collections.isError || !org) {
    return (
      <ErrorState
        title={t("knowledge.error.article")}
        description={t("knowledge.error.description")}
        onRetry={() => {
          void article.refetch();
          void collections.refetch();
        }}
      />
    );
  }
  const current = uuid ? (article.data ?? null) : null;
  const collectionUuid =
    current?.collection_id != null
      ? (collections.data?.find((c) => c.id === current.collection_id)?.uuid ??
        NONE)
      : NONE;

  return (
    <Editor
      // Remount when another revision is loaded so the form starts clean.
      key={`${current?.uuid ?? "new"}:${current?.version ?? 0}`}
      article={current}
      initial={draftOf(current, collectionUuid)}
      canWrite={canWrite}
      orgSlug={org.slug}
      collectionsTree={collectionTree(collections.data ?? [], uiLocale)}
      onSaved={(saved, created) => {
        qc.setQueryData(knowledgeKeys.article(org.slug, saved.uuid), saved);
        void qc.invalidateQueries({ queryKey: knowledgeKeys.all(org.slug) });
        if (created) {
          router.replace(routes.cms.settings.knowledge.detail(saved.uuid));
        }
      }}
      onReload={() => void article.refetch()}
      onDeleted={() => {
        void qc.invalidateQueries({ queryKey: knowledgeKeys.all(org.slug) });
        router.push(routes.cms.settings.knowledge.root);
      }}
    />
  );
}

function Editor({
  article,
  initial,
  canWrite,
  orgSlug,
  collectionsTree,
  onSaved,
  onReload,
  onDeleted,
}: {
  article: KnowledgeArticle | null;
  initial: Draft;
  canWrite: boolean;
  orgSlug: string;
  collectionsTree: ReturnType<typeof collectionTree>;
  onSaved: (article: KnowledgeArticle, created: boolean) => void;
  onReload: () => void;
  onDeleted: () => void;
}) {
  const { t, locale: uiLocale } = useLocale();
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Draft>(initial);
  /** Bumped to remount EditorX with new markdown (version restore). */
  const [editorKey, setEditorKey] = useState(0);
  const [conflict, setConflict] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const options = useMemo(
    () => collectionOptions(collectionsTree),
    [collectionsTree],
  );

  const slug = draft.slugTouched ? draft.slug : slugify(draft.title);
  const published = article?.published_version != null;
  const dirty = !article || !sameDraft(draft, initial);
  const slugOk = isValidSlug(slug);
  const valid =
    draft.title.trim() !== "" && draft.title.length <= 300 && slugOk;
  const state = article
    ? articleState(article.version, article.published_version)
    : "draft";

  useUnsavedChangesGuard(dirty && canWrite);

  const { active: activeOrg } = useActiveOrganization();
  const canReadDepartments = (activeOrg?.permissions ?? []).includes(
    Permission.TenantDepartmentsRead,
  );
  const { options: departmentOptions } = useDepartmentOptions();
  const set = <K extends keyof Draft>(key: K, value: Draft[K]) =>
    setDraft((d) => ({ ...d, [key]: value }));

  const input = (): KnowledgeArticleInput => ({
    title: draft.title.trim(),
    slug,
    locale: draft.locale,
    visibility: draft.visibility,
    body: draft.body,
    tags: draft.tags,
    collection_uuid: draft.collection === NONE ? null : draft.collection,
    department_uuid: draft.department === NONE ? null : draft.department,
    ...(article ? { version: article.version } : {}),
  });

  const fail = (error: unknown) => {
    if (isApiError(error) && error.status === 409) {
      setConflict(true);
      return;
    }
    appToast.error(
      isApiError(error) && error.status === 422
        ? t("knowledge.editor.invalid")
        : t("knowledge.toast.failed"),
    );
  };

  const saveRequest = async () =>
    article
      ? knowledgeService.update(article.uuid, input())
      : knowledgeService.create(input());

  const save = useMutation({
    mutationFn: saveRequest,
    onSuccess: (saved) => {
      appToast.success(t("knowledge.editor.saved"));
      onSaved(saved, !article);
    },
    onError: fail,
  });

  const publication = useMutation({
    mutationFn: async (publish: boolean) => {
      let target = article;
      if (publish && (dirty || !target)) target = await saveRequest();
      if (!target) throw new Error("unsaved");
      await knowledgeService.publish(target.uuid, target.version, publish);
      return {
        ...target,
        published_version: publish ? target.version : null,
      } as KnowledgeArticle;
    },
    onSuccess: (saved, publish) => {
      appToast.success(
        publish
          ? t("knowledge.editor.published")
          : t("knowledge.editor.unpublished"),
      );
      onSaved(saved, !article);
    },
    onError: fail,
  });

  const remove = useMutation({
    mutationFn: () => knowledgeService.remove(article!.uuid),
    onSuccess: () => {
      setConfirmDelete(false);
      appToast.success(t("knowledge.editor.deleted"));
      onDeleted();
    },
    onError: (error) => {
      setConfirmDelete(false);
      fail(error);
    },
  });

  const restore = (v: KnowledgeArticleVersion) => {
    setDraft((d) => ({
      ...d,
      title: v.title ?? d.title,
      body: v.body ?? "",
      visibility: v.visibility === "internal" ? "internal" : "public",
    }));
    setEditorKey((k) => k + 1);
    appToast.success(
      t("knowledge.versions.restored", { version: v.version ?? 0 }),
    );
  };

  const busy = save.isPending || publication.isPending || remove.isPending;
  const readOnly = !canWrite;
  const liveUrl =
    article && published && article.visibility === "public"
      ? routes.help.article(orgSlug, article.slug, article.locale)
      : null;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={<FileText className="size-7" />}
        title={article ? article.title : t("knowledge.editor.new_title")}
        breadcrumbs={[
          {
            label: t("knowledge.title"),
            href: routes.cms.settings.knowledge.root,
          },
          {
            label: article ? article.title : t("knowledge.editor.new_title"),
          },
        ]}
        actions={
          canWrite ? (
            <div className="flex flex-wrap gap-2">
              {article && (
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() => setConfirmDelete(true)}
                >
                  <Trash2 className="size-4" />
                  {t("knowledge.delete")}
                </Button>
              )}
              {published && (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => publication.mutate(false)}
                >
                  <EyeOff className="size-4" />
                  {t("knowledge.editor.unpublish")}
                </Button>
              )}
              <Button
                variant="outline"
                size="sm"
                disabled={busy || !valid || !dirty}
                onClick={() => save.mutate()}
              >
                <Save className="size-4" />
                {t("knowledge.editor.save_draft")}
              </Button>
              <Button
                size="sm"
                disabled={busy || !valid || (state === "published" && !dirty)}
                onClick={() => publication.mutate(true)}
              >
                <Send className="size-4" />
                {dirty || !article
                  ? t("knowledge.editor.save_publish")
                  : t("knowledge.editor.publish")}
              </Button>
            </div>
          ) : undefined
        }
      />

      {conflict && (
        <Alert variant="destructive">
          <AlertTriangle />
          <AlertTitle>{t("knowledge.editor.conflict_title")}</AlertTitle>
          <AlertDescription>
            <p>{t("knowledge.editor.conflict_description")}</p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => {
                setConflict(false);
                onReload();
                void qc.invalidateQueries({
                  queryKey: knowledgeKeys.all(orgSlug),
                });
              }}
            >
              {t("knowledge.editor.reload")}
            </Button>
          </AlertDescription>
        </Alert>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-4">
          <div className="space-y-2">
            <Label htmlFor="kb-title" className="sr-only">
              {t("knowledge.fields.title")}
            </Label>
            <Input
              id="kb-title"
              value={draft.title}
              maxLength={300}
              readOnly={readOnly}
              placeholder={t("knowledge.fields.title_placeholder")}
              className="font-display h-12 text-xl font-semibold md:text-2xl"
              onChange={(e) => set("title", e.target.value)}
            />
          </div>
          <div className="bg-card overflow-hidden rounded-xl border">
            <EditorX
              key={editorKey}
              initialMarkdown={draft.body}
              editable={!readOnly}
              placeholder={t("knowledge.fields.body_placeholder")}
              onMarkdownChange={(md) => set("body", md)}
              contentClassName="min-h-[420px]"
            />
          </div>
        </div>

        <div className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center justify-between gap-2 text-base">
                {t("knowledge.editor.details")}
                {article && (
                  <ArticleStateBadge
                    version={article.version}
                    publishedVersion={article.published_version}
                  />
                )}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="kb-slug">{t("knowledge.fields.slug")}</Label>
                <Input
                  id="kb-slug"
                  value={slug}
                  maxLength={200}
                  readOnly={readOnly || published}
                  aria-invalid={slug !== "" && !slugOk}
                  aria-describedby="kb-slug-hint"
                  className="font-mono text-sm"
                  onChange={(e) =>
                    setDraft((d) => ({
                      ...d,
                      slug: e.target.value.toLowerCase(),
                      slugTouched: true,
                    }))
                  }
                />
                <p id="kb-slug-hint" className="text-muted-foreground text-xs">
                  {published
                    ? t("knowledge.editor.slug_locked")
                    : slug !== "" && !slugOk
                      ? t("knowledge.slug_invalid")
                      : t("knowledge.slug_hint")}
                </p>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-2">
                  <Label htmlFor="kb-locale">
                    {t("knowledge.fields.locale")}
                  </Label>
                  <Select
                    value={draft.locale}
                    disabled={readOnly || article !== null}
                    onValueChange={(v) => set("locale", v as Draft["locale"])}
                  >
                    <SelectTrigger id="kb-locale" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="tr">
                        {t("knowledge.locale.tr")}
                      </SelectItem>
                      <SelectItem value="en">
                        {t("knowledge.locale.en")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="kb-visibility">
                    {t("knowledge.fields.visibility")}
                  </Label>
                  <Select
                    value={draft.visibility}
                    disabled={readOnly}
                    onValueChange={(v) =>
                      set("visibility", v as Draft["visibility"])
                    }
                  >
                    <SelectTrigger id="kb-visibility" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="public">
                        {t("knowledge.visibility.public")}
                      </SelectItem>
                      <SelectItem value="internal">
                        {t("knowledge.visibility.internal")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <p className="text-muted-foreground -mt-2 text-xs">
                {t(`knowledge.visibility.${draft.visibility}_hint`)}
              </p>
              <div className="space-y-2">
                <Label htmlFor="kb-collection">
                  {t("knowledge.fields.collection")}
                </Label>
                <Select
                  value={draft.collection}
                  disabled={readOnly}
                  onValueChange={(v) => set("collection", v)}
                >
                  <SelectTrigger id="kb-collection" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>
                      {t("knowledge.no_collection")}
                    </SelectItem>
                    {options.map(({ collection, depth }) => (
                      <SelectItem key={collection.uuid} value={collection.uuid}>
                        {depth > 0 ? `— ${collection.name}` : collection.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              {canReadDepartments ? (
                <div className="space-y-2">
                  <Label htmlFor="kb-department">
                    {t("knowledge.fields.department")}
                  </Label>
                  <Select
                    value={draft.department}
                    disabled={readOnly}
                    onValueChange={(v) => set("department", v)}
                  >
                    <SelectTrigger id="kb-department" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={NONE}>
                        {t("knowledge.department_organization_wide")}
                      </SelectItem>
                      {departmentOptions.map((d) => (
                        <SelectItem key={d.value} value={d.value}>
                          {d.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <p className="text-muted-foreground text-xs">
                    {t("knowledge.department_hint")}
                  </p>
                </div>
              ) : null}
              <div className="space-y-2">
                <Label htmlFor="kb-tags">{t("knowledge.fields.tags")}</Label>
                <TagsInput
                  id="kb-tags"
                  value={draft.tags}
                  disabled={readOnly}
                  onChange={(tags) => set("tags", tags)}
                />
              </div>
              {article && <ArticleBrandAssign articleUuid={article.uuid} />}
              {article && (
                <ArticleAudience
                  articleUuid={article.uuid}
                  readOnly={readOnly}
                />
              )}
              {article && (
                <dl className="text-muted-foreground grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 border-t pt-3 text-xs">
                  <dt>{t("knowledge.editor.version")}</dt>
                  <dd className="text-foreground tabular-nums">
                    {t("knowledge.version_short", { version: article.version })}
                    {article.published_version != null &&
                      ` · ${t("knowledge.editor.live_version", { version: article.published_version })}`}
                  </dd>
                  <dt>{t("knowledge.editor.updated_at")}</dt>
                  <dd className="text-foreground">
                    {datetime(article.updated_at, undefined, uiLocale)}
                  </dd>
                  <dt>{t("knowledge.editor.created_at")}</dt>
                  <dd className="text-foreground">
                    {datetime(article.created_at, undefined, uiLocale)}
                  </dd>
                </dl>
              )}
              {liveUrl && (
                <Button variant="outline" size="sm" className="w-full" asChild>
                  <a href={liveUrl} target="_blank" rel="noopener noreferrer">
                    <ExternalLink className="size-4" />
                    {t("knowledge.editor.view_live")}
                  </a>
                </Button>
              )}
              {dirty && article && canWrite && (
                <p className="text-muted-foreground text-xs" role="status">
                  {t("knowledge.editor.unsaved")}
                </p>
              )}
            </CardContent>
          </Card>
          {article && (
            <VersionHistory
              uuid={article.uuid}
              currentVersion={article.version}
              publishedVersion={article.published_version ?? null}
              canWrite={canWrite}
              onRestore={restore}
            />
          )}
        </div>
      </div>

      <ConfirmDialog
        open={confirmDelete}
        title={t("knowledge.editor.confirm_delete_title")}
        description={t("knowledge.editor.confirm_delete_description")}
        confirmLabel={t("knowledge.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onConfirm={() => remove.mutate()}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  );
}
