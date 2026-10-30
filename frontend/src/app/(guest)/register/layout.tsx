import type { Metadata } from "next";
import type { ReactNode } from "react";

export const metadata: Metadata = {
  title: "Kayıt ol",
  description:
    "ClickFix hesabınızı oluşturun ve müşteri desteğinizi tek yerde yönetin.",
  alternates: { canonical: "/register" },
  openGraph: {
    url: "/register",
    title: "Kayıt ol",
    description:
      "ClickFix hesabınızı oluşturun ve müşteri desteğinizi tek yerde yönetin.",
  },
  robots: { index: true, follow: true },
};
export default function Layout({ children }: { children: ReactNode }) {
  return children;
}
