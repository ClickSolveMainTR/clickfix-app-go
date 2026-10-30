import {
  FEATURE_PAGE_AREAS,
  findFeatureArea,
} from "@/features/landing/content/areas";
import { featureCopy } from "@/features/landing/content/features";
import { ogImage, OG_SIZE } from "@/features/landing/lib/og";

export const size = OG_SIZE;
export const contentType = "image/png";
export const alt = "ClickFix";

export function generateStaticParams() {
  return FEATURE_PAGE_AREAS.map((area) => ({ slug: area.slug }));
}

/** Share card of a product area page (Turkish, the primary language). */
export default async function OpenGraphImage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const area = findFeatureArea((await params).slug);
  const copy = area ? featureCopy("tr")[area.slug] : undefined;
  return ogImage({
    eyebrow: copy?.eyebrow ?? "ClickFix",
    title: copy?.title ?? "",
  });
}
