// Transportformate gemäß docs/api/openapi.yaml (Katalog-API v1).

export interface PaginationDto {
  readonly page: number;
  readonly limit: number;
  readonly total: number;
  readonly total_pages: number;
}

export interface ProductDto {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly price_cents: number;
  readonly currency: 'EUR';
  readonly available: boolean;
  readonly variant_count: number;
  readonly image_url: string;
}

export interface CategoryDto {
  readonly id: string;
  readonly parent_id: string | null;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly sort_order: number;
}

export interface PageDto<T> {
  readonly data: readonly T[];
  readonly pagination: PaginationDto;
}

export interface CategoryRefDto {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
}

export interface VariantOfferDto {
  readonly id: string;
  readonly sku: string;
  readonly label: string;
  readonly price_cents: number;
  readonly currency: 'EUR';
  readonly stock_status: 'in_stock' | 'low_stock' | 'out_of_stock';
}

export interface ProductDetailDto {
  readonly id: string;
  readonly slug: string;
  readonly name: string;
  readonly description: string;
  readonly sale_status: 'available' | 'sold_out' | 'unavailable' | 'discontinued';
  readonly categories: readonly CategoryRefDto[];
  readonly variants: readonly VariantOfferDto[];
  readonly images: readonly ProductImageDto[];
}

export interface ProductImageDto {
  readonly id: string;
  readonly src: string;
  readonly alt: string;
  readonly width: number;
  readonly height: number;
}
