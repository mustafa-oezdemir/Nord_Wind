import { Location } from '@angular/common';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Meta, Title } from '@angular/platform-browser';
import { Router, provideRouter } from '@angular/router';
import { vi } from 'vitest';
import {
  categoryDto,
  flushCategories,
  productDetailDto,
  textOf,
  variantDto,
} from '../../../../../testing/catalog-fixtures';
import { euros } from '../../../../shared/money/money';
import { ProductDetail } from '../../models/catalog.models';
import { ProductDetailPage, defaultVariantSku } from './product-detail';

@Component({ selector: 'app-detail-test-page', template: '' })
class EmptyPage {}

describe('ProductDetailPage', () => {
  let httpMock: HttpTestingController;
  let fixture: ComponentFixture<ProductDetailPage>;
  let element: HTMLElement;

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: '**', component: EmptyPage }]),
        provideHttpClient(),
        provideHttpClientTesting(),
      ],
    });
    httpMock = TestBed.inject(HttpTestingController);
    await TestBed.inject(Router).navigateByUrl('/produkte/gusseisenpfanne');
  });

  afterEach(() => httpMock.verify());

  function render(inputs: Record<string, string> = { slug: 'gusseisenpfanne' }): void {
    fixture = TestBed.createComponent(ProductDetailPage);
    for (const [name, value] of Object.entries(inputs)) {
      fixture.componentRef.setInput(name, value);
    }
    element = fixture.nativeElement as HTMLElement;
    TestBed.tick();
  }

  async function respond(body: object = productDetailDto()): Promise<void> {
    httpMock.expectOne('/api/v1/products/gusseisenpfanne').flush(body);
    flushCategories(httpMock, [
      categoryDto({ id: 'c-kueche', slug: 'kueche', name: 'Küche' }),
      categoryDto({ id: 'c-pfannen', parent_id: 'c-kueche', slug: 'pfannen', name: 'Pfannen' }),
    ]);
    await fixture.whenStable();
  }

  const price = () => textOf(element.querySelector('.detail-price'));
  const stock = () => textOf(element.querySelector('.detail-stock'));
  const sku = () => textOf(element.querySelector('.detail-sku-value'));

  it('zeigt beim Laden Platzhalter und danach Name, Preis und Bestand der ersten lieferbaren Variante', async () => {
    render();
    expect(element.querySelector('[aria-busy="true"]')).not.toBeNull();
    await respond();

    expect(element.querySelector('[aria-busy="true"]')).toBeNull();
    expect(textOf(element.querySelector('h1'))).toBe('Gusseisenpfanne');
    // 24 cm ist ausverkauft; vorgewählt wird die erste lieferbare Variante.
    expect(price()).toBe('59,90 €');
    expect(stock()).toBe('Nur noch wenige Stück');
    expect(sku()).toBe('NW-PFANNE-28');
    expect(textOf(element.querySelector('.detail-tax-note'))).toBe(
      'inkl. 19 % MwSt., zzgl. Versandkosten',
    );
    expect(TestBed.inject(Title).getTitle()).toBe('Gusseisenpfanne | NordWinds');
    expect(TestBed.inject(Meta).getTag('name="description"')?.content).toBe(
      'Für Herd und Backofen.',
    );
    expect(document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href).toBe(
      `${document.location.origin}/produkte/gusseisenpfanne`,
    );
    expect(TestBed.inject(Meta).getTag('property="og:title"')?.content).toBe('Gusseisenpfanne');
  });

  it('zeigt für jede gewählte Variante den Preis und Bestand der API und hält die Wahl in der URL fest', async () => {
    render();
    await respond();

    const expectations: [string, string, string][] = [
      ['NW-PFANNE-24', '49,90 €', 'Ausverkauft'],
      ['NW-PFANNE-32', '74,90 €', 'Auf Lager'],
      ['NW-PFANNE-28', '59,90 €', 'Nur noch wenige Stück'],
    ];
    for (const [variantSku, expectedPrice, expectedStock] of expectations) {
      (element.querySelector(`input[value="${variantSku}"]`) as HTMLInputElement).click();
      await fixture.whenStable();

      expect(price()).toBe(expectedPrice);
      expect(stock()).toBe(expectedStock);
      expect(sku()).toBe(variantSku);
      expect(TestBed.inject(Location).path()).toBe(
        `/produkte/gusseisenpfanne?variante=${variantSku}`,
      );
      expect(document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]')?.href).toBe(
        `${document.location.origin}/produkte/gusseisenpfanne`,
      );
    }
  });

  it('übernimmt die Variante aus der URL ohne Beachtung der Großschreibung', async () => {
    render({ slug: 'gusseisenpfanne', variante: 'nw-pfanne-32' });
    await respond();

    expect(sku()).toBe('NW-PFANNE-32');
    expect(price()).toBe('74,90 €');
    expect((element.querySelector('input[value="NW-PFANNE-32"]') as HTMLInputElement).checked).toBe(
      true,
    );
  });

  it('fügt die ausgewählte Variante mit Session-CSRF zum Warenkorb hinzu', async () => {
    render();
    await respond();
    const navigate = vi.spyOn(TestBed.inject(Router), 'navigate').mockResolvedValue(true);
    (element.querySelector('.buy-box button.btn-primary') as HTMLButtonElement).click();
    TestBed.tick();
    httpMock.expectOne('/api/v1/auth/session').flush({
      customer: { id: 'kunde', email: 'kunde@example.invalid', role: 'customer' },
      csrf_token: 'synthetic-csrf',
      expires_at: '2026-10-31T00:00:00Z',
    });
    const add = httpMock.expectOne('/api/v1/cart/items');
    expect(add.request.body).toEqual({ variant_id: 'v-28', quantity: 1 });
    expect(add.request.headers.get('X-CSRF-Token')).toBe('synthetic-csrf');
    add.flush({ items: [], subtotal_cents: 0, currency: 'EUR' });
    await Promise.resolve();
    await fixture.whenStable();
    expect(navigate).toHaveBeenCalledWith(['/warenkorb']);
  });

  it('baut die Brotkrumen aus dem Kategoriepfad auf', async () => {
    render();
    await respond();

    const crumbs = Array.from(element.querySelectorAll('.breadcrumb-item')).map((item) =>
      textOf(item),
    );
    expect(crumbs).toEqual(['Startseite', 'Küche', 'Pfannen', 'Gusseisenpfanne']);
    expect(element.querySelector('.breadcrumb-item.active')?.getAttribute('aria-current')).toBe(
      'page',
    );
  });

  it('kennzeichnet ausverkaufte Produkte und zeigt trotzdem Preise', async () => {
    render();
    await respond(
      productDetailDto({
        sale_status: 'sold_out',
        variants: [
          variantDto({
            sku: 'NW-TOPF',
            label: 'Standard',
            price_cents: 12_900,
            stock_status: 'out_of_stock',
          }),
        ],
      }),
    );

    expect(textOf(element.querySelector('.sale-notice'))).toBe(
      'Dieses Produkt ist derzeit ausverkauft.',
    );
    expect(price()).toBe('129,00 €');
    expect(stock()).toBe('Ausverkauft');
    expect(textOf(element.querySelector('.detail-single-variant'))).toBe('Variante: Standard');
  });

  it('zeigt bei nicht mehr angebotenen Produkten keinen Preis, aber einen Weg zur Kategorie', async () => {
    render();
    await respond(productDetailDto({ sale_status: 'discontinued', variants: [] }));

    expect(textOf(element.querySelector('.sale-notice'))).toBe(
      'Dieses Produkt wird nicht mehr angeboten.',
    );
    expect(element.querySelector('.detail-price')).toBeNull();
    expect(element.querySelector('app-variant-picker')).toBeNull();
    const link = element.querySelector('.buy-box a.btn');
    expect(textOf(link)).toBe('Weitere Produkte in Pfannen');
    expect(link?.getAttribute('href')).toBe('/kategorien/pfannen');
  });

  it('zeigt für einen unbekannten Slug eine 404-Ansicht', async () => {
    render();
    httpMock
      .expectOne('/api/v1/products/gusseisenpfanne')
      .flush({ code: 'NOT_FOUND' }, { status: 404, statusText: 'Not Found' });
    flushCategories(httpMock);
    await fixture.whenStable();

    expect(textOf(element.querySelector('h1'))).toBe('Produkt nicht gefunden');
    expect(TestBed.inject(Title).getTitle()).toBe('Produkt nicht gefunden | NordWinds');
    expect(TestBed.inject(Meta).getTag('name="robots"')?.content).toBe('noindex');
    expect(document.head.querySelector('link[rel="canonical"]')).toBeNull();
  });

  it('zeigt bei Serverfehler eine Meldung und lädt nach „Erneut versuchen“ neu', async () => {
    render();
    httpMock
      .expectOne('/api/v1/products/gusseisenpfanne')
      .flush({ code: 'INTERNAL_ERROR' }, { status: 500, statusText: 'Internal Server Error' });
    flushCategories(httpMock);
    await fixture.whenStable();
    expect(textOf(element.querySelector('[role="alert"] h2'))).toBe(
      'Produkt konnte nicht geladen werden',
    );

    (element.querySelector('[role="alert"] button') as HTMLButtonElement).click();
    TestBed.tick();
    httpMock.expectOne('/api/v1/products/gusseisenpfanne').flush(productDetailDto());
    await fixture.whenStable();

    expect(textOf(element.querySelector('h1'))).toBe('Gusseisenpfanne');
  });
});

describe('defaultVariantSku', () => {
  const detail = (stock: ('in_stock' | 'low_stock' | 'out_of_stock')[]): ProductDetail => ({
    id: 'p',
    slug: 'p',
    name: 'P',
    description: '',
    saleStatus: 'available',
    taxRateBp: 1900,
    categories: [],
    images: [],
    variants: stock.map((stockStatus, index) => ({
      id: `v${index}`,
      sku: `SKU-${index}`,
      label: `V${index}`,
      price: euros(100),
      stockStatus,
    })),
  });

  it('bevorzugt die angefragte, dann die erste lieferbare, sonst die erste Variante', () => {
    expect(defaultVariantSku(detail(['out_of_stock', 'in_stock']), 'sku-0')).toBe('SKU-0');
    expect(defaultVariantSku(detail(['out_of_stock', 'in_stock']), 'unbekannt')).toBe('SKU-1');
    expect(defaultVariantSku(detail(['out_of_stock', 'out_of_stock']), undefined)).toBe('SKU-0');
    expect(defaultVariantSku(detail([]), 'SKU-0')).toBeUndefined();
    expect(defaultVariantSku(null, undefined)).toBeUndefined();
  });
});
