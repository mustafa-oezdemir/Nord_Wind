import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { BerlinDatePipe } from '../../shared/date/berlin-date.pipe';
import { CartApi } from '../cart/data-access/cart-api';
import { OrderAddress, OrderStatus, OrdersApi } from '../checkout/data-access/orders-api';
import { AccountApi, SavedAddress } from './account-api';

const blankAddress = (): OrderAddress => ({
  first_name: '',
  last_name: '',
  company: '',
  street: '',
  house_number: '',
  address_extra: '',
  postal_code: '',
  city: '',
  country: 'DE',
});

@Component({
  selector: 'app-account-page',
  imports: [BerlinDatePipe, FormsModule, RouterLink],
  templateUrl: './account-page.html',
})
export class AccountPage implements OnInit {
  private readonly cart = inject(CartApi);
  private readonly api = inject(AccountApi);
  private readonly ordersApi = inject(OrdersApi);

  protected readonly state = signal<'loading' | 'ready' | 'signed-out' | 'error' | 'closed'>(
    'loading',
  );
  protected readonly email = signal('');
  protected readonly addresses = signal<SavedAddress[]>([]);
  protected readonly orders = signal<import('../checkout/data-access/orders-api').Order[]>([]);
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
  protected label = '';
  protected address = blankAddress();
  protected editingID: string | null = null;
  protected closePhrase = '';

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  protected async load(): Promise<void> {
    this.state.set('loading');
    try {
      const session = await firstValueFrom(this.cart.session());
      if (session.customer.role !== 'customer') {
        this.state.set('signed-out');
        return;
      }
      this.email.set(session.customer.email);
      const [addresses, orders] = await Promise.all([
        firstValueFrom(this.api.addresses()),
        firstValueFrom(this.ordersApi.list()),
      ]);
      this.addresses.set(addresses.data);
      this.orders.set(orders.data);
      this.state.set('ready');
    } catch (error) {
      this.state.set(
        error instanceof HttpErrorResponse && error.status === 401 ? 'signed-out' : 'error',
      );
    }
  }

  protected edit(item: SavedAddress): void {
    this.editingID = item.id;
    this.label = item.label;
    this.address = { ...item.address };
    this.notice.set('');
  }

  protected reset(): void {
    this.editingID = null;
    this.label = '';
    this.address = blankAddress();
  }

  protected async save(): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    try {
      await firstValueFrom(this.api.save(this.editingID, this.label, this.address));
      this.addresses.set((await firstValueFrom(this.api.addresses())).data);
      this.reset();
      this.notice.set('Adresse gespeichert.');
    } catch {
      this.notice.set('Adresse konnte nicht gespeichert werden. Bitte prüfen Sie die Eingaben.');
    } finally {
      this.busy.set(false);
    }
  }

  protected async remove(id: string): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    try {
      await firstValueFrom(this.api.remove(id));
      this.addresses.set(this.addresses().filter((item) => item.id !== id));
      this.notice.set('Adresse entfernt.');
    } catch {
      this.notice.set('Adresse konnte nicht entfernt werden.');
    } finally {
      this.busy.set(false);
    }
  }

  protected async exportData(): Promise<void> {
    try {
      const data = await firstValueFrom(this.api.export());
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }),
      );
      const link = document.createElement('a');
      link.href = url;
      link.download = 'nordwinds-konto.json';
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch {
      this.notice.set('Der Export konnte nicht erstellt werden.');
    }
  }

  protected async closeAccount(): Promise<void> {
    if (this.closePhrase !== 'KONTO SCHLIESSEN' || this.busy()) return;
    this.busy.set(true);
    try {
      await firstValueFrom(this.api.close());
      this.state.set('closed');
      this.email.set('');
      this.addresses.set([]);
      this.orders.set([]);
    } catch (error) {
      this.notice.set(
        error instanceof HttpErrorResponse && error.status === 409
          ? 'Das Konto kann erst geschlossen werden, wenn alle Bestellungen abgeschlossen sind.'
          : 'Das Konto konnte nicht geschlossen werden.',
      );
    } finally {
      this.busy.set(false);
    }
  }
}
