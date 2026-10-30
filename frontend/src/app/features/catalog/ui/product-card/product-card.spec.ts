import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { productSummary, textOf } from '../../../../../testing/catalog-fixtures';
import { ProductCard } from './product-card';

describe('ProductCard', () => {
  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideRouter([])] });
  });

  async function render(product = productSummary()) {
    const fixture = TestBed.createComponent(ProductCard);
    fixture.componentRef.setInput('product', product);
    await fixture.whenStable();
    return fixture.nativeElement as HTMLElement;
  }

  it('verlinkt das Produkt und zeigt den Ab-Preis bei mehreren Varianten', async () => {
    const element = await render();

    const link = element.querySelector('a');
    expect(link?.getAttribute('href')).toBe('/produkte/gusseisenpfanne-28');
    expect(textOf(link)).toBe('Gusseisenpfanne 28 cm');
    expect(textOf(element.querySelector('.product-card-price'))).toBe('ab 59,90 €');
    expect(textOf(element.querySelector('.product-card-stock'))).toBe('Auf Lager');
  });

  it('zeigt bei einer Variante den Preis ohne „ab“ und kennzeichnet fehlenden Bestand', async () => {
    const element = await render(
      productSummary({
        variantCount: 1,
        available: false,
        lowestPrice: { amountCents: 129_999, currency: 'EUR' },
      }),
    );

    expect(textOf(element.querySelector('.product-card-price'))).toBe('1.299,99 €');
    expect(textOf(element.querySelector('.product-card-stock'))).toBe('Derzeit nicht verfügbar');
  });

  it('lässt eine leere Beschreibung weg', async () => {
    const element = await render(productSummary({ description: '' }));
    expect(element.querySelector('.product-card-description')).toBeNull();
  });

  it('zeigt bei einer gelöschten Bilddatei wieder das Platzhalter-Motiv', async () => {
    const fixture = TestBed.createComponent(ProductCard);
    fixture.componentRef.setInput(
      'product',
      productSummary({
        imageUrl: '/api/v1/media/products/7d8c2c1e-0b7a-4c55-9a31-3b2f1c4d5e61',
      }),
    );
    await fixture.whenStable();
    const element = fixture.nativeElement as HTMLElement;
    const photo = element.querySelector('.product-card-photo') as HTMLImageElement;
    expect(photo).not.toBeNull();
    photo.dispatchEvent(new Event('error'));
    await fixture.whenStable();
    expect(element.querySelector('app-product-visual')).not.toBeNull();
  });
});
