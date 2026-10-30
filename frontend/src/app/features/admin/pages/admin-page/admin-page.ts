import { HttpErrorResponse } from '@angular/common/http';
import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { apiErrorMessage, toApiError } from '../../../../core/http/api-error';
import {
  AdminApi,
  AdminAuditEvent,
  AdminCategory,
  AdminImage,
  AdminProduct,
  AdminVariant,
  AuthSession,
  CategoryDraft,
  ProductDraft,
  VariantDraft,
} from '../../data-access/admin-api';

const emptyCategory = (): CategoryDraft => ({
  parent_id: null,
  name: '',
  slug: '',
  description: '',
  status: 'active',
  sort_order: 0,
});

const emptyProduct = (): ProductDraft => ({
  name: '',
  slug: '',
  description: '',
  status: 'draft',
});

const emptyVariant = (): VariantDraft => ({
  sku: '',
  label: '',
  price_cents: 0,
  stock_quantity: 0,
  is_active: true,
});

@Component({
  selector: 'app-admin-page',
  imports: [FormsModule, RouterLink],
  templateUrl: './admin-page.html',
  styleUrl: './admin-page.scss',
})
export class AdminPage implements OnInit {
  private readonly api = inject(AdminApi);

  protected readonly session = signal<AuthSession | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly notice = signal('');
  protected readonly categories = signal<AdminCategory[]>([]);
  protected readonly products = signal<AdminProduct[]>([]);
  protected readonly variants = signal<AdminVariant[]>([]);
  protected readonly images = signal<AdminImage[]>([]);
  protected readonly linkedCategories = signal<AdminCategory[]>([]);
  protected readonly events = signal<AdminAuditEvent[]>([]);
  protected readonly selectedProductId = signal('');
  protected readonly selectedProduct = computed(() =>
    this.products().find((item) => item.id === this.selectedProductId()),
  );
  protected readonly categoryTree = computed(() => {
    const source = this.categories();
    const visited = new Set<string>();
    const result: { category: AdminCategory; depth: number }[] = [];
    const add = (parentId: string | null, depth: number) => {
      for (const category of source.filter((item) => item.parent_id === parentId)) {
        if (visited.has(category.id)) continue;
        visited.add(category.id);
        result.push({ category, depth });
        add(category.id, depth + 1);
      }
    };
    add(null, 0);
    for (const category of source) {
      if (!visited.has(category.id)) result.push({ category, depth: 0 });
    }
    return result;
  });

  protected email = '';
  protected password = '';
  protected categoryForm = emptyCategory();
  protected productForm = emptyProduct();
  protected variantForm = emptyVariant();
  protected categoryToAttach = '';
  protected imageFile: File | null = null;
  protected imageAlt = '';

  async ngOnInit(): Promise<void> {
    try {
      const session = await firstValueFrom(this.api.session());
      this.session.set(session);
      if (session.customer.role === 'admin') await this.loadAll();
    } catch (error) {
      if (!(error instanceof HttpErrorResponse && error.status === 401)) this.showError(error);
    }
  }

  protected async login(): Promise<void> {
    await this.run(async () => {
      const session = await firstValueFrom(this.api.login(this.email, this.password));
      this.password = '';
      this.session.set(session);
      if (session.customer.role === 'admin') await this.loadAll();
    });
  }

  protected async logout(): Promise<void> {
    await this.run(async () => {
      await firstValueFrom(this.api.logout());
      this.session.set(null);
      this.categories.set([]);
      this.products.set([]);
      this.variants.set([]);
      this.images.set([]);
      this.events.set([]);
    });
  }

  protected editCategory(category: AdminCategory): void {
    this.categoryForm = { ...category };
    this.notice.set('');
  }

  protected resetCategory(): void {
    this.categoryForm = emptyCategory();
  }

  protected async saveCategory(): Promise<void> {
    await this.run(async () => {
      await firstValueFrom(this.api.saveCategory(this.categoryForm));
      this.resetCategory();
      await this.loadAll();
      this.notice.set('Kategorie gespeichert.');
    });
  }

  protected async deleteCategory(category: AdminCategory): Promise<void> {
    if (!globalThis.confirm(`Kategorie „${category.name}“ wirklich löschen?`)) return;
    await this.run(async () => {
      await firstValueFrom(this.api.deleteCategory(category.id));
      await this.loadAll();
      this.notice.set('Kategorie gelöscht.');
    });
  }

  protected editProduct(product: AdminProduct): void {
    this.productForm = { ...product };
    void this.selectProduct(product.id);
  }

  protected resetProduct(): void {
    this.productForm = emptyProduct();
  }

  protected async saveProduct(): Promise<void> {
    await this.run(async () => {
      const result = await firstValueFrom(this.api.saveProduct(this.productForm));
      this.resetProduct();
      await this.loadAll();
      await this.selectProduct(result.id);
      this.notice.set('Produkt gespeichert.');
    });
  }

  protected async deleteProduct(product: AdminProduct): Promise<void> {
    if (!globalThis.confirm(`Produkt „${product.name}“ wirklich löschen?`)) return;
    await this.run(async () => {
      await firstValueFrom(this.api.deleteProduct(product.id));
      if (this.selectedProductId() === product.id) this.selectedProductId.set('');
      await this.loadAll();
      this.notice.set('Produkt gelöscht.');
    });
  }

