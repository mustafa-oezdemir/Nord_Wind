import { formatPageTitle } from './page-title-strategy';

describe('formatPageTitle', () => {
  it('hängt den Shopnamen an', () => {
    expect(formatPageTitle('Alle Produkte')).toBe('Alle Produkte | NordWinds');
  });

  it('nutzt ohne Seitentitel nur den Shopnamen', () => {
    expect(formatPageTitle()).toBe('NordWinds');
    expect(formatPageTitle('   ')).toBe('NordWinds');
  });
});
