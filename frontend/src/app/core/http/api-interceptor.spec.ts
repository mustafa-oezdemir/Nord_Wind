import {
  HttpClient,
  HttpErrorResponse,
  provideHttpClient,
  withInterceptors,
} from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { TimeoutError, firstValueFrom } from 'rxjs';
import { API_REQUEST_POLICY } from '../config/api-config';
import { apiInterceptor } from './api-interceptor';

const API_URL = '/api/v1/products';
const waitForRetry = () => new Promise((resolve) => setTimeout(resolve, 5));

describe('apiInterceptor', () => {
  let http: HttpClient;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(withInterceptors([apiInterceptor])),
        provideHttpClientTesting(),
        { provide: API_REQUEST_POLICY, useValue: { timeoutMs: 50, retryDelayMs: 0 } },
      ],
    });
    http = TestBed.inject(HttpClient);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => httpMock.verify());

  it('handelt JSON nur für Anfragen an die eigene API aus', () => {
    http.get(API_URL).subscribe();
    http.get('/assets/info.json').subscribe();

    const apiRequest = httpMock.expectOne(API_URL);
    const assetRequest = httpMock.expectOne('/assets/info.json');
    expect(apiRequest.request.headers.get('Accept')).toContain('application/json');
    expect(assetRequest.request.headers.has('Accept')).toBe(false);
    apiRequest.flush({});
    assetRequest.flush({});
  });

  it('wiederholt eine GET-Anfrage nach 503 genau einmal', async () => {
    const result = firstValueFrom(http.get<{ ok: boolean }>(API_URL));
    httpMock.expectOne(API_URL).flush(null, { status: 503, statusText: 'Service Unavailable' });
    await waitForRetry();
    httpMock.expectOne(API_URL).flush({ ok: true });

    expect(await result).toEqual({ ok: true });
  });

  it('gibt den Fehler nach dem zweiten Fehlschlag weiter', async () => {
    const result = firstValueFrom(http.get(API_URL)).catch((error: unknown) => error);
    httpMock.expectOne(API_URL).flush(null, { status: 502, statusText: 'Bad Gateway' });
    await waitForRetry();
    httpMock.expectOne(API_URL).flush(null, { status: 502, statusText: 'Bad Gateway' });

    const error = await result;
    expect(error).toBeInstanceOf(HttpErrorResponse);
    expect((error as HttpErrorResponse).status).toBe(502);
  });

  it('wiederholt weder 404 noch schreibende Anfragen', async () => {
    http.get(API_URL).subscribe({ error: () => undefined });
    httpMock.expectOne(API_URL).flush(null, { status: 404, statusText: 'Not Found' });
    http.post('/api/v1/cart', {}).subscribe({ error: () => undefined });
    httpMock
      .expectOne('/api/v1/cart')
      .flush(null, { status: 503, statusText: 'Service Unavailable' });
    await waitForRetry();

    httpMock.expectNone(API_URL);
    httpMock.expectNone('/api/v1/cart');
  });

  it('bricht eine hängende Anfrage nach dem Zeitlimit ab', async () => {
    const result = firstValueFrom(http.get(API_URL)).catch((error: unknown) => error);
    const request = httpMock.expectOne(API_URL);

    expect(await result).toBeInstanceOf(TimeoutError);
    expect(request.cancelled).toBe(true);
  });
});