  protected async selectProduct(id: string): Promise<void> {
    this.selectedProductId.set(id);
    this.variantForm = emptyVariant();
    this.categoryToAttach = '';
    if (!id) {
      this.variants.set([]);
      this.images.set([]);
      this.linkedCategories.set([]);
      return;
    }
    try {
      const [variants, categories, images] = await Promise.all([
        firstValueFrom(this.api.listVariants(id)),
        firstValueFrom(this.api.productCategories(id)),
        firstValueFrom(this.api.listImages(id)),
      ]);
      if (this.selectedProductId() !== id) return;
      this.variants.set(variants.data);
      this.images.set(images.data);
      this.linkedCategories.set(categories.data);
    } catch (error) {
      this.showError(error);
    }
  }

  protected editVariant(variant: AdminVariant): void {
    this.variantForm = { ...variant };
  }

  protected resetVariant(): void {
    this.variantForm = emptyVariant();
  }

  protected async saveVariant(): Promise<void> {
    const productId = this.selectedProductId();
    if (!productId) return;
    await this.run(async () => {
      await firstValueFrom(this.api.saveVariant(productId, this.variantForm));
      this.resetVariant();
      await this.selectProduct(productId);
      await this.loadAudit();
      this.notice.set('Variante gespeichert.');
    });
  }

  protected async deleteVariant(variant: AdminVariant): Promise<void> {
    if (!globalThis.confirm(`Variante „${variant.sku}“ wirklich löschen?`)) return;
    await this.run(async () => {
      await firstValueFrom(this.api.deleteVariant(variant.id));
      await this.selectProduct(this.selectedProductId());
      await this.loadAudit();
      this.notice.set('Variante gelöscht.');
    });
  }

  protected chooseImage(event: Event): void {
    const input = event.target as HTMLInputElement;
    this.imageFile = input.files?.item(0) ?? null;
  }

  protected async uploadImage(input: HTMLInputElement): Promise<void> {
    const productId = this.selectedProductId();
    const file = this.imageFile;
    if (!productId || !file) return;
    if (file.size > 3 * 1024 * 1024 || !['image/jpeg', 'image/png'].includes(file.type)) {
      this.error.set('Nur JPEG oder PNG bis 3 MiB hochladen.');
      return;
    }
    await this.run(async () => {
      await firstValueFrom(this.api.uploadImage(productId, file, this.imageAlt.trim()));
      this.imageFile = null;
      this.imageAlt = '';
      input.value = '';
      await this.selectProduct(productId);
      await this.loadAudit();
      this.notice.set('Bild gespeichert.');
    });
  }

  protected async deleteImage(image: AdminImage): Promise<void> {
    if (!globalThis.confirm(`Bild „${image.alt}“ wirklich löschen?`)) return;
    await this.run(async () => {
      await firstValueFrom(this.api.deleteImage(image.id));
      await this.selectProduct(this.selectedProductId());
      await this.loadAudit();
      this.notice.set('Bild gelöscht.');
    });
  }

  protected async attachCategory(): Promise<void> {
    const productId = this.selectedProductId();
    if (!productId || !this.categoryToAttach) return;
    await this.run(async () => {
      await firstValueFrom(this.api.attachCategory(productId, this.categoryToAttach));
      await this.selectProduct(productId);
      await this.loadAudit();
      this.notice.set('Kategorie zugeordnet.');
    });
  }

  protected async detachCategory(categoryId: string): Promise<void> {
    const productId = this.selectedProductId();
    if (!productId) return;
    await this.run(async () => {
      await firstValueFrom(this.api.detachCategory(productId, categoryId));
      await this.selectProduct(productId);
      await this.loadAudit();
      this.notice.set('Kategoriezuordnung entfernt.');
    });
  }

  private async loadAll(): Promise<void> {
    const [categories, products, events] = await Promise.all([
      firstValueFrom(this.api.listCategories()),
      firstValueFrom(this.api.listProducts()),
      firstValueFrom(this.api.audit()),
    ]);
    this.categories.set(categories.data);
    this.products.set(products.data);
    this.events.set(events.data);
  }

  private async loadAudit(): Promise<void> {
    this.events.set((await firstValueFrom(this.api.audit())).data);
  }

  private async run(task: () => Promise<void>): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    try {
      await task();
    } catch (error) {
      this.showError(error);
    } finally {
      this.busy.set(false);
    }
  }

  private showError(error: unknown): void {
    if (error instanceof HttpErrorResponse) {
      switch (error.status) {
        case 401:
          this.session.set(null);
          this.error.set('Die Anmeldung ist ungültig oder abgelaufen. Melden Sie sich erneut an.');
          return;
        case 403:
          this.error.set(
            'Für diese Aktion fehlt die Berechtigung oder der CSRF-Token ist ungültig.',
          );
          return;
        case 409:
          this.error.set(
            'Die Änderung steht im Konflikt mit vorhandenen Daten, etwa einem doppelten Slug oder einer verknüpften Kategorie.',
          );
          return;
        case 429:
          this.error.set('Zu viele Anmeldeversuche. Versuchen Sie es später erneut.');
          return;
        case 413:
          this.error.set('Das Bild überschreitet die zulässige Größe.');
          return;
      }
    }
    this.error.set(apiErrorMessage(toApiError(error)));
  }
}
