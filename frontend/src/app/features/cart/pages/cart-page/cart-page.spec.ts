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
});
