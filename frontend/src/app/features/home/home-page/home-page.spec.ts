import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import {
  categoryDto,
  expectProductRequest,
  flushCategories,
  pageDto,
  productDto,
  textOf,
} from '../../../../testing/catalog-fixtures';
import { HomePage, NEW_ARRIVALS_COUNT } from './home-page';

describe('HomePage', () => {
  let httpMock: HttpTestingController;
  let fixture: ComponentFixture<HomePage>;
  let element: HTMLElement;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    httpMock = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(HomePage);
    element = fixture.nativeElement as HTMLElement;
    TestBed.tick();
  });

  afterEach(() => httpMock.verify());

  it('lädt die neuesten Produkte und zeigt sie nach dem Laden an', async () => {
    const request = expectProductRequest(httpMock);
    expect(request.request.params.get('sort')).toBe('newest');
    expect(request.request.params.get('limit')).toBe(String(NEW_ARRIVALS_COUNT));
    expect(element.querySelector('[aria-busy="true"]')).not.toBeNull();

    request.flush(pageDto([productDto()]));
    flushCategories(httpMock, [categoryDto()]);
    await fixture.whenStable();

    expect(textOf(element.querySelector('h1'))).toBe('Willkommen bei NordWinds');
    expect(element.querySelectorAll('app-product-card').length).toBe(1);
    expect(element.querySelector('.category-tile')?.getAttribute('href')).toBe(
      '/kategorien/kueche-haushalt',
    );
  });

  it('zeigt einen leeren Zustand ohne Produkte', async () => {
    expectProductRequest(httpMock).flush(pageDto([]));
    flushCategories(httpMock);
    await fixture.whenStable();

    expect(textOf(element.querySelector('app-empty-state h2'))).toBe(
      'Noch keine Produkte im Sortiment',
    );
    expect(element.querySelector('#kategorien-titel')).toBeNull();
  });

  it('zeigt bei Netzwerkfehler eine verständliche Meldung', async () => {
    expectProductRequest(httpMock).error(new ProgressEvent('error'), { status: 0 });
    flushCategories(httpMock);
    await fixture.whenStable();

    const alert = element.querySelector('[role="alert"]');
    expect(textOf(alert)).toContain('Neue Produkte konnten nicht geladen werden');
    expect(textOf(alert)).toContain('Internetverbindung');
  });
});
