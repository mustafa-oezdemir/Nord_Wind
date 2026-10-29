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
