import { InvalidApiResponseError } from '../../../core/http/api-error';
import { euros } from '../../../shared/money/money';
import { Category, Page, Pagination, ProductSummary } from '../models/catalog.models';

// Die API ist eine Vertrauensgrenze: Antworten werden geprüft, bevor die Oberfläche
// sie anzeigt. Abweichungen führen zu einem Fehlerzustand statt zu falschen Preisen.

type JsonRecord = Readonly<Record<string, unknown>>;

export function parseProductPage(body: unknown): Page<ProductSummary> {
  return parsePage(body, parseProduct);
}

export function parseCategoryPage(body: unknown): Page<Category> {
  return parsePage(body, parseCategory);
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

function bool(record: JsonRecord, field: string): boolean {
  const value = record[field];
  if (typeof value !== 'boolean') {
    throw new InvalidApiResponseError(field);
  }
  return value;
}
