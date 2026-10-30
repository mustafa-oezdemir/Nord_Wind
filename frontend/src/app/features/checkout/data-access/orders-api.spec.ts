import { HttpErrorResponse, provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';
import { customerSession, repricedCart, sampleOrder } from '../../../../testing/order-fixtures';
import { OrdersApi, newIdempotencyKey, readOrderProblem } from './orders-api';

describe('OrdersApi', () => {
  let api: OrdersApi;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    api = TestBed.inject(OrdersApi);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('holt eine frische Session und sendet CSRF-Token und Idempotency-Key nur im Header', async () => {
    const created = firstValueFrom(
      api.create(sampleOrder.address, 4690, 'nw-schluessel-0001', true),
    );
    http.expectOne('/api/v1/auth/session').flush(customerSession);
    const request = http.expectOne('/api/v1/orders');
    expect(request.request.method).toBe('POST');
    expect(request.request.body).toEqual({
      address: sampleOrder.address,
      expected_total_cents: 4690,
      preview_notice_acknowledged: true,
    });
    expect(request.request.headers.get('X-CSRF-Token')).toBe(customerSession.csrf_token);
    expect(request.request.headers.get('Idempotency-Key')).toBe('nw-schluessel-0001');
    expect(request.request.urlWithParams).toBe('/api/v1/orders');
    request.flush(sampleOrder, { status: 201, statusText: 'Created' });
    expect((await created).order_number).toBe('NW-2026-100001');
  });

  it('liest eine Bestellung über ihre kodierte Nummer', async () => {
    const order = firstValueFrom(api.get('NW-2026-100001'));
    http.expectOne('/api/v1/orders/NW-2026-100001').flush(sampleOrder);
    expect((await order).items.length).toBe(2);
  });

  it('erzeugt zufällige Schlüssel im erlaubten Format', () => {
    const first = newIdempotencyKey();
    const second = newIdempotencyKey();
    expect(/^nw-[0-9a-f]{32}$/.test(first)).toBe(true);
    expect(first === second).toBe(false);
  });

  it('wertet Problemantworten der Kasse aus und ignoriert fremde Felder', () => {
    const problem = readOrderProblem(
      new HttpErrorResponse({
        status: 409,
        error: { code: 'CART_CHANGED', reason: 'total_changed', cart: repricedCart },
      }),
    );
    expect(problem?.code).toBe('CART_CHANGED');
    expect(problem?.reason).toBe('total_changed');
    expect(problem?.cart?.pricing.total_cents).toBe(4690);

    const odd = readOrderProblem(
      new HttpErrorResponse({ status: 409, error: { code: 1, reason: 'egal', cart: 'x' } }),
    );
    expect(odd?.code).toBeUndefined();
    expect(odd?.reason).toBeUndefined();
    expect(odd?.cart).toBeUndefined();
    expect(readOrderProblem(new Error('kein HTTP'))).toBeNull();
  });
});
