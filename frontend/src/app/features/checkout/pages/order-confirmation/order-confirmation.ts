import { DatePipe } from '@angular/common';
import { HttpErrorResponse } from '@angular/common/http';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  effect,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { Title } from '@angular/platform-browser';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { formatPageTitle } from '../../../../core/routing/page-title-strategy';
import { euros } from '../../../../shared/money/money';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { PriceSummary } from '../../../cart/ui/price-summary/price-summary';
import { CheckoutSteps } from '../../ui/checkout-steps/checkout-steps';
import { Order, OrderStatus, OrdersApi } from '../../data-access/orders-api';

type ConfirmationState = 'loading' | 'ready' | 'signed-out' | 'not-found' | 'error';

const STATUS_LABELS: Record<OrderStatus, string> = {
  CREATED: 'Angelegt – Zahlung folgt',
  PAYMENT_PENDING: 'Zahlung ausstehend',
  PAID: 'Bezahlt',
  PROCESSING: 'In Bearbeitung',
  SHIPPED: 'Versendet',
  DELIVERED: 'Zugestellt',
  CANCELLED: 'Storniert',
  PAYMENT_FAILED: 'Zahlung fehlgeschlagen',
};

/** Bestätigung nach der Kasse; zeigt den gespeicherten Snapshot aus der API. */
@Component({
  selector: 'app-order-confirmation',
  imports: [DatePipe, RouterLink, MoneyPipe, PriceSummary, CheckoutSteps],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './order-confirmation.html',
  styleUrl: './order-confirmation.scss',
})
export class OrderConfirmation {
  private readonly api = inject(OrdersApi);
  private readonly title = inject(Title);
  private readonly heading = viewChild<ElementRef<HTMLElement>>('heading');

  /** Routenparameter `:orderNumber`. */
  readonly orderNumber = input<string>('');

  protected readonly state = signal<ConfirmationState>('loading');
  protected readonly order = signal<Order | null>(null);
  protected readonly money = euros;
  protected readonly statusLabel = (status: OrderStatus) => STATUS_LABELS[status] ?? status;

  constructor() {
    effect(() => {
      const number = this.orderNumber();
      void this.load(number);
    });
  }

  protected async load(number: string): Promise<void> {
    this.state.set('loading');
    try {
      const order = await firstValueFrom(this.api.get(number));
      this.order.set(order);
      this.state.set('ready');
      this.title.setTitle(formatPageTitle(`Bestellung ${order.order_number}`));
      setTimeout(() => this.heading()?.nativeElement.focus());
    } catch (error) {
      const status = error instanceof HttpErrorResponse ? error.status : 0;
      this.state.set(status === 401 ? 'signed-out' : status === 404 ? 'not-found' : 'error');
      if (status === 404) this.title.setTitle(formatPageTitle('Bestellung nicht gefunden'));
    }
  }
}
