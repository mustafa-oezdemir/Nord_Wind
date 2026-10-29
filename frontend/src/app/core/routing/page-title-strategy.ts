import { Injectable, inject } from '@angular/core';
import { Title } from '@angular/platform-browser';
import { RouterStateSnapshot, TitleStrategy } from '@angular/router';

export const SHOP_NAME = 'NordWinds';

export function formatPageTitle(pageTitle?: string): string {
  const trimmed = pageTitle?.trim();
  return trimmed ? `${trimmed} | ${SHOP_NAME}` : SHOP_NAME;
}

/** Setzt den Dokumenttitel einheitlich als „Seite | NordWinds“. */
@Injectable({ providedIn: 'root' })
export class PageTitleStrategy extends TitleStrategy {
  private readonly title = inject(Title);

  override updateTitle(snapshot: RouterStateSnapshot): void {
    this.title.setTitle(formatPageTitle(this.buildTitle(snapshot)));
  }
}
