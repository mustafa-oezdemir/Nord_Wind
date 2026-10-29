import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { RouterLink } from '@angular/router';

/** Einheitliche 404-Ansicht für unbekannte Seiten, Kategorien und Produkte. */
@Component({
  selector: 'app-not-found-view',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink],
  template: `
    <section class="container not-found" aria-labelledby="not-found-title">
      <p class="not-found-code" aria-hidden="true">404</p>
      <h1 id="not-found-title">{{ heading() }}</h1>
      <p class="not-found-message">{{ message() }}</p>
      <div class="not-found-actions">
        <a class="btn btn-primary" routerLink="/produkte">Alle Produkte ansehen</a>
        <a class="btn btn-outline-primary" routerLink="/">Zur Startseite</a>
      </div>
    </section>
  `,
  styleUrl: './not-found-view.scss',
})
export class NotFoundView {
  readonly heading = input('Seite nicht gefunden');
  readonly message = input('Die Adresse ist falsch oder die Seite existiert nicht mehr.');
}
