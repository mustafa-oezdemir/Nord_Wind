import { Routes } from '@angular/router';
import { StorefrontLayout } from './layouts/storefront-layout/storefront-layout';

const productList = () =>
  import('./features/catalog/pages/product-list/product-list').then((m) => m.ProductList);

export const routes: Routes = [
  {
    path: 'admin',
    title: 'Verwaltung',
    loadComponent: () =>
      import('./features/admin/pages/admin-page/admin-page').then((m) => m.AdminPage),
  },
  {
    path: '',
    component: StorefrontLayout,
    children: [
      {
        path: '',
        pathMatch: 'full',
        title: 'Onlineshop für Deutschland',
        loadComponent: () => import('./features/home/home-page/home-page').then((m) => m.HomePage),
      },
      { path: 'produkte', title: 'Alle Produkte', loadComponent: productList },
      {
        path: 'warenkorb',
        title: 'Warenkorb',
        loadComponent: () =>
          import('./features/cart/pages/cart-page/cart-page').then((m) => m.CartPage),
      },
      {
        path: 'produkte/:slug',
        title: 'Produkt',
        loadComponent: () =>
          import('./features/catalog/pages/product-detail/product-detail').then(
            (m) => m.ProductDetailPage,
          ),
      },
      { path: 'kategorien/:slug', title: 'Kategorie', loadComponent: productList },
      {
        path: '**',
        title: 'Seite nicht gefunden',
        loadComponent: () =>
          import('./features/not-found/not-found-page').then((m) => m.NotFoundPage),
      },
    ],
  },
];
