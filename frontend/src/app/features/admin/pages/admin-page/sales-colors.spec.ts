import { SalesCatalog, SalesReport, SalesSeries } from '../../data-access/admin-api';
import { categoryColor, seriesColors, totalColor } from './sales-colors';

const catalog: SalesCatalog = {
  categories: [
    { id: 'kueche', name: 'Küche', status: 'active', depth: 0 },
    { id: 'pfannen', name: 'Pfannen', parent_id: 'kueche', status: 'active', depth: 1 },
    { id: 'garten', name: 'Garten', status: 'active', depth: 0 },
    { id: 'buero', name: 'Büro', status: 'inactive', depth: 0 },
  ],
  products: [],
};

function series(key: string, categoryKey: string): SalesSeries {
  return {
    key,
    label: key,
    category_key: categoryKey,
    category_label: categoryKey,
    depth: 0,
    values_cents: [0],
    total_cents: 0,
    quantity: 0,
  };
}

function report(groupBy: SalesReport['group_by'], entries: SalesSeries[]): SalesReport {
  return {
    group_by: groupBy,
    period: 'week',
    granularity: 'day',
    currency: 'EUR',
    start_date: '2026-10-25',
    end_date: '2026-10-31',
    buckets: [{ key: '2026-10-31', label: '31.10.' }],
    series: entries,
    total_cents: 0,
  };
}

const hue = (hex: string) => {
  const [r, g, b] = [1, 3, 5].map((index) => parseInt(hex.slice(index, index + 2), 16) / 255);
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const delta = max - min;
  const raw =
    max === r ? ((g - b) / delta) % 6 : max === g ? (b - r) / delta + 2 : (r - g) / delta + 4;
  return Math.round((raw * 60 + 360) % 360);
};
const lightness = (hex: string) => {
  const values = [1, 3, 5].map((index) => parseInt(hex.slice(index, index + 2), 16));
  return (Math.max(...values) + Math.min(...values)) / 510;
};

describe('seriesColors', () => {
  it('gibt jeder obersten Kategorie einen eigenen Farbton und Unterkategorien einen helleren', () => {
    const colors = seriesColors(
      report('category', [
        series('kueche', 'kueche'),
        series('pfannen', 'pfannen'),
        series('garten', 'garten'),
        series('buero', 'buero'),
      ]),
      catalog,
    );
    expect(colors.every((color) => /^#[0-9a-f]{6}$/.test(color))).toBe(true);
    expect(new Set([colors[0], colors[2], colors[3]]).size).toBe(3);
    expect(Math.abs(hue(colors[0]) - hue(colors[1]))).toBeLessThan(3);
    expect(lightness(colors[1])).toBeGreaterThan(lightness(colors[0]));
  });

  it('bleibt bei einem Filter gleich, damit eine Kategorie überall dieselbe Farbe hat', () => {
    const all = seriesColors(
      report('category', [series('kueche', 'kueche'), series('garten', 'garten')]),
      catalog,
    );
    const filtered = seriesColors(report('category', [series('garten', 'garten')]), catalog);
    expect(filtered[0]).toBe(all[1]);
    expect(categoryColor('garten', catalog)).toBe(all[1]);
  });

  it('färbt Produkte in Abstufungen ihrer Kategorie', () => {
    const colors = seriesColors(
      report('product', [
        series('pfanne', 'kueche'),
        series('topf', 'kueche'),
        series('messer', 'kueche'),
        series('schere', 'garten'),
      ]),
      catalog,
    );
    const kitchen = hue(categoryColor('kueche', catalog));
    for (const color of colors.slice(0, 3)) expect(Math.abs(hue(color) - kitchen)).toBeLessThan(3);
    expect(new Set(colors.slice(0, 3)).size).toBe(3);
    expect(lightness(colors[0])).toBeLessThan(lightness(colors[2]));
    expect(colors[3]).toBe(categoryColor('garten', catalog));
  });

  it('nutzt Grau ohne Kategorie und die Markenfarbe für den Gesamtumsatz', () => {
    const [none] = seriesColors(report('product', [series('gutschein', 'uncategorized')]), catalog);
    const [r, g, b] = [1, 3, 5].map((index) => parseInt(none.slice(index, index + 2), 16));
    expect(Math.max(r, g, b) - Math.min(r, g, b)).toBeLessThan(25);
    expect(seriesColors(report('total', [series('total', '')]), catalog)).toEqual([totalColor]);
  });
});
