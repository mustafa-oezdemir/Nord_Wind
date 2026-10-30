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
  readonly imageUrl: string;
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

/** Bestand ohne exakte Menge, wie ihn die API veröffentlicht. */
export type StockStatus = 'in_stock' | 'low_stock' | 'out_of_stock';

/** Ob ein Produkt derzeit verkauft wird; die API entscheidet anhand von Status und Varianten. */
export type SaleStatus = 'available' | 'sold_out' | 'unavailable' | 'discontinued';

export interface CategoryRef {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
}

export interface VariantOffer {
  readonly id: string;
  readonly sku: string;
  readonly label: string;
  readonly price: Money;
  readonly stockStatus: StockStatus;
}

/** Serverseitig bereinigtes Produktbild für die Galerie. */
export interface ProductImage {
  readonly src: string;
  readonly alt: string;
  readonly width: number;
  readonly height: number;
}

export interface ProductDetail {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly saleStatus: SaleStatus;
  /** Alle Variantenpreise sind Bruttopreise mit diesem Umsatzsteuersatz (ADR-007). */
  readonly taxRateBp: number;
  readonly categories: readonly CategoryRef[];
  readonly variants: readonly VariantOffer[];
  readonly images: readonly ProductImage[];
}
