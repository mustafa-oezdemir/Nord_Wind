# ADR-011: Transaktionale E-Mail-Benachrichtigungen

- **Status:** Angenommen am 31. Oktober 2026.
- **Vorgaben des Projektinhabers (31. Oktober 2026):**
  - Nur Ereignisse, die es im Projekt gibt; fehlende Kontoabläufe werden nicht nachgebaut.
  - Mailpit statt MailHog („kostenlos und modern“).
  - Eigene Änderungen werden gemergt; fremde, nicht committete Arbeit bleibt unberührt.
- **Betrifft:**
  - `backend/internal/notification` und Migration 15 (`0015_notifications`);
  - `identity.Service` (Sicherheitshinweis);
  - `cmd/api`;
  - `compose.yaml`, `.env.example`, `.env.staging.example`;
  - Admin-API `/api/v1/admin/notifications`.
- **Baut auf:** [ADR-008](ADR-008-order-creation.md) (Statusverlauf), [ADR-009](ADR-009-simulierte-zahlungen.md) (simulierte Zahlungen), [ADR-004](ADR-004-authentication.md) (Anmeldung und Ratenbegrenzung).

## Kontext

Bis hierhin gab es nur eine Begrüßungs-E-Mail, die die API während der Registrierung direkt an MailHog schickte. Dieser Versand hatte drei Schwächen:

- Ein langsamer Mailserver verzögerte die Antwort.
- Ein Fehler ging ohne Wiederholung verloren.
- Bestellungen, Zahlungen und Versand lösten gar keine E-Mail aus.

MailHog erhält seit 2020 keine Releases mehr (zuletzt v1.0.1).

## Entscheidungen

### Transactional Outbox in PostgreSQL

Datenbank-Trigger schreiben jede Benachrichtigung in `notification_outbox`, und zwar in derselben Transaktion wie das fachliche Ereignis. Daraus folgt:

- Ohne Ereignis gibt es keine E-Mail, und ohne E-Mail-Eintrag gibt es kein Ereignis.
- Ein Rollback verwirft beides.
- Der bestehende Fachcode für Bestellung, Zahlung, Versand und Konto bleibt unverändert.

| Ereignis | Quelle | Art |
| --- | --- | --- |
| Registrierung | `INSERT customers` (Rolle `customer`) | `account.welcome` |
| Kontoschließung | `customers.anonymized_at` wird gesetzt | `account.closed` (an die bisherige Adresse) |
| Bestellung angelegt | `order_status_events` → `CREATED` | `order.created` |
| Zahlung erfolgreich | → `PAID` | `order.paid` |
| Zahlung fehlgeschlagen | → `PAYMENT_FAILED` | `order.payment_failed` |
| In Bearbeitung | → `PROCESSING` | `order.processing` |
| Versendet (mit Sendungsnummer) | → `SHIPPED` | `order.shipped` |
| Zugestellt | → `DELIVERED` | `order.delivered` |
| Storniert (Kunde, System, Admin) | → `CANCELLED` | `order.cancelled` |
| Erstattung | `mock_payments.state` → `refunded` | `order.refunded` |
| Mehrere falsche Passwörter | 6. Versuch im 15-Minuten-Fenster | `security.login_locked` |

**Doppelte E-Mails** verhindert ein eindeutiger `dedupe_key` je fachlichem Ereignis:

- je Statuseintrag;
- je Erstattung;
- je Konto;
- je Sperrfenster.

Eine wiederholte Webhook-Zustellung erzeugt keinen neuen Statuseintrag und damit keine zweite E-Mail.

**Keine E-Mail** gibt es bei `PAYMENT_PENDING` (der Kunde hat die Zahlung gerade selbst gestartet) und bei einer normalen Anmeldung.

### Versand

Ein Hintergrundjob in der API (`Dispatcher`, alle 5 Sekunden) arbeitet die Outbox ab:

