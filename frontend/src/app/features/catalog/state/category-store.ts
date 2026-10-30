import { Injectable, computed, inject } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { CatalogApi } from '../data-access/catalog-api';
import { Category, CategoryNode } from '../models/catalog.models';
import { buildCategoryTree } from '../models/category-tree';

/** Kategorien für Navigation und Überschriften; einmal pro Sitzung geladen. */
@Injectable({ providedIn: 'root' })
export class CategoryStore {
  private readonly api = inject(CatalogApi);

  private readonly resource = rxResource({ stream: () => this.api.listCategories() });

  readonly isLoading = this.resource.isLoading;
  readonly categories = computed<readonly Category[]>(() =>
    this.resource.hasValue() ? this.resource.value() : [],
  );
  readonly tree = computed<readonly CategoryNode[]>(() => buildCategoryTree(this.categories()));

  bySlug(slug: string): Category | undefined {
    const wanted = slug.toLowerCase();
    return this.categories().find((category) => category.slug.toLowerCase() === wanted);
  }

  /** Pfad von der Hauptkategorie bis zur angegebenen Kategorie, z. B. für Brotkrumen. */
  path(slug: string): readonly Category[] {
    const byId = new Map(this.categories().map((category) => [category.id, category]));
    const path: Category[] = [];
    const visited = new Set<string>();
    let current = this.bySlug(slug);
    while (current && !visited.has(current.id)) {
      visited.add(current.id);
      path.unshift(current);
      current = current.parentId ? byId.get(current.parentId) : undefined;
    }
    return path;
  }

  reload(): void {
    this.resource.reload();
  }
}
