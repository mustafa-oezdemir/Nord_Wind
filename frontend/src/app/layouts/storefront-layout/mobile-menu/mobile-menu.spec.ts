import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Component } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { categoryDto, flushCategories, textOf } from '../../../../testing/catalog-fixtures';
import { MobileMenu } from './mobile-menu';

@Component({ selector: 'app-menu-test-page', template: '' })
class EmptyPage {}

/** Wie Escape: Der Browser schließt den Dialog und meldet `close`. */
async function closeLikeBrowser(dialog: HTMLDialogElement): Promise<void> {
  if (typeof dialog.close === 'function') {
    const closed = new Promise((resolve) =>
      dialog.addEventListener('close', resolve, { once: true }),
    );
    dialog.close();
    await closed;
  } else {
    dialog.removeAttribute('open');
    dialog.dispatchEvent(new Event('close'));
  }
}

describe('MobileMenu', () => {
  let fixture: ComponentFixture<MobileMenu>;
  let menu: MobileMenu;
  let dialog: HTMLDialogElement;
  let trigger: HTMLButtonElement;

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [
        provideRouter([{ path: '**', component: EmptyPage }]),
        provideHttpClient(),
        provideHttpClientTesting(),
      ],
    });
    fixture = TestBed.createComponent(MobileMenu);
    menu = fixture.componentInstance;
    TestBed.tick();
    flushCategories(TestBed.inject(HttpTestingController), [
      categoryDto(),
      categoryDto({
        id: 'c2',
        parent_id: '0f1e2d3c-4b5a-4968-8776-655443322110',
        slug: 'kochgeschirr',
        name: 'Kochgeschirr',
      }),
    ]);
    await fixture.whenStable();
    dialog = (fixture.nativeElement as HTMLElement).querySelector('dialog') as HTMLDialogElement;
    trigger = document.createElement('button');
    document.body.appendChild(trigger);
    trigger.focus();
  });

  afterEach(() => trigger.remove());

  it('öffnet als benannter Dialog mit Haupt- und Unterkategorien', async () => {
    menu.open(trigger);
    await fixture.whenStable();

    expect(menu.isOpen()).toBe(true);
    expect(dialog.hasAttribute('open')).toBe(true);
    expect(textOf(document.getElementById(dialog.getAttribute('aria-labelledby') ?? ''))).toBe(
      'Menü',
    );
    const links = Array.from(dialog.querySelectorAll('a')).map((link) => link.getAttribute('href'));
    expect(links).toEqual([
      '/',
      '/produkte',
      '/warenkorb',
      '/kategorien/kueche-haushalt',
      '/kategorien/kochgeschirr',
    ]);
  });

  it('schließt über die Schaltfläche und gibt den Fokus an den Auslöser zurück', async () => {
    menu.open(trigger);
    await fixture.whenStable();
    (dialog.querySelector('button[aria-label="Menü schließen"]') as HTMLButtonElement).click();
    await fixture.whenStable();

    expect(menu.isOpen()).toBe(false);
    expect(dialog.hasAttribute('open')).toBe(false);
    expect(document.activeElement).toBe(trigger);
  });

  it('übernimmt das Schließen durch den Browser, z. B. per Escape', async () => {
    menu.open(trigger);
    await fixture.whenStable();
    await closeLikeBrowser(dialog);
    await fixture.whenStable();

    expect(menu.isOpen()).toBe(false);
    expect(document.activeElement).toBe(trigger);
  });

  it('schließt nach Auswahl einer Kategorie', async () => {
    menu.open(trigger);
    await fixture.whenStable();
    (dialog.querySelector('a[href="/kategorien/kochgeschirr"]') as HTMLAnchorElement).click();
    await fixture.whenStable();

    expect(menu.isOpen()).toBe(false);
  });
});
