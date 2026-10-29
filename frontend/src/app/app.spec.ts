import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { TitleStrategy, provideRouter, withComponentInputBinding } from '@angular/router';
import { RouterTestingHarness } from '@angular/router/testing';
import {
  categoryDto,
  expectProductRequest,
  flushCategories,
  pageDto,
  productDto,
  textOf,
} from '../testing/catalog-fixtures';
import { routes } from './app.routes';
import { PageTitleStrategy } from './core/routing/page-title-strategy';

describe('App-Routing', () => {
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter(routes, withComponentInputBinding()),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: TitleStrategy, useClass: PageTitleStrategy },
      ],
    });
    httpMock = TestBed.inject(HttpTestingController);
  });

  it('zeigt die Startseite im Shop-Layout mit Sprunglink, Navigation und Fußzeile', async () => {
    const harness = await RouterTestingHarness.create('/');
    expectProductRequest(httpMock).flush(pageDto([productDto()]));
    flushCategories(httpMock, [categoryDto()]);
    await harness.fixture.whenStable();
    const element = harness.fixture.nativeElement as HTMLElement;

    expect(textOf(element.querySelector('h1'))).toBe('Willkommen bei NordWinds');
    expect(textOf(element.querySelector('.skip-link'))).toBe('Zum Inhalt springen');
    expect(
      element.querySelector('nav[aria-label="Kategorien"] a[href="/kategorien/kueche-haushalt"]'),
    ).not.toBeNull();
    expect(element.querySelector('footer')).not.toBeNull();
    expect(TestBed.inject(Title).getTitle()).toBe('Onlineshop für Deutschland | NordWinds');
  });

  it('zeigt für unbekannte Adressen eine 404-Seite', async () => {
    const harness = await RouterTestingHarness.create('/gibt/es/nicht');
    flushCategories(httpMock);
    await harness.fixture.whenStable();

    const element = harness.fixture.nativeElement as HTMLElement;
    expect(textOf(element.querySelector('app-not-found-view h1'))).toBe('Seite nicht gefunden');
    expect(TestBed.inject(Title).getTitle()).toBe('Seite nicht gefunden | NordWinds');
  });
});
