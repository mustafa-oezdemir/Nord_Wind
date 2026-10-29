import { Money } from '../../../shared/money/money';

/** Produkt in Listen: Preis ist der niedrigste Preis einer aktiven Variante. */
export interface ProductSummary {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly lowestPrice: Money;
  readonly available: boolean;
  readonly variantCount: number;
}

export interface Category {
  readonly id: string;
  readonly parentId: string | null;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly sortOrder: number;
}

export interface CategoryNode extends Category {
  readonly children: readonly CategoryNode[];
}

export interface Pagination {
  readonly page: number;
  readonly limit: number;
  readonly total: number;
  readonly totalPages: number;
}

export interface Page<T> {
  readonly items: readonly T[];
  readonly pagination: Pagination;
}

export type ProductSort = 'newest' | 'name_asc' | 'price_asc' | 'price_desc';

export interface ProductQuery {
  readonly category?: string;
  readonly search?: string;
  readonly sort: ProductSort;
  readonly page: number;
  readonly limit: number;
}
