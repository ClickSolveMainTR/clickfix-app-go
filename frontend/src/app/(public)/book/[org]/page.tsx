"use client";

import { use } from "react";

import { BookingIndexPage } from "@/features/booking";

export default function Page({
  params,
  searchParams,
}: {
  params: Promise<{ org: string }>;
  searchParams: Promise<{ source?: string }>;
}) {
  const { org } = use(params);
  const { source } = use(searchParams);
  return (
    <BookingIndexPage
      org={org}
      source={source === "widget" ? "widget" : "public"}
    />
  );
}
