import { HttpErrorResponse } from '@angular/common/http';
import { TimeoutError } from 'rxjs';
import { ApiErrorKind, InvalidApiResponseError, apiErrorMessage, toApiError } from './api-error';

describe('toApiError', () => {
  it('erkennt Netzwerkfehler', () => {
    expect(toApiError(new HttpErrorResponse({ status: 0 }))).toEqual({
      kind: 'network',
      status: 0,
    });
  });

  it('übernimmt Problem-Code und Request-ID, aber keine Detailtexte', () => {
    const error = new HttpErrorResponse({
      status: 404,
      error: { code: 'NOT_FOUND', request_id: 'req-42', detail: 'interner Text' },
    });
    expect(toApiError(error)).toEqual({
      kind: 'not-found',
      status: 404,
      code: 'NOT_FOUND',
      requestId: 'req-42',
    });
  });

  it('ordnet Client-, Server- und sonstige Statuscodes zu', () => {
    expect(toApiError(new HttpErrorResponse({ status: 400 })).kind).toBe('invalid-request');
    expect(toApiError(new HttpErrorResponse({ status: 503 })).kind).toBe('server');
    expect(toApiError(new HttpErrorResponse({ status: 418 })).kind).toBe('unexpected');
  });

  it('erkennt Zeitüberschreitung, ungültige Antworten und fremde Fehler', () => {
    expect(toApiError(new TimeoutError()).kind).toBe('timeout');
    expect(toApiError(new InvalidApiResponseError('price_cents')).kind).toBe('invalid-response');
    expect(toApiError(new Error('unbekannt')).kind).toBe('unexpected');
    expect(toApiError('kaputt').kind).toBe('unexpected');
  });

  it('ignoriert Problem-Felder mit falschem Typ', () => {
    const error = new HttpErrorResponse({ status: 500, error: { code: 7, request_id: null } });
    expect(toApiError(error)).toEqual({ kind: 'server', status: 500 });
  });
});

describe('apiErrorMessage', () => {
  const kinds: ApiErrorKind[] = [
    'network',
    'timeout',
    'invalid-request',
    'not-found',
    'server',
    'invalid-response',
    'unexpected',
  ];

  it('liefert für jede Fehlerart eine deutsche Meldung ohne technische Angaben', () => {
    for (const kind of kinds) {
      const message = apiErrorMessage({
        kind,
        status: 500,
        code: 'INTERNAL_ERROR',
        requestId: 'req-1',
      });
      expect(message.length).toBeGreaterThan(20);
      expect(message).not.toContain('INTERNAL_ERROR');
      expect(message).not.toContain('req-1');
      expect(message).not.toContain('500');
    }
  });
});
