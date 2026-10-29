import { ProductSort } from './catalog.models';

// Öffentliche URL-Parameter sind deutsch und stabil; die API-Werte bleiben intern.
export const SORT_OPTIONS: readonly {
  readonly param: string;
  readonly sort: ProductSort;
  readonly label: string;
}[] = [
  { param: 'neu', sort: 'newest', label: 'Neueste zuerst' },
  { param: 'name', sort: 'name_asc', label: 'Name A–Z' },
  { param: 'preis-auf', sort: 'price_asc', label: 'Preis aufsteigend' },
  { param: 'preis-ab', sort: 'price_desc', label: 'Preis absteigend' },
];

export const DEFAULT_SORT_PARAM = 'neu';
export const MAX_SEARCH_LENGTH = 100;
const MAX_PAGE = 10_000;

export function sortFromParam(param: string | undefined): ProductSort {
  return SORT_OPTIONS.find((option) => option.param === param)?.sort ?? 'newest';
}

export function sortToParam(sort: ProductSort): string {
  return SORT_OPTIONS.find((option) => option.sort === sort)?.param ?? DEFAULT_SORT_PARAM;
}

/** Ungültige oder fehlende Seitenangaben führen auf Seite 1, nicht zu einem API-Fehler. */
export function pageFromParam(param: string | undefined): number {
  if (!param || !/^\d{1,5}$/.test(param)) {
    return 1;
  }
  const page = Number(param);
  return page >= 1 && page <= MAX_PAGE ? page : 1;
}

/** Leerzeichen werden entfernt; zu lange Suchbegriffe kürzt die Oberfläche auf das API-Limit. */
export function normalizeSearch(value: string | undefined): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed) {
    return undefined;
  }
  return [...trimmed].slice(0, MAX_SEARCH_LENGTH).join('');
}
