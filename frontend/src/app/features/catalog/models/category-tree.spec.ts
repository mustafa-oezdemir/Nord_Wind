import { Category } from './catalog.models';
import { buildCategoryTree } from './category-tree';

const category = (
  id: string,
  name: string,
  sortOrder: number,
  parentId: string | null = null,
): Category => ({
  id,
  parentId,
  slug: id,
  name,
  description: '',
  sortOrder,
});

describe('buildCategoryTree', () => {
  it('verschachtelt Unterkategorien und sortiert nach Pflegereihenfolge und Name', () => {
    const tree = buildCategoryTree([
      category('wohnen', 'Wohnen', 2),
      category('kaffee', 'Kaffee & Tee', 1, 'kueche'),
      category('kueche', 'Küche', 1),
      category('pfannen', 'Pfannen', 1, 'kueche'),
      category('bad', 'Bad', 2),
    ]);

    expect(tree.map((node) => node.id)).toEqual(['kueche', 'bad', 'wohnen']);
    expect(tree[0].children.map((node) => node.id)).toEqual(['kaffee', 'pfannen']);
    expect(tree[1].children).toEqual([]);
  });

  it('zeigt Kategorien mit unbekannter Elternkategorie als Hauptkategorie', () => {
    const tree = buildCategoryTree([category('jacken', 'Jacken', 1, 'inaktiv')]);
    expect(tree.map((node) => node.id)).toEqual(['jacken']);
  });
});
