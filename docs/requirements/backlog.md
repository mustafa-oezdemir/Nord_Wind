# Priorisierter Anforderungs-Backlog

Stand: 29. Oktober 2026. Die IDs verweisen auf [den Dokumentenabgleich](document-review.md). Priorität **P0** bedeutet entweder sofortige Grundlage oder eine zwingende Bedingung vor dem ersten echten Verkauf; die Abhängigkeitsspalte bestimmt die Reihenfolge.

| Schritt | Priorität | Anforderungen | Ergebnis und überprüfbares Kriterium | Abhängigkeit |
| --- | --- | --- | --- | --- |
| NW-00 | P0 | alle | Acht Quellen abgeglichen (S7 und S8 nachgetragen); Konflikte, Status und dieser Backlog dokumentiert. | keine |
| NW-01 | P0 | REQ-ARCH-01, REQ-OPS-01, REQ-TEST-01 | Ist-Zustand, fehlender Starter und ausführbare bzw. fehlende Prüfungen mit PASS/FAIL/NICHT AUSFÜHRBAR belegt. | NW-00 |
| NW-01a | P0 | REQ-ARCH-01, REQ-OPS-01, REQ-TEST-01 | Wegen fehlendem Starter: minimale Angular-/Go-/PostgreSQL-/Compose-Basis aufbauen und Compose, Builds, Health sowie Smoke-Test real prüfen. | NW-01 |
| NW-02 | P1 | REQ-BRAND-01 | Sichtbare Marke und neue technische Bezeichner sind konsistent; bestehende Daten werden nicht umbenannt oder gelöscht. | NW-01a |
| NW-03 | P1 | REQ-ARCH-01, REQ-OPS-01, REQ-SEC-01 | Lockfiles, Build, Compose, Migration und Healthcheck sind in einem sauberen Checkout reproduzierbar. | NW-01–02 |
| NW-04 | P1 | REQ-TEST-01, REQ-SEC-01 | CI führt Format-, Test-, Build- und geeignete Sicherheitsprüfungen aus; fehlerhafte Änderungen schlagen fehl. | NW-03 |
| NW-05 | P1 | REQ-API-01, REQ-OBS-01, REQ-ARCH-02 | Einheitliche sichere Problem-Antworten, Request-ID, Timeout und strukturierte Logs sind mit API-Tests belegt. | NW-03; HTTP-Schicht Gin ([ADR-001](../adr/ADR-001-http-router.md)) |
| NW-06–07 | P1 | REQ-CAT-01, REQ-CAT-02, REQ-MONEY-01 | Migrationen und echte PostgreSQL-Tests prüfen SKU-Eindeutigkeit, Fremdschlüssel und nichtnegative Werte. | NW-01a |
| NW-08 | P1 | REQ-CAT-01, REQ-CAT-03, REQ-API-01 | Katalog-API liefert begrenzte, filterbare und sortierbare Ergebnisse; ungültige Eingaben werden abgewiesen. | NW-07 |
| NW-08a | P1 | REQ-DATA-01, REQ-CAT-01 | Nach Annahme von [ADR-003](../adr/ADR-003-data-access.md): GORM auf dem bestehenden `*sql.DB`, Katalog-Repository umgestellt; Integrationstests aus NW-07/NW-08 bestehen unverändert, Nullwert-Updates und Duplicate-Key getestet. | NW-08; ADR-003 angenommen |
| NW-09–10 | P1 | REQ-UI-01, REQ-CAT-02, REQ-I18N-01 | Deutsche responsive Oberfläche zeigt API-Daten, Varianten sowie Lade-, Leer- und Fehlerzustände; sichtbare Texte bleiben für spätere Sprachen austauschbar. | NW-08 |
| NW-11–12 | P1 | REQ-AUTH-01, REQ-ADMIN-01 | Session, CSRF, Rollen und Besitzprüfung schützen Kunden- und Admin-Funktionen; negative Tests bestehen. | NW-05–06 |
| NW-13 | P2 | REQ-CAT-02, REQ-SEC-01 | Medien-Upload oder externer Bildbezug ist bezüglich Typ, Größe und Zugriff abgesichert. | NW-12 |
| NW-14 | P1 | REQ-CART-01 | Warenkorb speichert Variante und Menge; fremde Sessions bleiben getrennt; ungültige Mengen werden abgewiesen. | NW-10–11; Gast-Checkout-Entscheid |
| NW-15 | P0 vor Checkout | REQ-MONEY-01, REQ-ORDER-01, REQ-DE-01 | Brutto-/Netto-, Steuer-, Versand- und Rundungsregeln sind fachlich bestätigt und mit Grenzfällen getestet. | NW-06, NW-14; Geschäftsentscheid |
| NW-16–17 | P0 vor Verkauf | REQ-ORDER-01, REQ-INV-01 | Ein Checkout erzeugt höchstens eine Bestellung; Snapshots bleiben stabil; parallele Käufe überverkaufen nicht. | NW-11, NW-15 |
| NW-18 | P1 Testbetrieb | REQ-PAY-01 | Signierte, wiederholte Mock-Webhooks ändern Bestellung und Bestand genau einmal; Debitkarte, Klarna, PayPal und Überweisung werden ohne Geldfluss simuliert. Ein echter Anbieter und dessen Sandbox bleiben vor dem Verkauf offen. | NW-17; [ADR-009](../adr/ADR-009-simulierte-zahlungen.md) |
| NW-19–20 | P1/P2 | REQ-AUTH-01, REQ-ORDER-01 | Versand und Kundenhistorie zeigen nur eigene Bestellungen mit unveränderten Bestellwerten. | NW-18 |
| NW-21 | P0 vor Verkauf | REQ-DE-01 | Fachlich und rechtlich geprüfte Preis-, Bestell-, Datenschutz- und Widerrufsinformationen sind umgesetzt. | NW-15, NW-18; juristische Prüfung |
| NW-22 | P2 | REQ-UI-01, REQ-DE-01 | Zugänglichkeit und Auffindbarkeit kritischer Seiten geprüft; SSR nur bei begründetem Bedarf. | NW-10, NW-21 |
| NW-23–24 | P0/P1 vor Betrieb | REQ-SEC-01, REQ-OBS-01, REQ-OPS-01 | Sicherheitsprüfungen, Diagnostik sowie Backup **und Restore-Test** sind nachgewiesen. | Auth, Checkout, Zahlung |
| NW-25–26 | P0 vor Livegang | alle verkaufsrelevanten IDs | Staging, HTTPS, Migration, kritischer E2E-Ablauf und Rückfallplan bestanden; Livegang separat freigegeben. | NW-21–24 |

