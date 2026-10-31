import { SalesReport } from '../../data-access/admin-api';
import { salesChartConfig } from './sales-chart';

const report: SalesReport = {
  group_by: 'category',
  period: 'week',
  granularity: 'day',
  currency: 'EUR',
  start_date: '2026-10-25',
  end_date: '2026-10-31',
  buckets: [
    { key: '2026-10-30', label: '30.10.' },
    { key: '2026-10-31', label: '31.10.' },
  ],
  series: [
    {
      key: 'one',
      label: 'Küche',
      category_key: 'one',
      category_label: 'Küche',
      depth: 0,
      values_cents: [1700, 1000],
      total_cents: 2700,
      quantity: 1,
    },
    {
      key: 'two',
      label: 'Bücher',
      category_key: 'two',
      category_label: 'Bücher',
      depth: 0,
      values_cents: [500, 400],
      total_cents: 900,
      quantity: 1,
    },
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

  it('übernimmt die Farben je Reihe, auch für Flächen und Ringe', () => {
    const area = salesChartConfig(report, 'area', ['#112233', '#445566']);
    expect(area.data.datasets[0]).toMatchObject({
      borderColor: '#112233',
      backgroundColor: '#11223333',
    });
    expect(area.data.datasets[1]).toMatchObject({ borderColor: '#445566' });
    const ring = salesChartConfig(report, 'doughnut', ['#112233', '#445566']);
    expect(ring.data.datasets[0].backgroundColor).toEqual(['#112233', '#445566']);
  });

  it('stapelt Balken bei Kategorien und blendet leere Werte im Tooltip aus', () => {
    const config = salesChartConfig(report, 'bar');
    const scales = config.options?.scales as Record<string, { stacked?: boolean }>;
    expect(scales['x'].stacked).toBe(true);
    expect(scales['y'].stacked).toBe(true);
    const filter = config.options?.plugins?.tooltip?.filter as (item: { raw: unknown }) => boolean;
    expect(filter({ raw: 0 })).toBe(false);
    expect(filter({ raw: 12.5 })).toBe(true);
    const total = salesChartConfig(
      { ...report, group_by: 'total', series: [report.series[0]] },
      'bar',
    );
    expect((total.options?.scales as Record<string, { stacked?: boolean }>)['x'].stacked).toBe(
      false,
    );
  });

  it('verzichtet auf schmalen Bildschirmen bei vielen Reihen auf die Legende', () => {
    const many = {
      ...report,
      series: Array.from({ length: 13 }, (_, index) => ({ ...report.series[0], key: `p${index}` })),
    };
    expect(salesChartConfig(many, 'bar', [], true).options?.plugins?.legend?.display).toBe(false);
    expect(salesChartConfig(many, 'bar', [], false).options?.plugins?.legend?.display).toBe(true);
  });
});
