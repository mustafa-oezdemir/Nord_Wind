import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { CartItem, CartSnapshot, EMPTY_CART } from '../../data-access/cart-api';
import { CartPage } from './cart-page';

const item = (overrides: Partial<CartItem> = {}): CartItem => ({
  variant_id: 'variant-id',
  product_id: 'product-id',
  product_slug: 'pfanne',
  product_name: 'Pfanne',
  sku: 'NW-PFANNE',
  label: 'Standard',
  quantity: 1,
  unit_price_cents: 3000,
  line_total_cents: 3000,
  currency: 'EUR',
  available: true,
  unavailable_reason: '',
  tax_class: 'standard',
  tax_rate_bp: 1900,
  ...overrides,
});

/** Antwort der API für Pfanne 30,00 € (19 %) + Buch 10,00 € (7 %), siehe Go-Integrationstest. */
const mixedCart: CartSnapshot = {
  items: [
    item(),
    item({
      variant_id: 'book-id',
      product_slug: 'kochbuch',
      product_name: 'Kochbuch',
      sku: 'NW-BUCH',
      unit_price_cents: 1000,
      line_total_cents: 1000,
      tax_class: 'reduced',
      tax_rate_bp: 700,
    }),
  ],
  subtotal_cents: 4000,
  currency: 'EUR',
  discount: null,
  pricing: {
    goods_cents: 4000,
    discount_cents: 0,
    shipping_cents: 490,
    total_cents: 4490,
    missing_for_free_shipping_cents: 1000,
    shipping_flat_cents: 490,
    free_shipping_from_cents: 5000,
    currency: 'EUR',
    taxes: [
      { tax_class: 'standard', rate_bp: 1900, gross_cents: 3368, net_cents: 2830, vat_cents: 538 },
      { tax_class: 'reduced', rate_bp: 700, gross_cents: 1122, net_cents: 1049, vat_cents: 73 },
    ],
  },
};

const discountedCart: CartSnapshot = {
  ...mixedCart,
  discount: { code: 'HERBST10', percent_bp: 1000, applied: true },
  pricing: {
    ...mixedCart.pricing,
    goods_cents: 7000,
    discount_cents: 700,
    shipping_cents: 0,
    total_cents: 6300,
    missing_for_free_shipping_cents: 0,
    taxes: [
      { tax_class: 'standard', rate_bp: 1900, gross_cents: 5400, net_cents: 4538, vat_cents: 862 },
      { tax_class: 'reduced', rate_bp: 700, gross_cents: 900, net_cents: 841, vat_cents: 59 },
    ],
  },
};

