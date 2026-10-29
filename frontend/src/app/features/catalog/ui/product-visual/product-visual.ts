import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

interface WindLine {
  readonly path: string;
}

const PALETTES = ['mist', 'ink', 'fjord', 'amber'] as const;

/**
 * Platzhalter-Motiv, solange Produkte keine Bilder haben (Bilder folgen mit NW-13).
 * Farbe und Lage der Windlinien ergeben sich stabil aus dem Slug; das Motiv ist
 * dekorativ und wird von Screenreadern übersprungen.
 */
@Component({
  selector: 'app-product-visual',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { '[attr.data-palette]': 'palette()' },
  template: `
    <svg
      viewBox="0 0 320 240"
      preserveAspectRatio="xMidYMid slice"
      aria-hidden="true"
      focusable="false"
    >
      <rect class="visual-bg" width="320" height="240" />
      @for (line of lines(); track $index) {
        <path class="visual-line" [attr.d]="line.path" />
      }
    </svg>
  `,
  styleUrl: './product-visual.scss',
})
export class ProductVisual {
  /** Stabiler Schlüssel, in der Regel der Produkt-Slug. */
  readonly seed = input.required<string>();

  private readonly hash = computed(() => fnv1a(this.seed()));
  protected readonly palette = computed(() => PALETTES[this.hash() % PALETTES.length]);
  protected readonly lines = computed<readonly WindLine[]>(() => {
    const h = this.hash();
    const shift = (h >>> 3) % 36;
    return [
      { path: windPath(-12, 84 + shift, 150 + ((h >>> 7) % 70), false) },
      { path: windPath(-12, 128 + shift, 205 + ((h >>> 11) % 50), true) },
      { path: windPath(-12, 172 + shift, 105 + ((h >>> 15) % 80), true) },
    ];
  });
}

/** Linie mit Endschnörkel wie im Markenzeichen; `down` dreht den Schnörkel nach unten. */
function windPath(x: number, y: number, length: number, down: boolean): string {
  const k = 4.2;
  const s = down ? 1 : -1;
  const r = (value: number) => Math.round(value * 10) / 10;
  return (
    `M${x} ${y}h${length}` +
    `c${r(8 * k)} 0 ${r(8 * k)} ${r(9 * k * s)} ${r(2 * k)} ${r(9 * k * s)}` +
    `c${r(-3 * k)} 0 ${r(-5 * k)} ${r(-2 * k * s)} ${r(-5 * k)} ${r(-4 * k * s)}`
  );
}

function fnv1a(value: string): number {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index++) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return hash >>> 0;
}
