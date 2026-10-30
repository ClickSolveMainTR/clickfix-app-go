import type { ReactNode } from "react";

import {
  Breadcrumb,
  type BreadcrumbItem,
} from "@/components/layout/breadcrumb";

type PageHeaderProps = {
  title: string;
  description?: string;
  /** Small uppercase line above the title (section or organization). */
  eyebrow?: string;
  icon?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  actions?: ReactNode;
};

export function PageHeader({
  title,
  description,
  eyebrow,
  icon,
  breadcrumbs,
  actions,
}: PageHeaderProps) {
  return (
    <div className="mb-2 space-y-3" data-guide="page-header">
      {breadcrumbs?.length ? <Breadcrumb items={breadcrumbs} /> : null}
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="min-w-0 space-y-1.5">
          {eyebrow ? (
            <p className="font-accent text-muted-foreground flex items-center gap-2 text-[11px] font-semibold tracking-[0.16em] uppercase">
              <span aria-hidden className="signal-dot size-1.5!" />
              <span className="truncate">{eyebrow}</span>
            </p>
          ) : null}
          <h1 className="font-display flex items-center gap-2.5 text-2xl font-semibold tracking-tight text-balance sm:text-[1.75rem]">
            {icon ? (
              <span className="text-muted-foreground inline-flex shrink-0">
                {icon}
              </span>
            ) : null}
            {title}
          </h1>
          {description ? (
            <p className="text-muted-foreground max-w-2xl text-pretty">
              {description}
            </p>
          ) : null}
        </div>
        {actions ? (
          <div
            className="flex flex-wrap items-center gap-2"
            data-guide="page-actions"
          >
            {actions}
          </div>
        ) : null}
      </div>
    </div>
  );
}
