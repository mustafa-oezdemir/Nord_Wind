import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Fortschritt Warenkorb → Kasse → Bestätigung; abgeschlossene Schritte sind verlinkt, wo sinnvoll. */
@Component({
  selector: 'app-checkout-steps',
  imports: [RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <nav aria-label="Bestellschritte">
      <ol class="steps">
        @for (step of steps; track step.label; let index = $index) {
          @let number = index + 1;
          <li
            class="step"
            [class.step-done]="number < current()"
            [class.step-current]="number === current()"
            [attr.aria-current]="number === current() ? 'step' : null"
          >
            <span class="step-mark" aria-hidden="true">
              @if (number < current()) {
                <svg viewBox="0 0 16 16" width="14" height="14">
                  <path d="M3 8.5 6.5 12 13 4.5" />
                </svg>
              } @else {
                {{ number }}
              }
            </span>
            @if (step.link && number < current() && current() < steps.length) {
              <a class="step-label" [routerLink]="step.link">{{ step.label }}</a>
            } @else {
              <span class="step-label">{{ step.label }}</span>
            }
            @if (number < current()) {
              <span class="visually-hidden">(erledigt)</span>
            }
          </li>
        }
      </ol>
    </nav>
  `,
  styleUrl: './checkout-steps.scss',
})
export class CheckoutSteps {
  /** Aktueller Schritt, 1 bis 3. */
  readonly current = input.required<number>();

  protected readonly steps = [
    { label: 'Warenkorb', link: '/warenkorb' },
    { label: 'Kasse', link: '' },
    { label: 'Bestätigung', link: '' },
  ];
}
