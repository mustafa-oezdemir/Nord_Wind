import { Component, Input, OnChanges } from '@angular/core';
import { SalesReport } from '../../data-access/admin-api';

export type SalesChartType = 'bar' | 'line' | 'area' | 'doughnut';

interface ChartSeries {
  label: string;
  color: string;
  path: string;
  area: string;
  points: { x: number; y: number }[];
}

interface ChartBar {
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
}

interface ChartSlice {
  label: string;
  color: string;
  path: string;
  percentage: number;
}

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
const plot = { left: 66, right: 780, top: 20, bottom: 262 };

@Component({
  selector: 'app-sales-chart',
  template: `
    <div class="chart-shell">
      <div class="chart-plot">
        @if (chartType === 'doughnut') {
          <svg viewBox="0 0 360 320" role="img" [attr.aria-label]="accessibleTitle">
            @for (slice of slices; track slice.label) {
              <path [attr.d]="slice.path" [attr.fill]="slice.color" />
            }
            <circle cx="180" cy="155" r="57" fill="white" />
            <text x="180" y="149" text-anchor="middle" class="chart-center-label">Gesamt</text>
            <text x="180" y="172" text-anchor="middle" class="chart-center-value">
              {{ totalLabel }}
            </text>
          </svg>
        } @else {
          <svg viewBox="0 0 800 320" role="img" [attr.aria-label]="accessibleTitle">
            @for (tick of ticks; track tick.y) {
              <line
                [attr.x1]="plot.left"
                [attr.x2]="plot.right"
                [attr.y1]="tick.y"
                [attr.y2]="tick.y"
                class="chart-grid"
              />
              <text x="56" [attr.y]="tick.y + 4" text-anchor="end" class="chart-axis">
                {{ tick.label }}
              </text>
            }
            @if (chartType === 'bar') {
              @for (bar of bars; track $index) {
                <rect
                  [attr.x]="bar.x"
                  [attr.y]="bar.y"
                  [attr.width]="bar.width"
                  [attr.height]="bar.height"
                  [attr.fill]="bar.color"
                  rx="2"
                />
              }
            } @else {
              @for (series of lines; track series.label) {
                @if (chartType === 'area') {
                  <path [attr.d]="series.area" [attr.fill]="series.color" fill-opacity="0.16" />
                }
                <path
                  [attr.d]="series.path"
                  [attr.stroke]="series.color"
                  fill="none"
                  stroke-width="2.5"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                />
                @if (report.buckets.length <= 7) {
                  @for (point of series.points; track $index) {
                    <circle
                      [attr.cx]="point.x"
                      [attr.cy]="point.y"
                      r="3.5"
                      [attr.fill]="series.color"
                    />
                  }
                }
              }
            }
            @for (label of xLabels; track label.key) {
              <text [attr.x]="label.x" y="289" text-anchor="middle" class="chart-axis">
                {{ label.text }}
              </text>
            }
          </svg>
        }
      </div>
      @if (report.group_by !== 'total') {
        <ul class="chart-legend" aria-label="Datenreihen">
          @for (item of legend; track item.label) {
            <li>
              <span [style.background]="item.color" aria-hidden="true"></span>{{ item.label }}
            </li>
          }
        </ul>
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
    .chart-plot {
      overflow-x: auto;
    }
    svg {
      display: block;
      width: 100%;
      min-width: 520px;
      max-height: 360px;
      margin: auto;
    }
    svg[viewBox='0 0 360 320'] {
      min-width: 0;
      max-width: 360px;
    }
    .chart-grid {
      stroke: #e7edf0;
      stroke-width: 1;
    }
    .chart-axis {
      fill: #647481;
      font-size: 12px;
      font-family: inherit;
    }
    .chart-center-label {
      fill: #647481;
      font-size: 12px;
      font-family: inherit;
    }
    .chart-center-value {
      fill: #16323e;
      font-size: 18px;
      font-weight: 700;
      font-family: inherit;
    }
    .chart-legend {
      display: flex;
      flex-wrap: wrap;
      gap: 0.5rem 1rem;
      margin: 1rem 0 0;
      padding: 0;
      list-style: none;
      color: #425762;
      font-size: 0.85rem;
    }
    .chart-legend li {
      display: inline-flex;
      align-items: center;
      gap: 0.4rem;
    }
    .chart-legend span {
      display: inline-block;
      width: 0.7rem;
      height: 0.7rem;
      border-radius: 50%;
    }
  `,
})
export class SalesChart implements OnChanges {
  @Input({ required: true }) report!: SalesReport;
  @Input({ required: true }) chartType!: SalesChartType;

  protected readonly plot = plot;
  protected ticks: { y: number; label: string }[] = [];
  protected bars: ChartBar[] = [];
  protected lines: ChartSeries[] = [];
  protected slices: ChartSlice[] = [];
  protected legend: { label: string; color: string }[] = [];
  protected xLabels: { key: string; x: number; text: string }[] = [];
  protected totalLabel = '';
  protected accessibleTitle = '';

