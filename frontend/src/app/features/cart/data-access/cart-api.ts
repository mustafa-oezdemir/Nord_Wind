import { HttpClient, HttpErrorResponse, HttpHeaders } from '@angular/common/http';
import { Injectable, inject, signal } from '@angular/core';
import { Observable, catchError, switchMap, tap, throwError } from 'rxjs';
import { API_BASE_URL } from '../../../core/config/api-config';

export interface CartItem {
  variant_id: string;
  product_id: string;
  product_slug: string;
  product_name: string;
  sku: string;
  label: string;
  quantity: number;
  unit_price_cents: number;
  line_total_cents: number;
  currency: 'EUR';
  available: boolean;
  unavailable_reason: '' | 'product_unavailable' | 'insufficient_stock';
  tax_class: 'standard' | 'reduced';
  tax_rate_bp: number;
}

/** Steuerzeile je Satz; alle Beträge in Cent und von der API berechnet (ADR-007). */
export interface CartTaxLine {
  tax_class: 'standard' | 'reduced';
  rate_bp: number;
  gross_cents: number;
  net_cents: number;
  vat_cents: number;
}

export interface CartPricing {
  goods_cents: number;
  discount_cents: number;
  shipping_cents: number;
  total_cents: number;
  missing_for_free_shipping_cents: number;
  shipping_flat_cents: number;
  free_shipping_from_cents: number;
  currency: 'EUR';
  taxes: CartTaxLine[];
}

export interface CartDiscount {
  code: string;
  percent_bp: number;
  /** false, wenn der hinterlegte Code inzwischen deaktiviert oder abgelaufen ist. */
  applied: boolean;
}

export interface CartSnapshot {
  items: CartItem[];
  subtotal_cents: number;
  currency: 'EUR';
  discount: CartDiscount | null;
  pricing: CartPricing;
}

export const EMPTY_CART: CartSnapshot = {
  items: [],
  subtotal_cents: 0,
  currency: 'EUR',
  discount: null,
  pricing: {
    goods_cents: 0,
    discount_cents: 0,
    shipping_cents: 0,
    total_cents: 0,
    missing_for_free_shipping_cents: 0,
    shipping_flat_cents: 0,
    free_shipping_from_cents: 0,
    currency: 'EUR',
    taxes: [],
  },
};

export interface CustomerSession {
  customer: { id: string; email: string; role: 'customer' | 'admin' | 'employee' };
  csrf_token: string;
  expires_at: string;
}

@Injectable({ providedIn: 'root' })
export class CartApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private csrfToken = '';
  private readonly sessionState = signal<CustomerSession | null | undefined>(undefined);
  readonly activeSession = this.sessionState.asReadonly();

  session(): Observable<CustomerSession> {
    return this.http.get<CustomerSession>(`${this.base}/auth/session`).pipe(
      tap((value) => this.rememberSession(value)),
      catchError((error: unknown) => {
        if (error instanceof HttpErrorResponse && error.status === 401) this.clearSession();
        return throwError(() => error);
      }),
    );
  }

  register(email: string, password: string): Observable<unknown> {
    return this.http.post(`${this.base}/auth/register`, { email, password });
  }

  login(email: string, password: string): Observable<CustomerSession> {
    return this.http
      .post<CustomerSession>(`${this.base}/auth/login`, { email, password })
      .pipe(tap((value) => this.rememberSession(value)));
  }

  logout(): Observable<void> {
    return this.http
      .post<void>(`${this.base}/auth/logout`, {}, { headers: this.headers() })
      .pipe(tap(() => this.clearSession()));
  }

  clearSession(): void {
    this.csrfToken = '';
    this.sessionState.set(null);
  }

  get(): Observable<CartSnapshot> {
    return this.http.get<CartSnapshot>(`${this.base}/cart`);
  }

  add(variantId: string, quantity: number): Observable<CartSnapshot> {
    return this.session().pipe(
      switchMap(() =>
        this.http.post<CartSnapshot>(
          `${this.base}/cart/items`,
          { variant_id: variantId, quantity },
          { headers: this.headers() },
        ),
      ),
    );
  }

  set(variantId: string, quantity: number): Observable<CartSnapshot> {
    return this.http.put<CartSnapshot>(
      `${this.base}/cart/items/${variantId}`,
      { quantity },
      { headers: this.headers() },
    );
  }

  remove(variantId: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/cart/items/${variantId}`, {
      headers: this.headers(),
    });
  }

  applyDiscount(code: string): Observable<CartSnapshot> {
    return this.http.put<CartSnapshot>(
      `${this.base}/cart/discount`,
      { code },
      { headers: this.headers() },
    );
  }

  removeDiscount(): Observable<CartSnapshot> {
    return this.http.delete<CartSnapshot>(`${this.base}/cart/discount`, {
      headers: this.headers(),
    });
  }

  private headers(): HttpHeaders {
    return new HttpHeaders({ 'X-CSRF-Token': this.csrfToken });
  }

  private rememberSession(value: CustomerSession): void {
    this.csrfToken = value.csrf_token;
    this.sessionState.set(value);
  }
}
