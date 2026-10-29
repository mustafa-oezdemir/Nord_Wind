import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { apiErrorMessage, toApiError } from '../../../core/http/api-error';
import { EmptyState } from '../../../shared/ui/empty-state/empty-state';
import { ErrorState } from '../../../shared/ui/error-state/error-state';
import { CatalogApi } from '../../catalog/data-access/catalog-api';
import { CategoryStore } from '../../catalog/state/category-store';
import { ProductGrid } from '../../catalog/ui/product-grid/product-grid';

export const NEW_ARRIVALS_COUNT = 8;
const FEATURED_CATEGORY_COUNT = 6;

@Component({
  selector: 'app-home-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, ProductGrid, EmptyState, ErrorState],
  templateUrl: './home-page.html',
  styleUrl: './home-page.scss',
})
export class HomePage {
  private readonly api = inject(CatalogApi);
  private readonly categoryStore = inject(CategoryStore);

  protected readonly newArrivals = rxResource({
    stream: () => this.api.listProducts({ sort: 'newest', page: 1, limit: NEW_ARRIVALS_COUNT }),
  });

  protected readonly products = computed(() =>
    this.newArrivals.hasValue() ? this.newArrivals.value().items : [],
  );
  protected readonly errorMessage = computed(() => {
    const error = this.newArrivals.error();
    return error ? apiErrorMessage(toApiError(error)) : null;
  });
  protected readonly featuredCategories = computed(() =>
    this.categoryStore.tree().slice(0, FEATURED_CATEGORY_COUNT),
  );
}
