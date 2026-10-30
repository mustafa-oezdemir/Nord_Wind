import { ChangeDetectionStrategy, Component, input, model } from '@angular/core';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { STOCK_LABELS } from '../../models/catalog-labels';
import { VariantOffer } from '../../models/catalog.models';

let nextGroupId = 0;

/**
 * Variantenauswahl als native Radiogruppe: Pfeiltasten wechseln die Auswahl, der
 * Screenreader liest Bezeichnung, Preis und Bestand vor. Ausverkaufte Varianten bleiben
 * wählbar, damit Preis und Status sichtbar sind.
 */
@Component({
  selector: 'app-variant-picker',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MoneyPipe],
  templateUrl: './variant-picker.html',
  styleUrl: './variant-picker.scss',
})
export class VariantPicker {
  readonly variants = input.required<readonly VariantOffer[]>();
  readonly selectedSku = model<string>();

  protected readonly groupName = `variante-${++nextGroupId}`;
  protected readonly stockLabels = STOCK_LABELS;
}
