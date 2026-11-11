import { llmsFull } from "@/lib/docs/llms";

export const dynamic = "force-static";

/** `/llms-full.txt`: every Turkish docs page as Markdown (CF-208); English is `/en/llms-full.txt`. */
export async function GET() {
  return new Response(await llmsFull("tr"), {
    headers: { "content-type": "text/markdown; charset=utf-8" },
  });
}
