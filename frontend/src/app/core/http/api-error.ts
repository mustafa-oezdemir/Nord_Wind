import { HttpErrorResponse } from '@angular/common/http';
import { TimeoutError } from 'rxjs';

export type ApiErrorKind =
  | 'network'
  | 'timeout'
  | 'invalid-request'
  | 'not-found'
  | 'server'
  | 'invalid-response'
  | 'unexpected';

/** Für die Oberfläche aufbereiteter API-Fehler ohne technische Details. */
export interface ApiError {
  readonly kind: ApiErrorKind;
  /** HTTP-Status; 0 bei Netzwerkfehler, Zeitüberschreitung oder Clientfehler. */
  readonly status: number;
  /** Problem-Code der API nach RFC 9457, z. B. `NOT_FOUND`. */
  readonly code?: string;
  /** Request-ID für den Support, falls die API sie liefert. */
  readonly requestId?: string;
}

/** Die API hat mit einem Body geantwortet, der nicht zum Vertrag passt. */
export class InvalidApiResponseError extends Error {
  constructor(readonly field: string) {
    super(`Ungültige API-Antwort im Feld "${field}"`);
    this.name = 'InvalidApiResponseError';
  }
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof TimeoutError) {
    return { kind: 'timeout', status: 0 };
  }
  if (error instanceof InvalidApiResponseError) {
    return { kind: 'invalid-response', status: 0 };
  }
  if (!(error instanceof HttpErrorResponse)) {
    return { kind: 'unexpected', status: 0 };
  }
  const problem = readProblem(error.error);
  const base = { status: error.status, ...problem };
  if (error.status === 0) {
    return { ...base, kind: 'network' };
  }
  if (error.status === 404) {
    return { ...base, kind: 'not-found' };
  }
  if (error.status === 400 || error.status === 422) {
    return { ...base, kind: 'invalid-request' };
  }
  if (error.status >= 500) {
    return { ...base, kind: 'server' };
  }
  return { ...base, kind: 'unexpected' };
}

/** Verständliche deutsche Meldung mit Handlungshinweis; nie mit Server-Details. */
export function apiErrorMessage(error: ApiError): string {
  switch (error.kind) {
    case 'network':
      return 'Der Shop ist gerade nicht erreichbar. Prüfen Sie Ihre Internetverbindung und versuchen Sie es erneut.';
    case 'timeout':
      return 'Der Server hat nicht rechtzeitig geantwortet. Versuchen Sie es in einem Moment erneut.';
    case 'invalid-request':
      return 'Die Suche oder der Filter enthält ungültige Angaben. Passen Sie die Eingabe an.';
    case 'not-found':
      return 'Die angeforderten Daten wurden nicht gefunden.';
    default:
      return 'Der Server konnte die Anfrage nicht bearbeiten. Versuchen Sie es in einem Moment erneut.';
  }
}

function readProblem(body: unknown): Pick<ApiError, 'code' | 'requestId'> {
  if (typeof body !== 'object' || body === null) {
    return {};
  }
  const record = body as Record<string, unknown>;
  const code = typeof record['code'] === 'string' ? record['code'] : undefined;
  const requestId = typeof record['request_id'] === 'string' ? record['request_id'] : undefined;
  return { ...(code ? { code } : {}), ...(requestId ? { requestId } : {}) };
}
