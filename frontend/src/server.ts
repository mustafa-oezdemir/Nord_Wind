import {
  AngularNodeAppEngine,
  createNodeRequestHandler,
  isMainModule,
  writeResponseToNodeResponse,
} from '@angular/ssr/node';
import express from 'express';
import { randomBytes } from 'node:crypto';
import { join } from 'node:path';

const browserDistFolder = join(import.meta.dirname, '../browser');
const publicOrigin = process.env['NORDWINDS_PUBLIC_ORIGIN'];
const publicHostname = publicOrigin ? new URL(publicOrigin).hostname : undefined;
const allowedHosts =
  !publicHostname || ['localhost', '127.0.0.1'].includes(publicHostname)
    ? ['localhost', '127.0.0.1']
    : [publicHostname];

const app = express();
// Caddy überschreibt eingehende X-Forwarded-Header und ist der einzige öffentliche Einstieg.
const angularApp = new AngularNodeAppEngine({ allowedHosts, trustProxyHeaders: true });

/** Statische Browserdateien ohne Serverrendering ausliefern. */
app.use(
  express.static(browserDistFolder, {
    maxAge: '1y',
    index: false,
    redirect: false,
  }),
);

/** Öffentliche Katalogseiten serverseitig, andere Routen clientseitig ausliefern. */
app.use((req, res, next) => {
  angularApp
    .handle(req)
    .then(async (response) => {
      if (!response) return next();
      if (!response.headers.get('Content-Type')?.includes('text/html')) {
        return writeResponseToNodeResponse(response, res);
      }

      // SSR erzeugt Inline-Skripte und Komponenten-Styles. Ein Nonce pro Antwort
      // schützt beide und wird von Angular für spätere Styles weiterverwendet.
      const nonce = randomBytes(16).toString('base64');
      const html = (await response.text())
        .replace(/<app-root(?![^>]*\bngCspNonce=)/i, `<app-root ngCspNonce="${nonce}"`)
        .replace(/<(script|style)\b(?![^>]*\bnonce=)/gi, `<$1 nonce="${nonce}"`);
      const headers = new Headers(response.headers);
      headers.delete('Content-Length');
      headers.set(
        'Content-Security-Policy',
        `default-src 'self'; script-src 'self' 'nonce-${nonce}'; style-src 'self' 'nonce-${nonce}'; style-src-attr 'none'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'`,
      );
      return writeResponseToNodeResponse(
        new Response(html, { status: response.status, statusText: response.statusText, headers }),
        res,
      );
    })
    .catch(next);
});

/** Den Node-Server im Container oder direkt aus dem Build starten. */
if (isMainModule(import.meta.url) || process.env['pm_id']) {
  const port = process.env['PORT'] || 4000;
  app.listen(port, (error) => {
    if (error) {
      throw error;
    }

    console.log(`NordWinds SSR lauscht auf http://localhost:${port}`);
  });
}

/** Anfragebehandlung für den Angular-Entwicklungsserver und den Build. */
export const reqHandler = createNodeRequestHandler(app);
