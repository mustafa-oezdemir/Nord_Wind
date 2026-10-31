import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { of } from 'rxjs';
import {
  AdminApi,
  AdminCategory,
  AdminDiscountCode,
  AdminProduct,
  AuthSession,
  SalesCatalog,
  SalesQuery,
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

const salesCatalog: SalesCatalog = {
  categories: [
    { id: 'kueche-id', name: 'Küche', status: 'active', depth: 0 },
    { id: 'pfannen-id', name: 'Pfannen', parent_id: 'kueche-id', status: 'active', depth: 1 },
    { id: 'garten-id', name: 'Garten', status: 'active', depth: 0 },
  ],
  products: [
    { id: 'pfanne-id', name: 'Pfanne', status: 'published', category_id: 'kueche-id' },
    { id: 'grill-id', name: 'Grillpfanne', status: 'draft', category_id: 'pfannen-id' },
    { id: 'schere-id', name: 'Gartenschere', status: 'published', category_id: 'garten-id' },
  ],
};

function seriesFor(
  key: string,
  label: string,
  categoryKey: string,
  categoryLabel: string,
  cents: number,
  depth = 0,
) {
  return {
    key,
    label,
    category_key: categoryKey,
    category_label: categoryLabel,
    depth,
    values_cents: [cents],
    total_cents: cents,
    quantity: cents ? 1 : 0,
  };
}

/** Antwortet wie die API: Umfang je nach Ansicht und Filter, jede Reihe einzeln. */
function reportFor(query: SalesQuery): SalesReport {
  const categories = [
    seriesFor('kueche-id', 'Küche', 'kueche-id', 'Küche', 2700),
    {
      ...seriesFor('pfannen-id', 'Pfannen', 'pfannen-id', 'Pfannen', 0, 1),
      parent_key: 'kueche-id',
    },
    seriesFor('garten-id', 'Garten', 'garten-id', 'Garten', 900),
  ];
  const products = [
    seriesFor('pfanne-id', 'Pfanne', 'kueche-id', 'Küche', 2700),
    seriesFor('grill-id', 'Grillpfanne', 'pfannen-id', 'Pfannen', 0),
    seriesFor('schere-id', 'Gartenschere', 'garten-id', 'Garten', 900),
  ];
  const inCategory = (key: string) =>
    !query.category_id ||
    key === query.category_id ||
    (query.category_id === 'kueche-id' && key === 'pfannen-id');
  let series =
    query.group_by === 'category'
      ? categories.filter((entry) => inCategory(entry.key))
      : query.group_by === 'product'
        ? products.filter(
            (entry) =>
              inCategory(entry.category_key) &&
              (!query.product_id || entry.key === query.product_id),
          )
        : [seriesFor('total', 'Gesamt', '', '', 3600)];
  if (query.group_by === 'total') series = series.map((entry) => ({ ...entry, category_key: '' }));
  return {
    group_by: query.group_by,
    period: query.period,
    granularity: 'day',
    currency: 'EUR',
    start_date: query.from ?? '2026-10-25',
    end_date: query.to ?? '2026-10-31',
    buckets: [{ key: '2026-10-31', label: '31.10.' }],
    series,
    total_cents: series.reduce((sum, entry) => sum + entry.total_cents, 0),
  };
}

const sales = vi.fn((query: SalesQuery) => of(reportFor(query)));
const salesCatalogRequest = vi.fn(() => of(salesCatalog));

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
            salesCatalog: salesCatalogRequest,
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

  async function openAnalytics(): Promise<{
    fixture: ComponentFixture<AdminPage>;
    element: HTMLElement;
  }> {
    sales.mockClear();
    const rendered = await render('admin');
    tile(rendered.element, 'Admin Dashboard').click();
    rendered.fixture.detectChanges();
    await rendered.fixture.whenStable();
    rendered.fixture.detectChanges();
    return rendered;
  }

  function button(element: HTMLElement, group: string, label: string): HTMLButtonElement {
    return Array.from(
      element.querySelectorAll<HTMLButtonElement>(`[aria-label="${group}"] button`),
    ).find((entry) => entry.textContent?.trim() === label)!;
  }

  async function settle(fixture: ComponentFixture<AdminPage>): Promise<void> {
    fixture.detectChanges();
    await fixture.whenStable();
    await new Promise((resolve) => setTimeout(resolve));
    fixture.detectChanges();
  }

  function choose(select: HTMLSelectElement, value: string): void {
    select.value = value;
    select.dispatchEvent(new Event('change'));
  }

  /** Text mit normalen Leerzeichen; Beträge enthalten ein geschütztes Leerzeichen vor €. */
  function text(node: Element | null): string {
    return (node?.textContent ?? '').replace(/\u00a0/g, ' ');
  }

  function rowLabels(element: HTMLElement): string[] {
    return Array.from(element.querySelectorAll('.sales-table tbody th')).map((cell) =>
      (cell.textContent ?? '').replace(/\s+/g, ' ').trim(),
    );
  }

  it('lädt nach der Ansicht sofort 7 Tage als Balkendiagramm', async () => {
    const canvasContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const { fixture, element } = await openAnalytics();
    expect(element.querySelector('[aria-label="Umsatzzeitraum"]')).toBeNull();
    expect(sales.mock.calls).toHaveLength(0);

    button(element, 'Umsatzansicht', 'Gesamtumsatz').click();
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({ group_by: 'total', period: 'week' });
    expect(button(element, 'Umsatzzeitraum', '7 Tage').getAttribute('aria-pressed')).toBe('true');
    expect(button(element, 'Diagrammtyp', 'Balken').getAttribute('aria-pressed')).toBe('true');
    expect(element.querySelector('app-sales-chart canvas')).not.toBeNull();
    expect(element.querySelector('.sales-total')?.textContent).toContain(
      '25.10.2026 bis 31.10.2026',
    );
    expect(text(element.querySelector('.sales-table'))).toContain('36,00 €');
    expect(element.querySelector('[aria-label="Kategorie auswählen"]')).toBeNull();

    button(element, 'Umsatzzeitraum', '30 Tage').click();
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({ group_by: 'total', period: 'month' });
    button(element, 'Diagrammtyp', 'Fläche').click();
    fixture.detectChanges();
    expect(button(element, 'Diagrammtyp', 'Fläche').getAttribute('aria-pressed')).toBe('true');
    canvasContext.mockRestore();
  });

  it('zeigt alle Kategorien oder nur die gewählte samt Unterkategorien', async () => {
    const canvasContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const { fixture, element } = await openAnalytics();
    button(element, 'Umsatzansicht', 'Kategorien').click();
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({ group_by: 'category', period: 'week' });
    const select = element.querySelector<HTMLSelectElement>('[aria-label="Kategorie auswählen"]')!;
    expect(Array.from(select.options).map((option) => option.textContent?.trim())).toEqual([
      'Alle Kategorien',
      'Küche',
      '↳ Pfannen',
      'Garten',
    ]);
    expect(rowLabels(element)).toEqual(['Küche', 'Pfannen', 'Garten']);
    const swatches = Array.from(
      element.querySelectorAll<HTMLElement>('.sales-table tbody .sales-swatch'),
    );
    expect(new Set(swatches.map((swatch) => swatch.style.background)).size).toBe(3);

    choose(select, 'kueche-id');
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({
      group_by: 'category',
      period: 'week',
      category_id: 'kueche-id',
    });
    expect(rowLabels(element)).toEqual(['Küche', 'Pfannen']);

    choose(
      element.querySelector<HTMLSelectElement>('[aria-label="Kategorie auswählen"]')!,
      'pfannen-id',
    );
    await settle(fixture);
    expect(rowLabels(element)).toEqual(['Pfannen']);

    choose(element.querySelector<HTMLSelectElement>('[aria-label="Kategorie auswählen"]')!, '');
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({ group_by: 'category', period: 'week' });
    expect(rowLabels(element)).toEqual(['Küche', 'Pfannen', 'Garten']);
    canvasContext.mockRestore();
  });

  it('wählt bei Produkten zuerst die Kategorie, dann das Produkt und gruppiert nach Kategorie', async () => {
    const canvasContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const { fixture, element } = await openAnalytics();
    button(element, 'Umsatzansicht', 'Produkte').click();
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({ group_by: 'product', period: 'week' });
    expect(rowLabels(element)).toEqual([
      'Küche',
      'Pfanne',
      'Küche › Pfannen',
      'Grillpfanne',
      'Garten',
      'Gartenschere',
    ]);
    expect(element.querySelectorAll('.sales-group-row')).toHaveLength(3);
    expect(text(element.querySelector('.sales-group-row'))).toContain('27,00 €');

    const productSelect = () =>
      element.querySelector<HTMLSelectElement>('[aria-label="Produkt auswählen"]')!;
    expect(
      Array.from(productSelect().querySelectorAll('optgroup')).map((group) => group.label),
    ).toEqual(['Küche', 'Küche › Pfannen', 'Garten']);

    choose(
      element.querySelector<HTMLSelectElement>('[aria-label="Kategorie auswählen"]')!,
      'kueche-id',
    );
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({
      group_by: 'product',
      period: 'week',
      category_id: 'kueche-id',
    });
    expect(Array.from(productSelect().options).map((option) => option.textContent?.trim())).toEqual(
      ['Alle Produkte dieser Kategorie', 'Pfanne', 'Grillpfanne (Entwurf)'],
    );
    expect(rowLabels(element)).toEqual(['Küche', 'Pfanne', 'Küche › Pfannen', 'Grillpfanne']);

    choose(productSelect(), 'pfanne-id');
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({
      group_by: 'product',
      period: 'week',
      category_id: 'kueche-id',
      product_id: 'pfanne-id',
    });
    expect(rowLabels(element)).toEqual(['Küche', 'Pfanne']);

    // Wechsel zu einer anderen Kategorie wählt das nicht passende Produkt ab.
    choose(
      element.querySelector<HTMLSelectElement>('[aria-label="Kategorie auswählen"]')!,
      'garten-id',
    );
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({
      group_by: 'product',
      period: 'week',
      category_id: 'garten-id',
    });
    expect(rowLabels(element)).toEqual(['Garten', 'Gartenschere']);
    canvasContext.mockRestore();
  });

  it('wertet einen eigenen Zeitraum aus und meldet ungültige Angaben', async () => {
    const canvasContext = vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    const { fixture, element } = await openAnalytics();
    button(element, 'Umsatzansicht', 'Gesamtumsatz').click();
    await settle(fixture);
    expect(element.querySelector('.sales-range')).toBeNull();

    button(element, 'Umsatzzeitraum', 'Eigener Zeitraum').click();
    await settle(fixture);
    const from = element.querySelector<HTMLInputElement>('.sales-range input[name="from"]')!;
    const to = element.querySelector<HTMLInputElement>('.sales-range input[name="to"]')!;
    expect(to.max).toBe(to.value);
    const [call] = sales.mock.calls.slice(-1) as [SalesQuery][];
    expect(call[0].period).toBe('custom');
    expect(call[0].from).toBe(from.value);
    expect(call[0].to).toBe(to.value);

    from.value = '2026-09-01';
    from.dispatchEvent(new Event('input'));
    to.value = '2026-09-30';
    to.dispatchEvent(new Event('input'));
    (element.querySelector('.sales-apply') as HTMLButtonElement).click();
    await settle(fixture);
    expect(sales).toHaveBeenCalledWith({
      group_by: 'total',
      period: 'custom',
      from: '2026-09-01',
      to: '2026-09-30',
    });
    expect(element.querySelector('.sales-total')?.textContent).toContain(
      '01.09.2026 bis 30.09.2026',
    );

    const calls = sales.mock.calls.length;
    from.value = '2026-10-10';
    from.dispatchEvent(new Event('input'));
    to.value = '2026-10-01';
    to.dispatchEvent(new Event('input'));
    (element.querySelector('.sales-apply') as HTMLButtonElement).click();
    await settle(fixture);
    expect(sales.mock.calls).toHaveLength(calls);
    expect(element.querySelector('.sales-field-error')?.textContent).toContain('vor dem Enddatum');

    from.value = '2020-01-01';
    from.dispatchEvent(new Event('input'));
    to.value = '2026-10-01';
    to.dispatchEvent(new Event('input'));
    (element.querySelector('.sales-apply') as HTMLButtonElement).click();
    await settle(fixture);
    expect(sales.mock.calls).toHaveLength(calls);
    expect(element.querySelector('.sales-field-error')?.textContent).toContain(
      'höchstens zwei Jahre',
    );
    canvasContext.mockRestore();
  });
});
