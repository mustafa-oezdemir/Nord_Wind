import { Component, OnDestroy, OnInit, computed, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { apiErrorMessage, toApiError } from '../../../../core/http/api-error';
import { euros, formatMoney } from '../../../../shared/money/money';
import {
  AdminApi,
  SalesCatalog,
  SalesGroupBy,
  SalesPeriod,
  SalesQuery,
  SalesReport,
  SalesSeries,
} from '../../data-access/admin-api';
import { SalesChart, SalesChartType } from './sales-chart';
import { categoryColor, categoryPath, seriesColors } from './sales-colors';

/** Höchstlänge eines eigenen Zeitraums, wie in der API (`analytics.MaxCustomDays`). */
const maxCustomDays = 731;
const dayMs = 24 * 60 * 60 * 1000;
const productStatus: Record<string, string> = { draft: ' (Entwurf)', archived: ' (archiviert)' };

/** Kalendertag in Berlin als YYYY-MM-DD, wie ihn die API erwartet. */
export function berlinDate(date: Date): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: 'Europe/Berlin' }).format(date);
}

function shiftDate(iso: string, days: number): string {
  const date = new Date(`${iso}T00:00:00Z`);
  return new Date(date.getTime() + days * dayMs).toISOString().slice(0, 10);
}

function dayCount(from: string, to: string): number {
  return Math.round((Date.parse(`${to}T00:00:00Z`) - Date.parse(`${from}T00:00:00Z`)) / dayMs) + 1;
}

export function germanDate(iso: string): string {
  const [year, month, day] = iso.split('-');
  return `${day}.${month}.${year}`;
}

interface SeriesRow {
  series: SalesSeries;
  color: string;
}

interface SeriesGroup {
  key: string;
  label: string;
  color: string;
  totalCents: number;
  quantity: number;
  rows: SeriesRow[];
}

@Component({
  selector: 'app-sales-analytics',
  imports: [SalesChart],
  templateUrl: './sales-analytics.html',
  styleUrl: './sales-analytics.scss',
})
export class SalesAnalytics implements OnInit, OnDestroy {
  private readonly api = inject(AdminApi);
  private salesRequest = 0;

  protected readonly today = berlinDate(new Date());
  protected readonly salesGroup = signal<SalesGroupBy | null>(null);
  /** Voreinstellung: die letzten 7 Tage. */
  protected readonly salesPeriod = signal<SalesPeriod>('week');
  protected readonly customFrom = signal(shiftDate(this.today, -6));
  protected readonly customTo = signal(this.today);
  protected readonly customError = signal('');
  protected readonly categoryId = signal('');
  protected readonly productId = signal('');
  protected readonly catalog = signal<SalesCatalog | null>(null);
  protected readonly catalogError = signal('');
  /** Voreinstellung: Balken; das Diagramm erscheint sofort mit den Daten. */
  protected readonly salesChartType = signal<SalesChartType>('bar');
  protected readonly salesReport = signal<SalesReport | null>(null);
  protected readonly salesLoading = signal(false);
  protected readonly salesError = signal('');
  protected readonly salesMoney = (amountCents: number) => formatMoney(euros(amountCents));
  protected readonly germanDate = germanDate;

  /** Kategorien als eingerückter Baum für die Auswahlliste. */
  protected readonly categoryOptions = computed(() =>
    (this.catalog()?.categories ?? []).map((category) => ({
      id: category.id,
      label:
        '   '.repeat(category.depth) +
        (category.depth > 0 ? '↳ ' : '') +
        category.name +
        (category.status === 'inactive' ? ' (inaktiv)' : ''),
    })),
  );

  /** Produkte der gewählten Kategorie samt Unterkategorien, gruppiert nach Kategorie. */
  protected readonly productOptionGroups = computed(() => {
    const catalog = this.catalog();
    if (!catalog) return [];
    const selected = this.categoryId();
    const scope = selected ? this.descendants(catalog, selected) : null;
    const groups: { key: string; label: string; products: { id: string; label: string }[] }[] = [];
    for (const product of catalog.products) {
      const key = product.category_id ?? 'uncategorized';
      if (scope && !scope.has(key)) continue;
      let group = groups.find((entry) => entry.key === key);
      if (!group) {
        group = { key, label: categoryPath(key, catalog, 'Ohne Kategorie'), products: [] };
        groups.push(group);
      }
      group.products.push({
        id: product.id,
        label: product.name + (productStatus[product.status] ?? ''),
      });
    }
    return groups;
  });

  protected readonly colors = computed(() => {
    const report = this.salesReport();
    return report ? seriesColors(report, this.catalog()) : [];
  });

