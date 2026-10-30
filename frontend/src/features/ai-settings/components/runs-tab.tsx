"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { useLocale } from "@/providers/locale-provider";

import { aiSettingsService } from "../services/ai-settings.service";
import { RunLog } from "./run-log";
import type { AISettingsTabProps } from "./tab-registry";

/** "Çalışma kaydı" tab (CF-69). The open run lives in `?run=` so it can be linked. */
export function RunsTab({ slug }: AISettingsTabProps) {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const selected = searchParams.get("run");
  const onSelect = (uuid: string | null) => {
    const params = new URLSearchParams(searchParams.toString());
    if (uuid) params.set("run", uuid);
    else params.delete("run");
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };
  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("ai_settings.runs.description")}
      </p>
      <RunLog
        queryKey={["tenant", slug, "ai", "runs"]}
        fetchPage={(offset, limit) =>
          aiSettingsService.runs(slug, offset, limit)
        }
        fetchRun={(uuid) => aiSettingsService.run(slug, uuid)}
        selected={selected}
        onSelect={onSelect}
      />
    </div>
  );
}
