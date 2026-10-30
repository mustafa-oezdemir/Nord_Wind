import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';
import {
  categoryDto,
  pageDto,
  productDetailDto,
  productDto,
  variantDto,
} from '../../../../testing/catalog-fixtures';
import { InvalidApiResponseError } from '../../../core/http/api-error';
import { CatalogApi } from './catalog-api';

describe('CatalogApi', () => {
  let api: CatalogApi;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    api = TestBed.inject(CatalogApi);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('sendet gesetzte Filter und bildet die Produktseite auf das UI-Modell ab', async () => {
    const result = firstValueFrom(
      api.listProducts({
        category: 'kueche-haushalt',
        search: 'pfanne',
        sort: 'price_asc',
        page: 2,
        limit: 24,
      }),
    );
    const request = httpMock.expectOne((req) => req.url === '/api/v1/products');
    expect(request.request.method).toBe('GET');
    expect(request.request.params.toString()).toBe(
      'sort=price_asc&limit=24&category=kueche-haushalt&q=pfanne&page=2',
    );
    request.flush(
      pageDto([productDto({ price_cents: 129_999, variant_count: 3 })], {
        page: 2,
        total: 25,
        total_pages: 2,
      }),
    );

    const page = await result;
    expect(page.items).toEqual([
      {
        id: '7d8c2c1e-0b7a-4c55-9a31-3b2f1c4d5e60',
        slug: 'gusseisenpfanne-28',
        name: 'Gusseisenpfanne 28 cm',
        description: 'Für Herd und Backofen.',
        lowestPrice: { amountCents: 129_999, currency: 'EUR' },
        available: true,
        variantCount: 3,
      },
    ]);
    expect(page.pagination).toEqual({ page: 2, limit: 24, total: 25, totalPages: 2 });
  });

  it('lässt Seite 1 und leere Filter in der Anfrage weg', () => {
    api.listProducts({ sort: 'newest', page: 1, limit: 8 }).subscribe();
    const request = httpMock.expectOne((req) => req.url === '/api/v1/products');
    expect(request.request.params.toString()).toBe('sort=newest&limit=8');
    request.flush(pageDto([]));
  });

  it('weist Preise mit Nachkommastellen, fremde Währungen und fehlende Felder ab', async () => {
    const invalidBodies: (object | null)[] = [
      pageDto([productDto({ price_cents: 12.5 })]),
      pageDto([{ ...productDto(), currency: 'USD' }]),
      pageDto([{ ...productDto(), name: '' }]),
      { data: 'keine Liste', pagination: {} },
      null,
    ];
    for (const body of invalidBodies) {
      const result = firstValueFrom(api.listProducts({ sort: 'newest', page: 1, limit: 8 })).catch(
        (error: unknown) => error,
      );
      httpMock.expectOne((req) => req.url === '/api/v1/products').flush(body);
      expect(await result).toBeInstanceOf(InvalidApiResponseError);
    }
  });

  it('lädt alle Kategorien in Pflegereihenfolge mit dem API-Höchstlimit', async () => {
    const result = firstValueFrom(api.listCategories());
    const request = httpMock.expectOne((req) => req.url === '/api/v1/categories');
    expect(request.request.params.toString()).toBe('sort=sort_order&limit=100');
    request.flush(
      pageDto(
        [
          categoryDto(),
          categoryDto({
            id: 'c2',
            parent_id: '0f1e2d3c-4b5a-4968-8776-655443322110',
            slug: 'kochgeschirr',
            name: 'Kochgeschirr',
            sort_order: 0,
          }),
        ],
        { limit: 100 },
      ),
    );

    const categories = await result;
    expect(categories.map((category) => category.slug)).toEqual([
      'kueche-haushalt',
      'kochgeschirr',
    ]);
    expect(categories[1].parentId).toBe('0f1e2d3c-4b5a-4968-8776-655443322110');
  });
});

describe('CatalogApi.getProduct', () => {
  let api: CatalogApi;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    api = TestBed.inject(CatalogApi);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('lädt das Produkt über den kodierten Slug und bildet Varianten mit Cent-Preisen ab', async () => {
    const result = firstValueFrom(api.getProduct('pfanne/28 cm'));
    const request = httpMock.expectOne('/api/v1/products/pfanne%2F28%20cm');
    expect(request.request.params.keys()).toEqual([]);
    request.flush(productDetailDto());

    const product = await result;
    expect(product.saleStatus).toBe('available');
    expect(product.categories).toEqual([{ id: 'c-pfannen', slug: 'pfannen', name: 'Pfannen' }]);
    expect(
      product.variants.map((variant) => [
        variant.sku,
        variant.price.amountCents,
        variant.stockStatus,
      ]),
    ).toEqual([
      ['NW-PFANNE-24', 4_990, 'out_of_stock'],
      ['NW-PFANNE-28', 5_990, 'low_stock'],
      ['NW-PFANNE-32', 7_490, 'in_stock'],
    ]);
    expect(product.images).toEqual([]);
  });

  it('weist unbekannte Status, fremde Währungen und fehlende Listen ab', async () => {
    const invalidBodies: object[] = [
      productDetailDto({ sale_status: 'bald' as never }),
      productDetailDto({ variants: [variantDto({ stock_status: 'viele' as never })] }),
      productDetailDto({ variants: [{ ...variantDto(), currency: 'USD' as never }] }),
      productDetailDto({ variants: [variantDto({ price_cents: -1 })] }),
      { ...productDetailDto(), categories: null },
    ];
    for (const body of invalidBodies) {
      const result = firstValueFrom(api.getProduct('gusseisenpfanne')).catch(
        (error: unknown) => error,
      );
      httpMock.expectOne('/api/v1/products/gusseisenpfanne').flush(body);
      expect(await result).toBeInstanceOf(InvalidApiResponseError);
    }
  });
});
