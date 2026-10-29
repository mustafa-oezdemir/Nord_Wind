import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/** Leerer Zustand mit Erklärung; Aktionen werden über Content Projection ergänzt. */
@Component({
  selector: 'app-empty-state',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="state-box" role="status">
      <h2 class="state-title">{{ heading() }}</h2>
      @if (message()) {
        <p class="state-message">{{ message() }}</p>
      }
      <div class="state-actions"><ng-content /></div>
    </div>
  `,
  styleUrl: '../state-box.scss',
})
export class EmptyState {
  readonly heading = input.required<string>();
  readonly message = input<string>();
}
