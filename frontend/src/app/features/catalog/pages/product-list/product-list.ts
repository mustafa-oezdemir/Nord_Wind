import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  input,
  viewChild,
} from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { Title } from '@angular/platform-browser';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { apiErrorMessage, toApiError } from '../../../../core/http/api-error';
import { formatPageTitle } from '../../../../core/routing/page-title-strategy';
import { EmptyState } from '../../../../shared/ui/empty-state/empty-state';
import { ErrorState } from '../../../../shared/ui/error-state/error-state';
import { NotFoundView } from '../../../../shared/ui/not-found-view/not-found-view';
import { Pagination } from '../../../../shared/ui/pagination/pagination';
import { CatalogApi } from '../../data-access/catalog-api';
import { ProductQuery } from '../../models/catalog.models';
import {
  DEFAULT_SORT_PARAM,
  SORT_OPTIONS,
  normalizeSearch,
  pageFromParam,
  sortFromParam,
  sortToParam,
} from '../../models/catalog-query';
import { CategoryStore } from '../../state/category-store';
import { ProductGrid } from '../../ui/product-grid/product-grid';

export const PRODUCT_PAGE_SIZE = 24;

const countFormat = new Intl.NumberFormat('de-DE');

/** Produktliste für `/produkte` und `/kategorien/:slug` mit Suche, Sortierung und Seiten. */
@Component({
  selector: 'app-product-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, ProductGrid, EmptyState, ErrorState, NotFoundView, Pagination],
  templateUrl: './product-list.html',
  styleUrl: './product-list.scss',
})
export class ProductList {
  private readonly api = inject(CatalogApi);
  private readonly categoryStore = inject(CategoryStore);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly title = inject(Title);
  private readonly heading = viewChild<ElementRef<HTMLElement>>('heading');

  // Routen- und Query-Parameter über withComponentInputBinding().
  readonly slug = input<string>();
  readonly q = input<string>();
  readonly sortierung = input<string>();
  readonly seite = input<string>();

  protected readonly sortOptions = SORT_OPTIONS;

  protected readonly query = computed<ProductQuery>(() => ({
    category: this.slug()?.trim() || undefined,
    search: normalizeSearch(this.q()),
    sort: sortFromParam(this.sortierung()),
    page: pageFromParam(this.seite()),
    limit: PRODUCT_PAGE_SIZE,
  }));

  protected readonly products = rxResource({
    params: () => this.query(),
    stream: ({ params }) => this.api.listProducts(params),
  });

  protected readonly page = computed(() =>
    this.products.hasValue() ? this.products.value() : null,
  );
  protected readonly apiError = computed(() => {
    const error = this.products.error();
    return error ? toApiError(error) : null;
  });
  protected readonly categoryMissing = computed(
    () => this.apiError()?.kind === 'not-found' && !!this.query().category,
  );
  protected readonly errorMessage = computed(() => {
    const error = this.apiError();
    return error ? apiErrorMessage(error) : '';
  });

  protected readonly category = computed(() => {
    const slug = this.query().category;
    return slug ? this.categoryStore.bySlug(slug) : undefined;
  });

  protected readonly headingText = computed(() => {
    const search = this.query().search;
    if (search) {
      return `Suchergebnisse für „${search}“`;
    }
    if (this.query().category) {
      return this.category()?.name ?? 'Kategorie';
    }
    return 'Alle Produkte';
  });

  protected readonly resultText = computed(() => {
    const total = this.page()?.pagination.total ?? 0;
    return `${countFormat.format(total)} ${total === 1 ? 'Produkt' : 'Produkte'}`;
  });

  protected readonly selectedSort = computed(() => sortToParam(this.query().sort));
  protected readonly pageBeyondEnd = computed(() => {
    const page = this.page();
    return !!page && page.items.length === 0 && page.pagination.total > 0;
  });

  constructor() {
    effect(() => {
      const pageTitle = this.categoryMissing() ? 'Kategorie nicht gefunden' : this.headingText();
      this.title.setTitle(formatPageTitle(pageTitle));
    });

    // Beim Blättern bleibt der Fokus sonst am Seitenende; er springt zur Überschrift.
    let previousPage: number | null = null;
    effect(() => {
      const page = this.query().page;
      if (previousPage !== null && previousPage !== page) {
        this.heading()?.nativeElement.focus({ preventScroll: true });
      }
      previousPage = page;
    });
  }

  protected changeSort(event: Event): void {
    const value = (event.target as HTMLSelectElement).value;
    void this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { sortierung: value === DEFAULT_SORT_PARAM ? null : value, seite: null },
      queryParamsHandling: 'merge',
    });
  }
}
