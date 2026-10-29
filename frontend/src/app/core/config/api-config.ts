import { InjectionToken } from '@angular/core';

/**
 * Basis-URL der Go-API. Standard ist derselbe Origin, den Caddy unter `/api` an die
 * API weiterleitet; Umgebungen mit anderer Adresse überschreiben den Provider.
 */
export const API_BASE_URL = new InjectionToken<string>('API_BASE_URL', {
  providedIn: 'root',
  factory: () => '/api/v1',
});

/** Zeitlimit und Wiederholungsregel für API-Anfragen. */
export interface ApiRequestPolicy {
  /** Abbruch einer einzelnen Anfrage nach dieser Zeit. */
  readonly timeoutMs: number;
  /** Wartezeit vor dem einzigen Wiederholungsversuch sicherer Anfragen. */
  readonly retryDelayMs: number;
}

export const API_REQUEST_POLICY = new InjectionToken<ApiRequestPolicy>('API_REQUEST_POLICY', {
  providedIn: 'root',
  factory: () => ({ timeoutMs: 15_000, retryDelayMs: 400 }),
});
