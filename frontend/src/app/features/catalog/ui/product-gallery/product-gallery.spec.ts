import { TestBed } from '@angular/core/testing';
import { textOf } from '../../../../../testing/catalog-fixtures';
import { ProductImage } from '../../models/catalog.models';
import { ProductGallery } from './product-gallery';

const images: ProductImage[] = [
  { src: '/bilder/pfanne-vorne.jpg', alt: 'Pfanne von vorne', width: 800, height: 600 },
  { src: '/bilder/pfanne-seite.jpg', alt: 'Pfanne von der Seite', width: 800, height: 600 },
];

describe('ProductGallery', () => {
  async function render(list: ProductImage[]) {
    const fixture = TestBed.createComponent(ProductGallery);
    fixture.componentRef.setInput('seed', 'gusseisenpfanne');
    fixture.componentRef.setInput('images', list);
    await fixture.whenStable();
    return { fixture, element: fixture.nativeElement as HTMLElement };
  }

  it('zeigt ohne Bilder das Platzhalter-Motiv und einen Hinweis', async () => {
    const { element } = await render([]);

    expect(element.querySelector('app-product-visual')).not.toBeNull();
    expect(element.querySelector('img')).toBeNull();
    expect(textOf(element.querySelector('figcaption'))).toBe('Produktfotos folgen.');
  });

  it('wechselt das Hauptbild über benannte Vorschauschaltflächen', async () => {
    const { fixture, element } = await render(images);
    const main = () => element.querySelector('.gallery-image') as HTMLImageElement;
    const buttons = Array.from(element.querySelectorAll<HTMLButtonElement>('.gallery-thumb'));

    expect(main().getAttribute('alt')).toBe('Pfanne von vorne');
    expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual([
      'Bild 1 von 2 anzeigen',
      'Bild 2 von 2 anzeigen',
    ]);
    expect(buttons.map((button) => button.getAttribute('aria-pressed'))).toEqual(['true', 'false']);

    buttons[1].click();
    await fixture.whenStable();

    expect(main().getAttribute('src')).toBe('/bilder/pfanne-seite.jpg');
    expect(buttons[1].getAttribute('aria-pressed')).toBe('true');
  });

  it('zeigt nach einem gelöschten Bild das Platzhalter-Motiv, ohne abzustürzen', async () => {
    const { fixture, element } = await render([images[0]]);
    const main = element.querySelector('.gallery-image') as HTMLImageElement;
    main.dispatchEvent(new Event('error'));
    await fixture.whenStable();
    expect(element.querySelector('app-product-visual')).not.toBeNull();
    expect(element.querySelector('.gallery-image')).toBeNull();
  });
});