  /** Tabellenzeilen; bei Produkten nach Kategorie gruppiert mit Zwischensumme. */
  protected readonly tableGroups = computed<SeriesGroup[]>(() => {
    const report = this.salesReport();
    if (!report) return [];
    const colors = this.colors();
    const rows = report.series.map((series, index) => ({ series, color: colors[index] }));
    if (report.group_by !== 'product') {
      return [
        { key: 'all', label: '', color: '', totalCents: report.total_cents, quantity: 0, rows },
      ];
    }
    const groups: SeriesGroup[] = [];
    for (const row of rows) {
      let group = groups.find((entry) => entry.key === row.series.category_key);
      if (!group) {
        group = {
          key: row.series.category_key,
          label: categoryPath(row.series.category_key, this.catalog(), row.series.category_label),
          color: categoryColor(row.series.category_key, this.catalog()),
          totalCents: 0,
          quantity: 0,
          rows: [],
        };
        groups.push(group);
      }
      group.rows.push(row);
      group.totalCents += row.series.total_cents;
      group.quantity += row.series.quantity;
    }
    return groups;
  });

  ngOnInit(): void {
    void firstValueFrom(this.api.salesCatalog())
      .then((catalog) => this.catalog.set(catalog))
      .catch((error: unknown) => this.catalogError.set(apiErrorMessage(toApiError(error))));
  }

  protected chooseSalesGroup(group: SalesGroupBy): void {
    this.salesGroup.set(group);
    if (group !== 'product') this.productId.set('');
    this.load();
  }

  protected chooseCategory(id: string): void {
    this.categoryId.set(id);
    const catalog = this.catalog();
    const product = catalog?.products.find((entry) => entry.id === this.productId());
    // Ein Produkt außerhalb der neuen Kategorie wird abgewählt.
    if (
      product &&
      id &&
      catalog &&
      !this.descendants(catalog, id).has(product.category_id ?? 'uncategorized')
    ) {
      this.productId.set('');
    }
    this.load();
  }

  protected chooseProduct(id: string): void {
    this.productId.set(id);
    this.load();
  }

  protected chooseSalesPeriod(period: SalesPeriod): void {
    this.salesPeriod.set(period);
    this.customError.set('');
    this.load();
  }

  protected applyCustomRange(event: Event): void {
    event.preventDefault();
    this.load();
  }

  protected chooseSalesChartType(type: SalesChartType): void {
    this.salesChartType.set(type);
  }

  protected salesBucketTotal(report: SalesReport, index: number): number {
    return report.series.reduce((sum, series) => sum + (series.values_cents[index] ?? 0), 0);
  }

  protected periodText(report: SalesReport): string {
    return `${germanDate(report.start_date)} bis ${germanDate(report.end_date)}`;
  }

  /** Prüft einen eigenen Zeitraum wie die API und liefert eine verständliche Meldung. */
  private customRangeProblem(): string {
    const from = this.customFrom();
    const to = this.customTo();
    if (!from || !to) return 'Bitte Anfangs- und Enddatum wählen.';
    if (from > to) return 'Das Anfangsdatum muss vor dem Enddatum liegen.';
    if (to > this.today) return 'Das Enddatum darf nicht in der Zukunft liegen.';
    if (dayCount(from, to) > maxCustomDays)
      return 'Der Zeitraum darf höchstens zwei Jahre umfassen.';
    return '';
  }

  private load(): void {
    const group = this.salesGroup();
    if (!group) return;
    const query: SalesQuery = { group_by: group, period: this.salesPeriod() };
    if (query.period === 'custom') {
      const problem = this.customRangeProblem();
      this.customError.set(problem);
      if (problem) return;
      query.from = this.customFrom();
      query.to = this.customTo();
    }
    if (group !== 'total' && this.categoryId()) query.category_id = this.categoryId();
    if (group === 'product' && this.productId()) query.product_id = this.productId();
    const request = ++this.salesRequest;
    this.salesLoading.set(true);
    this.salesError.set('');
    void firstValueFrom(this.api.sales(query))
      .then((report) => {
        if (request === this.salesRequest) this.salesReport.set(report);
      })
      .catch((error: unknown) => {
        if (request === this.salesRequest) {
          this.salesReport.set(null);
          this.salesError.set(apiErrorMessage(toApiError(error)));
        }
      })
      .finally(() => {
        if (request === this.salesRequest) this.salesLoading.set(false);
      });
  }

  private descendants(catalog: SalesCatalog, root: string): Set<string> {
    const scope = new Set([root]);
    for (let changed = true; changed;) {
      changed = false;
      for (const category of catalog.categories) {
        if (!scope.has(category.id) && category.parent_id && scope.has(category.parent_id)) {
          scope.add(category.id);
          changed = true;
        }
      }
    }
    return scope;
  }

  ngOnDestroy(): void {
    this.salesRequest++;
  }
}
