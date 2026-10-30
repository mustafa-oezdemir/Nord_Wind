import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { CartApi, CartItem, CartSnapshot, CustomerSession } from '../../data-access/cart-api';

@Component({
  selector: 'app-cart-page',
  imports: [FormsModule, RouterLink, MoneyPipe],
  templateUrl: './cart-page.html',
  styleUrl: './cart-page.scss',
})
export class CartPage implements OnInit {
  private readonly api = inject(CartApi);

  protected readonly session = signal<CustomerSession | null>(null);
  protected readonly cart = signal<CartSnapshot>({ items: [], subtotal_cents: 0, currency: 'EUR' });
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly notice = signal('');
  protected readonly quantities = signal<Record<string, number>>({});
  protected email = '';
  protected password = '';
  protected registerMode = false;

  async ngOnInit(): Promise<void> {
    try {
      const session = await firstValueFrom(this.api.session());
      this.session.set(session);
      if (session.customer.role === 'customer') await this.load();
    } catch (error) {
      if (!(error instanceof HttpErrorResponse && error.status === 401)) this.showError(error);
    }
  }

  protected async authenticate(): Promise<void> {
    await this.run(async () => {
      if (this.registerMode) await firstValueFrom(this.api.register(this.email, this.password));
      const session = await firstValueFrom(this.api.login(this.email, this.password));
      this.password = '';
      this.session.set(session);
      if (session.customer.role === 'customer') await this.load();
    });
  }

  protected async logout(): Promise<void> {
    await this.run(async () => {
      await firstValueFrom(this.api.logout());
      this.session.set(null);
      this.setCart({ items: [], subtotal_cents: 0, currency: 'EUR' });
    });
  }

  protected setDraft(variantId: string, quantity: number): void {
    this.quantities.update((previous) => ({ ...previous, [variantId]: quantity }));
  }

  protected async saveQuantity(item: CartItem): Promise<void> {
    const quantity = Number(this.quantities()[item.variant_id]);
    if (!Number.isInteger(quantity) || quantity < 1 || quantity > 99) {
      this.error.set('Die Menge muss zwischen 1 und 99 liegen.');
      return;
    }
    await this.run(async () =>
      this.setCart(await firstValueFrom(this.api.set(item.variant_id, quantity))),
    );
  }

  protected async remove(item: CartItem): Promise<void> {
    await this.run(async () => {
      await firstValueFrom(this.api.remove(item.variant_id));
      await this.load();
      this.notice.set('Artikel entfernt.');
    });
  }

  protected money(cents: number): { amountCents: number; currency: 'EUR' } {
    return { amountCents: cents, currency: 'EUR' };
  }

  private async load(): Promise<void> {
    this.setCart(await firstValueFrom(this.api.get()));
  }

  private setCart(value: CartSnapshot): void {
    this.cart.set(value);
    this.quantities.set(
      Object.fromEntries(value.items.map((item) => [item.variant_id, item.quantity])),
    );
  }

  private async run(task: () => Promise<void>): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    try {
      await task();
    } catch (error) {
      this.showError(error);
      if (
        this.session() &&
        error instanceof HttpErrorResponse &&
        (error.status === 404 || error.status === 409)
      ) {
        try {
          await this.load();
        } catch {
          /* ursprünglichen Fehler anzeigen */
        }
      }
    } finally {
      this.busy.set(false);
    }
  }

  private showError(error: unknown): void {
    if (error instanceof HttpErrorResponse) {
      if (error.status === 401) {
        const wasSignedIn = this.session() !== null;
        this.session.set(null);
        this.error.set(
          wasSignedIn
            ? 'Die Anmeldung ist abgelaufen. Bitte erneut anmelden.'
            : 'E-Mail oder Passwort ist ungültig.',
        );
        return;
      }
      if (error.status === 403) {
        this.error.set('Für diese Aktion fehlt die Berechtigung.');
        return;
      }
      if (error.status === 409) {
        this.error.set(
          !this.session() && this.registerMode
            ? 'Dieses Konto existiert bereits. Bitte anmelden.'
            : 'Der Artikel oder die gewünschte Menge ist derzeit nicht verfügbar.',
        );
        return;
      }
      if (error.status === 429) {
        this.error.set('Zu viele Anmeldeversuche. Bitte später erneut versuchen.');
        return;
      }
      if (error.status === 404) {
        this.error.set('Der Artikel ist nicht mehr vorhanden.');
        return;
      }
    }
    this.error.set('Die Anfrage konnte nicht verarbeitet werden. Bitte später erneut versuchen.');
  }
}
