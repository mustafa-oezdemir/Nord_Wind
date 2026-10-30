import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';
import { AdminApi, AuthSession } from './admin-api';

describe('AdminApi', () => {
  let api: AdminApi;
  let http: HttpTestingController;
  const session: AuthSession = {
    customer: { id: 'admin-id', email: 'admin@example.invalid', role: 'admin' },
    csrf_token: 'synthetic-csrf-token',
    expires_at: '2026-10-31T00:00:00Z',
  };

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting()],
    });
    api = TestBed.inject(AdminApi);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('setzt den Session-CSRF-Token bei Admin-Schreibzugriffen und sendet ihn nicht als URL-Parameter', async () => {
    const restored = firstValueFrom(api.session());
    http.expectOne('/api/v1/auth/session').flush(session);
    await restored;

    const saved = firstValueFrom(
      api.saveProduct({
        name: 'Kochbuch',
        slug: 'kochbuch',
        description: '',
        status: 'draft',
        tax_class: 'reduced',
      }),
    );
    const request = http.expectOne('/api/v1/admin/products');
    expect(request.request.method).toBe('POST');
    expect(request.request.body.tax_class).toBe('reduced');
    expect(request.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    expect(request.request.urlWithParams).not.toContain(session.csrf_token);
    request.flush({
      id: 'product-id',
      name: 'Kochbuch',
      slug: 'kochbuch',
      description: '',
      status: 'draft',
      tax_class: 'reduced',
    });
    await saved;

    const logout = firstValueFrom(api.logout());
    const logoutRequest = http.expectOne('/api/v1/auth/logout');
    expect(logoutRequest.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    logoutRequest.flush(null);
    await logout;
  });

  it('legt Rabattcodes an und ändert nur Aktivierung und Gültigkeit', async () => {
    const restored = firstValueFrom(api.session());
    http.expectOne('/api/v1/auth/session').flush(session);
    await restored;

    const created = firstValueFrom(
      api.createDiscountCode({
        code: 'HERBST10',
        percent_bp: 1000,
        valid_from: null,
        valid_until: null,
      }),
    );
    const post = http.expectOne('/api/v1/admin/discount-codes');
    expect(post.request.method).toBe('POST');
    expect(post.request.body).toEqual({
      code: 'HERBST10',
      percent_bp: 1000,
      valid_from: null,
      valid_until: null,
      is_active: true,
    });
    expect(post.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    const code = {
      id: 'code-id',
      code: 'HERBST10',
      percent_bp: 1000,
      is_active: true,
      valid_from: null,
      valid_until: null,
      created_at: '2026-10-30T10:00:00Z',
      updated_at: '2026-10-30T10:00:00Z',
    };
    post.flush(code);
    await created;

    const updated = firstValueFrom(api.updateDiscountCode(code, false));
    const put = http.expectOne('/api/v1/admin/discount-codes/code-id');
    expect(put.request.method).toBe('PUT');
    expect(put.request.body).toEqual({
      percent_bp: 1000,
      is_active: false,
      valid_from: null,
      valid_until: null,
    });
    put.flush({ ...code, is_active: false });
    await updated;
  });
});
