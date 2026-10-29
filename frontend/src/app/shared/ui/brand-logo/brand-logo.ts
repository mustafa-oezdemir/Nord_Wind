import { ChangeDetectionStrategy, Component, input } from '@angular/core';

/** Wortmarke mit den drei Windlinien; rein darstellend. */
@Component({
  selector: 'app-brand-logo',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'brand-logo', '[class.brand-logo--inverse]': 'inverse()' },
  template: `
    <svg class="brand-logo-mark" viewBox="0 0 48 48" aria-hidden="true" focusable="false">
      <path d="M7 16h23c8 0 8-9 2-9-3 0-5 2-5 4" />
      <path d="M5 24h32c8 0 8 9 2 9-3 0-5-2-5-4" />
      <path d="M9 32h15c8 0 8 9 2 9-3 0-5-2-5-4" />
    </svg>
    <span class="brand-logo-word">NordWinds</span>
  `,
  styleUrl: './brand-logo.scss',
})
export class BrandLogo {
  readonly inverse = input(false);
}
