// Testdaten nach docs/api/openapi.yaml; nur für Unit-Tests.
import { HttpTestingController, TestRequest } from '@angular/common/http/testing';
import {
  CategoryDto,
  PageDto,
  PaginationDto,
  ProductDetailDto,
  ProductDto,
  VariantOfferDto,
} from '../app/features/catalog/data-access/catalog.dto';
import { ProductSummary } from '../app/features/catalog/models/catalog.models';

export function productDto(overrides: Partial<ProductDto> = {}): ProductDto {
  return {
    id: '7d8c2c1e-0b7a-4c55-9a31-3b2f1c4d5e60',
    slug: 'gusseisenpfanne-28',
    name: 'Gusseisenpfanne 28 cm',
    description: 'Für Herd und Backofen.',
    price_cents: 5_990,
    currency: 'EUR',
    available: true,
    variant_count: 2,
    image_url: '',
    ...overrides,
  };
}

export function categoryDto(overrides: Partial<CategoryDto> = {}): CategoryDto {
  return {
    id: '0f1e2d3c-4b5a-4968-8776-655443322110',
    parent_id: null,
    slug: 'kueche-haushalt',
    name: 'Küche & Haushalt',
    description: 'Alles für die Küche.',
    sort_order: 1,
    ...overrides,
  };
}

export function variantDto(overrides: Partial<VariantOfferDto> = {}): VariantOfferDto {
  return {
    id: 'v-28',
    sku: 'NW-PFANNE-28',
    label: '28 cm',
    price_cents: 5_990,
    currency: 'EUR',
    stock_status: 'in_stock',
    ...overrides,
  };
}

/** Pfanne mit drei Varianten: ausverkauft, knapp und auf Lager. */
export function productDetailDto(overrides: Partial<ProductDetailDto> = {}): ProductDetailDto {
  return {
    id: '7d8c2c1e-0b7a-4c55-9a31-3b2f1c4d5e60',
    slug: 'gusseisenpfanne',
    name: 'Gusseisenpfanne',
    description: 'Für Herd und Backofen.',
    sale_status: 'available',
    tax_class: 'standard',
    tax_rate_bp: 1900,
    categories: [{ id: 'c-pfannen', slug: 'pfannen', name: 'Pfannen' }],
    variants: [
      variantDto({
        id: 'v-24',
        sku: 'NW-PFANNE-24',
        label: '24 cm',
        price_cents: 4_990,
        stock_status: 'out_of_stock',
      }),
      variantDto({
        id: 'v-28',
        sku: 'NW-PFANNE-28',
        label: '28 cm',
        price_cents: 5_990,
        stock_status: 'low_stock',
      }),
      variantDto({
        id: 'v-32',
        sku: 'NW-PFANNE-32',
        label: '32 cm',
        price_cents: 7_490,
        stock_status: 'in_stock',
      }),
    ],
    images: [],
    ...overrides,
  };
}

export function pageDto<T>(
  data: readonly T[],
  pagination: Partial<PaginationDto> = {},
): PageDto<T> {
  const total = pagination.total ?? data.length;
  const limit = pagination.limit ?? 24;
  return {
    data,
    pagination: {
      page: 1,
      limit,
      total,
      total_pages: total === 0 ? 0 : Math.ceil(total / limit),
      ...pagination,
    },
  };
}

export function productSummary(overrides: Partial<ProductSummary> = {}): ProductSummary {
  return {
    id: 'a1',
    slug: 'gusseisenpfanne-28',
    name: 'Gusseisenpfanne 28 cm',
    description: 'Für Herd und Backofen.',
    lowestPrice: { amountCents: 5_990, currency: 'EUR' },
    available: true,
    variantCount: 2,
    imageUrl: '',
    ...overrides,
  };
}

/** Beantwortet alle offenen Kategorie-Anfragen der Navigation. */
export function flushCategories(
  httpMock: HttpTestingController,
  categories: readonly CategoryDto[] = [],
): void {
  for (const request of httpMock.match((req) => req.url.endsWith('/categories'))) {
    request.flush(pageDto(categories, { limit: 100 }));
  }
}

export function expectProductRequest(httpMock: HttpTestingController): TestRequest {
  return httpMock.expectOne((req) => req.url.endsWith('/products'));
}

/** Normalisierter sichtbarer Text, inklusive geschützter Leerzeichen. */
export function textOf(element: Element | null | undefined): string {
  return (element?.textContent ?? '').replace(/\s+/g, ' ').trim();
}
