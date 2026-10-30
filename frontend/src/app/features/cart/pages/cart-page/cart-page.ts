import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { formatPercentBp } from '../../../../shared/money/money';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import {
  CartApi,
  CartItem,
  CartSnapshot,
  CustomerSession,
  EMPTY_CART,
} from '../../data-access/cart-api';

@Component({
  selector: 'app-cart-page',
  imports: [FormsModule, RouterLink, MoneyPipe],
  templateUrl: './cart-page.html',
  styleUrl: './cart-page.scss',
})
export class CartPage implements OnInit {
  private readonly api = inject(CartApi);

  protected readonly session = signal<CustomerSession | null>(null);
  protected readonly cart = signal<CartSnapshot>(EMPTY_CART);
  protected readonly percent = formatPercentBp;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly notice = signal('');
  protected readonly quantities = signal<Record<string, number>>({});
  protected discountCode = '';
  protected email = '';
  protected password = '';
  protected confirmPassword = '';
  protected revealPassword = false;
  protected generatedPassword = false;
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
    if (this.registerMode && this.password !== this.confirmPassword) {
      this.error.set('Die Passwörter stimmen nicht überein.');
      return;
    }
    await this.run(async () => {
      if (this.registerMode) await firstValueFrom(this.api.register(this.email, this.password));
      const session = await firstValueFrom(this.api.login(this.email, this.password));
      this.password = '';
      this.confirmPassword = '';
      this.revealPassword = false;
      this.generatedPassword = false;
      this.session.set(session);
      if (session.customer.role === 'customer') await this.load();
    });
  }

  protected generatePassword(): void {
    try {
      this.password = generateStrongPassword();
      this.confirmPassword = this.password;
      this.generatedPassword = true;
      this.error.set('');
    } catch {
      this.error.set('Ein sicheres Passwort konnte nicht erzeugt werden. Bitte selbst eingeben.');
    }
  }

  protected toggleMode(): void {
    this.registerMode = !this.registerMode;
    this.password = '';
    this.confirmPassword = '';
    this.revealPassword = false;
    this.generatedPassword = false;
    this.error.set('');
  }

  protected async logout(): Promise<void> {
    await this.run(async () => {
      await firstValueFrom(this.api.logout());
      this.session.set(null);
      this.setCart(EMPTY_CART);
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

  protected async applyDiscount(): Promise<void> {
    const code = this.discountCode.trim();
    if (!code) {
      return;
    }
    await this.run(async () => {
      try {
        this.setCart(await firstValueFrom(this.api.applyDiscount(code)));
        this.discountCode = '';
        this.notice.set('Rabattcode eingelöst.');
      } catch (error) {
        throw this.discountError(error);
      }
    });
  }

  protected async removeDiscount(): Promise<void> {
    await this.run(async () => {
      this.setCart(await firstValueFrom(this.api.removeDiscount()));
      this.notice.set('Rabattcode entfernt.');
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

  /** Rabattcode-Fehler bekommen eigene Meldungen; andere Fehler laufen normal weiter. */
  private discountError(error: unknown): unknown {
    if (error instanceof HttpErrorResponse && error.status === 404) {
      return new DiscountMessage('Der Rabattcode ist ungültig oder abgelaufen.');
    }
    if (error instanceof HttpErrorResponse && error.status === 429) {
      return new DiscountMessage(
        'Zu viele ungültige Rabattcodes. Versuchen Sie es in 15 Minuten erneut.',
      );
    }
    return error;
  }

  private showError(error: unknown): void {
    if (error instanceof DiscountMessage) {
      this.error.set(error.message);
      return;
    }
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

const passwordAlphabet = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_';

/** 24 kryptografisch zufällige Zeichen aus einem 64-Zeichen-Alphabet (144 Bit). */
export function generateStrongPassword(): string {
  const bytes = new Uint8Array(24);
  globalThis.crypto.getRandomValues(bytes);
  return Array.from(bytes, (value) => passwordAlphabet[value & 63]).join('');
}

/** Fachliche Meldung für Rabattcodes; wird nicht als Artikel-Fehler behandelt. */
class DiscountMessage extends Error {}
