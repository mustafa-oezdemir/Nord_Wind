import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import { ProductSummary } from '../../models/catalog.models';
import { ProductCard } from '../product-card/product-card';

/** Produktraster mit Bootstrap-Grid; zeigt beim Laden Platzhalter in gleicher Größe. */
@Component({
  selector: 'app-product-grid',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [ProductCard],
  templateUrl: './product-grid.html',
  styleUrl: './product-grid.scss',
})
export class ProductGrid {
  readonly products = input<readonly ProductSummary[]>([]);
  readonly loading = input(false);
  readonly skeletonCount = input(8);

  protected readonly skeletons = computed(() =>
    Array.from({ length: this.skeletonCount() }, (_, index) => index),
  );
}
