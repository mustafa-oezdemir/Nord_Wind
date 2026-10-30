import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';
import { CartApi, CartSnapshot, CustomerSession } from './cart-api';

describe('CartApi', () => {
  let api: CartApi;
  let http: HttpTestingController;
  const session: CustomerSession = {
    customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
    csrf_token: 'synthetic-csrf-token',
    expires_at: '2026-10-31T00:00:00Z',
  };
  const empty: CartSnapshot = { items: [], subtotal_cents: 0, currency: 'EUR' };

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    api = TestBed.inject(CartApi);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('holt eine frische Session und sendet den CSRF-Token nur im Header', async () => {
    const added = firstValueFrom(api.add('variant-id', 2));
    http.expectOne('/api/v1/auth/session').flush(session);
    const request = http.expectOne('/api/v1/cart/items');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({ variant_id: 'variant-id', quantity: 2 });
    expect(request.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    expect(request.request.urlWithParams).not.toContain(session.csrf_token);
    request.flush(empty);
    expect(await added).toEqual(empty);

    const updated = firstValueFrom(api.set('variant-id', 1));
    const put = http.expectOne('/api/v1/cart/items/variant-id');
    expect(put.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    put.flush(empty);
    await updated;
  });
});
