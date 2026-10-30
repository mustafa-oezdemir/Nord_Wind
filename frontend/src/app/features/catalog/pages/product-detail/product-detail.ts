import { Location } from '@angular/common';
import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  input,
  linkedSignal,
  signal,
} from '@angular/core';
import { rxResource } from '@angular/core/rxjs-interop';
import { Meta, Title } from '@angular/platform-browser';
import { HttpErrorResponse } from '@angular/common/http';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { apiErrorMessage, toApiError } from '../../../../core/http/api-error';
import { firstValueFrom } from 'rxjs';
import { CartApi } from '../../../cart/data-access/cart-api';
import { formatPageTitle } from '../../../../core/routing/page-title-strategy';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { ErrorState } from '../../../../shared/ui/error-state/error-state';
import { NotFoundView } from '../../../../shared/ui/not-found-view/not-found-view';
import { CatalogApi } from '../../data-access/catalog-api';
import { SALE_NOTICES, STOCK_LABELS } from '../../models/catalog-labels';
import { CategoryRef, ProductDetail } from '../../models/catalog.models';
import { CategoryStore } from '../../state/category-store';
import { ProductGallery } from '../../ui/product-gallery/product-gallery';
import { VariantPicker } from '../../ui/variant-picker/variant-picker';

const DEFAULT_DESCRIPTION = 'NordWinds – Onlineshop für Deutschland.';
const MAX_META_DESCRIPTION = 160;

/** Produktdetail unter `/produkte/:slug`; `?variante=SKU` wählt eine Variante vor. */
@Component({
  selector: 'app-product-detail',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, MoneyPipe, ErrorState, NotFoundView, ProductGallery, VariantPicker],
  templateUrl: './product-detail.html',
  styleUrl: './product-detail.scss',
})
export class ProductDetailPage {
  private readonly api = inject(CatalogApi);
  private readonly categoryStore = inject(CategoryStore);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);
  private readonly location = inject(Location);
  private readonly title = inject(Title);
  private readonly meta = inject(Meta);
  private readonly cartApi = inject(CartApi);

  protected readonly cartBusy = signal(false);
  protected readonly cartError = signal('');

  // Routen- und Query-Parameter über withComponentInputBinding().
  readonly slug = input<string>();
  readonly variante = input<string>();

  protected readonly stockLabels = STOCK_LABELS;

  protected readonly product = rxResource({
    params: () => this.slug()?.trim() || undefined,
    stream: ({ params }) => this.api.getProduct(params),
  });

  protected readonly detail = computed(() =>
    this.product.hasValue() ? this.product.value() : null,
  );
  protected readonly apiError = computed(() => {
    const error = this.product.error();
    return error ? toApiError(error) : null;
  });
  protected readonly notFound = computed(
    () => !this.slug()?.trim() || this.apiError()?.kind === 'not-found',
  );
  protected readonly errorMessage = computed(() => {
    const error = this.apiError();
    return error ? apiErrorMessage(error) : '';
  });

  /** Vorauswahl aus der URL, sonst die erste lieferbare Variante; danach frei wählbar. */
  protected readonly selectedSku = linkedSignal({
    source: () => ({ detail: this.detail(), requested: this.variante() }),
    computation: ({ detail, requested }) => defaultVariantSku(detail, requested),
  });
  protected readonly selectedVariant = computed(
    () => this.detail()?.variants.find((variant) => variant.sku === this.selectedSku()) ?? null,
  );

  protected readonly saleNotice = computed(() => {
    const status = this.detail()?.saleStatus;
    return status && status !== 'available' ? SALE_NOTICES[status] : null;
  });

  /** Brotkrumen über den Pfad der ersten aktiven Kategorie. */
  protected readonly breadcrumb = computed<readonly CategoryRef[]>(() => {
    const primary = this.detail()?.categories[0];
    if (!primary) {
      return [];
    }
    const path = this.categoryStore.path(primary.slug);
    return path.length > 0 ? path : [primary];
  });

  protected readonly alternative = computed(() => {
    const category = this.detail()?.categories[0];
    return category
      ? { link: ['/kategorien', category.slug], label: `Weitere Produkte in ${category.name}` }
      : { link: ['/produkte'], label: 'Alle Produkte ansehen' };
  });

  constructor() {
    effect(() => {
      const detail = this.detail();
      if (this.notFound()) {
        this.title.setTitle(formatPageTitle('Produkt nicht gefunden'));
      } else if (detail) {
        this.title.setTitle(formatPageTitle(detail.name));
        this.meta.updateTag({ name: 'description', content: metaDescription(detail) });
      }
    });
    inject(DestroyRef).onDestroy(() =>
      this.meta.updateTag({ name: 'description', content: DEFAULT_DESCRIPTION }),
    );
  }

  /** Auswahl in der URL festhalten, ohne neu zu navigieren oder nach oben zu springen. */
  protected selectVariant(sku: string | undefined): void {
    if (!sku) {
      return;
    }
    this.selectedSku.set(sku);
    const tree = this.router.createUrlTree([], {
      relativeTo: this.route,
      queryParams: { variante: sku },
      queryParamsHandling: 'merge',
    });
    this.location.replaceState(this.router.serializeUrl(tree));
  }

  protected async addToCart(): Promise<void> {
    const variant = this.selectedVariant();
    if (!variant || variant.stockStatus === 'out_of_stock' || this.cartBusy()) return;
    this.cartBusy.set(true);
    this.cartError.set('');
    try {
      await firstValueFrom(this.cartApi.add(variant.id, 1));
      await this.router.navigate(['/warenkorb']);
    } catch (error) {
      if (error instanceof HttpErrorResponse && error.status === 401) {
        await this.router.navigate(['/warenkorb']);
      } else if (error instanceof HttpErrorResponse && error.status === 409) {
        this.cartError.set('Bestand oder Produktstatus hat sich geändert. Bitte erneut prüfen.');
        this.product.reload();
      } else {
        this.cartError.set('Der Artikel konnte nicht hinzugefügt werden.');
      }
    } finally {
      this.cartBusy.set(false);
    }
  }
}

export function defaultVariantSku(
  detail: ProductDetail | null,
  requested: string | undefined,
): string | undefined {
  const variants = detail?.variants ?? [];
  const wanted = requested?.trim().toLowerCase();
  const match = wanted
    ? variants.find((variant) => variant.sku.toLowerCase() === wanted)
    : undefined;
  return (
    match ??
    variants.find((variant) => variant.stockStatus !== 'out_of_stock') ??
    variants[0]
  )?.sku;
}

function metaDescription(detail: ProductDetail): string {
  const text = detail.description.trim().replace(/\s+/g, ' ');
  if (!text) {
    return `${detail.name} bei NordWinds.`;
  }
  return text.length > MAX_META_DESCRIPTION
    ? `${text.slice(0, MAX_META_DESCRIPTION - 1).trimEnd()}…`
    : text;
}
