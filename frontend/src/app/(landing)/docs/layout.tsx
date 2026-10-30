import type { ReactNode } from "react";
import { DocsSiteLayout } from "@/features/docs/layout";
import "@/app/(landing)/docs/docs.css";
export default function Layout({ children }: { children: ReactNode }) {
  return <DocsSiteLayout locale="tr">{children}</DocsSiteLayout>;
}
