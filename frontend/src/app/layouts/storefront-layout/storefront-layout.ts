import { ChangeDetectionStrategy, Component, ElementRef, inject, viewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router, RouterOutlet } from '@angular/router';
import { filter, map, pairwise } from 'rxjs';
import { CategoryNav } from '../../features/catalog/ui/category-nav/category-nav';
import { MobileMenu } from './mobile-menu/mobile-menu';
import { SiteFooter } from './site-footer/site-footer';
import { SiteHeader } from './site-header/site-header';

@Component({
  selector: 'app-storefront-layout',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterOutlet, SiteHeader, CategoryNav, MobileMenu, SiteFooter],
  templateUrl: './storefront-layout.html',
  styleUrl: './storefront-layout.scss',
})
export class StorefrontLayout {
  private readonly main = viewChild.required<ElementRef<HTMLElement>>('main');

  constructor() {
    // Nach einem Seitenwechsel springt der Fokus an den Anfang des Inhalts, damit
    // Tastatur- und Screenreader-Nutzung nicht in der alten Position hängen bleibt.
    inject(Router)
      .events.pipe(
        filter((event) => event instanceof NavigationEnd),
        map((event) => pathOf(event.urlAfterRedirects)),
        pairwise(),
        filter(([previous, current]) => previous !== current),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.focusMain());
  }

  protected skipToContent(event: Event): void {
    event.preventDefault();
    this.focusMain();
  }

  private focusMain(): void {
    this.main().nativeElement.focus({ preventScroll: true });
  }
}

function pathOf(url: string): string {
  return url.split(/[?#]/, 1)[0];
}
