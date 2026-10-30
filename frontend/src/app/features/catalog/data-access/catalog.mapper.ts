import { InvalidApiResponseError } from '../../../core/http/api-error';
import { euros } from '../../../shared/money/money';
import {
  Category,
  CategoryRef,
  Page,
  Pagination,
  ProductDetail,
  ProductImage,
  ProductSummary,
  SaleStatus,
  StockStatus,
  VariantOffer,
} from '../models/catalog.models';

// Die API ist eine Vertrauensgrenze: Antworten werden geprüft, bevor die Oberfläche
// sie anzeigt. Abweichungen führen zu einem Fehlerzustand statt zu falschen Preisen.

type JsonRecord = Readonly<Record<string, unknown>>;

export function parseProductPage(body: unknown): Page<ProductSummary> {
  return parsePage(body, parseProduct);
}

export function parseCategoryPage(body: unknown): Page<Category> {
  return parsePage(body, parseCategory);
}

const SALE_STATUSES: readonly SaleStatus[] = [
  'available',
  'sold_out',
  'unavailable',
  'discontinued',
];
const STOCK_STATUSES: readonly StockStatus[] = ['in_stock', 'low_stock', 'out_of_stock'];

export function parseProductDetail(body: unknown): ProductDetail {
  const record = asRecord(body, 'product');
  return {
    id: text(record, 'id'),
    slug: text(record, 'slug'),
    name: text(record, 'name'),
    description: text(record, 'description', true),
    saleStatus: oneOf(record, 'sale_status', SALE_STATUSES),
    categories: list(record, 'categories').map(parseCategoryRef),
    variants: list(record, 'variants').map(parseVariant),
    images: list(record, 'images').map(parseImage),
  };
}

function parseImage(value: unknown): ProductImage {
  const record = asRecord(value, 'images');
  const id = text(record, 'id');
  const src = text(record, 'src');
  if (
    !/^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(id) ||
    src !== `/api/v1/media/products/${id}`
  ) {
    throw new InvalidApiResponseError('images.src');
  }
  return {
    src,
    alt: text(record, 'alt'),
    width: integer(record, 'width', 1),
    height: integer(record, 'height', 1),
  };
}

function mediaPath(value: string): string {
  if (
    value !== '' &&
    !/^\/api\/v1\/media\/products\/[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(value)
  ) {
    throw new InvalidApiResponseError('image_url');
  }
  return value;
}

function parseCategoryRef(value: unknown): CategoryRef {
  const record = asRecord(value, 'categories');
  return { id: text(record, 'id'), slug: text(record, 'slug'), name: text(record, 'name') };
}

function parseVariant(value: unknown): VariantOffer {
  const record = asRecord(value, 'variants');
  if (record['currency'] !== 'EUR') {
    throw new InvalidApiResponseError('currency');
  }
  return {
    id: text(record, 'id'),
    sku: text(record, 'sku'),
    label: text(record, 'label'),
    price: euros(integer(record, 'price_cents', 0)),
    stockStatus: oneOf(record, 'stock_status', STOCK_STATUSES),
  };
}

function parsePage<T>(body: unknown, parseItem: (item: unknown) => T): Page<T> {
  const page = asRecord(body, 'page');
  const data = page['data'];
  if (!Array.isArray(data)) {
    throw new InvalidApiResponseError('data');
  }
  return { items: data.map(parseItem), pagination: parsePagination(page['pagination']) };
}

function parsePagination(value: unknown): Pagination {
  const record = asRecord(value, 'pagination');
  return {
    page: integer(record, 'page', 1),
    limit: integer(record, 'limit', 1),
    total: integer(record, 'total', 0),
    totalPages: integer(record, 'total_pages', 0),
  };
}

function parseProduct(value: unknown): ProductSummary {
  const record = asRecord(value, 'product');
  if (record['currency'] !== 'EUR') {
    throw new InvalidApiResponseError('currency');
  }
  return {
    id: text(record, 'id'),
    slug: text(record, 'slug'),
    name: text(record, 'name'),
    description: text(record, 'description', true),
    lowestPrice: euros(integer(record, 'price_cents', 0)),
    available: bool(record, 'available'),
    variantCount: integer(record, 'variant_count', 1),
    imageUrl: mediaPath(text(record, 'image_url', true)),
  };
}

function parseCategory(value: unknown): Category {
  const record = asRecord(value, 'category');
  const parentId = record['parent_id'];
  if (parentId !== null && typeof parentId !== 'string') {
    throw new InvalidApiResponseError('parent_id');
  }
  return {
    id: text(record, 'id'),
    parentId,
    slug: text(record, 'slug'),
    name: text(record, 'name'),
    description: text(record, 'description', true),
    sortOrder: integer(record, 'sort_order', 0),
  };
}

function asRecord(value: unknown, field: string): JsonRecord {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new InvalidApiResponseError(field);
  }
  return value as JsonRecord;
}

function text(record: JsonRecord, field: string, allowEmpty = false): string {
  const value = record[field];
  if (typeof value !== 'string' || (!allowEmpty && value.trim() === '')) {
    throw new InvalidApiResponseError(field);
  }
  return value;
}

function integer(record: JsonRecord, field: string, min: number): number {
  const value = record[field];
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < min) {
    throw new InvalidApiResponseError(field);
  }
  return value;
}

function list(record: JsonRecord, field: string): readonly unknown[] {
  const value = record[field];
  if (!Array.isArray(value)) {
    throw new InvalidApiResponseError(field);
  }
  return value;
}

function oneOf<T extends string>(record: JsonRecord, field: string, allowed: readonly T[]): T {
  const value = record[field];
  if (typeof value !== 'string' || !(allowed as readonly string[]).includes(value)) {
    throw new InvalidApiResponseError(field);
  }
  return value as T;
}

function bool(record: JsonRecord, field: string): boolean {
  const value = record[field];
  if (typeof value !== 'boolean') {
    throw new InvalidApiResponseError(field);
  }
  return value;
}
