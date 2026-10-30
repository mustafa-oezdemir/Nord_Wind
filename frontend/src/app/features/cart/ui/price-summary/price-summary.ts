import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { euros, formatPercentBp } from '../../../../shared/money/money';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { CartTaxLine } from '../../data-access/cart-api';

/** Beträge einer Preisaufstellung in Cent; Warenkorb und Bestellung liefern dieselben Felder. */
export interface PriceBreakdown {
  goods_cents: number;
  discount_cents: number;
  shipping_cents: number;
  total_cents: number;
  taxes: CartTaxLine[];
}

export interface AppliedDiscount {
  code: string;
  percent_bp: number;
}

/**
 * Zeigt Warenwert, Rabatt, Versand, Gesamtsumme und enthaltene MwSt. genau so, wie die API sie
 * berechnet hat (ADR-007). Die Komponente rechnet nichts selbst.
 */
@Component({
  selector: 'app-price-summary',
  imports: [MoneyPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './price-summary.html',
  styleUrl: './price-summary.scss',
})
export class PriceSummary {
  readonly pricing = input.required<PriceBreakdown>();
  readonly discount = input<AppliedDiscount | null>(null);

  protected readonly money = euros;
  protected readonly percent = formatPercentBp;
}
