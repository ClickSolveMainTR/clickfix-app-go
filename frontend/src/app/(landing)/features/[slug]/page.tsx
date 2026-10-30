import {
  renderMetadata,
  renderPage,
} from "@/features/landing/pages/features/detail";
import { FEATURE_PAGE_AREAS } from "@/features/landing/content/areas";

export const revalidate = 300;
export function generateStaticParams() {
  return FEATURE_PAGE_AREAS.map((area) => ({ slug: area.slug }));
}
export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  return renderMetadata({ params, searchParams: Promise.resolve({}) });
}
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  return renderPage({ params, searchParams: Promise.resolve({}) });
}
