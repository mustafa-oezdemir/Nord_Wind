import { ChangeDetectionStrategy, Component, inject, input, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs';
import { MAX_SEARCH_LENGTH, normalizeSearch } from '../../models/catalog-query';

/** Produktsuche; übernimmt den Begriff aus der URL, damit Suchfeld und Ergebnis übereinstimmen. */
@Component({
  selector: 'app-search-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './search-form.html',
  styleUrl: './search-form.scss',
})
export class SearchForm {
  private readonly router = inject(Router);

  /** Eindeutige ID, falls das Formular mehrfach auf einer Seite steht. */
  readonly inputId = input('produktsuche');

  protected readonly maxLength = MAX_SEARCH_LENGTH;
  protected readonly term = signal('');

  constructor() {
    this.syncFromUrl();
    this.router.events
      .pipe(
        filter((event) => event instanceof NavigationEnd),
        takeUntilDestroyed(),
      )
      .subscribe(() => this.syncFromUrl());
  }

  protected onInput(event: Event): void {
    this.term.set((event.target as HTMLInputElement).value);
  }

  protected submit(event: Event): void {
    event.preventDefault();
    const q = normalizeSearch(this.term());
    void this.router.navigate(['/produkte'], { queryParams: q ? { q } : {} });
  }

  private syncFromUrl(): void {
    const tree = this.router.parseUrl(this.router.url);
    this.term.set(tree.queryParamMap.get('q') ?? '');
  }
}
