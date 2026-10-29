import { ChangeDetectionStrategy, Component } from '@angular/core';
import { NotFoundView } from '../../shared/ui/not-found-view/not-found-view';

/**
 * Routenkomponente für unbekannte Adressen. Eigene Komponente, weil
 * `withComponentInputBinding()` fehlende Routenparameter als `undefined` an Inputs
 * übergibt und so die Standardtexte der Ansicht überschreiben würde.
 */
@Component({
  selector: 'app-not-found-page',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [NotFoundView],
  template: '<app-not-found-view />',
})
export class NotFoundPage {}
