import { notFound } from "next/navigation";

import { HelpNotFoundError } from "./api";

/** Runs a help center fetch; a missing organization or page becomes a 404. */
export async function orNotFound<T>(promise: Promise<T>): Promise<T> {
  try {
    return await promise;
  } catch (err) {
    if (err instanceof HelpNotFoundError) notFound();
    throw err;
  }
}
