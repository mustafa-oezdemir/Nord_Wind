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
import { BerlinDatePipe } from '../../../../shared/date/berlin-date.pipe';
import { euros } from '../../../../shared/money/money';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { PriceSummary } from '../../../cart/ui/price-summary/price-summary';
import { CheckoutSteps } from '../../ui/checkout-steps/checkout-steps';
import { Order, OrderStatus, OrdersApi } from '../../data-access/orders-api';
import {
  MockPayment,
  MockPaymentApi,
  MockPaymentMethod,
  MockPaymentOutcome,
} from '../../data-access/mock-payment-api';

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

const METHOD_LABELS: Record<MockPaymentMethod, string> = {
  debit_card: 'Debitkarte',
  klarna: 'Klarna',
  paypal: 'PayPal',
  bank_transfer: 'Überweisung',
};

const PAYMENT_LABELS: Record<MockPayment['state'], string> = {
  pending: 'Zahlung ausstehend',
  succeeded: 'Simuliert bezahlt',
  failed: 'Simulierte Zahlung fehlgeschlagen',
  refunded: 'Simuliert erstattet',
};

/** Bestätigung nach der Kasse; zeigt den gespeicherten Snapshot aus der API. */
@Component({
  selector: 'app-order-confirmation',
  imports: [BerlinDatePipe, RouterLink, MoneyPipe, PriceSummary, CheckoutSteps],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './order-confirmation.html',
  styleUrl: './order-confirmation.scss',
})
export class OrderConfirmation {
  private readonly api = inject(OrdersApi);
  private readonly mockApi = inject(MockPaymentApi);
  private readonly title = inject(Title);
  private readonly heading = viewChild<ElementRef<HTMLElement>>('heading');

  /** Routenparameter `:orderNumber`. */
  readonly orderNumber = input<string>('');

  protected readonly state = signal<ConfirmationState>('loading');
  protected readonly order = signal<Order | null>(null);
  protected readonly methods = signal<MockPaymentMethod[]>([]);
  protected readonly selectedMethod = signal<MockPaymentMethod>('debit_card');
  protected readonly payment = signal<MockPayment | null>(null);
  protected readonly paymentBusy = signal(false);
  protected readonly paymentNotice = signal('');
  protected readonly money = euros;
  protected readonly statusLabel = (status: OrderStatus) => STATUS_LABELS[status] ?? status;
  protected readonly methodLabel = (method: MockPaymentMethod) => METHOD_LABELS[method];
  protected readonly paymentLabel = (state: MockPayment['state']) => PAYMENT_LABELS[state];

  constructor() {
    effect(() => {
      const number = this.orderNumber();
      void this.load(number);
    });
  }

  protected async load(number: string): Promise<void> {
    this.state.set('loading');
    this.payment.set(null);
    this.methods.set([]);
    this.paymentNotice.set('');
    try {
      const order = await firstValueFrom(this.api.get(number));
      this.order.set(order);
      this.state.set('ready');
      this.title.setTitle(formatPageTitle(`Bestellung ${order.order_number}`));
      setTimeout(() => this.heading()?.nativeElement.focus());
      try {
        const available = await firstValueFrom(this.mockApi.methods());
        this.methods.set(available.methods);
        try {
          const existing = await firstValueFrom(this.mockApi.forOrder(order.id));
          this.payment.set(existing);
          this.selectedMethod.set(existing.method);
        } catch (error) {
          if (!(error instanceof HttpErrorResponse && error.status === 404)) {
            this.paymentNotice.set('Der Zahlungsstatus konnte nicht geladen werden.');
          }
        }
      } catch (error) {
        // Außerhalb der Entwicklungsumgebung gibt es keine simulierten Zahlungsarten.
        if (!(error instanceof HttpErrorResponse && error.status === 404)) {
          this.paymentNotice.set('Die Zahlungsarten konnten nicht geladen werden.');
        }
      }
    } catch (error) {
      const status = error instanceof HttpErrorResponse ? error.status : 0;
      this.state.set(status === 401 ? 'signed-out' : status === 404 ? 'not-found' : 'error');
      if (status === 404) this.title.setTitle(formatPageTitle('Bestellung nicht gefunden'));
    }
  }

  protected selectMethod(method: MockPaymentMethod): void {
    this.selectedMethod.set(method);
  }

  protected async initiatePayment(): Promise<void> {
    const order = this.order();
    if (!order || this.paymentBusy() || this.payment()) return;
    this.paymentBusy.set(true);
    this.paymentNotice.set('');
    try {
      this.payment.set(
        await firstValueFrom(this.mockApi.initiate(order.id, this.selectedMethod())),
      );
      this.order.set(await firstValueFrom(this.api.get(order.order_number)));
    } catch {
      this.paymentNotice.set(
        'Die Simulation konnte nicht gestartet werden. Bitte laden Sie die Bestellung neu.',
      );
    } finally {
      this.paymentBusy.set(false);
    }
  }

  protected async simulate(outcome: MockPaymentOutcome): Promise<void> {
    const payment = this.payment();
    const order = this.order();
    if (!payment || !order || this.paymentBusy()) return;
    this.paymentBusy.set(true);
    this.paymentNotice.set('');
    try {
      const result = await firstValueFrom(this.mockApi.simulate(payment.id, outcome));
      this.payment.set(result.payment);
      this.order.set(await firstValueFrom(this.api.get(order.order_number)));
    } catch {
      this.paymentNotice.set(
        'Das Testereignis konnte nicht verarbeitet werden. Bitte laden Sie die Bestellung neu.',
      );
    } finally {
      this.paymentBusy.set(false);
    }
  }

  protected async cancelOrder(): Promise<void> {
    const order = this.order();
    if (!order || this.paymentBusy()) return;
    this.paymentBusy.set(true);
    this.paymentNotice.set('');
    try {
      this.order.set(await firstValueFrom(this.api.cancel(order.order_number)));
      this.paymentNotice.set('Die Bestellung wurde storniert und der Bestand freigegeben.');
    } catch {
      this.paymentNotice.set(
        'Die Bestellung konnte nicht storniert werden. Bitte laden Sie sie neu.',
      );
    } finally {
      this.paymentBusy.set(false);
    }
  }
}
