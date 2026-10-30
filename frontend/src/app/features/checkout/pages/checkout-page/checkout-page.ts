import { HttpErrorResponse } from '@angular/common/http';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  OnInit,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import {
  AbstractControl,
  FormControl,
  FormGroup,
  ReactiveFormsModule,
  ValidationErrors,
  ValidatorFn,
} from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { euros } from '../../../../shared/money/money';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { CartApi, CartSnapshot, EMPTY_CART } from '../../../cart/data-access/cart-api';
import { PriceSummary } from '../../../cart/ui/price-summary/price-summary';
import { CheckoutSteps } from '../../ui/checkout-steps/checkout-steps';
import {
  OrderAddress,
  OrderFieldError,
  OrdersApi,
  newIdempotencyKey,
  readOrderProblem,
} from '../../data-access/orders-api';

type CheckoutState = 'loading' | 'signed-out' | 'not-customer' | 'empty' | 'error' | 'ready';
type AddressField = Exclude<keyof OrderAddress, 'country'>;
type FieldCode = OrderFieldError['code'];

interface Notice {
  kind: 'danger' | 'warning';
  text: string;
}

/** Prüft getrimmte Texte wie die API (ADR-008); Fehlercodes entsprechen der Problemantwort. */
function text(required: boolean, min: number, max: number): ValidatorFn {
  return (control: AbstractControl<string>): ValidationErrors | null => {
    const value = (control.value ?? '').trim();
    if (!value) return required ? { required: true } : null;
    if ([...value].length > max) return { too_long: true };
    if ([...value].length < min) return { too_short: true };
    return null;
  };
}

function pattern(re: RegExp): ValidatorFn {
  return (control: AbstractControl<string>): ValidationErrors | null => {
    const value = (control.value ?? '').trim();
    if (!value) return { required: true };
    return re.test(value) ? null : { invalid_format: true };
  };
}

const field = (validator: ValidatorFn) =>
  new FormControl('', { nonNullable: true, validators: [validator] });

const LABELS: Record<AddressField, string> = {
  first_name: 'Vorname',
  last_name: 'Nachname',
  company: 'Firma',
  street: 'Straße',
  house_number: 'Hausnummer',
  address_extra: 'Adresszusatz',
  postal_code: 'Postleitzahl',
  city: 'Ort',
};

const FIELD_ORDER = Object.keys(LABELS) as AddressField[];

interface FieldConfig {
  name: AddressField;
  autocomplete: string;
  optional?: boolean;
  size?: 'wide' | 'grow' | 'narrow';
  numeric?: boolean;
  hint?: string;
}

interface FieldGroup {
  legend: string;
  fields: FieldConfig[];
  country?: boolean;
}

const GROUPS: FieldGroup[] = [
  {
    legend: 'Name',
    fields: [
      { name: 'first_name', autocomplete: 'given-name' },
      { name: 'last_name', autocomplete: 'family-name' },
      { name: 'company', autocomplete: 'organization', optional: true, size: 'wide' },
    ],
  },
  {
    legend: 'Anschrift',
    country: true,
    fields: [
      { name: 'street', autocomplete: 'address-line1', size: 'grow' },
      { name: 'house_number', autocomplete: 'off', size: 'narrow' },
      {
        name: 'address_extra',
        autocomplete: 'address-line2',
        optional: true,
        size: 'wide',
        hint: 'z. B. Hinterhaus, 3. OG oder c/o',
      },
      { name: 'postal_code', autocomplete: 'postal-code', size: 'narrow', numeric: true },
      { name: 'city', autocomplete: 'address-level2', size: 'grow' },
    ],
  },
];

