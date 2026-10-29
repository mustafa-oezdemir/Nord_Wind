import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { Router, provideRouter } from '@angular/router';
import {
  categoryDto,
  expectProductRequest,
  flushCategories,
  pageDto,
  productDto,
  textOf,
} from '../../../../../testing/catalog-fixtures';
import { PRODUCT_PAGE_SIZE, ProductList } from './product-list';

describe('ProductList', () => {
  let httpMock: HttpTestingController;
  let fixture: ComponentFixture<ProductList>;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  function render(inputs: Record<string, string> = {}): HTMLElement {
    fixture = TestBed.createComponent(ProductList);
    for (const [name, value] of Object.entries(inputs)) {
      fixture.componentRef.setInput(name, value);
    }
    TestBed.tick();
    return fixture.nativeElement as HTMLElement;
  }

  async function settle(): Promise<void> {
    flushCategories(httpMock, [categoryDto()]);
    await fixture.whenStable();
  }

  it('zeigt beim Laden Platzhalter und danach die Produkte der API', async () => {
    const element = render();
    const request = expectProductRequest(httpMock);
    expect(request.request.params.get('limit')).toBe(String(PRODUCT_PAGE_SIZE));
    expect(element.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(element.querySelectorAll('[data-testid="product-skeleton"]').length).toBeGreaterThan(0);

    request.flush(
      pageDto([productDto(), productDto({ id: 'p2', slug: 'wolldecke', name: 'Wolldecke' })], {
        total: 2,
      }),
    );
    await settle();

    expect(element.querySelector('[aria-busy="true"]')).toBeNull();
    expect(element.querySelectorAll('app-product-card').length).toBe(2);
    expect(textOf(element.querySelector('h1'))).toBe('Alle Produkte');
    expect(textOf(element.querySelector('.product-list-count'))).toBe('2 Produkte');
  });

  it('übergibt Kategorie, Suche, Sortierung und Seite aus der URL an die API', async () => {
    const element = render({
      slug: 'kueche-haushalt',
      q: ' pfanne ',
      sortierung: 'preis-ab',
      seite: '2',
    });
    const request = expectProductRequest(httpMock);
    expect(request.request.params.get('category')).toBe('kueche-haushalt');
    expect(request.request.params.get('q')).toBe('pfanne');
    expect(request.request.params.get('sort')).toBe('price_desc');
    expect(request.request.params.get('page')).toBe('2');

    request.flush(pageDto([productDto()], { page: 2, total: 25, limit: 24 }));
    await settle();

    expect(textOf(element.querySelector('h1'))).toBe('Suchergebnisse für „pfanne“');
    expect(textOf(element.querySelector('.pagination-status'))).toBe('Seite 2 von 2');
    expect((element.querySelector('#sortierung') as HTMLSelectElement).value).toBe('preis-ab');
  });

  it('zeigt den Kategorienamen als Überschrift und Seitentitel', async () => {
    const element = render({ slug: 'kueche-haushalt' });
    expectProductRequest(httpMock).flush(pageDto([productDto()]));
    await settle();

    expect(textOf(element.querySelector('h1'))).toBe('Küche & Haushalt');
    expect(TestBed.inject(Title).getTitle()).toBe('Küche & Haushalt | NordWinds');
    expect(element.querySelector('[aria-current="page"]')?.textContent).toContain(
      'Küche & Haushalt',
    );
  });

  it('zeigt einen leeren Zustand mit Ausweg, wenn die Suche nichts findet', async () => {
    const element = render({ q: 'xyz' });
    expectProductRequest(httpMock).flush(pageDto([]));
    await settle();

    expect(textOf(element.querySelector('app-empty-state h2'))).toBe('Keine Produkte gefunden');
    expect(element.querySelector('app-empty-state a')?.getAttribute('href')).toBe('/produkte');
    expect(element.querySelector('app-product-card')).toBeNull();
  });

  it('zeigt bei Serverfehler eine Meldung und lädt nach „Erneut versuchen“ neu', async () => {
    const element = render();
    expectProductRequest(httpMock).flush(
      { code: 'INTERNAL_ERROR', detail: 'geheim' },
      { status: 500, statusText: 'Internal Server Error' },
    );
    await settle();

    const alert = element.querySelector('[role="alert"]');
    expect(textOf(alert)).toContain('Produkte konnten nicht geladen werden');
    expect(textOf(alert)).not.toContain('geheim');

    (alert?.querySelector('button') as HTMLButtonElement).click();
    TestBed.tick();
    expectProductRequest(httpMock).flush(pageDto([productDto()]));
    await fixture.whenStable();

    expect(element.querySelector('[role="alert"]')).toBeNull();
    expect(element.querySelectorAll('app-product-card').length).toBe(1);
  });

  it('zeigt für eine unbekannte Kategorie eine 404-Ansicht', async () => {
    const element = render({ slug: 'gibt-es-nicht' });
    expectProductRequest(httpMock).flush(
      { code: 'NOT_FOUND' },
      { status: 404, statusText: 'Not Found' },
    );
    await settle();

    expect(textOf(element.querySelector('h1'))).toBe('Kategorie nicht gefunden');
    expect(TestBed.inject(Title).getTitle()).toBe('Kategorie nicht gefunden | NordWinds');
  });

  it('setzt beim Ändern der Sortierung die Seite zurück', async () => {
    const router = TestBed.inject(Router);
    const element = render({ seite: '3' });
    expectProductRequest(httpMock).flush(pageDto([productDto()], { page: 3, total: 60 }));
    await settle();

    const select = element.querySelector('#sortierung') as HTMLSelectElement;
    select.value = 'preis-auf';
    select.dispatchEvent(new Event('change'));
    await fixture.whenStable();

    expect(router.url).toBe('/?sortierung=preis-auf');
  });

  it('bietet bei einer Seite hinter dem Ende einen Link zur ersten Seite an', async () => {
    const element = render({ seite: '9' });
    expectProductRequest(httpMock).flush(pageDto([], { page: 9, total: 5 }));
    await settle();

    expect(textOf(element.querySelector('app-empty-state h2'))).toBe('Diese Seite gibt es nicht');
  });
});
