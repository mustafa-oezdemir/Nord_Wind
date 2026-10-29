import { normalizeSearch, pageFromParam, sortFromParam, sortToParam } from './catalog-query';

describe('Katalog-URL-Parameter', () => {
  it('übersetzt deutsche Sortierparameter in API-Werte und zurück', () => {
    expect(sortFromParam('preis-auf')).toBe('price_asc');
    expect(sortFromParam('preis-ab')).toBe('price_desc');
    expect(sortFromParam('name')).toBe('name_asc');
    expect(sortToParam('price_desc')).toBe('preis-ab');
  });

  it('fällt bei unbekannter Sortierung auf „Neueste zuerst“ zurück', () => {
    expect(sortFromParam(undefined)).toBe('newest');
    expect(sortFromParam('price_asc; DROP TABLE products')).toBe('newest');
  });

  it('akzeptiert nur gültige Seitenzahlen', () => {
    expect(pageFromParam('3')).toBe(3);
    for (const invalid of [undefined, '', '0', '-1', '1.5', 'abc', '10001', '99999999']) {
      expect(pageFromParam(invalid)).toBe(1);
    }
  });

  it('entfernt Leerraum und begrenzt die Suche auf 100 Zeichen', () => {
    expect(normalizeSearch('  Kopfhörer  ')).toBe('Kopfhörer');
    expect(normalizeSearch('   ')).toBeUndefined();
    expect(normalizeSearch(undefined)).toBeUndefined();
    expect(normalizeSearch('ä'.repeat(120))).toBe('ä'.repeat(100));
  });
});
