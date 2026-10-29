/** Geldbetrag in ganzen Cent; die API liefert Preise nur in Euro. */
export interface Money {
  readonly amountCents: number;
  readonly currency: 'EUR';
}

const euroFormat = new Intl.NumberFormat('de-DE', { style: 'currency', currency: 'EUR' });

export function euros(amountCents: number): Money {
  return { amountCents, currency: 'EUR' };
}

/** Formatiert Cent-Beträge nach deutscher Schreibweise, z. B. `1.299,99 €`. */
export function formatMoney(money: Money): string {
  // Division erst bei der Anzeige; gerechnet wird ausschließlich in Cent.
  return euroFormat.format(money.amountCents / 100);
}
