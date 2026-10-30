import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import {
  customerSession,
  mixedCart,
  repricedCart,
  sampleOrder,
} from '../../../../../testing/order-fixtures';
import { CartSnapshot } from '../../../cart/data-access/cart-api';
import { CheckoutPage } from './checkout-page';

@Component({ template: '' })
class Blank {}

const validAddress: Record<string, string> = {
  first_name: ' Erika ',
  last_name: 'Musterfrau',
  street: 'Heidestraße',
  house_number: '17',
  address_extra: 'Hinterhaus',
  postal_code: '51147',
  city: 'Köln',
};

/** Lässt Antworten, Mikrotasks und den zonenlosen Scheduler vollständig durchlaufen. */
async function settle(target: { whenStable(): Promise<unknown> }): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve));
  await target.whenStable();
}

describe('CheckoutPage', () => {
  let fixture: ComponentFixture<CheckoutPage>;
  let http: HttpTestingController;
  let element: HTMLElement;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: 'bestellung/:orderNumber', component: Blank }]),
        provideHttpClient(),
        provideHttpClientTesting(),
      ],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render(snapshot: CartSnapshot = mixedCart): Promise<void> {
    fixture = TestBed.createComponent(CheckoutPage);
    fixture.detectChanges();
    http.expectOne('/api/v1/auth/session').flush(customerSession);
    await Promise.resolve();
    http.expectOne('/api/v1/cart').flush(snapshot);
    await settle(fixture);
    element = fixture.nativeElement as HTMLElement;
  }

  const text = (testId: string) =>
    (element.querySelector(`[data-testid="${testId}"]`)?.textContent ?? '')
      .replace(/\s+/g, ' ')
      .trim();

  function fill(values: Record<string, string>): void {
    for (const [name, value] of Object.entries(values)) {
      const input = element.querySelector<HTMLInputElement>(`#checkout-${name}`)!;
      input.value = value;
      input.dispatchEvent(new Event('input'));
    }
  }

  async function submit(): Promise<void> {
    element.querySelector<HTMLButtonElement>('[data-testid="checkout-submit"]')!.click();
    await settle(fixture);
  }

  async function expectOrderRequest() {
    await Promise.resolve();
    http.expectOne('/api/v1/auth/session').flush(customerSession);
    return http.expectOne('/api/v1/orders');
  }

  it('verweist ohne Anmeldung auf den Warenkorb', async () => {
    fixture = TestBed.createComponent(CheckoutPage);
    fixture.detectChanges();
    http.expectOne('/api/v1/auth/session').flush({}, { status: 401, statusText: 'Unauthorized' });
    await settle(fixture);
    element = fixture.nativeElement as HTMLElement;
    expect(element.textContent).toContain('Bitte melden Sie sich im Warenkorb an');
  });

  it('zeigt die Beträge der API und bestellt nicht mit unvollständiger Adresse', async () => {
    await render();
    expect(text('cart-total')).toBe('44,90 €');
    expect(text('cart-vat-700')).toBe('0,73 €');
    expect(element.textContent).toContain('Zahlungspflichtig bestellen');
    expect(element.querySelector('[aria-current="step"]')?.textContent).toContain('Kasse');

    fill({ postal_code: '1234', house_number: 'a' });
    await submit();
    expect(http.match('/api/v1/auth/session')).toHaveLength(0);
    expect(http.match('/api/v1/orders')).toHaveLength(0);
    expect(element.textContent).toContain('Bitte Vorname angeben.');
    expect(element.textContent).toContain('Bitte eine fünfstellige deutsche Postleitzahl angeben.');
    expect(element.textContent).toContain('Bitte eine Hausnummer wie 17, 17a oder 5-7 angeben.');
    const firstName = element.querySelector('#checkout-first_name')!;
    expect(firstName.getAttribute('aria-invalid')).toBe('true');
    expect(firstName.getAttribute('aria-describedby')).toBe('checkout-first_name-error');
  });

  it('sendet Adresse, angezeigte Summe und Schlüssel und öffnet die Bestätigung', async () => {
    await render();
    fill(validAddress);
    await submit();
    const request = await expectOrderRequest();
    expect(request.request.body).toEqual({
      address: {
        first_name: 'Erika',
        last_name: 'Musterfrau',
        company: '',
        street: 'Heidestraße',
        house_number: '17',
        address_extra: 'Hinterhaus',
        postal_code: '51147',
        city: 'Köln',
        country: 'DE',
      },
      expected_total_cents: 4490,
    });
    expect(request.request.headers.get('X-CSRF-Token')).toBe(customerSession.csrf_token);
    expect(/^nw-[0-9a-f]{32}$/.test(request.request.headers.get('Idempotency-Key') ?? '')).toBe(
      true,
    );
    request.flush(sampleOrder, { status: 201, statusText: 'Created' });
    await settle(fixture);
    expect(TestBed.inject(Router).url).toBe('/bestellung/NW-2026-100001');
  });

  it('zeigt bei geänderter Summe die neue Übersicht und sendet danach mit demselben Schlüssel', async () => {
    await render();
    fill(validAddress);
    await submit();
    const first = await expectOrderRequest();
    const key = first.request.headers.get('Idempotency-Key');
    first.flush(
      { code: 'CART_CHANGED', reason: 'total_changed', cart: repricedCart },
      { status: 409, statusText: 'Conflict' },
    );
    await settle(fixture);
    expect(text('cart-total')).toBe('46,90 €');
    expect(element.querySelector('[role="alert"]')?.textContent).toContain(
      'Bitte prüfen Sie die neue Gesamtsumme',
    );

    await submit();
    const second = await expectOrderRequest();
    expect(second.request.body.expected_total_cents).toBe(4690);
    expect(second.request.headers.get('Idempotency-Key') === key).toBe(true);
    second.flush(sampleOrder, { status: 201, statusText: 'Created' });
    await settle(fixture);
  });

  it('ordnet Feldfehler der API den Eingaben zu', async () => {
    await render();
    fill(validAddress);
    await submit();
    (await expectOrderRequest()).flush(
      { code: 'VALIDATION_FAILED', errors: [{ field: 'postal_code', code: 'invalid_format' }] },
      { status: 422, statusText: 'Unprocessable Entity' },
    );
    await settle(fixture);
    expect(element.textContent).toContain('Bitte eine fünfstellige deutsche Postleitzahl angeben.');
    fill({ postal_code: '51149' });
    await settle(fixture);
    expect(element.textContent).not.toContain('Bitte eine fünfstellige deutsche Postleitzahl');
  });

  it('meldet einen Verbindungsabbruch und wiederholt ohne neuen Schlüssel', async () => {
    await render();
    fill(validAddress);
    await submit();
    const first = await expectOrderRequest();
    const key = first.request.headers.get('Idempotency-Key');
    first.error(new ProgressEvent('error'));
    await settle(fixture);
    expect(element.querySelector('[role="alert"]')?.textContent).toContain(
      'legt keine zweite Bestellung an',
    );
    await submit();
    const retry = await expectOrderRequest();
    expect(retry.request.headers.get('Idempotency-Key') === key).toBe(true);
    retry.flush(sampleOrder, { status: 200, statusText: 'OK' });
    await settle(fixture);
    expect(TestBed.inject(Router).url).toBe('/bestellung/NW-2026-100001');
  });

  it('sperrt die Bestellung, solange eine Position nicht lieferbar ist', async () => {
    await render({
      ...mixedCart,
      items: [
        { ...mixedCart.items[0], available: false, unavailable_reason: 'insufficient_stock' },
      ],
    });
    const button = element.querySelector<HTMLButtonElement>('[data-testid="checkout-submit"]')!;
    expect(button.disabled).toBe(true);
    expect(element.textContent).toContain('Nicht lieferbar');
  });
});