@Component({
  selector: 'app-checkout-page',
  imports: [ReactiveFormsModule, RouterLink, MoneyPipe, PriceSummary, CheckoutSteps],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './checkout-page.html',
  styleUrl: './checkout-page.scss',
})
export class CheckoutPage implements OnInit {
  private readonly cartApi = inject(CartApi);
  private readonly ordersApi = inject(OrdersApi);
  private readonly router = inject(Router);
  private readonly host = inject(ElementRef<HTMLElement>);
  private readonly noticeBox = viewChild<ElementRef<HTMLElement>>('noticeBox');

  protected readonly state = signal<CheckoutState>('loading');
  protected readonly cart = signal<CartSnapshot>(EMPTY_CART);
  protected readonly email = signal('');
  protected readonly busy = signal(false);
  protected readonly notice = signal<Notice | null>(null);
  protected readonly submitted = signal(false);
  protected readonly serverErrors = signal<Partial<Record<AddressField, FieldCode>>>({});
  protected readonly labels = LABELS;
  protected readonly groups = GROUPS;
  protected readonly money = euros;

  protected readonly allAvailable = computed(() =>
    this.cart().items.every((item) => item.available),
  );
  protected readonly appliedDiscount = computed(() => {
    const discount = this.cart().discount;
    return discount?.applied ? discount : null;
  });

  /** Ein Schlüssel je Besuch der Kasse; Wiederholungen nach Fehlern nutzen denselben. */
  private readonly idempotencyKey = newIdempotencyKey();

  protected readonly form = new FormGroup({
    first_name: field(text(true, 1, 100)),
    last_name: field(text(true, 1, 100)),
    company: field(text(false, 1, 100)),
    street: field(text(true, 2, 100)),
    house_number: field(pattern(/^[0-9]{1,4} ?[A-Za-z]?( ?[-/] ?[0-9]{1,4} ?[A-Za-z]?)?$/)),
    address_extra: field(text(false, 1, 100)),
    postal_code: field(pattern(/^(0[1-9]|[1-9][0-9])[0-9]{3}$/)),
    city: field(text(true, 2, 100)),
  });

  async ngOnInit(): Promise<void> {
    await this.load();
  }

  protected async load(): Promise<void> {
    this.state.set('loading');
    try {
      const session = await firstValueFrom(this.cartApi.session());
      this.email.set(session.customer.email);
      if (session.customer.role !== 'customer') {
        this.state.set('not-customer');
        return;
      }
      this.showCart(await firstValueFrom(this.cartApi.get()));
    } catch (error) {
      this.state.set(
        error instanceof HttpErrorResponse && error.status === 401 ? 'signed-out' : 'error',
      );
    }
  }

  /** Fehlertext eines Feldes, sobald er angezeigt werden soll; Serverfehler gehen vor. */
  protected fieldError(name: AddressField): string {
    const control = this.form.controls[name];
    const code =
      this.serverErrors()[name] ??
      (control.invalid && (control.touched || this.submitted())
        ? (Object.keys(control.errors ?? {})[0] as FieldCode)
        : undefined);
    return code ? message(name, code) : '';
  }

  protected clearServerError(name: AddressField): void {
    if (this.serverErrors()[name]) {
      this.serverErrors.update((errors) => {
        const rest = { ...errors };
        delete rest[name];
        return rest;
      });
    }
  }

  protected async submit(): Promise<void> {
    if (this.busy()) return;
    this.submitted.set(true);
    this.notice.set(null);
    if (this.form.invalid || Object.keys(this.serverErrors()).length > 0) {
      this.focusFirstInvalid();
      return;
    }
    if (!this.allAvailable()) {
      this.showNotice(
        'warning',
        'Ein Artikel ist nicht mehr lieferbar. Bitte passen Sie den Warenkorb an.',
      );
      return;
    }
    this.busy.set(true);
    try {
      const order = await firstValueFrom(
        this.ordersApi.create(this.address(), this.cart().pricing.total_cents, this.idempotencyKey),
      );
      await this.router.navigate(['/bestellung', order.order_number], { replaceUrl: true });
    } catch (error) {
      this.handleError(error);
    } finally {
      this.busy.set(false);
    }
  }

