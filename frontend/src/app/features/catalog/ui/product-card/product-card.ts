import { ChangeDetectionStrategy, Component, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { MoneyPipe } from '../../../../shared/money/money.pipe';
import { ProductSummary } from '../../models/catalog.models';
import { ProductVisual } from '../product-visual/product-visual';

@Component({
  selector: 'app-product-card',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, MoneyPipe, ProductVisual],
  templateUrl: './product-card.html',
  styleUrl: './product-card.scss',
})
export class ProductCard {
  readonly product = input.required<ProductSummary>();
  protected readonly imageFailed = signal(false);
}
