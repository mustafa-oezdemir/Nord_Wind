import { HttpClient, HttpErrorResponse, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, switchMap } from 'rxjs';
import { API_BASE_URL } from '../../../core/config/api-config';
import { CartApi, CartSnapshot, CartTaxLine } from '../../cart/data-access/cart-api';

/** Liefer- und Rechnungsadresse; nur Deutschland (ADR-008). */
export interface OrderAddress {
  first_name: string;
  last_name: string;
  company: string;
  street: string;
  house_number: string;
  address_extra: string;
  postal_code: string;
  city: string;
  country: 'DE';
}

export type OrderStatus =
  | 'CREATED'
  | 'PAYMENT_PENDING'
  | 'PAID'
  | 'PROCESSING'
  | 'SHIPPED'
  | 'DELIVERED'
  | 'CANCELLED'
  | 'PAYMENT_FAILED';

export interface OrderItem {
  position: number;
  variant_id: string;
  product_name: string;
  variant_label: string;
  sku: string;
  quantity: number;
  unit_price_cents: number;
  line_total_cents: number;
  tax_class: 'standard' | 'reduced';
  tax_rate_bp: number;
}

export interface Order {
  id: string;
  order_number: string;
  status: OrderStatus;
  customer_email: string;
  address: OrderAddress;
  items: OrderItem[];
  pricing: {
    goods_cents: number;
    discount_cents: number;
    shipping_cents: number;
    total_cents: number;
    currency: 'EUR';
    taxes: CartTaxLine[];
  };
  discount: { code: string; percent_bp: number } | null;
  shipment: { carrier: 'MANUAL'; tracking_number: string; created_at: string } | null;
  status_events: { from_status: OrderStatus | null; to_status: OrderStatus; created_at: string }[];
  created_at: string;
}

export type OrderAddressField = keyof OrderAddress;

export interface OrderFieldError {
  field: OrderAddressField;
  code: 'required' | 'too_short' | 'too_long' | 'invalid_format' | 'unsupported_country';
}

/** Ausgewertete Problemantwort der Kasse (RFC 9457 mit Zusatzfeldern). */
export interface OrderProblem {
  status: number;
  code?: string;
  reason?: 'total_changed' | 'item_unavailable';
  errors?: OrderFieldError[];
  cart?: CartSnapshot;
}

export function readOrderProblem(error: unknown): OrderProblem | null {
  if (!(error instanceof HttpErrorResponse) || typeof error.error !== 'object' || !error.error) {
    return null;
  }
  const body = error.error as Record<string, unknown>;
  return {
    status: error.status,
    code: typeof body['code'] === 'string' ? body['code'] : undefined,
    reason:
      body['reason'] === 'total_changed' || body['reason'] === 'item_unavailable'
        ? body['reason']
        : undefined,
    errors: Array.isArray(body['errors']) ? (body['errors'] as OrderFieldError[]) : undefined,
    cart:
      typeof body['cart'] === 'object' && body['cart'] !== null
        ? (body['cart'] as CartSnapshot)
        : undefined,
  };
}

/**
 * Schlüssel für einen Bestellversuch. Er bleibt für Wiederholungen derselben Bestellung gleich,
 * damit die API höchstens eine Bestellung anlegt. `getRandomValues` funktioniert auch ohne HTTPS.
 */
export function newIdempotencyKey(): string {
  const bytes = new Uint8Array(16);
  globalThis.crypto.getRandomValues(bytes);
  return 'nw-' + Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
}

@Injectable({ providedIn: 'root' })
export class OrdersApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private readonly cart = inject(CartApi);

  /** Legt die Bestellung an; ein frisches CSRF-Token kommt aus der aktuellen Session. */
  create(
    address: OrderAddress,
    expectedTotalCents: number,
    key: string,
    noticeAcknowledged: boolean,
  ): Observable<Order> {
    return this.cart.session().pipe(
      switchMap((session) =>
        this.http.post<Order>(
          `${this.base}/orders`,
          {
            address,
            expected_total_cents: expectedTotalCents,
            preview_notice_acknowledged: noticeAcknowledged,
          },
          {
            headers: new HttpHeaders({
              'X-CSRF-Token': session.csrf_token,
              'Idempotency-Key': key,
            }),
          },
        ),
      ),
    );
  }

  get(orderNumber: string): Observable<Order> {
    return this.http.get<Order>(`${this.base}/orders/${encodeURIComponent(orderNumber)}`);
  }

  list(): Observable<{ data: Order[] }> {
    return this.http.get<{ data: Order[] }>(`${this.base}/orders`);
  }

  cancel(orderNumber: string): Observable<Order> {
    return this.cart
      .session()
      .pipe(
        switchMap((session) =>
          this.http.post<Order>(
            `${this.base}/orders/${encodeURIComponent(orderNumber)}/cancel`,
            {},
            { headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }) },
          ),
        ),
      );
  }
}
