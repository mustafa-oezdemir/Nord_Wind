import { isPlatformBrowser } from '@angular/common';
import {
  AfterViewInit,
  Component,
  ElementRef,
  Input,
  OnChanges,
  OnDestroy,
  PLATFORM_ID,
  ViewChild,
  inject,
  signal,
} from '@angular/core';
import type { ChartConfiguration } from 'chart.js';
import { SalesReport } from '../../data-access/admin-api';

export type SalesChartType = 'bar' | 'line' | 'area' | 'doughnut';

const colors = [
  '#126f87',
  '#edaf3f',
  '#5448a9',
  '#178b74',
  '#bf5b56',
  '#6389a8',
  '#a56d9c',
  '#7e9654',
  '#77838c',
];
const euro = new Intl.NumberFormat('de-DE', { style: 'currency', currency: 'EUR' });

/** Chart.js erhält Euro-Werte für Achse und Tooltip; die API rechnet weiterhin in Cent. */
export function salesChartConfig(
  report: SalesReport,
  chartType: SalesChartType,
): ChartConfiguration {
  const doughnut = chartType === 'doughnut';
  const area = chartType === 'area';
  const type = doughnut ? 'doughnut' : chartType === 'bar' ? 'bar' : 'line';
  const datasets = doughnut
    ? [
        {
          label: 'Umsatz',
          data: report.series.map((series) => series.total_cents / 100),
          backgroundColor: report.series.map((_, index) => colors[index % colors.length]),
          borderColor: '#ffffff',
          borderWidth: 2,
        },
      ]
    : report.series.map((series, index) => ({
        label: series.label,
        data: series.values_cents.map((cents) => cents / 100),
        borderColor: colors[index % colors.length],
        backgroundColor: area
          ? `${colors[index % colors.length]}33`
          : colors[index % colors.length],
        borderWidth: 2,
        borderRadius: chartType === 'bar' ? 3 : 0,
        fill: area,
        tension: 0.28,
        pointRadius: report.buckets.length <= 7 ? 3 : 0,
        pointHoverRadius: 5,
      }));

  return {
    type,
    data: {
      labels: doughnut
        ? report.series.map((series) => series.label)
        : report.buckets.map((bucket) => bucket.label),
      datasets,
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      animation: false,
      interaction: { mode: 'index', intersect: false },
      plugins: {
        legend: {
          display: report.group_by !== 'total',
          position: 'bottom',
          labels: { usePointStyle: true },
        },
        tooltip: {
          callbacks: {
            label: (context) =>
              `${doughnut ? context.label : `${context.dataset.label} · ${context.label}`}: ${euro.format(Number(context.raw) || 0)}`,
          },
        },
      },
      scales: doughnut
        ? {}
        : {
            x: {
              grid: { display: false },
              ticks: { maxTicksLimit: report.buckets.length > 15 ? 7 : 10 },
            },
            y: { beginAtZero: true, ticks: { callback: (value) => euro.format(Number(value)) } },
          },
    },
  };
}

@Component({
  selector: 'app-sales-chart',
  template: `
    <div class="chart-shell">
      <div class="chart-frame"><canvas #canvas aria-hidden="true"></canvas></div>
      @if (failed()) {
        <p role="status">
          Das Diagramm konnte nicht geladen werden. Die Werte stehen in der Tabelle.
        </p>
      }
    </div>
  `,
  styles: `
    .chart-shell {
      border: 1px solid #dce6ea;
      border-radius: 1rem;
      background: #fff;
      padding: 1rem;
    }
    .chart-frame {
      position: relative;
      width: 100%;
      height: clamp(18rem, 32vw, 25rem);
    }
    p {
      margin: 0.75rem 0 0;
      color: #425762;
    }
  `,
})
export class SalesChart implements AfterViewInit, OnChanges, OnDestroy {
  @Input({ required: true }) report!: SalesReport;
  @Input({ required: true }) chartType!: SalesChartType;
  @ViewChild('canvas') private canvas?: ElementRef<HTMLCanvasElement>;

  private readonly browser = isPlatformBrowser(inject(PLATFORM_ID));
  private chart: { destroy(): void } | null = null;
  private generation = 0;
  protected readonly failed = signal(false);

  ngAfterViewInit(): void {
    void this.render();
  }

  ngOnChanges(): void {
    if (this.canvas) void this.render();
  }

  ngOnDestroy(): void {
    this.generation++;
    this.chart?.destroy();
    this.chart = null;
  }

  private async render(): Promise<void> {
    if (!this.browser || !this.canvas || !this.report || !this.chartType) return;
    const generation = ++this.generation;
    this.failed.set(false);
    try {
      const context = this.canvas.nativeElement.getContext('2d');
      if (!context) throw new Error('Canvas ist nicht verfügbar');
      const chartjs = await import('chart.js');
      if (generation !== this.generation) return;
      chartjs.Chart.register(
        chartjs.BarController,
        chartjs.BarElement,
        chartjs.CategoryScale,
        chartjs.LinearScale,
        chartjs.LineController,
        chartjs.LineElement,
        chartjs.PointElement,
        chartjs.DoughnutController,
        chartjs.ArcElement,
        chartjs.Filler,
        chartjs.Legend,
        chartjs.Tooltip,
      );
      this.chart?.destroy();
      this.chart = new chartjs.Chart(context, salesChartConfig(this.report, this.chartType));
    } catch {
      if (generation === this.generation) this.failed.set(true);
    }
  }
}
