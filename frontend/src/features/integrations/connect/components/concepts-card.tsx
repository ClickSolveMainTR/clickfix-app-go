"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Boxes, Plus } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

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
import { routes } from "@/config/routes";
import {
  customFieldsKeys,
  customFieldsService,
} from "@/features/custom-fields";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import { toKey } from "../lib/keys";
import { connectKeys, type SystemConcept } from "../services/connect.service";
import { useConnectError } from "./use-connect-error";

/**
 * Concepts are the organization's custom object types (Fatura, Lisans,
 * Sipariş...). Picking one opens where its data comes from.
 */
export function ConceptsCard({
  slug,
  concepts,
  selected,
  onSelect,
  canCreate,
}: {
  slug: string;
  concepts: SystemConcept[];
  selected: string | null;
  onSelect: (uuid: string) => void;
  canCreate: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const onError = useConnectError();
  const [label, setLabel] = useState("");
  const [plural, setPlural] = useState("");
  const create = useMutation({
    mutationFn: () =>
      customFieldsService.createObjectType({
        key: toKey(label, 60),
        singular_label: label.trim(),
        plural_label: (plural || label).trim(),
      }),
    onSuccess: async (type) => {
      setLabel("");
      setPlural("");
      await Promise.all([
        qc.invalidateQueries({ queryKey: connectKeys.overview(slug) }),
        qc.invalidateQueries({ queryKey: customFieldsKeys.objectTypes(slug) }),
      ]);
      onSelect(type.uuid);
      appToast.success(t("integrations_connect.concepts.created"));
    },
    onError,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Boxes className="size-4" />
          {t("integrations_connect.concepts.title")}
        </CardTitle>
        <CardDescription>
          {t("integrations_connect.concepts.description")}{" "}
          <Link
            className="underline underline-offset-2"
            href={routes.cms.settings.customObjects}
          >
            {t("integrations_connect.concepts.manage_fields")}
          </Link>
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {concepts.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("integrations_connect.concepts.empty")}
          </p>
        ) : (
          <div className="flex flex-wrap gap-2">
            {concepts.map((c) => (
              <button
                key={c.object_type_uuid}
                type="button"
                onClick={() => onSelect(c.object_type_uuid)}
                className={cn(
                  "hover:bg-muted flex items-center gap-2 rounded-md border px-3 py-2 text-sm",
                  selected === c.object_type_uuid &&
                    "border-primary bg-primary/5",
                )}
              >
                <span className="font-medium">{c.label}</span>
                <code className="text-muted-foreground text-xs">{c.key}</code>
                {c.source ? (
                  <Badge variant={c.source.enabled ? "success" : "secondary"}>
                    {t(
                      c.source.enabled
                        ? "integrations_connect.concepts.connected"
                        : "integrations_connect.concepts.off",
                    )}
                  </Badge>
                ) : (
                  <Badge variant="outline">
                    {t("integrations_connect.concepts.no_source")}
                  </Badge>
                )}
              </button>
            ))}
          </div>
        )}
        {canCreate && (
          <form
            className="flex flex-wrap items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (toKey(label, 60)) create.mutate();
            }}
          >
            <Input
              className="w-48"
              value={label}
              maxLength={80}
              placeholder={t("integrations_connect.concepts.new_singular")}
              onChange={(e) => setLabel(e.target.value)}
            />
            <Input
              className="w-48"
              value={plural}
              maxLength={80}
              placeholder={t("integrations_connect.concepts.new_plural")}
              onChange={(e) => setPlural(e.target.value)}
            />
            <Button
              type="submit"
              variant="outline"
              disabled={!toKey(label, 60) || create.isPending}
            >
              <Plus className="size-4" />
              {t("integrations_connect.concepts.add")}
            </Button>
          </form>
        )}
      </CardContent>
    </Card>
  );
}
