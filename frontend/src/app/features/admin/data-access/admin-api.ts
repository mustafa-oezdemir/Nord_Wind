import { HttpClient, HttpHeaders } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, tap } from 'rxjs';
import { API_BASE_URL } from '../../../core/config/api-config';

export interface AdminCustomer {
  id: string;
  email: string;
  role: 'admin' | 'customer';
}

export interface AuthSession {
  customer: AdminCustomer;
  csrf_token: string;
  expires_at: string;
}

export interface AdminCategory {
  id: string;
  parent_id: string | null;
  name: string;
  slug: string;
  description: string;
  status: 'active' | 'inactive';
  sort_order: number;
}

export interface AdminProduct {
  id: string;
  name: string;
  slug: string;
  description: string;
  status: 'draft' | 'published' | 'archived';
}

export interface AdminVariant {
  id: string;
  product_id: string;
  sku: string;
  label: string;
  price_cents: number;
  stock_quantity: number;
  is_active: boolean;
}

export interface AdminImage {
  id: string;
  src: string;
  alt: string;
  width: number;
  height: number;
}

export interface AdminAuditEvent {
  id: number;
  actor_customer_id: string;
  entity_type: string;
  entity_id: string;
  related_id: string | null;
  action: string;
  created_at: string;
}

export type CategoryDraft = Omit<AdminCategory, 'id'> & { id?: string };
export type ProductDraft = Omit<AdminProduct, 'id'> & { id?: string };
export type VariantDraft = Omit<AdminVariant, 'id' | 'product_id'> & { id?: string };

@Injectable({ providedIn: 'root' })
export class AdminApi {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE_URL);
  private csrfToken = '';

  session(): Observable<AuthSession> {
    return this.http
      .get<AuthSession>(`${this.base}/auth/session`)
      .pipe(tap((value) => this.remember(value)));
  }

  login(email: string, password: string): Observable<AuthSession> {
    return this.http
      .post<AuthSession>(`${this.base}/auth/login`, { email, password })
      .pipe(tap((value) => this.remember(value)));
  }

  logout(): Observable<void> {
    return this.http
      .post<void>(`${this.base}/auth/logout`, {}, { headers: this.writeHeaders() })
      .pipe(tap(() => (this.csrfToken = '')));
  }

  listCategories(): Observable<{ data: AdminCategory[] }> {
    return this.http.get<{ data: AdminCategory[] }>(`${this.base}/admin/categories`);
  }

  saveCategory(input: CategoryDraft): Observable<AdminCategory> {
    const body = {
      parent_id: input.parent_id || null,
      name: input.name,
      slug: input.slug,
      description: input.description,
      status: input.status,
      sort_order: input.sort_order,
    };
    return input.id
      ? this.http.put<AdminCategory>(`${this.base}/admin/categories/${input.id}`, body, {
          headers: this.writeHeaders(),
        })
      : this.http.post<AdminCategory>(`${this.base}/admin/categories`, body, {
          headers: this.writeHeaders(),
        });
  }

  deleteCategory(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/admin/categories/${id}`, {
      headers: this.writeHeaders(),
    });
  }

  listProducts(): Observable<{ data: AdminProduct[] }> {
    return this.http.get<{ data: AdminProduct[] }>(`${this.base}/admin/products`);
  }

  saveProduct(input: ProductDraft): Observable<AdminProduct> {
    const body = {
      name: input.name,
      slug: input.slug,
      description: input.description,
      status: input.status,
    };
    return input.id
      ? this.http.put<AdminProduct>(`${this.base}/admin/products/${input.id}`, body, {
          headers: this.writeHeaders(),
        })
      : this.http.post<AdminProduct>(`${this.base}/admin/products`, body, {
          headers: this.writeHeaders(),
        });
  }

  deleteProduct(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/admin/products/${id}`, {
      headers: this.writeHeaders(),
    });
  }

  listVariants(productId: string): Observable<{ data: AdminVariant[] }> {
    return this.http.get<{ data: AdminVariant[] }>(
      `${this.base}/admin/products/${productId}/variants`,
    );
  }

  saveVariant(productId: string, input: VariantDraft): Observable<AdminVariant> {
    const body = {
      sku: input.sku,
      label: input.label,
      price_cents: input.price_cents,
      stock_quantity: input.stock_quantity,
      is_active: input.is_active,
    };
    return input.id
      ? this.http.put<AdminVariant>(`${this.base}/admin/variants/${input.id}`, body, {
          headers: this.writeHeaders(),
        })
      : this.http.post<AdminVariant>(`${this.base}/admin/products/${productId}/variants`, body, {
          headers: this.writeHeaders(),
        });
  }

  deleteVariant(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/admin/variants/${id}`, {
      headers: this.writeHeaders(),
    });
  }

  productCategories(productId: string): Observable<{ data: AdminCategory[] }> {
    return this.http.get<{ data: AdminCategory[] }>(
      `${this.base}/admin/products/${productId}/categories`,
    );
  }

  attachCategory(productId: string, categoryId: string): Observable<void> {
    return this.http.post<void>(
      `${this.base}/admin/products/${productId}/categories`,
      { category_id: categoryId },
      { headers: this.writeHeaders() },
    );
  }

  detachCategory(productId: string, categoryId: string): Observable<void> {
    return this.http.delete<void>(
      `${this.base}/admin/products/${productId}/categories/${categoryId}`,
      {
        headers: this.writeHeaders(),
      },
    );
  }

  audit(): Observable<{ data: AdminAuditEvent[] }> {
    return this.http.get<{ data: AdminAuditEvent[] }>(`${this.base}/admin/audit`);
  }

  listImages(productId: string): Observable<{ data: AdminImage[] }> {
    return this.http.get<{ data: AdminImage[] }>(`${this.base}/admin/products/${productId}/images`);
  }

  uploadImage(productId: string, file: File, alt: string): Observable<AdminImage> {
    const body = new FormData();
    body.append('image', file);
    body.append('alt', alt);
    return this.http.post<AdminImage>(`${this.base}/admin/products/${productId}/images`, body, {
      headers: this.writeHeaders(),
    });
  }

  deleteImage(id: string): Observable<void> {
    return this.http.delete<void>(`${this.base}/admin/images/${id}`, {
      headers: this.writeHeaders(),
    });
  }

  private remember(value: AuthSession): void {
    this.csrfToken = value.csrf_token;
  }

  private writeHeaders(): HttpHeaders {
    return new HttpHeaders({ 'X-CSRF-Token': this.csrfToken });
  }
}
