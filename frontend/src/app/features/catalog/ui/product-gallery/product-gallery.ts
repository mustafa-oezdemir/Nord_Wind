import {
  ChangeDetectionStrategy,
  Component,
  computed,
  input,
  linkedSignal,
  signal,
} from '@angular/core';
import { ProductImage } from '../../models/catalog.models';
import { ProductVisual } from '../product-visual/product-visual';

/** Bildergalerie mit Vorschaubildern; ohne Bilder erscheint das Platzhalter-Motiv. */
@Component({
  selector: 'app-product-gallery',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ProductVisual],
  templateUrl: './product-gallery.html',
  styleUrl: './product-gallery.scss',
})
export class ProductGallery {
  readonly images = input<readonly ProductImage[]>([]);
  /** Stabiler Schlüssel für das Platzhalter-Motiv, in der Regel der Slug. */
  readonly seed = input.required<string>();

  /** Bei neuen Bildern (anderes Produkt) beginnt die Galerie wieder beim ersten Bild. */
  protected readonly activeIndex = linkedSignal({ source: this.images, computation: () => 0 });
  private readonly failedSources = signal<ReadonlySet<string>>(new Set());
  protected readonly visibleImages = computed(() =>
    this.images().filter((item) => !this.failedSources().has(item.src)),
  );
  protected readonly activeImage = computed(
    () => this.visibleImages()[this.activeIndex()] ?? this.visibleImages()[0],
  );

  protected select(index: number): void {
    this.activeIndex.set(index);
  }

  protected imageFailed(src: string): void {
    this.failedSources.update((previous) => new Set([...previous, src]));
    this.activeIndex.set(0);
  }
}
