import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { RouterLink } from '@angular/router';
import { SearchForm } from '../../../features/catalog/ui/search-form/search-form';
import { BrandLogo } from '../../../shared/ui/brand-logo/brand-logo';

@Component({
  selector: 'app-site-header',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, BrandLogo, SearchForm],
  templateUrl: './site-header.html',
  styleUrl: './site-header.scss',
})
export class SiteHeader {
  /** Zustand des mobilen Menüs für `aria-expanded`. */
  readonly menuOpen = input(false);
  readonly menuRequested = output<HTMLElement>();
}
