import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, switchMap } from 'rxjs';
import { API_BASE_URL } from '../../core/config/api-config';
import { CartApi } from '../cart/data-access/cart-api';
import { Order, OrderAddress } from '../checkout/data-access/orders-api';

export interface SavedAddress {
  id: string;
  label: string;
  address: OrderAddress;
}

export interface AccountExport {
  email: string;
  addresses: SavedAddress[];
  orders: Order[];
  exported_at: string;
}

@Injectable({ providedIn: 'root' })
export class AccountApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private readonly cart = inject(CartApi);

  addresses(): Observable<{ data: SavedAddress[] }> {
    return this.http.get<{ data: SavedAddress[] }>(`${this.base}/customer/addresses`);
  }

  save(id: string | null, label: string, address: OrderAddress): Observable<SavedAddress> {
    return this.cart.session().pipe(
      switchMap((session) => {
        const options = { headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }) };
        const body = { label, address };
        return id
          ? this.http.put<SavedAddress>(
              `${this.base}/customer/addresses/${encodeURIComponent(id)}`,
              body,
              options,
            )
          : this.http.post<SavedAddress>(`${this.base}/customer/addresses`, body, options);
      }),
    );
  }

  remove(id: string): Observable<void> {
    return this.cart.session().pipe(
      switchMap((session) =>
        this.http.delete<void>(`${this.base}/customer/addresses/${encodeURIComponent(id)}`, {
          headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }),
        }),
      ),
    );
  }

  export(): Observable<AccountExport> {
    return this.http.get<AccountExport>(`${this.base}/customer/export`);
  }

  close(): Observable<void> {
    return this.cart.session().pipe(
      switchMap((session) =>
        this.http.delete<void>(`${this.base}/customer/me`, {
          headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }),
        }),
      ),
    );
  }
}
