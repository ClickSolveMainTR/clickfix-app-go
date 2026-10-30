"use client";

import { use } from "react";

import { BookingTypePage } from "@/features/booking";

export default function Page({
  params,
  searchParams,
}: {
  params: Promise<{ org: string; type: string }>;
  searchParams: Promise<{ source?: string }>;
}) {
  const { org, type } = use(params);
  const { source } = use(searchParams);
  return (
    <BookingTypePage
      org={org}
      type={type}
      source={source === "widget" ? "widget" : "public"}
    />
  );
}
