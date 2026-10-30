import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, map } from 'rxjs';
import { API_BASE_URL } from '../../../core/config/api-config';
import {
  Category,
  Page,
  ProductDetail,
  ProductQuery,
  ProductSummary,
} from '../models/catalog.models';
import { parseCategoryPage, parseProductDetail, parseProductPage } from './catalog.mapper';

/** Höchstwert der API für `limit`; die Navigation lädt alle Kategorien auf einer Seite. */
export const MAX_PAGE_SIZE = 100;

/** Lesender Zugriff auf die öffentliche Katalog-API. */
@Injectable({ providedIn: 'root' })
export class CatalogApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = inject(API_BASE_URL);

  listProducts(query: ProductQuery): Observable<Page<ProductSummary>> {
    return this.http
      .get<unknown>(`${this.baseUrl}/products`, { params: productParams(query) })
      .pipe(map(parseProductPage));
  }

  /** Produkt mit aktiven Varianten; Preis und Bestand kommen ausschließlich von der API. */
  getProduct(slug: string): Observable<ProductDetail> {
    return this.http
      .get<unknown>(`${this.baseUrl}/products/${encodeURIComponent(slug)}`)
      .pipe(map(parseProductDetail));
  }

  /** Alle aktiven Kategorien in Pflegereihenfolge (derzeit höchstens 100). */
  listCategories(): Observable<readonly Category[]> {
    const params = new HttpParams().set('sort', 'sort_order').set('limit', MAX_PAGE_SIZE);
    return this.http
      .get<unknown>(`${this.baseUrl}/categories`, { params })
      .pipe(map((body) => parseCategoryPage(body).items));
  }
}

function productParams(query: ProductQuery): HttpParams {
  let params = new HttpParams().set('sort', query.sort).set('limit', query.limit);
  if (query.category) {
    params = params.set('category', query.category);
  }
  if (query.search) {
    params = params.set('q', query.search);
  }
  if (query.page > 1) {
    params = params.set('page', query.page);
  }
  return params;
}
