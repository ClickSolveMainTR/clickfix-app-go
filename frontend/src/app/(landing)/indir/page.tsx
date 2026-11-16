import { renderMetadata, renderPage } from "@/features/landing/pages/download";

export const revalidate = 60;

export async function generateMetadata() {
  return renderMetadata({ searchParams: Promise.resolve({}) });
}
export default async function Page() {
  return renderPage({ searchParams: Promise.resolve({}) });
}
