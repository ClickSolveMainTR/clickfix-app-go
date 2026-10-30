import type { KnowledgeCollection } from "../services/knowledge.service";

export type CollectionNode = KnowledgeCollection & {
  children: KnowledgeCollection[];
};

/**
 * Collections (top level) with their categories, both sorted by name. A
 * category whose parent is missing is listed at the top level so it stays
 * reachable.
 */
export function collectionTree(
  rows: KnowledgeCollection[],
  locale: string,
): CollectionNode[] {
  const byName = (a: KnowledgeCollection, b: KnowledgeCollection) =>
    a.name.localeCompare(b.name, locale);
  const ids = new Set(rows.map((r) => r.id));
  const roots = rows
    .filter((r) => r.parent_id == null || !ids.has(r.parent_id))
    .sort(byName);
  return roots.map((root) => ({
    ...root,
    children: rows.filter((r) => r.parent_id === root.id).sort(byName),
  }));
}

/** Flat option list for selects: collections followed by their categories. */
export function collectionOptions(tree: CollectionNode[]) {
  return tree.flatMap((node) => [
    { collection: node as KnowledgeCollection, depth: 0 },
    ...node.children.map((child) => ({ collection: child, depth: 1 })),
  ]);
}
