"use client";

import { useQuery } from "@tanstack/react-query";
import { Share2, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

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
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useDepartmentOptions,
  useDepartmentShares,
  useShareKnowledge,
  useUnshareKnowledge,
} from "@/features/departments/hooks/use-departments";
import type { Department, ShareKind } from "@/features/departments/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Item = { uuid: string; title: string };

/** Knowledge items of the department, per kind, for the share picker. */
function useDepartmentItems(department: string, kind: ShareKind) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: ["tenant", slug, "department-items", department, kind],
    enabled: Boolean(slug),
    queryFn: async (): Promise<Item[]> => {
      if (kind === "article") {
        const page = await tenantRequest<{
          items: { uuid: string; title: string }[];
        }>(slug, "GET", "/v1/tenant/knowledge/articles", {
          query: { department, limit: 100 },
        });
        return page.items;
      }
      const path =
        kind === "source"
          ? "/v1/tenant/knowledge/sources"
          : "/v1/tenant/ai/business-context";
      const page = await tenantRequest<{
        items: {
          uuid: string;
          title: string;
          department?: { uuid: string } | null;
        }[];
      }>(slug, "GET", path, { query: { limit: 100 } });
      return page.items.filter((i) => i.department?.uuid === department);
    },
  });
}

function ShareDialog({
  department,
  open,
  onClose,
}: {
  department: Department;
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [kind, setKind] = useState<ShareKind>("article");
  const [item, setItem] = useState("");
  const [target, setTarget] = useState("");
  const items = useDepartmentItems(department.uuid, kind);
  const { options } = useDepartmentOptions();
  const share = useShareKnowledge();
  const targets = options.filter((o) => o.value !== department.uuid);
  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("departments.shares.dialog_title")}</DialogTitle>
          <DialogDescription>
            {t("departments.shares.dialog_description", {
              name: department.name,
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="share-kind">{t("departments.shares.kind")}</Label>
            <Select
              value={kind}
              onValueChange={(v) => {
                setKind(v as ShareKind);
                setItem("");
              }}
            >
              <SelectTrigger id="share-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {(["article", "source", "context"] as const).map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(`departments.shares.kinds.${k}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="share-item">{t("departments.shares.item")}</Label>
            <Select value={item} onValueChange={setItem}>
              <SelectTrigger id="share-item" className="w-full">
                <SelectValue
                  placeholder={
                    items.data?.length
                      ? t("departments.shares.item_placeholder")
                      : t("departments.shares.no_items")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {(items.data ?? []).map((i) => (
                  <SelectItem key={i.uuid} value={i.uuid}>
                    {i.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="share-target">
              {t("departments.shares.target")}
            </Label>
            <Select value={target} onValueChange={setTarget}>
              <SelectTrigger id="share-target" className="w-full">
                <SelectValue
                  placeholder={t("departments.shares.target_placeholder")}
                />
              </SelectTrigger>
              <SelectContent>
                {targets.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!item || !target || share.isPending}
            onClick={() =>
              share.mutate(
                {
                  target,
                  body: { resource_kind: kind, resource_uuid: item },
                },
                { onSuccess: onClose },
              )
            }
          >
            {t("departments.shares.submit")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Knowledge is closed per department (CF-215): this tab lists what the
 * department received and what it shared. A lead of the department (or a
 * manager) shares one item with another department; the API refuses others.
 */
export function DepartmentSharesTab({
  department,
  canShare,
}: {
  department: Department;
  canShare: boolean;
}) {
  const { t, locale } = useLocale();
  const shares = useDepartmentShares(department.uuid);
  const unshare = useUnshareKnowledge();
  const [open, setOpen] = useState(false);
  const rows = useMemo(() => shares.data?.items ?? [], [shares.data]);
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between gap-4">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("departments.shares.explainer")}
        </p>
        {canShare ? (
          <Button type="button" size="sm" onClick={() => setOpen(true)}>
            <Share2 className="size-4" />
            {t("departments.shares.add")}
          </Button>
        ) : null}
      </div>
      {rows.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("departments.shares.empty")}
        </p>
      ) : (
        <ul className="divide-y rounded-md border">
          {rows.map((s) => {
            const received = s.department.uuid === department.uuid;
            return (
              <li key={s.uuid} className="flex items-center gap-3 px-3 py-2">
                <Badge variant="outline">
                  {t(`departments.shares.kinds.${s.resource_kind}`)}
                </Badge>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {s.resource_title || s.resource_uuid}
                  </p>
                  <p className="text-muted-foreground truncate text-xs">
                    {received
                      ? t("departments.shares.received_from", {
                          name: s.owner?.name ?? "—",
                        })
                      : t("departments.shares.shared_with", {
                          name: s.department.name,
                        })}
                    {" · "}
                    {datetime(s.created_at, undefined, locale)}
                  </p>
                </div>
                {canShare ? (
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    aria-label={t("departments.shares.remove")}
                    disabled={unshare.isPending}
                    onClick={() =>
                      unshare.mutate({
                        department: s.department.uuid,
                        share: s.uuid,
                      })
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
      <ShareDialog
        department={department}
        open={open}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}
