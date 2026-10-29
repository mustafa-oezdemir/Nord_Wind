import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';

/** Fehlerzustand mit verständlicher Meldung und optionalem neuem Versuch. */
@Component({
  selector: 'app-error-state',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="state-box state-box--error" role="alert">
      <h2 class="state-title">{{ heading() }}</h2>
      <p class="state-message">{{ message() }}</p>
      <div class="state-actions">
        @if (retryable()) {
          <button type="button" class="btn btn-primary" (click)="retry.emit()">
            Erneut versuchen
          </button>
        }
        <ng-content />
      </div>
    </div>
  `,
  styleUrl: '../state-box.scss',
})
export class ErrorState {
  readonly heading = input.required<string>();
  readonly message = input.required<string>();
  readonly retryable = input(true);
  readonly retry = output<void>();
}
