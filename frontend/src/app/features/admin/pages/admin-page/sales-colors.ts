import { SalesCatalog, SalesReport } from '../../data-access/admin-api';

/**
 * Farbtöne der obersten Kategorien in Katalogreihenfolge: zwölf Töne im Abstand von 30° auf dem
 * Farbkreis, beginnend mit dem Fjord-Blau der Marke und so sortiert, dass benachbarte Kategorien
 * weit auseinanderliegen. Ab der 13. Kategorie geht es im goldenen Winkel weiter.
 */
const rootHues = [193, 43, 283, 133, 343, 73, 223, 13, 163, 313, 103, 253];
const saturation = 62;
/** Helligkeit nach Tiefe: Unterkategorien sind hellere Töne ihrer Oberkategorie. */
const depthLightness = [36, 47, 56];
const uncategorized = { hue: 205, saturation: 8, lightness: 46 };
export const totalColor = '#126f87';

interface Tone {
  hue: number;
  saturation: number;
  lightness: number;
}

function hex({ hue, saturation: s, lightness: l }: Tone): string {
  const sat = s / 100;
  const light = l / 100;
  const k = (n: number) => (n + hue / 30) % 12;
  const a = sat * Math.min(light, 1 - light);
  const channel = (n: number) =>
    Math.round(255 * (light - a * Math.max(-1, Math.min(k(n) - 3, 9 - k(n), 1))))
      .toString(16)
      .padStart(2, '0');
  return `#${channel(0)}${channel(8)}${channel(4)}`;
}

/** Farbton jeder Kategorie, unabhängig vom gewählten Filter immer gleich. */
export function categoryTones(catalog: SalesCatalog | null): Map<string, Tone> {
  const tones = new Map<string, Tone>([['uncategorized', uncategorized]]);
  if (!catalog) return tones;
  const parents = new Map(catalog.categories.map((category) => [category.id, category.parent_id]));
  const roots = catalog.categories.filter(
    (category) => !category.parent_id || !parents.has(category.parent_id),
  );
  const rootHue = new Map(
    roots.map((root, index) => [
      root.id,
      index < rootHues.length ? rootHues[index] : (193 + index * 137.508) % 360,
    ]),
  );
  for (const category of catalog.categories) {
    let root = category.id;
    let depth = 0;
    for (
      let parent = parents.get(root);
      parent && parents.has(parent) && depth < 20;
      parent = parents.get(root)
    ) {
      root = parent;
      depth++;
    }
    tones.set(category.id, {
      hue: rootHue.get(root) ?? 193,
      saturation,
      lightness: depthLightness[Math.min(depth, depthLightness.length - 1)],
    });
  }
  return tones;
}

/** Pfad einer Kategorie im Baum, z. B. „Küche & Haushalt › Pfannen & Töpfe“. */
export function categoryPath(key: string, catalog: SalesCatalog | null, fallback: string): string {
  const categories = new Map(
    (catalog?.categories ?? []).map((category) => [category.id, category]),
  );
  const names: string[] = [];
  for (
    let current = categories.get(key);
    current && names.length < 20;
    current = current.parent_id ? categories.get(current.parent_id) : undefined
  ) {
    names.unshift(current.name);
  }
  return names.length ? names.join(' › ') : fallback;
}

/** Farbe einer Kategorie (oder „Ohne Kategorie“) für Legenden und Gruppenzeilen. */
export function categoryColor(key: string, catalog: SalesCatalog | null): string {
  return hex(categoryTones(catalog).get(key) ?? uncategorized);
}

/** Farbe je Reihe: Kategorien in ihrem Ton, Produkte als Abstufungen ihrer Kategorie. */
export function seriesColors(report: SalesReport, catalog: SalesCatalog | null): string[] {
  if (report.group_by === 'total') return report.series.map(() => totalColor);
  const tones = categoryTones(catalog);
  const fallback = (index: number): Tone => ({
    hue: rootHues[index % rootHues.length],
    saturation,
    lightness: 40,
  });
  if (report.group_by === 'category') {
    return report.series.map((series, index) => hex(tones.get(series.key) ?? fallback(index)));
  }
  const groupSize = new Map<string, number>();
  for (const series of report.series) {
    groupSize.set(series.category_key, (groupSize.get(series.category_key) ?? 0) + 1);
  }
  const position = new Map<string, number>();
  let unknown = 0;
  return report.series.map((series) => {
    const tone = tones.get(series.category_key) ?? fallback(unknown++);
    const size = groupSize.get(series.category_key) ?? 1;
    const index = position.get(series.category_key) ?? 0;
    position.set(series.category_key, index + 1);
    // Abstufung von dunkel nach hell rund um den Kategorieton (28 % bis 58 % Helligkeit).
    const lightness = size === 1 ? tone.lightness : 28 + (30 * index) / (size - 1);
    return hex({ ...tone, lightness });
  });
}
