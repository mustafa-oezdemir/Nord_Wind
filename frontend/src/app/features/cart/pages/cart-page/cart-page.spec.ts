import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { CartPage } from './cart-page';

describe('CartPage', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
  });

  it('zeigt eine nicht mehr lieferbare Position und erlaubt ihre Entfernung', async () => {
    const fixture = TestBed.createComponent(CartPage);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne('/api/v1/auth/session').flush({
      customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
      csrf_token: 'synthetic-csrf',
      expires_at: '2026-10-31T00:00:00Z',
    });
    await Promise.resolve();
    http.expectOne('/api/v1/cart').flush({
      items: [
        {
          variant_id: 'variant-id',
          product_id: 'product-id',
          product_slug: 'pfanne',
          product_name: 'Pfanne',
          sku: 'NW-PFANNE',
          label: 'Standard',
          quantity: 4,
          unit_price_cents: 1000,
          line_total_cents: 4000,
          currency: 'EUR',
          available: false,
          unavailable_reason: 'insufficient_stock',
        },
      ],
      subtotal_cents: 0,
      currency: 'EUR',
    });
    await fixture.whenStable();
    const text = (fixture.nativeElement as HTMLElement).textContent ?? '';
    expect(text).toContain('Bestand hat sich geändert.');
    expect(text).toMatch(/0,00\s€/);
    expect(
      (fixture.nativeElement as HTMLElement).querySelector('button.btn-outline-danger'),
    ).not.toBeNull();
    http.verify();
  });

  it('erzeugt ein Passwort für beide Registrierungsfelder und blockiert Abweichungen', async () => {
    const fixture = TestBed.createComponent(CartPage);
    fixture.detectChanges();
    const http = TestBed.inject(HttpTestingController);
    http.expectOne('/api/v1/auth/session').flush({}, { status: 401, statusText: 'Unauthorized' });
    await fixture.whenStable();

    const root = fixture.nativeElement as HTMLElement;
    (
      Array.from(root.querySelectorAll('button')).find((button) =>
        button.textContent?.includes('Neues Konto?'),
      ) as HTMLButtonElement
    ).click();
    await fixture.whenStable();

    const email = root.querySelector<HTMLInputElement>('#cart-email')!;
    const password = root.querySelector<HTMLInputElement>('#cart-password')!;
    const confirmation = root.querySelector<HTMLInputElement>('#cart-confirm-password')!;
    const submit = root.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    email.value = 'kunde@example.invalid';
    email.dispatchEvent(new Event('input', { bubbles: true }));
    password.value = 'a long synthetic password';
    password.dispatchEvent(new Event('input', { bubbles: true }));
    confirmation.value = 'a different synthetic password';
    confirmation.dispatchEvent(new Event('input', { bubbles: true }));
    await fixture.whenStable();
    expect(submit.disabled).toBe(true);
    expect(root.textContent).toContain('Die Passwörter stimmen nicht überein.');
    expect(http.match('/api/v1/auth/register')).toHaveLength(0);

    (
      Array.from(root.querySelectorAll('button')).find((button) =>
        button.textContent?.includes('Sicheres Passwort erstellen'),
      ) as HTMLButtonElement
    ).click();
    await fixture.whenStable();
    expect(/^[a-zA-Z0-9_-]{24}$/.test(password.value)).toBe(true);
    expect(confirmation.value === password.value).toBe(true);
    expect(submit.disabled).toBe(false);
    expect(root.textContent).toContain('Bitte in einem Passwortmanager');

    (
      Array.from(root.querySelectorAll('button')).find((button) =>
        button.textContent?.includes('Passwörter anzeigen'),
      ) as HTMLButtonElement
    ).click();
    await fixture.whenStable();
    expect(password.type).toBe('text');
    expect(confirmation.type).toBe('text');

    submit.click();
    const register = http.expectOne('/api/v1/auth/register');
    expect(register.request.body.email).toBe('kunde@example.invalid');
    expect(register.request.body.password === password.value).toBe(true);
    expect(Object.keys(register.request.body)).toEqual(['email', 'password']);
    register.flush(
      { customer: { id: 'customer-id', email: 'kunde@example.invalid' } },
      { status: 201, statusText: 'Created' },
    );
    await Promise.resolve();
    const login = http.expectOne('/api/v1/auth/login');
    login.flush({
      customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
      csrf_token: 'synthetic-csrf',
      expires_at: '2026-10-31T00:00:00Z',
    });
    await Promise.resolve();
    http.expectOne('/api/v1/cart').flush({ items: [], subtotal_cents: 0, currency: 'EUR' });
    await fixture.whenStable();
    http.verify();
  });
});
