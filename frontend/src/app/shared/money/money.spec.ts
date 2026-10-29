import { MoneyPipe } from './money.pipe';
import { euros, formatMoney } from './money';

// Intl trennt Betrag und Währungszeichen mit einem geschützten Leerzeichen.
const NBSP = ' ';

describe('formatMoney', () => {
  it('formatiert Cent-Beträge nach de-DE mit Tausenderpunkt und Komma', () => {
    expect(formatMoney(euros(129_999))).toBe(`1.299,99${NBSP}€`);
  });

  it('zeigt kleine und runde Beträge mit zwei Nachkommastellen', () => {
    expect(formatMoney(euros(0))).toBe(`0,00${NBSP}€`);
    expect(formatMoney(euros(5))).toBe(`0,05${NBSP}€`);
    expect(formatMoney(euros(1_000))).toBe(`10,00${NBSP}€`);
  });

  it('verarbeitet große Beträge ohne Rundungsfehler', () => {
    expect(formatMoney(euros(123_456_789_01))).toBe(`123.456.789,01${NBSP}€`);
  });
});

describe('MoneyPipe', () => {
  it('liefert für fehlende Werte einen leeren Text', () => {
    const pipe = new MoneyPipe();
    expect(pipe.transform(null)).toBe('');
    expect(pipe.transform(undefined)).toBe('');
    expect(pipe.transform(euros(1_999))).toBe(`19,99${NBSP}€`);
  });
});
