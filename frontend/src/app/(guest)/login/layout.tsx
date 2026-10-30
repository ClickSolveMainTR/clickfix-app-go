import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Giriş yap",
  description: "ClickFix hesabınıza giriş yapın.",
  alternates: { canonical: "/login" },
  openGraph: {
    url: "/login",
    title: "Giriş yap",
    description: "ClickFix hesabınıza giriş yapın.",
  },
  robots: { index: false, follow: true },
};
export default function Layout({ children }: { children: ReactNode }) {
  return children;
}
