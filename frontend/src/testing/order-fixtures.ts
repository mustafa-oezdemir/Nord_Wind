import { CartItem, CartSnapshot, CustomerSession } from '../app/features/cart/data-access/cart-api';
import { Order } from '../app/features/checkout/data-access/orders-api';

export const customerSession: CustomerSession = {
  customer: { id: 'customer-id', email: 'kunde@example.invalid', role: 'customer' },
  csrf_token: 'synthetic-csrf-token',
  expires_at: '2026-10-31T00:00:00Z',
};

export const cartItem = (overrides: Partial<CartItem> = {}): CartItem => ({
  variant_id: 'pan-id',
  product_id: 'product-id',
  product_slug: 'pfanne',
  product_name: 'Pfanne',
  sku: 'NW-PFANNE',
  label: '28 cm',
  quantity: 1,
  unit_price_cents: 3000,
  line_total_cents: 3000,
  currency: 'EUR',
  available: true,
  unavailable_reason: '',
  tax_class: 'standard',
  tax_rate_bp: 1900,
  ...overrides,
});

/** Antwort der API für Pfanne 30,00 € (19 %) + Buch 10,00 € (7 %), siehe Go-Integrationstest. */
export const mixedCart: CartSnapshot = {
  items: [
    cartItem(),
    cartItem({
      variant_id: 'book-id',
      product_slug: 'kochbuch',
      product_name: 'Kochbuch',
      sku: 'NW-BUCH',
      label: 'Gebunden',
      unit_price_cents: 1000,
      line_total_cents: 1000,
      tax_class: 'reduced',
      tax_rate_bp: 700,
    }),
  ],
  subtotal_cents: 4000,
  currency: 'EUR',
  discount: null,
  pricing: {
    goods_cents: 4000,
    discount_cents: 0,
    shipping_cents: 490,
    total_cents: 4490,
    missing_for_free_shipping_cents: 1000,
    shipping_flat_cents: 490,
    free_shipping_from_cents: 5000,
    currency: 'EUR',
    taxes: [
      { tax_class: 'standard', rate_bp: 1900, gross_cents: 3368, net_cents: 2830, vat_cents: 538 },
      { tax_class: 'reduced', rate_bp: 700, gross_cents: 1122, net_cents: 1049, vat_cents: 73 },
    ],
  },
};

/** Derselbe Warenkorb, nachdem die Pfanne 32,00 € kostet. */
export const repricedCart: CartSnapshot = {
  ...mixedCart,
  items: [cartItem({ unit_price_cents: 3200, line_total_cents: 3200 }), mixedCart.items[1]],
  subtotal_cents: 4200,
  pricing: {
    ...mixedCart.pricing,
    goods_cents: 4200,
    total_cents: 4690,
    missing_for_free_shipping_cents: 800,
    taxes: [
      { tax_class: 'standard', rate_bp: 1900, gross_cents: 3573, net_cents: 3003, vat_cents: 570 },
      { tax_class: 'reduced', rate_bp: 700, gross_cents: 1117, net_cents: 1044, vat_cents: 73 },
    ],
  },
};

export const sampleOrder: Order = {
  id: 'order-id',
  order_number: 'NW-2026-100001',
  status: 'CREATED',
  customer_email: 'kunde@example.invalid',
  address: {
    first_name: 'Erika',
    last_name: 'Musterfrau',
    company: '',
    street: 'Heidestraße',
    house_number: '17',
    address_extra: 'Hinterhaus',
    postal_code: '51147',
    city: 'Köln',
    country: 'DE',
  },
  items: [
    {
      position: 1,
      variant_id: 'pan-id',
      product_name: 'Pfanne',
      variant_label: '28 cm',
      sku: 'NW-PFANNE',
      quantity: 1,
      unit_price_cents: 3200,
      line_total_cents: 3200,
      tax_class: 'standard',
      tax_rate_bp: 1900,
    },
    {
      position: 2,
      variant_id: 'book-id',
      product_name: 'Kochbuch',
      variant_label: 'Gebunden',
      sku: 'NW-BUCH',
      quantity: 1,
      unit_price_cents: 1000,
      line_total_cents: 1000,
      tax_class: 'reduced',
      tax_rate_bp: 700,
    },
  ],
  pricing: {
    goods_cents: 4200,
    discount_cents: 0,
    shipping_cents: 490,
    total_cents: 4690,
    currency: 'EUR',
    taxes: repricedCart.pricing.taxes,
  },
  discount: null,
  shipment: null,
  status_events: [{ from_status: null, to_status: 'CREATED', created_at: '2026-10-30T14:12:00Z' }],
  created_at: '2026-10-30T14:12:00Z',
};
