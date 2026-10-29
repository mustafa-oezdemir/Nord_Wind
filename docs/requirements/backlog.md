# Priorisierter Anforderungs-Backlog

Stand: 29. Oktober 2026. Die IDs verweisen auf [den Dokumentenabgleich](document-review.md). Priorität **P0** bedeutet entweder sofortige Grundlage oder eine zwingende Bedingung vor dem ersten echten Verkauf; die Abhängigkeitsspalte bestimmt die Reihenfolge.

| Schritt | Priorität | Anforderungen | Ergebnis und überprüfbares Kriterium | Abhängigkeit |
| --- | --- | --- | --- | --- |
| NW-00 | P0 | alle | Sechs Quellen abgeglichen; Konflikte, Status und dieser Backlog dokumentiert. | keine |
| NW-01 | P0 | REQ-ARCH-01, REQ-OPS-01, REQ-TEST-01 | Ist-Zustand, fehlender Starter und ausführbare bzw. fehlende Prüfungen mit PASS/FAIL/NICHT AUSFÜHRBAR belegt. | NW-00 |
| NW-02 | P1 | REQ-BRAND-01 | Sichtbare Marke und neue technische Bezeichner sind konsistent; bestehende Daten werden nicht umbenannt oder gelöscht. | NW-01 |
| NW-03 | P1 | REQ-ARCH-01, REQ-OPS-01, REQ-SEC-01 | Lockfiles, Build, Compose, Migration und Healthcheck sind in einem sauberen Checkout reproduzierbar. | NW-01–02 |
| NW-04 | P1 | REQ-TEST-01, REQ-SEC-01 | CI führt Format-, Test-, Build- und geeignete Sicherheitsprüfungen aus; fehlerhafte Änderungen schlagen fehl. | NW-03 |
| NW-05 | P1 | REQ-API-01, REQ-OBS-01 | Einheitliche sichere Problem-Antworten, Request-ID, Timeout und strukturierte Logs sind mit API-Tests belegt. | NW-03 |
| NW-06–07 | P1 | REQ-CAT-01, REQ-CAT-02, REQ-MONEY-01 | Migrationen und echte PostgreSQL-Tests prüfen SKU-Eindeutigkeit, Fremdschlüssel und nichtnegative Werte. | NW-01 |
| NW-08 | P1 | REQ-CAT-01, REQ-CAT-03, REQ-API-01 | Katalog-API liefert begrenzte, filterbare und sortierbare Ergebnisse; ungültige Eingaben werden abgewiesen. | NW-07 |
| NW-09–10 | P1 | REQ-UI-01, REQ-CAT-02 | Deutsche responsive Oberfläche zeigt API-Daten, Varianten sowie Lade-, Leer- und Fehlerzustände. | NW-08 |
| NW-11–12 | P1 | REQ-AUTH-01, REQ-ADMIN-01 | Session, CSRF, Rollen und Besitzprüfung schützen Kunden- und Admin-Funktionen; negative Tests bestehen. | NW-05–06 |
| NW-13 | P2 | REQ-CAT-02, REQ-SEC-01 | Medien-Upload oder externer Bildbezug ist bezüglich Typ, Größe und Zugriff abgesichert. | NW-12 |
| NW-14 | P1 | REQ-CART-01 | Warenkorb speichert Variante und Menge; fremde Sessions bleiben getrennt; ungültige Mengen werden abgewiesen. | NW-10–11; Gast-Checkout-Entscheid |
| NW-15 | P0 vor Checkout | REQ-MONEY-01, REQ-ORDER-01, REQ-DE-01 | Brutto-/Netto-, Steuer-, Versand- und Rundungsregeln sind fachlich bestätigt und mit Grenzfällen getestet. | NW-06, NW-14; Geschäftsentscheid |
| NW-16–17 | P0 vor Verkauf | REQ-ORDER-01, REQ-INV-01 | Ein Checkout erzeugt höchstens eine Bestellung; Snapshots bleiben stabil; parallele Käufe überverkaufen nicht. | NW-11, NW-15 |
| NW-18 | P0 vor Verkauf | REQ-PAY-01 | Signierte und wiederholte Webhooks ändern Bestellung/Bestand genau einmal; Anbieter-Sandbox belegt den Ablauf. | NW-17; Anbieterentscheid |
| NW-19–20 | P1/P2 | REQ-AUTH-01, REQ-ORDER-01 | Versand und Kundenhistorie zeigen nur eigene Bestellungen mit unveränderten Bestellwerten. | NW-18 |
| NW-21 | P0 vor Verkauf | REQ-DE-01 | Fachlich und rechtlich geprüfte Preis-, Bestell-, Datenschutz- und Widerrufsinformationen sind umgesetzt. | NW-15, NW-18; juristische Prüfung |
| NW-22 | P2 | REQ-UI-01, REQ-DE-01 | Zugänglichkeit und Auffindbarkeit kritischer Seiten geprüft; SSR nur bei begründetem Bedarf. | NW-10, NW-21 |
| NW-23–24 | P0/P1 vor Betrieb | REQ-SEC-01, REQ-OBS-01, REQ-OPS-01 | Sicherheitsprüfungen, Diagnostik sowie Backup **und Restore-Test** sind nachgewiesen. | Auth, Checkout, Zahlung |
| NW-25–26 | P0 vor Livegang | alle verkaufsrelevanten IDs | Staging, HTTPS, Migration, kritischer E2E-Ablauf und Rückfallplan bestanden; Livegang separat freigegeben. | NW-21–24 |

## Vorläufig zurückgestellt (P3)

Marketplace und Verkäufer-Auszahlungen, Empfehlungsmaschine, komplexe Kampagnen, Kafka, Kubernetes, Redis und ein eigener Suchdienst werden erst bei konkretem Geschäfts- oder Leistungsbedarf bewertet (S1 §13; S2 §2; S3 §3; S4 §22; S5 §2; S6 §6).

## Entscheidungen vor der jeweiligen Umsetzung

1. **Vor NW-06/NW-10:** Produktarten, Varianten und Preis-/Bestandsmodell festlegen.
2. **Vor NW-14/NW-16:** Gast-Checkout oder Konto-Pflicht bestimmen.
3. **Vor NW-15:** Steuerklassen, Brutto-/Netto-Quelle, Rundung, Versand und Rabatte fachlich bestätigen.
4. **Vor NW-17/NW-18:** Reservierungsdauer, Zahlungsanbieter, Retry- und Refund-Regeln festlegen.
5. **Vor NW-21/NW-25:** Rechtliche Geltung, Hosting, Datenschutz, interne TLS-Anforderungen und Betriebsziele prüfen.

Der nächste Arbeitsschritt ist **NW-01: tatsächlichen Projektstand als Baseline prüfen**. Die in älteren Entwürfen beschriebene Starter-Anwendung ist in diesem Repository nicht vorhanden.
