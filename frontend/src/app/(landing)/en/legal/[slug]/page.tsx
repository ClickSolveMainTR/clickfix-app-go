import {
  renderMetadata,
  renderPage,
} from "@/features/landing/pages/legal/detail";
import { LEGAL_DOCS } from "@/features/landing/constants";

export const revalidate = 300;
export function generateStaticParams() {
  return LEGAL_DOCS.map((slug) => ({ slug }));
}
export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  return renderMetadata({
    params,
    searchParams: Promise.resolve({ lang: "en" }),
  });
}
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  return renderPage({ params, searchParams: Promise.resolve({ lang: "en" }) });
}