describe('CartPage', () => {
  let fixture: ComponentFixture<CartPage>;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideRouter([]), provideHttpClient(), provideHttpClientTesting()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render(snapshot: CartSnapshot): Promise<HTMLElement> {
    fixture = TestBed.createComponent(CartPage);
    fixture.detectChanges();
    http.expectOne('/api/v1/auth/session').flush({
      customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
      csrf_token: 'synthetic-csrf',
      expires_at: '2026-10-31T00:00:00Z',
    });
    await Promise.resolve();
    http.expectOne('/api/v1/cart').flush(snapshot);
    await fixture.whenStable();
    return fixture.nativeElement as HTMLElement;
  }

  const text = (element: HTMLElement, testId: string) =>
    (element.querySelector(`[data-testid="${testId}"]`)?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();

  it('zeigt eine nicht mehr lieferbare Position und erlaubt ihre Entfernung', async () => {
    const element = await render({
      ...EMPTY_CART,
      items: [item({ quantity: 4, available: false, unavailable_reason: 'insufficient_stock' })],
    });
    expect(element.textContent).toContain('Bestand hat sich geändert.');
    expect(text(element, 'cart-goods')).toBe('0,00 €');
    expect(element.querySelector('button.btn-outline-danger')).not.toBeNull();
    expect(element.querySelector('[data-testid="cart-checkout"]')).toBeNull();
    expect(element.textContent).toContain('Entfernen Sie nicht lieferbare Artikel');
  });

  it('zeigt genau die Beträge der API: Warenwert, Versand, Gesamtsumme und MwSt. je Satz', async () => {
    const element = await render(mixedCart);

    expect(text(element, 'cart-goods')).toBe('40,00 €');
    expect(text(element, 'cart-shipping')).toBe('4,90 €');
    expect(text(element, 'cart-total')).toBe('44,90 €');
    expect(text(element, 'cart-vat-1900')).toBe('5,38 €');
    expect(text(element, 'cart-vat-700')).toBe('0,73 €');
    expect(text(element, 'cart-free-shipping-hint')).toBe(
      'Noch 10,00 € bis zum kostenlosen Versand.',
    );
    expect(element.querySelector('[data-testid="cart-discount"]')).toBeNull();
    expect(element.querySelector('[data-testid="cart-checkout"]')?.getAttribute('href')).toBe(
      '/kasse',
    );
    // Intl trennt Zahl und Prozentzeichen mit einem geschützten Leerzeichen.
    expect(element.textContent).toMatch(/enthaltene MwSt\. 19\s%/);
    expect(element.textContent).toMatch(/enthaltene MwSt\. 7\s%/);
  });

  it('speichert Mengen automatisch und behält eine Änderung während der ersten Anfrage', async () => {
    const element = await render(mixedCart);
    const input = element.querySelector<HTMLInputElement>('#cart-qty-variant-id')!;
    input.value = '2';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    expect(element.querySelector('[data-testid="cart-checkout"]')).toBeNull();

    await new Promise((resolve) => setTimeout(resolve, 380));
    const first = http.expectOne('/api/v1/cart/items/variant-id');
    expect(first.request.method).toBe('PUT');
    expect(first.request.body).toEqual({ quantity: 2 });

    input.value = '3';
    input.dispatchEvent(new Event('input', { bubbles: true }));
    fixture.detectChanges();
    first.flush({
      ...mixedCart,
      items: [item({ quantity: 2, line_total_cents: 6000 }), mixedCart.items[1]],
    });
    await new Promise((resolve) => setTimeout(resolve, 380));
    const second = http.expectOne('/api/v1/cart/items/variant-id');
    expect(second.request.body).toEqual({ quantity: 3 });
    second.flush({
      ...mixedCart,
      items: [item({ quantity: 3, line_total_cents: 9000 }), mixedCart.items[1]],
    });
    await Promise.resolve();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(input.value).toBe('3');
    expect(text(element, 'cart-line-total-variant-id')).toContain('90,00 €');
    expect(element.querySelector('[data-testid="cart-checkout"]')).not.toBeNull();
  });

  it('löst einen Rabattcode ein und übernimmt die neue Aufstellung der API', async () => {
    const element = await render(mixedCart);
    const input = element.querySelector('#cart-discount-code') as HTMLInputElement;
    input.value = ' herbst10 ';
    input.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    (element.querySelector('.cart-discount-form button') as HTMLButtonElement).click();

    const request = http.expectOne('/api/v1/cart/discount');
    expect(request.request.method).toBe('PUT');
    expect(request.request.body).toEqual({ code: 'herbst10' });
    request.flush(discountedCart);
    await fixture.whenStable();

    expect(text(element, 'cart-discount')).toBe('−7,00 €');
    expect(element.textContent).toMatch(/Rabatt HERBST10 \(10\s%\)/);
    expect(text(element, 'cart-shipping')).toBe('kostenlos');
    expect(text(element, 'cart-total')).toBe('63,00 €');
    expect(element.querySelector('[data-testid="cart-free-shipping-hint"]')).toBeNull();
  });

  it('meldet einen ungültigen Rabattcode ohne den Warenkorb zu verändern', async () => {
    const element = await render(mixedCart);
    const input = element.querySelector('#cart-discount-code') as HTMLInputElement;
    input.value = 'FALSCH';
    input.dispatchEvent(new Event('input'));
    await fixture.whenStable();
    (element.querySelector('.cart-discount-form button') as HTMLButtonElement).click();
    http
      .expectOne('/api/v1/cart/discount')
      .flush({ code: 'NOT_FOUND' }, { status: 404, statusText: 'Not Found' });
    await fixture.whenStable();

    expect(element.querySelector('[role="alert"]')?.textContent).toContain(
      'Der Rabattcode ist ungültig oder abgelaufen.',
    );
    expect(text(element, 'cart-total')).toBe('44,90 €');
  });

  it('weist auf einen nicht mehr gültigen hinterlegten Code hin', async () => {
    const element = await render({
      ...mixedCart,
      discount: { code: 'SOMMER', percent_bp: 1500, applied: false },
    });

    expect(element.textContent).toContain(
      'Der Rabattcode SOMMER ist nicht mehr gültig und wird nicht angerechnet.',
    );
    expect(element.querySelector('[data-testid="cart-discount"]')).toBeNull();
    expect(text(element, 'cart-total')).toBe('44,90 €');
  });

  it('setzt den Fokus nach dem Abmelden auf die Warenkorb-Überschrift', async () => {
    const element = await render(EMPTY_CART);
    const logout = Array.from(element.querySelectorAll('button')).find((button) =>
      button.textContent?.includes('Abmelden'),
    )!;
    logout.focus();
    logout.click();
    http.expectOne('/api/v1/auth/logout').flush(null);
    await fixture.whenStable();
    await new Promise((resolve) => setTimeout(resolve));

    expect(document.activeElement).toBe(element.querySelector('h1'));
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
    expect(/^[a-zA-Z0-9!?]{24}$/.test(password.value)).toBe(true);
    expect(/[!?]/.test(password.value)).toBe(true);
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
    await new Promise((resolve) => setTimeout(resolve));
    expect(document.activeElement).toBe(root.querySelector('h1'));
    http.verify();
  });
});
