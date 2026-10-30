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
      api.saveProduct({ name: 'Kamera', slug: 'kamera', description: '', status: 'draft' }),
    );
    const request = http.expectOne('/api/v1/admin/products');
    expect(request.request.method).toBe('POST');
    expect(request.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    expect(request.request.urlWithParams).not.toContain(session.csrf_token);
    request.flush({
      id: 'product-id',
      name: 'Kamera',
      slug: 'kamera',
      description: '',
      status: 'draft',
    });
    await saved;

    const logout = firstValueFrom(api.logout());
    const logoutRequest = http.expectOne('/api/v1/auth/logout');
    expect(logoutRequest.request.headers.get('X-CSRF-Token')).toBe(session.csrf_token);
    logoutRequest.flush(null);
    await logout;
  });
});
