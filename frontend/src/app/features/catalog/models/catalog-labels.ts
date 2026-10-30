import { SaleStatus, StockStatus } from './catalog.models';

/** Bestandsangaben für Kunden; exakte Mengen veröffentlicht die API bewusst nicht. */
export const STOCK_LABELS: Readonly<Record<StockStatus, string>> = {
  in_stock: 'Auf Lager',
  low_stock: 'Nur noch wenige Stück',
  out_of_stock: 'Ausverkauft',
};

/** Hinweis für Produkte, die derzeit nicht oder nicht mehr verkauft werden. */
export const SALE_NOTICES: Readonly<Record<Exclude<SaleStatus, 'available'>, string>> = {
  sold_out: 'Dieses Produkt ist derzeit ausverkauft.',
  unavailable: 'Dieses Produkt ist derzeit nicht erhältlich.',
  discontinued: 'Dieses Produkt wird nicht mehr angeboten.',
};
