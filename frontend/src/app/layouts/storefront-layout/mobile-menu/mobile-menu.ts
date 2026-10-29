import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationStart, Router, RouterLink } from '@angular/router';
import { filter } from 'rxjs';
import { CategoryStore } from '../../../features/catalog/state/category-store';

/**
 * Mobiles Menü als natives modales `<dialog>` im Offcanvas-Stil. Der Browser übernimmt
 * Fokusfalle, Escape und das Sperren des Hintergrunds; `closedby="any"` schließt in
 * unterstützenden Browsern zusätzlich per Klick auf den Hintergrund. Ohne Bootstrap-JavaScript.
 */
@Component({
  selector: 'app-mobile-menu',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  templateUrl: './mobile-menu.html',
  styleUrl: './mobile-menu.scss',
})
export class MobileMenu {
  protected readonly store = inject(CategoryStore);
  private readonly dialog = viewChild.required<ElementRef<HTMLDialogElement>>('dialog');
  private returnFocusTo: HTMLElement | null = null;

  readonly isOpen = signal(false);

  constructor() {
    // Beim Seitenwechsel schließen, auch wenn der Wechsel nicht aus dem Menü kam.
    inject(Router)
      .events.pipe(
        filter((event) => event instanceof NavigationStart),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.close());
  }

  open(trigger?: HTMLElement): void {
    if (this.isOpen()) {
      return;
    }
    this.returnFocusTo = trigger ?? null;
    const element = this.dialog().nativeElement;
    if (typeof element.showModal === 'function') {
      element.showModal();
    } else {
      element.setAttribute('open', '');
    }
    this.isOpen.set(true);
  }

  close(): void {
    if (!this.isOpen()) {
      return;
    }
    const element = this.dialog().nativeElement;
    if (typeof element.close === 'function') {
      element.close();
    } else {
      element.removeAttribute('open');
    }
    this.handleClosed();
  }

  /** `close`-Ereignis des Dialogs, z. B. nach Escape. */
  protected handleClosed(): void {
    if (!this.isOpen()) {
      return;
    }
    this.isOpen.set(false);
    this.returnFocusTo?.focus();
    this.returnFocusTo = null;
  }
}
