import { Component, OnDestroy, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { apiErrorMessage, toApiError } from '../../../../core/http/api-error';
import { euros, formatMoney } from '../../../../shared/money/money';
import { AdminApi, SalesGroupBy, SalesPeriod, SalesReport } from '../../data-access/admin-api';
import { SalesChart, SalesChartType } from './sales-chart';

@Component({
  selector: 'app-sales-analytics',
  imports: [SalesChart],
  templateUrl: './sales-analytics.html',
  styleUrl: './sales-analytics.scss',
})
export class SalesAnalytics implements OnDestroy {
  private readonly api = inject(AdminApi);
  private salesRequest = 0;

  protected readonly salesGroup = signal<SalesGroupBy | null>(null);
  protected readonly salesPeriod = signal<SalesPeriod | null>(null);
  protected readonly salesChartType = signal<SalesChartType | null>(null);
  protected readonly salesReport = signal<SalesReport | null>(null);
  protected readonly salesLoading = signal(false);
  protected readonly salesError = signal('');
  protected readonly salesMoney = (amountCents: number) => formatMoney(euros(amountCents));

  protected chooseSalesGroup(group: SalesGroupBy): void {
    this.salesRequest++;
    this.salesGroup.set(group);
    this.salesPeriod.set(null);
    this.salesChartType.set(null);
    this.salesReport.set(null);
    this.salesLoading.set(false);
    this.salesError.set('');
  }

  protected chooseSalesPeriod(period: SalesPeriod): void {
    const group = this.salesGroup();
    if (!group) return;
    const request = ++this.salesRequest;
    this.salesPeriod.set(period);
    this.salesChartType.set(null);
    this.salesReport.set(null);
    this.salesLoading.set(true);
    this.salesError.set('');
    void firstValueFrom(this.api.sales(group, period))
      .then((report) => {
        if (request === this.salesRequest) this.salesReport.set(report);
      })
      .catch((error: unknown) => {
        if (request === this.salesRequest) this.salesError.set(apiErrorMessage(toApiError(error)));
      })
      .finally(() => {
        if (request === this.salesRequest) this.salesLoading.set(false);
      });
  }

  protected chooseSalesChartType(type: SalesChartType): void {
    if (this.salesReport()) this.salesChartType.set(type);
  }

  protected salesBucketTotal(report: SalesReport, index: number): number {
    return report.series.reduce((sum, series) => sum + (series.values_cents[index] ?? 0), 0);
  }

  ngOnDestroy(): void {
    this.salesRequest++;
  }
}
