import { HttpErrorResponse } from '@angular/common/http';
import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  HostListener,
  inject,
  input,
  output,
  signal,
  viewChild,
} from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { SearchForm } from '../../../features/catalog/ui/search-form/search-form';
import { CartApi } from '../../../features/cart/data-access/cart-api';
import { BrandLogo } from '../../../shared/ui/brand-logo/brand-logo';

@Component({
  selector: 'app-site-header',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, BrandLogo, SearchForm],
  templateUrl: './site-header.html',
  styleUrl: './site-header.scss',
})
export class SiteHeader {
  private readonly api = inject(CartApi);
  private readonly router = inject(Router);
  private readonly accountMenu = viewChild<ElementRef<HTMLElement>>('accountMenuRoot');
  private readonly accountButton = viewChild<ElementRef<HTMLButtonElement>>('accountButton');

  /** Zustand des mobilen Menüs für `aria-expanded`. */
  readonly menuOpen = input(false);
  readonly menuRequested = output<HTMLElement>();
  protected readonly session = this.api.activeSession;
  protected readonly accountOpen = signal(false);
  protected readonly loading = signal(false);
  protected readonly busy = signal(false);
  protected readonly error = signal('');

  protected toggleAccountMenu(): void {
    this.accountOpen.update((open) => !open);
    if (this.accountOpen() && this.session() === undefined) void this.loadSession();
  }

  protected closeAccountMenu(): void {
    this.accountOpen.set(false);
    this.error.set('');
  }

  protected async loadSession(): Promise<void> {
    if (this.loading()) return;
    this.loading.set(true);
    this.error.set('');
    try {
      await firstValueFrom(this.api.session());
    } catch (error) {
      if (!(error instanceof HttpErrorResponse && error.status === 401)) {
        this.error.set('Das Konto konnte nicht geladen werden. Bitte erneut versuchen.');
      }
    } finally {
      this.loading.set(false);
    }
  }

  protected async logout(): Promise<void> {
    if (this.busy()) return;
    this.busy.set(true);
    this.error.set('');
    try {
      await firstValueFrom(this.api.logout());
      this.closeAccountMenu();
      await this.router.navigateByUrl('/');
    } catch (error) {
      if (error instanceof HttpErrorResponse && error.status === 401) {
        this.api.clearSession();
        this.closeAccountMenu();
        await this.router.navigateByUrl('/');
      } else {
        this.error.set('Abmelden fehlgeschlagen. Bitte erneut versuchen.');
      }
    } finally {
      this.busy.set(false);
    }
  }

  @HostListener('document:click', ['$event'])
  protected onDocumentClick(event: MouseEvent): void {
    if (this.accountOpen() && !this.accountMenu()?.nativeElement.contains(event.target as Node)) {
      this.closeAccountMenu();
    }
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    if (!this.accountOpen()) return;
    this.closeAccountMenu();
    this.accountButton()?.nativeElement.focus();
  }
}