1. Er holt höchstens 20 fällige Einträge mit `FOR UPDATE SKIP LOCKED` und einer Reservierung von 2 Minuten. Mehrere API-Instanzen teilen sich so die Arbeit. Nach einem Absturz wird ein Eintrag mit abgelaufener Reservierung erneut geholt.
2. Der Empfänger wird erst beim Versand aus dem aktiven Konto gelesen. Geschlossene Konten erhalten nichts mehr (`skipped`), außer der Schließungsbestätigung selbst.
3. Die Einträge gehen nach `id` sortiert hinaus, also in der Reihenfolge ihres Entstehens („unterwegs“ vor „zugestellt“).
4. Für Bestellarten wird der unveränderliche Bestell-Snapshot (ADR-008) gerendert, nie der aktuelle Katalog.
5. **Fehler:**
   - SMTP 5xx gilt als dauerhaft (`failed`), 4xx sowie Verbindungs-, TLS- und Zeitfehler als vorübergehend.
   - Wiederholt wird nach 1 min, 5 min, 15 min, 1 h und 6 h; nach 6 Versuchen ist der Eintrag `failed`.
   - Admins können fehlgeschlagene Einträge erneut einplanen (`POST /api/v1/admin/notifications/{id}/retry`).
6. Bestellung und Konto sind vom Versand vollständig entkoppelt: Ein Mailfehler ändert keine HTTP-Antwort und keinen Status.

**Zustellgarantie:** Der Versand ist „mindestens einmal“. Stürzt die API genau zwischen der Annahme durch den Mailserver und dem Speichern von `sent` ab, kann eine E-Mail ein zweites Mal hinausgehen. Doppelte Ereignisse sind ausgeschlossen, doppelte Zustellungen nur in diesem seltenen Fall.

### Vorlagen

- Ein gemeinsames Layout für HTML (`html/template`) und Text (`text/template`).
- **Darstellung:**
  - Tabellenlayout bis 600 px mit Inline-Styles, damit es auch in Outlook und Gmail funktioniert.
  - Mobil passen sich Ränder, Überschrift und Knopf an.
  - Ein „Bulletproof“-Knopf mit Ersatzlink.
  - Kein JavaScript, keine externen Bilder und kein Tracking-Pixel.
- **Barrierefreiheit:** `lang="de"`, Layouttabellen mit `role="presentation"`, Kontraste der NordWinds-Farben (Tiefsee `#102b44`, Fjord `#126b82`, Bernstein `#f0b33c`).
- **Sicherheit:** `html/template` maskiert alle Kundendaten kontextabhängig. Fremde Link-Schemata wie `javascript:` werden entschärft. Kopfzeilen mit Zeilenumbrüchen werden abgewiesen (Header Injection).
- **MIME:** `multipart/alternative`, UTF-8, Quoted-Printable, RFC-2047-Betreff, eigene `Message-ID`, `Auto-Submitted: auto-generated` (RFC 3834).
- **Inhalt:**
  - Sprache Deutsch. Außerhalb der Produktion steht in jeder E-Mail „Vorschau: … keine echten Zahlungen …“.
  - Personenbezogene Daten nur, wo nötig: Vorname zur Anrede; die vollständige Lieferadresse nur in der Bestellbestätigung.
  - Keine Passwörter, Tokens oder internen IDs.

### Konfiguration und Schutz vor Fehlversand

| Variable | Bedeutung |
| --- | --- |
| `MAIL_TRANSPORT` | `smtp` oder `disabled`. Ohne Wert gilt `disabled`; Einträge bleiben dann in der Outbox und gehen nach dem Einschalten hinaus. |
| `SMTP_HOST`, `SMTP_PORT` | Mailserver; Port-Standard je TLS-Modus 587, 465 bzw. 1025 |
| `SMTP_TLS` | `starttls` (Standard), `tls` (implizit) oder `none` |
| `SMTP_USERNAME`, `SMTP_PASSWORD` oder `SMTP_PASSWORD_FILE` | AUTH PLAIN, nur über TLS; die Datei-Variante für Docker Secrets |
| `MAIL_FROM`, `MAIL_REPLY_TO` | Absender und optionale Antwortadresse |
| `MAIL_ALLOW_EXTERNAL_DELIVERY` | Außerhalb der Produktion `true`, um bewusst an einen echten Server zu senden |

Die API startet nicht, wenn:

- in der Produktion ein Testpostfach (`mailpit`, `mailhog`, `localhost`, `127.0.0.1`, Port 1025), unverschlüsseltes SMTP, fehlende Zugangsdaten, eine reservierte Absenderdomain (`.local`, `.invalid`, `.test`, `example.com` …) oder eine HTTP-Origin konfiguriert ist;
- außerhalb der Produktion ein echter Mailserver ohne `MAIL_ALLOW_EXTERNAL_DELIVERY=true` eingetragen ist; das schützt echte Kunden vor Testmails;
- `SMTP_TLS=none` für einen anderen Host als ein lokales Testpostfach gesetzt ist.

STARTTLS hat keinen stillen Rückfall auf Klartext. Zertifikate werden gegen den Hostnamen geprüft, Mindestversion ist TLS 1.2.

