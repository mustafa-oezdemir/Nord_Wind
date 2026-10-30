import { registerLocaleData } from '@angular/common';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import localeDe from '@angular/common/locales/de';
import { LOCALE_ID } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Title } from '@angular/platform-browser';
import { provideRouter } from '@angular/router';
import { customerSession, sampleOrder } from '../../../../../testing/order-fixtures';
import { OrderConfirmation } from './order-confirmation';

registerLocaleData(localeDe);

/** Lässt Antworten, Mikrotasks und den zonenlosen Scheduler vollständig durchlaufen. */
async function settle(target: { whenStable(): Promise<unknown> }): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve));
  await target.whenStable();
}

describe('OrderConfirmation', () => {
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: LOCALE_ID, useValue: 'de-DE' },
      ],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render(number: string) {
    const fixture = TestBed.createComponent(OrderConfirmation);
    fixture.componentRef.setInput('orderNumber', number);
    fixture.detectChanges();
    return { fixture, element: fixture.nativeElement as HTMLElement };
  }

  it('zeigt den gespeicherten Snapshot der Bestellung', async () => {
    const { fixture, element } = await render('NW-2026-100001');
    http.expectOne('/api/v1/orders/NW-2026-100001').flush(sampleOrder);
    await new Promise((resolve) => setTimeout(resolve));
    http.expectOne('/api/v1/mock-payments/methods').flush({
      mode: 'mock',
      methods: ['debit_card', 'klarna', 'paypal', 'bank_transfer'],
    });
    await new Promise((resolve) => setTimeout(resolve));
    http
      .expectOne(`/api/v1/mock-payments/by-order/${sampleOrder.id}`)
      .flush({ code: 'NOT_FOUND' }, { status: 404, statusText: 'Not Found' });
    await settle(fixture);

    const text = element.textContent?.replace(/\s+/g, ' ') ?? '';
    expect(element.querySelector('h1')?.textContent).toContain('Danke, Erika!');
    expect(element.querySelector('[data-testid="order-number"]')?.textContent).toBe(
      'NW-2026-100001',
    );
    expect(element.querySelector('[data-testid="order-status"]')?.textContent).toBe(
      'Angelegt – Zahlung folgt',
    );
    expect(text).toContain('30. Oktober 2026, 15:12 Uhr');
    expect(text).toContain('Heidestraße 17');
    expect(text).toContain('Hinterhaus');
    expect(text).toContain('51147 Köln');
    expect(
      element.querySelector('[data-testid="cart-total"]')?.textContent?.replace(/\s+/g, ' ').trim(),
    ).toBe('46,90 €');
    expect(element.querySelector('[aria-current="step"]')?.textContent).toContain('Bestätigung');
    expect(TestBed.inject(Title).getTitle()).toBe('Bestellung NW-2026-100001 | NordWinds');
    expect(element.querySelectorAll('input[name="mock-payment-method"]').length).toBe(4);
  });

  it('simuliert eine gewählte Zahlungsart bis zum bezahlten Status', async () => {
    const { fixture, element } = await render('NW-2026-100001');
    http.expectOne('/api/v1/orders/NW-2026-100001').flush(sampleOrder);
    await new Promise((resolve) => setTimeout(resolve));
    http.expectOne('/api/v1/mock-payments/methods').flush({
      mode: 'mock',
      methods: ['debit_card', 'klarna', 'paypal', 'bank_transfer'],
    });
    await new Promise((resolve) => setTimeout(resolve));
    http
      .expectOne(`/api/v1/mock-payments/by-order/${sampleOrder.id}`)
      .flush({ code: 'NOT_FOUND' }, { status: 404, statusText: 'Not Found' });
    await settle(fixture);

    const paypal = element.querySelector<HTMLInputElement>('input[value="paypal"]');
    expect(paypal).not.toBeNull();
    paypal!.click();
    element.querySelector<HTMLButtonElement>('.mock-payment button')!.click();
    http.expectOne('/api/v1/auth/session').flush(customerSession);
    const initiation = http.expectOne('/api/v1/mock-payments');
    expect(initiation.request.body).toEqual({ order_id: sampleOrder.id, method: 'paypal' });
    expect(initiation.request.headers.get('X-CSRF-Token')).toBe(customerSession.csrf_token);
    const payment = {
      id: `pay_${'a'.repeat(32)}`,
      order_id: sampleOrder.id,
      method: 'paypal',
      state: 'pending',
      amount_cents: 4690,
      currency: 'EUR',
    };
    initiation.flush(payment);
    await new Promise((resolve) => setTimeout(resolve));
    http.expectOne('/api/v1/orders/NW-2026-100001').flush({
      ...sampleOrder,
      status: 'PAYMENT_PENDING',
    });
    await settle(fixture);

    const success = Array.from(
      element.querySelectorAll<HTMLButtonElement>('.mock-payment button'),
    ).find((button) => button.textContent?.includes('Erfolg simulieren'));
    success!.click();
    http.expectOne('/api/v1/auth/session').flush(customerSession);
    const event = http.expectOne(`/api/v1/mock-payments/${payment.id}/simulate`);
    expect(event.request.body).toEqual({ outcome: 'succeeded' });
    event.flush({ event_id: `evt_${'b'.repeat(32)}`, payment: { ...payment, state: 'succeeded' } });
    await new Promise((resolve) => setTimeout(resolve));
    http.expectOne('/api/v1/orders/NW-2026-100001').flush({ ...sampleOrder, status: 'PAID' });
    await settle(fixture);
    expect(element.querySelector('[data-testid="order-status"]')?.textContent).toBe('Bezahlt');
    expect(element.querySelector('[data-testid="mock-payment-status"]')?.textContent).toContain(
      'PayPal · Simuliert bezahlt',
    );
  });

  it('zeigt einen eigenen Zustand für unbekannte oder fremde Bestellungen', async () => {
    const { fixture, element } = await render('NW-2026-999999');
    http
      .expectOne('/api/v1/orders/NW-2026-999999')
      .flush({ code: 'NOT_FOUND' }, { status: 404, statusText: 'Not Found' });
    await settle(fixture);
    expect(element.querySelector('h1')?.textContent).toContain('Bestellung nicht gefunden');
  });
});
