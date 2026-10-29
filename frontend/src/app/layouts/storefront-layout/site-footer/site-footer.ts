import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { RouterLink } from '@angular/router';
import { CategoryStore } from '../../../features/catalog/state/category-store';
import { BrandLogo } from '../../../shared/ui/brand-logo/brand-logo';

const FOOTER_CATEGORY_LIMIT = 5;

@Component({
  selector: 'app-site-footer',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, BrandLogo],
  templateUrl: './site-footer.html',
  styleUrl: './site-footer.scss',
})
export class SiteFooter {
  private readonly store = inject(CategoryStore);

  protected readonly year = new Date().getFullYear();
  protected readonly categories = computed(() => this.store.tree().slice(0, FOOTER_CATEGORY_LIMIT));
}
