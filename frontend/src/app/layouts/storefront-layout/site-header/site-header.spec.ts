import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { CartApi, CustomerSession } from '../../../features/cart/data-access/cart-api';
import { SiteHeader } from './site-header';

@Component({ template: '' })
class EmptyPage {}

const session: CustomerSession = {
  customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
  csrf_token: 'synthetic-csrf',
  expires_at: '2026-11-01T00:00:00Z',
};

describe('SiteHeader', () => {
  let fixture: ComponentFixture<SiteHeader>;
  let http: HttpTestingController;
  let element: HTMLElement;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: '', component: EmptyPage }]),
        provideHttpClient(),
        provideHttpClientTesting(),
      ],
    });
    http = TestBed.inject(HttpTestingController);
    fixture = TestBed.createComponent(SiteHeader);
    fixture.detectChanges();
    element = fixture.nativeElement as HTMLElement;
  });

  afterEach(() => http.verify());

  function openMenu(): HTMLButtonElement {
    const button = element.querySelector<HTMLButtonElement>('.site-header-account')!;
    button.click();
    fixture.detectChanges();
    return button;
  }

  it('zeigt den Warenkorb immer als benanntes Symbol und bietet Gästen Anmeldung und Registrierung', async () => {
    const cart = element.querySelector<HTMLAnchorElement>('.site-header-cart')!;
    expect(cart.getAttribute('href')).toBe('/warenkorb');
    expect(cart.getAttribute('aria-label')).toBe('Warenkorb öffnen');
    expect(cart.querySelector('svg path')).not.toBeNull();
    expect(cart.textContent?.trim()).toBe('');

    const button = openMenu();
    expect(button.getAttribute('aria-expanded')).toBe('true');
    http.expectOne('/api/v1/auth/session').flush({}, { status: 401, statusText: 'Unauthorized' });
    await fixture.whenStable();
    fixture.detectChanges();

    const links = Array.from(
      element.querySelectorAll<HTMLAnchorElement>('.site-header-dropdown-link'),
    );
    expect(links.map((link) => link.textContent?.trim())).toEqual(['Anmelden', 'Registrieren']);
    expect(links[0].getAttribute('href')).toBe('/warenkorb?anmeldung=login');
    expect(links[1].getAttribute('href')).toBe('/warenkorb?anmeldung=registrieren');
    expect(element.querySelector('.site-header-cart')).not.toBeNull();

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    fixture.detectChanges();
    expect(button.getAttribute('aria-expanded')).toBe('false');
    expect(document.activeElement).toBe(button);
  });

  it('zeigt nach Anmeldung das Konto und meldet mit CSRF sicher ab', async () => {
    const button = openMenu();
    http.expectOne('/api/v1/auth/session').flush(session);
    await fixture.whenStable();
    fixture.detectChanges();

    expect(element.textContent).toContain(session.customer.email);
    expect(
      element.querySelector<HTMLAnchorElement>('.site-header-dropdown-link')?.getAttribute('href'),
    ).toBe('/konto');
    const logout = element.querySelector<HTMLButtonElement>(
      '.site-header-dropdown-link[type="button"]',
    )!;
    logout.click();
    const request = http.expectOne('/api/v1/auth/logout');
    expect(request.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    request.flush(null);
    await fixture.whenStable();
    fixture.detectChanges();

    expect(TestBed.inject(CartApi).activeSession()).toBeNull();
    expect(button.getAttribute('aria-expanded')).toBe('false');
    openMenu();
    fixture.detectChanges();
    expect(element.textContent).toContain('Registrieren');
    expect(element.querySelector('.site-header-cart')).not.toBeNull();
  });
});
