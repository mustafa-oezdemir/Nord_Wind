import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { ActivatedRoute, RouterLink } from '@angular/router';

interface LegalDocument {
  title: string;
  lead: string;
  sections: { title: string; paragraphs: string[] }[];
}

const documents: Record<string, LegalDocument> = {
  impressum: {
    title: 'Impressum',
    lead: 'Diese NordWinds Vorschau hat noch keinen benannten gewerblichen Betreiber. Ein rechtsgültiges Impressum wird vor einem Verkaufsstart mit echten Betreiberangaben veröffentlicht.',
    sections: [
      {
        title: 'Betreiberangaben fehlen',
        paragraphs: [
          'Name, ladungsfähige Anschrift und schnelle Kontaktmöglichkeit sind noch nicht festgelegt. Falls später eine Gesellschaft Betreiberin wird, kommen je nach Rechtsform weitere Register- und Vertretungsangaben hinzu.',
        ],
      },
      {
        title: 'Verkaufsstatus',
        paragraphs: [
          'Bestellungen und Zahlungen sind ausschließlich Simulationen. Ein Verkauf an Verbraucher wird erst nach Prüfung der tatsächlichen Betreiber- und Produktdaten freigegeben.',
        ],
      },
    ],
  },
  datenschutz: {
    title: 'Datenschutz',
    lead: 'Hier wird der technische Datenfluss der Vorschau erklärt. Eine vollständige Datenschutzerklärung benötigt den tatsächlichen Verantwortlichen, Hostingort und alle späteren Dienstleister.',
    sections: [
      {
        title: 'Daten in der Vorschau',
        paragraphs: [
          'Bei Registrierung und Anmeldung werden E-Mail-Adresse, Passwort-Hash und eine zeitlich begrenzte Sitzung verarbeitet. Bei Testbestellungen werden Lieferadresse, Warenkorb, Preis- und Bestellsnapshots gespeichert. Die Kontoseite bietet einen JSON-Export und eine Kontoschließung.',
          'Beim Schließen werden Zugang und Adressbuch entfernt. Bestellbelege mit E-Mail-Adresse und Lieferanschrift bleiben derzeit erhalten. Aufbewahrungsfristen und eine spätere Löschung sind noch nicht rechtlich festgelegt.',
        ],
      },
      {
        title: 'Technischer Betrieb',
        paragraphs: [
          'Der Shop nutzt einen notwendigen, HttpOnly geschützten Sitzungscookie. Im aktuellen Frontend wurden keine Analyse- oder Werbeskripte und keine Nutzung von localStorage festgestellt. Serverlogs enthalten technische Anfragekennungen und Statuscodes.',
          'Verantwortlicher, Empfänger, Übermittlungen in Drittländer, Rechtsgrundlagen, konkrete Fristen und Kontaktwege für Betroffenenrechte müssen vor einem echten Betrieb ergänzt und geprüft werden.',
        ],
      },
    ],
  },
  cookies: {
    title: 'Cookies und ähnliche Technologien',
    lead: 'Die Vorschau verwendet einen notwendigen Sitzungscookie für Anmeldung und Schutz vor unberechtigten Aktionen.',
    sections: [
      {
        title: 'Notwendige Sitzung',
        paragraphs: [
          'Der Cookie nordwinds_session beziehungsweise bei HTTPS __Host-nordwinds_session enthält einen zufälligen Sitzungstoken. Er ist HttpOnly und SameSite=Strict. Die Sitzung endet serverseitig spätestens nach zwölf Stunden oder nach 30 Minuten Inaktivität.',
          'Für diese technische Sitzung ist derzeit kein optionales Tracking vorgesehen. Werden Analyse-, Marketing- oder Drittanbieterfunktionen ergänzt, sind Information und Einwilligung erneut zu prüfen, bevor solche Technologien aktiviert werden.',
        ],
      },
    ],
  },
  widerruf: {
    title: 'Widerruf',
    lead: 'In der Vorschau entstehen keine echten Kaufverträge. Die endgültige Widerrufsbelehrung und das Musterformular hängen vom tatsächlichen Sortiment und Betreiber ab.',
    sections: [
      {
        title: 'Vor dem Verkauf zu klären',
        paragraphs: [
          'Ein rechtlich geprüftes Muster muss Fristbeginn, Rücksendekosten, Ausnahmen und die richtige Rücksendeanschrift angeben. Bei mehreren Sendungen oder bestimmten Waren können andere Regeln gelten.',
          'Die elektronische Widerrufsfunktion benötigt eine dauerhaft zugängliche Schaltfläche „Vertrag widerrufen“, einen Bestätigungsschritt „Widerruf bestätigen“ sowie eine unverzügliche Eingangsbestätigung auf dauerhaftem Datenträger. Diese Funktionen dürfen erst mit echtem Betreiber, Zustellkanal und geprüftem Prozess für reale Verträge aktiviert werden.',
        ],
      },
    ],
  },
  versand: {
    title: 'Versand und Zahlung',
    lead: 'Die Vorschau zeigt die kalkulierten Versandkosten vor der Testbestellung. Es wird nichts abgebucht oder physisch versendet.',
    sections: [
      {
        title: 'Liefergebiet und Kosten',
        paragraphs: [
          'Liefergebiet: Deutschland. Standardversand: 4,90 € brutto. Ab 50,00 € Warenwert nach Rabatt ist der Versand kostenlos. Die Gesamtsumme und enthaltene Umsatzsteuer werden vor dem Bestellknopf angezeigt.',
          'Lieferzeiten, Versanddienst, Rücksendeanschrift und gegebenenfalls Versandbeschränkungen müssen vor einem echten Verkauf festgelegt werden.',
        ],
      },
      {
        title: 'Zahlung',
        paragraphs: [
          'Debitkarte, Klarna, PayPal und Überweisung sind derzeit ausschließlich simulierte Testarten. Ein echter Zahlungsdienst und dessen Vertragsbedingungen sind noch nicht ausgewählt.',
        ],
      },
    ],
  },
  barrierefreiheit: {
    title: 'Barrierefreiheit',
    lead: 'NordWinds soll für alle Kundinnen und Kunden nutzbar sein. Die rechtliche Einordnung nach BFSG hängt von der tatsächlichen Unternehmensgröße und dem Geschäftsmodell ab.',
    sections: [
      {
        title: 'Stand der Vorschau',
        paragraphs: [
          'Die Oberfläche verwendet beschriftete Formulare, Tastaturfokus und Statusmeldungen. Eine vollständige Prüfung mit Tastatur, Screenreader und geeigneten automatisierten Werkzeugen steht noch aus.',
          'Falls der elektronische Geschäftsverkehr unter das BFSG fällt, müssen die Barrierefreiheitsanforderungen und die vorgeschriebenen Informationen über die Dienstleistung vor dem Verkauf nachgewiesen werden.',
        ],
      },
    ],
  },
};

@Component({
  selector: 'app-legal-page',
  imports: [RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './legal-page.html',
  styleUrl: './legal-page.scss',
})
export class LegalPage {
  private readonly route = inject(ActivatedRoute);
  private readonly params = toSignal(this.route.paramMap, {
    initialValue: this.route.snapshot.paramMap,
  });
  protected readonly document = computed(() => documents[this.params().get('slug') ?? '']);
}