  private address(): OrderAddress {
    const value = this.form.getRawValue();
    const trimmed = Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, entry.trim()]),
    ) as Omit<OrderAddress, 'country'>;
    return { ...trimmed, country: 'DE' };
  }

  private showCart(cart: CartSnapshot): void {
    this.cart.set(cart);
    this.state.set(cart.items.length === 0 ? 'empty' : 'ready');
  }

  private handleError(error: unknown): void {
    const problem = readOrderProblem(error);
    switch (problem?.code) {
      case 'CART_CHANGED':
        if (problem.cart) this.showCart(problem.cart);
        this.showNotice(
          'warning',
          problem.reason === 'item_unavailable'
            ? 'Ein Artikel ist nicht mehr lieferbar. Bitte passen Sie den Warenkorb an.'
            : 'Preis, Bestand oder Rabatt haben sich geändert. Bitte prüfen Sie die neue Gesamtsumme und bestellen Sie erneut.',
        );
        return;
      case 'CART_EMPTY':
        this.showCart(EMPTY_CART);
        return;
      case 'VALIDATION_FAILED': {
        const errors: Partial<Record<AddressField, FieldCode>> = {};
        for (const entry of problem.errors ?? []) {
          if (entry.field !== 'country') errors[entry.field] ??= entry.code;
        }
        this.serverErrors.set(errors);
        this.focusFirstInvalid();
        return;
      }
      case 'IDEMPOTENCY_KEY_REUSED':
        this.showNotice(
          'danger',
          'Diese Bestellung wurde bereits mit anderen Angaben übermittelt. Laden Sie die Seite neu und prüfen Sie Ihren Warenkorb.',
        );
        return;
    }
    if (error instanceof HttpErrorResponse && error.status === 401) {
      this.state.set('signed-out');
      return;
    }
    if (error instanceof HttpErrorResponse && error.status === 403) {
      this.showNotice('danger', 'Die Sitzung ist abgelaufen. Bitte laden Sie die Seite neu.');
      return;
    }
    // Netzabbruch oder Zeitüberschreitung: Die Bestellung kann angelegt sein. Derselbe
    // Schlüssel sorgt dafür, dass ein neuer Versuch keine zweite Bestellung anlegt.
    this.showNotice(
      'danger',
      'Die Bestellung konnte nicht bestätigt werden. Sie wurde eventuell schon angelegt – ein erneuter Klick auf „Zahlungspflichtig bestellen“ legt keine zweite Bestellung an.',
    );
  }

  private showNotice(kind: Notice['kind'], text: string): void {
    this.notice.set({ kind, text });
    setTimeout(() => this.noticeBox()?.nativeElement.focus());
  }

  private focusFirstInvalid(): void {
    const name = FIELD_ORDER.find(
      (key) => this.form.controls[key].invalid || this.serverErrors()[key],
    );
    if (!name) return;
    setTimeout(() =>
      (this.host.nativeElement as HTMLElement)
        .querySelector<HTMLInputElement>(`#checkout-${name}`)
        ?.focus(),
    );
  }
}

function message(name: AddressField, code: FieldCode): string {
  const label = LABELS[name];
  switch (code) {
    case 'required':
      return `Bitte ${label} angeben.`;
    case 'too_short':
      return `${label} ist zu kurz.`;
    case 'too_long':
      return `${label} darf höchstens 100 Zeichen lang sein.`;
    case 'invalid_format':
      if (name === 'postal_code') return 'Bitte eine fünfstellige deutsche Postleitzahl angeben.';
      if (name === 'house_number') return 'Bitte eine Hausnummer wie 17, 17a oder 5-7 angeben.';
      return `${label} enthält ungültige Zeichen.`;
    case 'unsupported_country':
      return 'Wir liefern derzeit nur innerhalb Deutschlands.';
  }
}
