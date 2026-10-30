import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, switchMap } from 'rxjs';
import { API_BASE_URL } from '../../../core/config/api-config';
import { CartApi } from '../../cart/data-access/cart-api';

export type MockPaymentMethod = 'debit_card' | 'klarna' | 'paypal' | 'bank_transfer';
export type MockPaymentState = 'pending' | 'succeeded' | 'failed' | 'refunded';
export type MockPaymentOutcome = 'succeeded' | 'failed' | 'refunded';

export interface MockPayment {
  id: string;
  order_id: string;
  method: MockPaymentMethod;
  state: MockPaymentState;
  amount_cents: number;
  currency: 'EUR';
}

export interface MockPaymentMethods {
  mode: 'mock';
  methods: MockPaymentMethod[];
}

export interface MockSimulation {
  event_id: string;
  payment: MockPayment;
}

@Injectable({ providedIn: 'root' })
export class MockPaymentApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private readonly cart = inject(CartApi);

  methods(): Observable<MockPaymentMethods> {
    return this.http.get<MockPaymentMethods>(`${this.base}/mock-payments/methods`);
  }

  forOrder(orderId: string): Observable<MockPayment> {
    return this.http.get<MockPayment>(
      `${this.base}/mock-payments/by-order/${encodeURIComponent(orderId)}`,
    );
  }

  initiate(orderId: string, method: MockPaymentMethod): Observable<MockPayment> {
    return this.cart
      .session()
      .pipe(
        switchMap((session) =>
          this.http.post<MockPayment>(
            `${this.base}/mock-payments`,
            { order_id: orderId, method },
            { headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }) },
          ),
        ),
      );
  }

  simulate(paymentId: string, outcome: MockPaymentOutcome): Observable<MockSimulation> {
    return this.cart
      .session()
      .pipe(
        switchMap((session) =>
          this.http.post<MockSimulation>(
            `${this.base}/mock-payments/${encodeURIComponent(paymentId)}/simulate`,
            { outcome },
            { headers: new HttpHeaders({ 'X-CSRF-Token': session.csrf_token }) },
          ),
        ),
      );
  }
}
