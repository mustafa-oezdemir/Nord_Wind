import { DOCUMENT, InjectionToken, inject } from '@angular/core';

/** Öffentliche Origin für kanonische URLs; der Server überschreibt den Browserwert. */
export const PUBLIC_SITE_ORIGIN = new InjectionToken<string>('nordwinds-public-site-origin', {
  providedIn: 'root',
  factory: () => inject(DOCUMENT).location?.origin ?? '',
});
