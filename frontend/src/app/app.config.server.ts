import { HTTP_TRANSFER_CACHE_ORIGIN_MAP } from '@angular/common/http';
import { ApplicationConfig, REQUEST, inject, mergeApplicationConfig } from '@angular/core';
import { provideServerRendering, withRoutes } from '@angular/ssr';
import { appConfig } from './app.config';
import { serverRoutes } from './app.routes.server';
import { API_BASE_URL } from './core/config/api-config';
import { PUBLIC_SITE_ORIGIN } from './core/config/site-origin';

const internalOrigin = process.env['NORDWINDS_INTERNAL_API_ORIGIN'] || 'http://127.0.0.1:8080';

const serverConfig: ApplicationConfig = {
  providers: [
    provideServerRendering(withRoutes(serverRoutes)),
    { provide: API_BASE_URL, useValue: `${internalOrigin}/api/v1` },
    {
      provide: PUBLIC_SITE_ORIGIN,
      useFactory: () =>
        process.env['NORDWINDS_PUBLIC_ORIGIN'] ||
        new URL(inject(REQUEST)?.url ?? 'http://127.0.0.1:4000').origin,
    },
    {
      provide: HTTP_TRANSFER_CACHE_ORIGIN_MAP,
      useFactory: () => ({
        [internalOrigin]:
          process.env['NORDWINDS_PUBLIC_ORIGIN'] ||
          new URL(inject(REQUEST)?.url ?? 'http://127.0.0.1:4000').origin,
      }),
    },
  ],
};

export const config = mergeApplicationConfig(appConfig, serverConfig);
