import type { Category, CategoryNode } from "@/features/categories/types";

/** Builds the tree from the flat, parent-first API list. */
export function buildTree(items: Category[]): CategoryNode[] {
  const byId = new Map<string, CategoryNode>();
  for (const c of items) byId.set(c.uuid, { ...c, children: [] });
  const roots: CategoryNode[] = [];
  for (const node of byId.values()) {
    const parent = node.parent_uuid ? byId.get(node.parent_uuid) : undefined;
    if (parent) parent.children.push(node);
    else roots.push(node);
  }
  return roots;
}

/** Depth-first order with indentation, for lists and selects. */
export function flatten(nodes: CategoryNode[]): CategoryNode[] {
  const out: CategoryNode[] = [];
  const walk = (list: CategoryNode[]) => {
    for (const n of list) {
      out.push(n);
      walk(n.children);
    }
  };
  walk(nodes);
  return out;
}

/** Uuids of a node and everything below it (invalid parents on move). */
export function subtree(node: CategoryNode): Set<string> {
  const ids = new Set<string>();
  const walk = (n: CategoryNode) => {
    ids.add(n.uuid);
    n.children.forEach(walk);
  };
  walk(node);
  return ids;
}

/** One example per line; blank lines dropped. */
export function parseExamples(text: string): string[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}
