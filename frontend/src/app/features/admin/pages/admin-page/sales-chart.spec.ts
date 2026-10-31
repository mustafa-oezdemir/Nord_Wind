import { SalesReport } from '../../data-access/admin-api';
import { salesChartConfig } from './sales-chart';

const report: SalesReport = {
  group_by: 'category',
  period: 'week',
  currency: 'EUR',
  start_date: '2026-10-25',
  end_date: '2026-10-31',
  buckets: [
    { key: '2026-10-30', label: '30.10.' },
    { key: '2026-10-31', label: '31.10.' },
  ],
  series: [
    { key: 'one', label: 'Küche', values_cents: [1700, 1000], total_cents: 2700, quantity: 1 },
    { key: 'two', label: 'Bücher', values_cents: [500, 400], total_cents: 900, quantity: 1 },
  ],
  total_cents: 3600,
};

describe('salesChartConfig', () => {
  it('stellt Flächen aus Tageswerten in Euro dar', () => {
    const config = salesChartConfig(report, 'area');
    expect(config.type).toBe('line');
    expect(config.data.labels).toEqual(['30.10.', '31.10.']);
    expect(config.data.datasets[0]).toMatchObject({ label: 'Küche', data: [17, 10], fill: true });
    expect(config.data.datasets[1]).toMatchObject({ label: 'Bücher', data: [5, 4], fill: true });
  });

  it('stellt beim Ringdiagramm die Summen je Kategorie dar', () => {
    const config = salesChartConfig(report, 'doughnut');
    expect(config.type).toBe('doughnut');
    expect(config.data.labels).toEqual(['Küche', 'Bücher']);
    expect(config.data.datasets[0].data).toEqual([27, 9]);
  });
});
