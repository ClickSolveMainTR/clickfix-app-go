import { llmsFull } from "@/lib/docs/llms";

export const dynamic = "force-static";

/** `/en/llms-full.txt`: every English docs page as Markdown (CF-208). */
export async function GET() {
  return new Response(await llmsFull("en"), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
