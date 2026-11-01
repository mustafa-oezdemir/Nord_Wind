import { BerlinDatePipe } from './berlin-date.pipe';

describe('BerlinDatePipe', () => {
  const pipe = new BerlinDatePipe();

  it('formatiert UTC-Zeitpunkte im Sommer und Winter in Berliner Ortszeit', () => {
    expect(pipe.transform('2026-10-30T14:12:00Z', 'longDateTime')).toBe('30. Oktober 2026, 15:12');
    expect(pipe.transform('2026-07-01T23:30:00Z', 'dateTime')).toBe('02.07.2026 01:30');
    expect(pipe.transform('2026-12-31T23:30:00Z', 'date')).toBe('01.01.2027');
  });
});