## Vorläufig zurückgestellt (P3)

Marketplace und Verkäufer-Auszahlungen, Empfehlungsmaschine, komplexe Kampagnen, Kafka, Kubernetes, Redis und ein eigener Suchdienst werden erst bei konkretem Geschäfts- oder Leistungsbedarf bewertet (S1 §13; S2 §2; S3 §3; S4 §22; S5 §2; S6 §6; S7 §7; S8 §3).

## Entscheidungen vor der jeweiligen Umsetzung

1. **Erledigt am 29. Oktober 2026:** Arbeitsweise nach S7b (Umsetzung nach Fahrplan) und HTTP-Router nach [ADR-001](../adr/ADR-001-http-router.md) (Gin).
2. **Vor weiteren Repositories nach NW-08:** Die noch vorgeschlagene Repository-Umstellung aus [ADR-003](../adr/ADR-003-data-access.md) (GORM) annehmen oder ablehnen. NW-08a Step 1 hat bereits den gemeinsamen Pool und GORM eingerichtet.
3. **Vor NW-06/NW-10:** Produktarten, Varianten und Preis-/Bestandsmodell festlegen.
4. **Vor NW-14/NW-16:** Gast-Checkout oder Konto-Pflicht bestimmen.
5. **Erledigt am 30. Oktober 2026:** Steuerklassen, Brutto-/Netto-Quelle, Rundung, Versand und Rabatte nach [ADR-007](../adr/ADR-007-pricing-policy.md) (NW-15). Die steuerliche Prüfung der Einordnung folgt vor NW-21.
6. **Erledigt am 30. Oktober 2026:** Bestellstatus und erlaubte Übergänge nach [ADR-008](../adr/ADR-008-order-creation.md) (NW-16). Zahlungsstatus, Stornierung nach Zahlung und Erstattung ergänzt NW-18.
7. **Erledigt am 30. Oktober 2026 für den Testbetrieb:** 15 Minuten Reservierungsdauer und Deadlock-Retry nach [ADR-010](../adr/ADR-010-bestandsreservierung.md); Debitkarte, Klarna, PayPal und Überweisung ausschließlich simuliert, mit Erstattungsregel nach [ADR-009](../adr/ADR-009-simulierte-zahlungen.md). Die Auswahl und Anbindung eines echten Zahlungsanbieters bleibt vor dem Verkaufsstart offen.
8. **Vor NW-21/NW-25:** Rechtliche Geltung, Hosting, Datenschutz, interne TLS-Anforderungen und Betriebsziele prüfen.

Die [NW-01-Baseline](../verification/baseline.md) weist den damaligen fehlenden Starter nach. Die in älteren Entwürfen beschriebene Starter-Anwendung war nicht im Repository; NW-03 hat deshalb eine neue minimale Basis erstellt und die Prüfungen für NW-01a nachgeholt. [Der NW-03-Nachweis](../progress/NW-03.md) beschreibt den ausführbaren Stand. NW-04 bis NW-08 und NW-08a Step 1 sind implementiert; die offene CI-Prüfung und die nächsten Schritte stehen in der [Claude-Übergabe](../tasks/CLAUDE_HANDOFF.md).
