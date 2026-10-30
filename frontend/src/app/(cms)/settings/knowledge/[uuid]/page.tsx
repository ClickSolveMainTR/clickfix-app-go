"use client";

import { useParams } from "next/navigation";

import { ArticleEditorPage } from "@/features/knowledge/components/article-editor-page";

export default function KnowledgeArticleRoute() {
  const params = useParams<{ uuid: string }>();
  return <ArticleEditorPage uuid={params.uuid} />;
}
