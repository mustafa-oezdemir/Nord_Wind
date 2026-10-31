import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import {
  AdminApi,
  AdminCategory,
  AdminDiscountCode,
  AdminProduct,
  AuthSession,
  SalesReport,
} from '../../data-access/admin-api';
import { AdminPage } from './admin-page';

const category: AdminCategory = {
  id: 'category-id',
  parent_id: null,
  name: 'Küche',
  slug: 'kueche',
  description: '',
  status: 'active',
  sort_order: 0,
};

const product: AdminProduct = {
  id: 'product-id',
  name: 'Pfanne',
  slug: 'pfanne',
  description: '',
  status: 'published',
  tax_class: 'standard',
};

const discount: AdminDiscountCode = {
  id: 'discount-id',
  code: 'SOMMER10',
  percent_bp: 1000,
  is_active: true,
  valid_from: null,
  valid_until: null,
  created_at: '2026-10-31T00:00:00Z',
  updated_at: '2026-10-31T00:00:00Z',
};

const salesReport: SalesReport = {
  group_by: 'category',
  period: 'month',
  currency: 'EUR',
  start_date: '2026-10-02',
  end_date: '2026-10-31',
  buckets: [{ key: '2026-10-31', label: '31.10.' }],
  series: [
    { key: 'category-id', label: 'Küche', values_cents: [2700], total_cents: 2700, quantity: 1 },
  ],
  total_cents: 2700,
};

const sales = vi.fn(() => of(salesReport));

describe('AdminPage', () => {
  async function render(role: 'admin' | 'employee'): Promise<{
    fixture: ComponentFixture<AdminPage>;
    element: HTMLElement;
  }> {
    const session: AuthSession = {
      customer: { id: 'staff-id', email: 'staff@example.invalid', role },
      csrf_token: 'synthetic-csrf',
      expires_at: '2026-11-01T00:00:00Z',
    };
    TestBed.configureTestingModule({
      providers: [
        provideRouter([]),
        {
          provide: AdminApi,
          useValue: {
            session: () => of(session),
            listCategories: () => of({ data: [category] }),
            listProducts: () => of({ data: [product] }),
            listDiscountCodes: () => of({ data: [discount] }),
            audit: () => of({ data: [] }),
            sales,
          },
        },
      ],
    });
    const fixture = TestBed.createComponent(AdminPage);
    fixture.detectChanges();
    await fixture.whenStable();
    await new Promise((resolve) => setTimeout(resolve));
    fixture.detectChanges();
    return { fixture, element: fixture.nativeElement as HTMLElement };
  }

  function tile(element: HTMLElement, label: string): HTMLButtonElement {
    return Array.from(element.querySelectorAll<HTMLButtonElement>('.admin-tile')).find(
      (button) => button.querySelector('.admin-tile-title')?.textContent?.trim() === label,
    )!;
  }

  it('zeigt eine schlichte Startseite mit sechs Bereichen und öffnet das Dashboard', async () => {
    const { fixture, element } = await render('admin');
    expect(element.querySelectorAll('.admin-tile')).toHaveLength(6);
    expect(element.querySelector('#categories-title')).toBeNull();
    expect(element.querySelector('#product-form-title')).toBeNull();
    expect(element.querySelector('#discounts-title')).toBeNull();
    expect(element.querySelector('a[href="/admin/bestellungen"]')).not.toBeNull();

    tile(element, 'Admin Dashboard').click();
    fixture.detectChanges();
    expect(element.querySelector('#dashboard-title')).not.toBeNull();
    expect(
      Array.from(element.querySelectorAll('.admin-stat')).map((stat) => stat.textContent?.trim()),
    ).toEqual(['Kategorien1', 'Produkte1', 'Veröffentlicht1', 'Aktive Rabattcodes1']);
    expect(element.querySelector('#audit-title')).not.toBeNull();
  });

  it('trennt Kategoriebaum, Produktliste und Erfassungsformulare', async () => {
    const { fixture, element } = await render('admin');
    tile(element, 'Kategoriebaum').click();
    fixture.detectChanges();
    expect(element.querySelector('#categories-title')).not.toBeNull();
    expect(element.querySelector('#category-name')).toBeNull();

    (
      element
        .querySelector('#categories-title')!
        .parentElement!.querySelector('button') as HTMLButtonElement
    ).click();
    fixture.detectChanges();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(element.querySelector('#category-form-title')?.textContent).toContain(
      'Kategorie bearbeiten',
    );
    expect((element.querySelector('#category-name') as HTMLInputElement).value).toBe('Küche');

    (element.querySelector('.admin-view-nav button') as HTMLButtonElement).click();
    fixture.detectChanges();
    tile(element, 'Produkt anlegen').click();
    fixture.detectChanges();
    expect(element.querySelector('#product-form-title')).not.toBeNull();
    expect(element.querySelector('#products-title')).toBeNull();
    expect((element.querySelector('#product-name') as HTMLInputElement).value).toBe('');

    (element.querySelector('.admin-view-nav button') as HTMLButtonElement).click();
    fixture.detectChanges();
    tile(element, 'Rabattcodes').click();
    fixture.detectChanges();
    expect(element.querySelector('#discounts-title')).not.toBeNull();
    expect(element.querySelector('#categories-title')).toBeNull();
  });

  it('beschränkt Mitarbeiter weiterhin auf Bestellungen und Versand', async () => {
    const { element } = await render('employee');
    expect(element.querySelectorAll('.admin-tile')).toHaveLength(0);
    expect(element.querySelector('a[href="/admin/bestellungen"]')).not.toBeNull();
    expect(element.textContent).toContain('nur Bestellungen und Versand');
  });

  it('führt nacheinander durch Ansicht, Zeitraum und Diagramm und zeigt die Umsatztabelle', async () => {
    sales.mockClear();
    const { fixture, element } = await render('admin');
    tile(element, 'Admin Dashboard').click();
    fixture.detectChanges();
    const group = element.querySelector('[aria-label="Umsatzansicht"]')!;
    expect(element.querySelector('[aria-label="Umsatzzeitraum"]')).toBeNull();
    expect(element.querySelector('[aria-label="Diagrammtyp"]')).toBeNull();

    Array.from(group.querySelectorAll('button'))
      .find((button) => button.textContent?.trim() === 'Kategorien')!
      .click();
    fixture.detectChanges();
    expect(element.querySelector('[aria-label="Umsatzzeitraum"]')).not.toBeNull();
    expect(element.querySelector('[aria-label="Diagrammtyp"]')).toBeNull();
    const period = element.querySelector('[aria-label="Umsatzzeitraum"]')!;
    Array.from(period.querySelectorAll('button'))
      .find((button) => button.textContent?.trim() === '30 Tage')!
      .click();
    await fixture.whenStable();
    fixture.detectChanges();
    expect(sales).toHaveBeenCalledWith('category', 'month');
    expect(element.querySelector('[aria-label="Diagrammtyp"]')).not.toBeNull();
    expect(element.querySelector('.sales-table')?.textContent).toContain('27,00 €');

    Array.from(element.querySelectorAll<HTMLButtonElement>('[aria-label="Diagrammtyp"] button'))
      .find((button) => button.textContent?.trim() === 'Fläche')!
      .click();
    fixture.detectChanges();
    expect(element.querySelector('app-sales-chart svg')).not.toBeNull();
  });
});
