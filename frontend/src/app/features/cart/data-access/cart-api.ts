import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, switchMap, tap } from 'rxjs';
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
}

export interface CartSnapshot {
  items: CartItem[];
  subtotal_cents: number;
  currency: 'EUR';
}

export interface CustomerSession {
  customer: { id: string; email: string; role: 'customer' | 'admin' };
  csrf_token: string;
  expires_at: string;
}

@Injectable({ providedIn: 'root' })
export class CartApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private csrfToken = '';

  session(): Observable<CustomerSession> {
    return this.http
      .get<CustomerSession>(`${this.base}/auth/session`)
      .pipe(tap((value) => (this.csrfToken = value.csrf_token)));
  }

  register(email: string, password: string): Observable<unknown> {
    return this.http.post(`${this.base}/auth/register`, { email, password });
  }

  login(email: string, password: string): Observable<CustomerSession> {
    return this.http
      .post<CustomerSession>(`${this.base}/auth/login`, { email, password })
      .pipe(tap((value) => (this.csrfToken = value.csrf_token)));
  }

  logout(): Observable<void> {
    return this.http
      .post<void>(`${this.base}/auth/logout`, {}, { headers: this.headers() })
      .pipe(tap(() => (this.csrfToken = '')));
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

  private headers(): HttpHeaders {
    return new HttpHeaders({ 'X-CSRF-Token': this.csrfToken });
  }
}
