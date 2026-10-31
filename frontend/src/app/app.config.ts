import { registerLocaleData } from '@angular/common';
import { provideHttpClient, withFetch, withInterceptors } from '@angular/common/http';
import localeDe from '@angular/common/locales/de';
import {
  ApplicationConfig,
  DEFAULT_CURRENCY_CODE,
  LOCALE_ID,
  provideBrowserGlobalErrorListeners,
} from '@angular/core';
import {
  TitleStrategy,
  provideRouter,
  withComponentInputBinding,
  withInMemoryScrolling,
} from '@angular/router';
import { routes } from './app.routes';
import { API_BASE_URL } from './core/config/api-config';
import { apiInterceptor } from './core/http/api-interceptor';
import { PageTitleStrategy } from './core/routing/page-title-strategy';
import { provideClientHydration } from '@angular/platform-browser';

registerLocaleData(localeDe);

export const appConfig: ApplicationConfig = {
  providers: [
    provideBrowserGlobalErrorListeners(),
    provideRouter(
      routes,
      withComponentInputBinding(),
      withInMemoryScrolling({ scrollPositionRestoration: 'enabled' }),
    ),
    provideHttpClient(withFetch(), withInterceptors([apiInterceptor])),
    { provide: LOCALE_ID, useValue: 'de-DE' },
    { provide: DEFAULT_CURRENCY_CODE, useValue: 'EUR' },
    // Same-Origin über Caddy; lokal leitet proxy.conf.json `/api` an den Compose-Stack weiter.
    { provide: API_BASE_URL, useValue: '/api/v1' },
    { provide: TitleStrategy, useClass: PageTitleStrategy },
    provideClientHydration(),
  ],
};