  ngOnChanges(): void {
    if (!this.report || !this.chartType) return;
    const { report } = this;
    this.totalLabel =
      new Intl.NumberFormat('de-DE', { notation: 'compact', maximumFractionDigits: 1 }).format(
        report.total_cents / 100,
      ) + ' €';
    this.accessibleTitle = `${this.chartType === 'doughnut' ? 'Ring' : this.chartType === 'bar' ? 'Balken' : this.chartType === 'area' ? 'Flächen' : 'Linien'}diagramm: ${report.group_by === 'total' ? 'Gesamtumsatz' : report.group_by === 'category' ? 'Kategorieumsatz' : 'Produktumsatz'} vom ${report.start_date} bis ${report.end_date}. Werte in der folgenden Tabelle.`;
    this.legend = report.series.map((series, index) => ({
      label: series.label,
      color: colors[index % colors.length],
    }));

    const max = Math.max(1, ...report.series.flatMap((series) => series.values_cents));
    const ceiling = niceCeiling(max);
    this.ticks = Array.from({ length: 5 }, (_, index) => ({
      y: plot.bottom - (index / 4) * (plot.bottom - plot.top),
      label: compactEuro((ceiling * index) / 4),
    }));
    const bucketCount = report.buckets.length;
    const bucketWidth = (plot.right - plot.left) / Math.max(bucketCount, 1);
    const x = (index: number) => plot.left + bucketWidth * (index + 0.5);
    const y = (value: number) => plot.bottom - (value / ceiling) * (plot.bottom - plot.top);
    const labelStep = bucketCount > 15 ? 5 : bucketCount > 7 ? 3 : 1;
    this.xLabels = report.buckets
      .map((bucket, index) => ({ key: bucket.key, x: x(index), text: bucket.label, index }))
      .filter((bucket) => bucket.index % labelStep === 0 || bucket.index === bucketCount - 1);
    this.bars = [];
    this.lines = [];
    report.series.forEach((series, seriesIndex) => {
      const color = colors[seriesIndex % colors.length];
      const points = report.buckets.map((_, index) => ({
        x: x(index),
        y: y(series.values_cents[index] ?? 0),
      }));
      this.lines.push({
        label: series.label,
        color,
        points,
        path: points.map((point, index) => `${index ? 'L' : 'M'}${point.x} ${point.y}`).join(' '),
        area: points.length
          ? `M${points[0].x} ${plot.bottom} ` +
            points.map((point) => `L${point.x} ${point.y}`).join(' ') +
            ` L${points[points.length - 1].x} ${plot.bottom} Z`
          : '',
      });
      const barWidth = Math.max(
        1,
        Math.min(22, (bucketWidth * 0.78) / Math.max(report.series.length, 1)),
      );
      report.buckets.forEach((_, index) => {
        const value = series.values_cents[index] ?? 0;
        if (value <= 0) return;
        const height = plot.bottom - y(value);
        this.bars.push({
          x: x(index) - (barWidth * report.series.length) / 2 + barWidth * seriesIndex,
          y: y(value),
          width: barWidth - 1,
          height,
          color,
        });
      });
    });
    this.slices = [];
    const total = report.series.reduce((sum, series) => sum + series.total_cents, 0);
    let angle = -Math.PI / 2;
    report.series.forEach((series, index) => {
      if (series.total_cents <= 0 || total <= 0) return;
      const next = angle + (series.total_cents / total) * Math.PI * 2;
      this.slices.push({
        label: series.label,
        color: colors[index % colors.length],
        path: arcPath(180, 155, 116, angle, next),
        percentage: Math.round((series.total_cents / total) * 100),
      });
      angle = next;
    });
  }
}

function niceCeiling(value: number): number {
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const scaled = value / magnitude;
  return (scaled <= 1 ? 1 : scaled <= 2 ? 2 : scaled <= 5 ? 5 : 10) * magnitude;
}

function compactEuro(cents: number): string {
  return (
    new Intl.NumberFormat('de-DE', { notation: 'compact', maximumFractionDigits: 1 }).format(
      cents / 100,
    ) + ' €'
  );
}

function arcPath(cx: number, cy: number, radius: number, start: number, end: number): string {
  // Ein voller Kreis braucht zwei SVG-Bögen, sonst verschwindet das Segment.
  if (end - start >= Math.PI * 2 - 0.0001) {
    const middle = start + Math.PI;
    return `${arcPath(cx, cy, radius, start, middle)} ${arcPath(cx, cy, radius, middle, end)}`;
  }
  const sx = cx + radius * Math.cos(start);
  const sy = cy + radius * Math.sin(start);
  const ex = cx + radius * Math.cos(end);
  const ey = cy + radius * Math.sin(end);
  return `M${cx} ${cy} L${sx} ${sy} A${radius} ${radius} 0 ${end - start > Math.PI ? 1 : 0} 1 ${ex} ${ey} Z`;
}
