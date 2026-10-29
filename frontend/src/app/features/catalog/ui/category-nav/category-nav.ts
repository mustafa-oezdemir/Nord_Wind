import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { IsActiveMatchOptions, RouterLink, RouterLinkActive } from '@angular/router';
import { CategoryStore } from '../../state/category-store';

/** Horizontale Kategorienavigation ab Tablet-Querformat; nutzt die Hauptkategorien. */
@Component({
  selector: 'app-category-nav',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, RouterLinkActive],
  templateUrl: './category-nav.html',
  styleUrl: './category-nav.scss',
})
export class CategoryNav {
  protected readonly store = inject(CategoryStore);

  /** „Alle Produkte“ bleibt bei Suche und Sortierung aktiv. */
  protected readonly allProductsMatch: IsActiveMatchOptions = {
    paths: 'exact',
    queryParams: 'ignored',
    matrixParams: 'ignored',
    fragment: 'ignored',
  };
}
