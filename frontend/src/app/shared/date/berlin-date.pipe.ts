import { Pipe, PipeTransform } from '@angular/core';

export type BerlinDateFormat = 'date' | 'dateTime' | 'longDateTime';

const formatters: Record<BerlinDateFormat, Intl.DateTimeFormat> = {
  date: new Intl.DateTimeFormat('de-DE', {
    timeZone: 'Europe/Berlin',
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  }),
  dateTime: new Intl.DateTimeFormat('de-DE', {
    timeZone: 'Europe/Berlin',
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }),
  longDateTime: new Intl.DateTimeFormat('de-DE', {
    timeZone: 'Europe/Berlin',
    day: '2-digit',
    month: 'long',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }),
};

/** Formatiert Bestellzeitpunkte unabhängig von der Zeitzone des Browsers oder SSR-Hosts. */
@Pipe({ name: 'berlinDate' })
export class BerlinDatePipe implements PipeTransform {
  transform(
    value: string | number | Date | null | undefined,
    format: BerlinDateFormat = 'dateTime',
  ) {
    if (value == null) return '';
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return '';
    const parts = Object.fromEntries(
      formatters[format].formatToParts(date).map(({ type, value: part }) => [type, part]),
    );
    const day = parts['day'];
    const month = parts['month'];
    const year = parts['year'];
    if (format === 'date') return `${day}.${month}.${year}`;
    const time = `${parts['hour']}:${parts['minute']}`;
    return format === 'longDateTime'
      ? `${Number(day)}. ${month} ${year}, ${time}`
      : `${day}.${month}.${year} ${time}`;
  }
}
