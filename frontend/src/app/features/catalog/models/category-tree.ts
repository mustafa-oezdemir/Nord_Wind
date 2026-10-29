import { Category, CategoryNode } from './catalog.models';

/** Baut aus der flachen API-Liste einen Baum; verwaiste Einträge werden zu Wurzeln. */
export function buildCategoryTree(categories: readonly Category[]): readonly CategoryNode[] {
  const ids = new Set(categories.map((category) => category.id));
  const childrenByParent = new Map<string | null, Category[]>();
  for (const category of categories) {
    const parentKey = category.parentId && ids.has(category.parentId) ? category.parentId : null;
    const siblings = childrenByParent.get(parentKey) ?? [];
    siblings.push(category);
    childrenByParent.set(parentKey, siblings);
  }

  const build = (parentId: string | null, ancestors: ReadonlySet<string>): CategoryNode[] =>
    (childrenByParent.get(parentId) ?? [])
      .filter((category) => !ancestors.has(category.id))
      .sort(compareCategories)
      .map((category) => ({
        ...category,
        children: build(category.id, new Set([...ancestors, category.id])),
      }));

  return build(null, new Set());
}

function compareCategories(a: Category, b: Category): number {
  return a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, 'de');
}
