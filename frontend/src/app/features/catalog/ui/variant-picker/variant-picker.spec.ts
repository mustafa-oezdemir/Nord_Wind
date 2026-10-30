import { TestBed } from '@angular/core/testing';
import { euros } from '../../../../shared/money/money';
import { textOf } from '../../../../../testing/catalog-fixtures';
import { VariantOffer } from '../../models/catalog.models';
import { VariantPicker } from './variant-picker';

const variants: VariantOffer[] = [
  { id: 'v1', sku: 'NW-24', label: '24 cm', price: euros(4_990), stockStatus: 'out_of_stock' },
  { id: 'v2', sku: 'NW-28', label: '28 cm', price: euros(5_990), stockStatus: 'low_stock' },
];

describe('VariantPicker', () => {
  async function render(selectedSku: string) {
    const fixture = TestBed.createComponent(VariantPicker);
    fixture.componentRef.setInput('variants', variants);
    fixture.componentRef.setInput('selectedSku', selectedSku);
    await fixture.whenStable();
    return { fixture, element: fixture.nativeElement as HTMLElement };
  }

  it('zeigt eine benannte Radiogruppe mit Preis und Bestand je Variante', async () => {
    const { element } = await render('NW-28');

    expect(textOf(element.querySelector('legend'))).toBe('Variante wählen');
    const radios = Array.from(element.querySelectorAll<HTMLInputElement>('input[type="radio"]'));
    expect(radios.map((radio) => radio.value)).toEqual(['NW-24', 'NW-28']);
    expect(radios.map((radio) => radio.checked)).toEqual([false, true]);
    expect(new Set(radios.map((radio) => radio.name)).size).toBe(1);
    const labels = Array.from(element.querySelectorAll('label')).map((label) => textOf(label));
    expect(labels).toEqual([
      '24 cm, 49,90 €, Ausverkauft',
      '28 cm, 59,90 €, Nur noch wenige Stück',
    ]);
  });

  it('meldet die gewählte Variante', async () => {
    const { fixture, element } = await render('NW-28');
    const chosen: (string | undefined)[] = [];
    fixture.componentInstance.selectedSku.subscribe((sku) => chosen.push(sku));

    const first = element.querySelector<HTMLInputElement>(
      'input[value="NW-24"]',
    ) as HTMLInputElement;
    first.click();
    await fixture.whenStable();

    expect(chosen).toEqual(['NW-24']);
    expect(first.checked).toBe(true);
  });
});
