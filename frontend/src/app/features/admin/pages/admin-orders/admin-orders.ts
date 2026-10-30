import { HttpClient, HttpErrorResponse, HttpHeaders } from '@angular/common/http';
import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { API_BASE_URL } from '../../../../core/config/api-config';
import { Order, OrderStatus } from '../../../checkout/data-access/orders-api';
import { AuthSession } from '../../data-access/admin-api';

@Component({
  selector: 'app-admin-orders',
  imports: [DatePipe, FormsModule, RouterLink],
  templateUrl: './admin-orders.html',
})
export class AdminOrders implements OnInit {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  protected readonly state = signal<'loading' | 'ready' | 'unauthorized' | 'error'>('loading');
  protected readonly orders = signal<Order[]>([]);
  protected readonly metrics = signal<Record<string, number>>({});
  protected readonly notice = signal('');
  protected readonly busy = signal(false);
  protected readonly statusLabel = (status: OrderStatus) =>
    ({
      CREATED: 'Angelegt',
      PAYMENT_PENDING: 'Zahlung offen',
      PAID: 'Bezahlt',
      PROCESSING: 'In Bearbeitung',
      SHIPPED: 'Versendet',
      DELIVERED: 'Zugestellt',
      CANCELLED: 'Storniert',
      PAYMENT_FAILED: 'Zahlung fehlgeschlagen',
    })[status];
  protected tracking: Record<string, string> = {};
  private csrf = '';

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  protected async load(): Promise<void> {
    this.state.set('loading');
    try {
      const session = await firstValueFrom(this.http.get<AuthSession>(`${this.base}/auth/session`));
      if (session.customer.role !== 'admin') {
        this.state.set('unauthorized');
        return;
      }
      this.csrf = session.csrf_token;
      const [orders, metrics] = await Promise.all([
        firstValueFrom(this.http.get<{ data: Order[] }>(`${this.base}/admin/orders`)),
        firstValueFrom(
          this.http.get<{ data: Record<string, number> }>(
            `${this.base}/admin/orders/shipment-metrics`,
          ),
        ),
      ]);
      this.orders.set(orders.data);
      this.metrics.set(metrics.data);
      this.state.set('ready');
    } catch (error) {
      this.state.set(
        error instanceof HttpErrorResponse && [401, 403].includes(error.status)
          ? 'unauthorized'
          : 'error',
      );
    }
  }

  protected async advance(
    order: Order,
    status: 'PROCESSING' | 'SHIPPED' | 'DELIVERED',
  ): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    this.notice.set('');
    try {
      const updated = await firstValueFrom(
        this.http.patch<Order>(
          `${this.base}/admin/orders/${encodeURIComponent(order.order_number)}/status`,
          {
            status,
            tracking_number: status === 'SHIPPED' ? (this.tracking[order.id]?.trim() ?? '') : '',
          },
          { headers: new HttpHeaders({ 'X-CSRF-Token': this.csrf }) },
        ),
      );
      this.orders.update((orders) =>
        orders.map((item) => (item.id === updated.id ? updated : item)),
      );
      this.notice.set(`Bestellung ${order.order_number} aktualisiert.`);
    } catch (error) {
      this.notice.set(
        error instanceof HttpErrorResponse && error.status === 409
          ? 'Der Statuswechsel ist nicht erlaubt. Bitte laden Sie die Bestellungen neu.'
          : 'Der Versandversuch ist fehlgeschlagen. Bitte prüfen Sie die Sendungsnummer und den Status.',
      );
    } finally {
      this.busy.set(false);
      try {
        const metrics = await firstValueFrom(
          this.http.get<{ data: Record<string, number> }>(
            `${this.base}/admin/orders/shipment-metrics`,
          ),
        );
        this.metrics.set(metrics.data);
      } catch {
        /* Der Hauptvorgang ist bereits abgeschlossen. */
      }
    }
  }
}
