"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Check, Database, Play, Trash2, X } from "lucide-react";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { display, getPath, itemsAt } from "../lib/json";
import {
  connectKeys,
  connectService,
  type SystemConcept,
  type SystemField,
  type SystemTestResult,
} from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

const PLACEHOLDERS = ["{external_id}", "{company_external_id}"] as const;

function lastSegment(path: string): string {
  const parts = path.split(".").filter((p) => !/^\d+$/.test(p));
  return parts[parts.length - 1] ?? path;
}

/**
 * "Bilgi nereden gelsin": the concept's endpoint, tried with a real
 * external id. Fields are picked by clicking the answer, not typed.
 */
export function SourceCard({
  slug,
  concept,
  connected,
  canManage,
}: {
  slug: string;
  concept: SystemConcept;
  connected: boolean;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const source = concept.source;
  const [path, setPath] = useState(
    source?.path_template ?? `/${concept.key}?musteri={external_id}`,
  );
  const [itemsPath, setItemsPath] = useState(source?.items_path ?? "");
  const [fields, setFields] = useState<SystemField[]>(source?.fields ?? []);
  const [enabled, setEnabled] = useState(source?.enabled ?? true);
  const [externalId, setExternalId] = useState("");
  const [companyId, setCompanyId] = useState("");
  const [result, setResult] = useState<SystemTestResult | null>(null);

  const identityKey = path.includes("{external_id}")
    ? "external_id"
    : "company_external_id";

  const test = useMutation({
    mutationFn: (items: string) =>
      connectService.testSource(slug, concept.object_type_uuid, {
        external_id: externalId,
        company_external_id: companyId,
        path_template: path,
        items_path: items,
      }),
    onSuccess: async (r) => {
      setResult(r);
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
    },
    onError,
  });
  const save = useMutation({
    mutationFn: () =>
      connectService.saveSource(slug, concept.object_type_uuid, {
        path_template: path,
        identity_key: identityKey,
        items_path: itemsPath,
        fields,
        enabled,
      }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.source.saved"));
    },
    onError,
  });
  const remove = useMutation({
    mutationFn: () =>
      connectService.deleteSource(slug, concept.object_type_uuid),
    onSuccess: async () => {
      setFields([]);
      setResult(null);
      await qc.invalidateQueries({ queryKey: connectKeys.overview(slug) });
      appToast.success(t("integrations_connect.source.deleted"));
    },
    onError,
  });

  const leafPaths = (result?.paths ?? []).filter((p) => p.type !== "array");
  const arrayPaths = (result?.paths ?? []).filter((p) => p.type === "array");
  const preview = useMemo(() => {
    if (!result?.ok) return [];
    return itemsAt(result.body, itemsPath).map((item) =>
      fields.map((f) => getPath(item, f.path)),
    );
  }, [result, itemsPath, fields]);

  const toggle = (p: string) =>
    setFields((cur) =>
      cur.some((f) => f.path === p)
        ? cur.filter((f) => f.path !== p)
        : [...cur, { path: p, label: lastSegment(p) }],
    );
  const pickItems = (p: string) => {
    setItemsPath(p);
    setFields([]);
    test.mutate(p);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Database className="size-4" />
          {t("integrations_connect.source.title", { concept: concept.label })}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.source.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {!connected && (
          <p className="text-sm text-amber-600">
            {t("integrations_connect.source.connect_first")}
          </p>
        )}
        <div className="space-y-2">
          <Label htmlFor="src-path">
            {t("integrations_connect.source.path")}
          </Label>
          <div className="flex flex-wrap gap-2">
            <Input
              id="src-path"
              className="min-w-64 flex-1 font-mono"
              value={path}
              maxLength={500}
              disabled={!canManage}
              onChange={(e) => setPath(e.target.value)}
            />
            {PLACEHOLDERS.map((ph) => (
              <Button
                key={ph}
                type="button"
                size="sm"
                variant="outline"
                disabled={!canManage}
                onClick={() => setPath((p) => p + ph)}
              >
                <code>{ph}</code>
              </Button>
            ))}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("integrations_connect.source.path_help")}
          </p>
        </div>

        <div className="flex flex-wrap items-end gap-2">
          <div className="space-y-2">
            <Label htmlFor="src-ext">
              {t("integrations_connect.source.try_external_id")}
            </Label>
            <Input
              id="src-ext"
              className="w-52"
              value={externalId}
              maxLength={255}
              placeholder="CUST-42"
              onChange={(e) => setExternalId(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="src-comp">
              {t("integrations_connect.source.try_company_id")}
            </Label>
            <Input
              id="src-comp"
              className="w-52"
              value={companyId}
              maxLength={255}
              onChange={(e) => setCompanyId(e.target.value)}
            />
          </div>
          <Button
            type="button"
            disabled={!canManage || !connected || test.isPending}
            onClick={() => test.mutate(itemsPath)}
          >
            <Play className="size-4" />
            {t("integrations_connect.source.try")}
          </Button>
        </div>

        {result && (
          <div className="space-y-4 rounded-md border p-3">
            <div className="flex flex-wrap items-center gap-2 text-sm">
              <Badge variant={result.ok ? "success" : "danger"}>
                {result.ok
                  ? `HTTP ${result.status_code}`
                  : result.error_code || `HTTP ${result.status_code}`}
              </Badge>
              <code className="text-muted-foreground text-xs break-all">
                {result.request}
              </code>
              <span className="text-muted-foreground text-xs">
                {result.duration_ms} ms
              </span>
            </div>
            {!result.ok && <p className="text-sm">{result.message}</p>}
            {arrayPaths.length > 0 && (
              <div className="space-y-2">
                <p className="text-sm font-medium">
                  {t("integrations_connect.source.items_path")}
                </p>
                <div className="flex flex-wrap gap-2">
                  {[{ path: "" }, ...arrayPaths].map((a) => (
                    <Button
                      key={a.path || "root"}
                      type="button"
                      size="sm"
                      variant={itemsPath === a.path ? "default" : "outline"}
                      onClick={() => pickItems(a.path)}
                    >
                      <code>
                        {a.path || t("integrations_connect.source.root")}
                      </code>
                    </Button>
                  ))}
                </div>
              </div>
            )}
            {leafPaths.length > 0 && (
              <div className="space-y-2">
                <p className="text-sm font-medium">
                  {t("integrations_connect.source.pick_fields")}
                </p>
                <div className="grid gap-1 sm:grid-cols-2 lg:grid-cols-3">
                  {leafPaths.map((p) => {
                    const on = fields.some((f) => f.path === p.path);
                    return (
                      <button
                        key={p.path}
                        type="button"
                        disabled={!canManage}
                        onClick={() => toggle(p.path)}
                        className={cn(
                          "hover:bg-muted flex min-w-0 items-center gap-2 rounded border px-2 py-1 text-left text-xs",
                          on && "border-primary bg-primary/5",
                        )}
                      >
                        {on ? (
                          <Check className="size-3 shrink-0" />
                        ) : (
                          <span className="size-3 shrink-0" />
                        )}
                        <code className="shrink-0">{p.path}</code>
                        <span className="text-muted-foreground truncate">
                          {display(p.sample)}
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        )}

        {fields.length > 0 && (
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {t("integrations_connect.source.fields")}
            </p>
            {fields.map((f, i) => (
              <div key={f.path} className="flex items-center gap-2">
                <code className="w-48 truncate text-xs">{f.path}</code>
                <Input
                  className="h-8 max-w-xs"
                  value={f.label}
                  maxLength={80}
                  disabled={!canManage}
                  aria-label={t("integrations_connect.source.field_label")}
                  onChange={(e) =>
                    setFields((cur) =>
                      cur.map((x, j) =>
                        j === i ? { ...x, label: e.target.value } : x,
                      ),
                    )
                  }
                />
                {canManage && (
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    aria-label={t("integrations_connect.remove")}
                    onClick={() => toggle(f.path)}
                  >
                    <X className="size-4" />
                  </Button>
                )}
              </div>
            ))}
            {preview.length > 0 && (
              <div className="overflow-x-auto rounded-md border">
                <table className="w-full text-xs">
                  <thead className="bg-muted/50">
                    <tr>
                      {fields.map((f) => (
                        <th key={f.path} className="px-2 py-1 text-left">
                          {f.label || f.path}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {preview.map((row, i) => (
                      <tr key={i} className="border-t">
                        {row.map((v, j) => (
                          <td key={j} className="px-2 py-1">
                            {display(v)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}

        {canManage && (
          <div className="flex flex-wrap items-center gap-3">
            <label className="flex items-center gap-2 text-sm">
              <Switch checked={enabled} onCheckedChange={setEnabled} />
              {t("integrations_connect.enabled")}
            </label>
            <Button
              type="button"
              disabled={save.isPending || !path}
              onClick={() => save.mutate()}
            >
              {t("integrations_connect.save")}
            </Button>
            {source && (
              <Button
                type="button"
                variant="ghost"
                disabled={remove.isPending}
                onClick={() => remove.mutate()}
              >
                <Trash2 className="size-4" />
                {t("integrations_connect.source.delete")}
              </Button>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
