import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Blättern über Query-Parameter `seite`; Links bleiben teil- und lesezeichenfähig. */
@Component({
  selector: 'app-pagination',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    @if (totalPages() > 1) {
      <nav class="pagination-nav" aria-label="Seiten">
        @if (page() > 1) {
          <a
            class="btn btn-outline-primary"
            [routerLink]="[]"
            [queryParams]="{ seite: previousPage() }"
            queryParamsHandling="merge"
            rel="prev"
            >Vorherige Seite</a
          >
        }
        <p class="pagination-status">Seite {{ page() }} von {{ totalPages() }}</p>
        @if (page() < totalPages()) {
          <a
            class="btn btn-outline-primary"
            [routerLink]="[]"
            [queryParams]="{ seite: page() + 1 }"
            queryParamsHandling="merge"
            rel="next"
            >Nächste Seite</a
          >
        }
      </nav>
    }
  `,
  styleUrl: './pagination.scss',
})
export class Pagination {
  readonly page = input.required<number>();
  readonly totalPages = input.required<number>();

  /** Seite 1 erscheint ohne Parameter in der URL. */
  protected readonly previousPage = computed(() => (this.page() > 2 ? this.page() - 1 : null));
}
