import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';
import { inject } from '@angular/core';
import { retry, throwError, timeout, timer } from 'rxjs';
import { API_BASE_URL, API_REQUEST_POLICY } from '../config/api-config';

const SAFE_METHODS = new Set(['GET', 'HEAD']);
const TRANSIENT_STATUS = new Set([0, 502, 503, 504]);

/**
 * Gilt nur für Anfragen an die eigene API: JSON aushandeln, Zeitlimit setzen und
 * sichere Anfragen bei vorübergehenden Fehlern genau einmal wiederholen. Schreibende
 * Anfragen (Warenkorb, Checkout) werden nie automatisch wiederholt.
 */
export const apiInterceptor: HttpInterceptorFn = (request, next) => {
  const baseUrl = inject(API_BASE_URL);
  const policy = inject(API_REQUEST_POLICY);
  if (!request.url.startsWith(baseUrl)) {
    return next(request);
  }

  const apiRequest = request.headers.has('Accept')
    ? request
    : request.clone({ setHeaders: { Accept: 'application/json, application/problem+json' } });
  // `each` statt `first`: Das Backend meldet sofort ein Sent-Ereignis; gemessen wird die
  // Zeit bis zur eigentlichen Antwort.
  const response = next(apiRequest).pipe(timeout({ each: policy.timeoutMs }));
  if (!SAFE_METHODS.has(request.method)) {
    return response;
  }
  return response.pipe(
    retry({
      count: 1,
      delay: (error: unknown) =>
        isTransient(error) ? timer(policy.retryDelayMs) : throwError(() => error),
    }),
  );
};

function isTransient(error: unknown): boolean {
  return error instanceof HttpErrorResponse && TRANSIENT_STATUS.has(error.status);
}