### Mailpit statt MailHog

- Mailpit (`axllent/mailpit:v1.31.6`, MIT-Lizenz) wird aktiv entwickelt.
- Es nutzt dieselben Ports (SMTP 1025, Oberfläche 8025) und bringt einen eigenen Healthcheck mit.
- Die Oberfläche ist nur über `127.0.0.1:${HOST_MAILPIT_PORT:-8025}` erreichbar, SMTP nur im Compose-Netz.
- Der Container läuft schreibgeschützt, ohne Capabilities, mit höchstens 500 Nachrichten.
- Gewählt ist v1.31.6 (veröffentlicht am 15. Oktober) statt v1.32.0 (22. Oktober), weil die neueste Version noch keine zwei Wochen alt war.

### Werbung und Einwilligung

Alle Arten sind transaktional (Vertrag, Konto, Sicherheit) und brauchen keine Einwilligung. Newsletter gibt es nicht. Eine spätere Werbe-E-Mail braucht:

- ein eigenes Opt-in mit Double-Opt-in;
- eine Abmeldung (`List-Unsubscribe`);
- eine eigene Art, die nicht in dieser Outbox-Liste steht.

Der CHECK auf `kind` verhindert, dass Werbung versehentlich über diesen Weg geht.

### Datenschutz

- `recipient` wird nur für `account.closed` gespeichert und nach Abschluss gelöscht; ein CHECK erzwingt das.
- Protokolle enthalten Art, ID, Versuch, Fehlerkategorie und eine maskierte Adresse (`e***@example.org`), nie Inhalte oder SMTP-Antworttexte.
- Die Admin-API zeigt keine Adressen.
- Der Sicherheitshinweis entsteht nur für existierende, aktive Kundenkonten. Die HTTP-Antwort ist für unbekannte Adressen identisch, sodass niemand erfährt, ob eine Adresse registriert ist.

## Betrachtete Alternativen

- **Direkter Versand im HTTP-Request (bisher):** langsam, verlustanfällig, nicht wiederholbar. Abgelehnt.
- **Externe Queue (Redis, RabbitMQ, Kafka):** zusätzlicher Dienst ohne Bedarf; die Outbox in PostgreSQL ist atomar mit dem Ereignis. Abgelehnt (vgl. Backlog P3).
- **Einreihen im Go-Code jeder Fachfunktion:** viele Änderungen an Bestell-, Zahlungs- und Kontocode, und eine vergessene Stelle bliebe unbemerkt. Trigger am Statusverlauf decken alle heutigen und künftigen Wege ab.
- **Externe Template-Engine oder MJML:** zusätzliche Abhängigkeit; die Standardbibliothek reicht und maskiert sicher.

## Folgen

- Die Outbox wächst. Erledigte Einträge enthalten keine Adressen mehr; eine Aufräumfrist (z. B. 90 Tage) wird mit dem Betrieb festgelegt.
- Migration 15 verweigert beim Down vorhandene Einträge. Testdatenbanken räumen mit `TRUNCATE` auf.
- `AuthConfig.RegistrationNotifier` in `server/auth.go` wird nicht mehr gesetzt, weil die Begrüßung jetzt der Trigger einreiht. Der Registrierungscode bleibt auf Wunsch unverändert; das ungenutzte Feld kann später entfernt werden.
- **Echte Zustellung verlangt:**
  - einen SMTP-Anbieter (Auftragsverarbeitung nach Art. 28 DSGVO, Server in der EU);
  - SPF, DKIM und DMARC für die Absenderdomain;
  - eine erreichbare `MAIL_REPLY_TO` oder einen Kontaktweg im Impressum.

## Nicht umgesetzt, weil es den Ablauf im Projekt nicht gibt

- **Konto:** E-Mail-Verifizierung, „Passwort vergessen“, „Passwort geändert“, E-Mail-Änderung, Zwei-Faktor-Authentifizierung, Anmeldung von neuem Gerät.
- **Bestellung:** Rücksendung, Rechnung.
- **Weitere Benachrichtigungen:** wieder vorrätig, Supportantwort, Benachrichtigungseinstellungen.

Jeder dieser Abläufe bekommt seine Benachrichtigung zusammen mit dem Ablauf selbst: eine neue Art im CHECK, einen Trigger bzw. Aufruf und einen Text in `content.go`.

## Validierung

Siehe [E-Mail-Benachrichtigungen](../progress/e-mail-benachrichtigungen.md).
