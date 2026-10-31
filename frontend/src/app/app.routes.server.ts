import { RenderMode, ServerRoute } from '@angular/ssr';

export const serverRoutes: ServerRoute[] = [
  { path: '', renderMode: RenderMode.Server },
  { path: 'produkte', renderMode: RenderMode.Server },
  { path: 'produkte/:slug', renderMode: RenderMode.Server },
  { path: 'kategorien/:slug', renderMode: RenderMode.Server },
  { path: 'rechtliches/:slug', renderMode: RenderMode.Server },
  {
    path: '**',
    renderMode: RenderMode.Client,
  },
];
