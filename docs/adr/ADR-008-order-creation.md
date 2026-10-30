# ADR-008: Bestellanlage, Idempotenz und Bestellstatus

- **Status:** Angenommen am 30. Oktober 2026 (NW-16). Vier fachliche Vorgaben hat der Projektinhaber am selben Tag festgelegt:
  - Bei geänderter Summe antwortet die API mit 409 und einer neuen Übersicht.
  - Es gibt eine Adresse, nur in Deutschland.
  - NW-16 umfasst API und Kasse.
  - Der Warenkorb wird in derselben Transaktion geleert.

  Idempotenz, Statusmodell und Datenbankschutz sind technische Entscheidungen dieses ADR.
- **Betrifft:**
  - `backend/internal/orders`;
  - `POST /api/v1/orders` und `GET /api/v1/orders/{orderNumber}`;
  - Migration 9 (`0009_orders`);
  - die Angular-Seiten `/kasse` und `/bestellung/:orderNumber`;
  - REQ-ORDER-01 und REQ-AUTH-01.
- **Baut auf:** [ADR-002](ADR-002-domain-schema.md) (Tabellen `orders`, `order_items`), [ADR-006](ADR-006-customer-cart.md) (kontogebundener Warenkorb), [ADR-007](ADR-007-pricing-policy.md) (Preisregel).

## Kontext

Bis NW-15 konnte ein Kunde einen Warenkorb füllen, aber nicht bestellen. Eine Bestellung muss:

- genau einmal entstehen, auch bei Doppelklick, Netzabbruch oder zweitem Tab;
- den Preis serverseitig neu berechnen;
- Positionen, Preise, Steuer, Rabatt und Adresse zum Kaufzeitpunkt festhalten;
- ohne Teildaten scheitern.

Das Schema aus Migration 2 hatte Status, Gesamtsumme und JSON-Snapshots, aber weder Preisaufstellung noch Idempotenz, Statusverlauf oder Schutz gegen spätere Änderungen.

## Entscheidungen

**Ablauf von `POST /api/v1/orders`:**

1. Voraussetzungen: Kundensession (Rolle `customer`), CSRF-Token und der Header `Idempotency-Key` (16–64 Zeichen `A–Z a–z 0–9 _ -`, z. B. eine UUID).
2. Anfrage: Adresse und `expected_total_cents`, die Gesamtsumme, die der Kunde vor dem Klick auf „Zahlungspflichtig bestellen“ gesehen hat.
3. Die Adresse wird normalisiert und geprüft. Fehler ergeben 422 `VALIDATION_FAILED` mit Feld und Code, ohne die eingegebenen Werte zu wiederholen.
4. In einer Transaktion:
   1. Die Kundenzeile wird gesperrt (`SELECT … FOR UPDATE`). Das serialisiert Checkouts und Warenkorbänderungen desselben Kunden.
   2. Gibt es den Schlüssel für diesen Kunden schon, wird die vorhandene Bestellung geliefert (200, `Idempotent-Replayed: true`). Mit anderer Adresse oder Summe ergibt er 422 `IDEMPOTENCY_KEY_REUSED`.
   3. Der Warenkorb wird mit derselben Funktion wie die Warenkorbanzeige neu berechnet (`cart.Load` → `pricing.Calculate`).
   4. Bei leerem Warenkorb folgt 409 `CART_EMPTY`. Bei einer nicht lieferbaren Position folgt 409 `CART_CHANGED` (`reason: item_unavailable`). Bei abweichender Summe folgt 409 `CART_CHANGED` (`reason: total_changed`). Beide `CART_CHANGED`-Antworten enthalten den aktuellen Warenkorb.
   5. Bestellung, Positionen und Statuseintrag `NULL → CREATED` (Akteur `customer`) werden angelegt; Warenkorb und Rabattcode werden geleert.
   6. Commit. Jeder Fehler davor rollt alles zurück.
5. Ein abgewiesener Versuch speichert den Schlüssel nicht. Nach einem 409 bestätigt der Kunde die neue Summe mit demselben Schlüssel.

**Snapshots:**

| Daten | Ort |
| --- | --- |
| Name, Variante, SKU, Menge, Bruttostückpreis, Positionssumme, Steuerklasse und -satz | `order_items` (Position 1–50) |
| Warenwert, Rabatt, Versand, Gesamtsumme, Steuerzeilen | `orders.goods_cents`, `discount_cents`, `shipping_cents`, `total_cents`, `tax_lines` |
| Rabattcode und Satz | `orders.discount_code`, `discount_percent_bp` |
| Adresse, Versandart und Versandregel | `orders.shipping_snapshot` |
| E-Mail des Kontos | `orders.customer_email` |

CHECKs erzwingen: Gesamtsumme = Warenwert − Rabatt + Versand, Positionssumme = Stückpreis × Menge, Rabatt nur paarweise mit Satz.

**Adresse:**

- Liefer- und Rechnungsadresse sind dieselbe, nur Deutschland:
  - Pflicht: Vorname, Nachname, Straße, Hausnummer, PLZ (fünfstellig, 01–99), Ort, Land `DE`;
  - optional: Firma und Adresszusatz.
- Steuerzeichen sind unzulässig.
- Eine Prüfung gegen echte Adressdaten folgt mit NW-19.

**Bestellnummer:**

- Format `NW-<Jahr>-<laufende Nummer>`, z. B. `NW-2026-100001`; das Jahr gilt in Europe/Berlin.
- Die Nummern kommen aus einer Datenbanksequenz. Lücken nach abgebrochenen Transaktionen sind möglich und zulässig; die Nummer ist keine Rechnungsnummer.

**Status und erlaubte Wechsel:**

| Von | Nach |
| --- | --- |
| `CREATED` | `PAYMENT_PENDING`, `CANCELLED` |
| `PAYMENT_PENDING` | `PAID`, `PAYMENT_FAILED`, `CANCELLED` |
| `PAYMENT_FAILED` | `PAYMENT_PENDING` (neuer Versuch), `CANCELLED` |
| `PAID` | `PROCESSING` |
| `PROCESSING` | `SHIPPED` |
| `SHIPPED` | `DELIVERED` |
| `DELIVERED`, `CANCELLED` | — (Endzustände) |

- Neue Bestellungen beginnen mit `CREATED`. Die Zahlungseinleitung (NW-18) setzt `PAYMENT_PENDING`.
- Stornierung nach Zahlung, Erstattung und Rücksendung kommen mit NW-18 und NW-19 hinzu und erweitern diese Tabelle per ADR-Änderung.
- `orders.CanTransition` und der Trigger `nordwinds_guard_order` enthalten dieselbe Liste.
- Jeder Wechsel wird in `order_status_events` mit Akteur (`customer`, `admin`, `system`) protokolliert.

**Schutz in der Datenbank:**

- `order_items` und `order_status_events` sind nur anfügbar; UPDATE und DELETE ergeben SQLSTATE `23000`.
- Bei `orders` dürfen sich nur `status` (entlang der Tabelle oben) und `updated_at` ändern. Andere Änderungen und DELETE ergeben `23000`; unerlaubte Statuswechsel ergeben `23514`.
- Varianten und Kunden mit Bestellungen sind über `ON DELETE RESTRICT` geschützt.
- Migration 9 verweigert Altbestellungen ohne Preisaufstellung; ihr Down-Lauf verweigert vorhandene Bestellungen.

**Lesen:** `GET /api/v1/orders/{orderNumber}` liefert nur eigene Bestellungen. Fremde und unbekannte Nummern ergeben einheitlich 404.

**Bestand:** NW-16 prüft den Bestand, reserviert oder bucht ihn aber nicht. Das gleichzeitige Kaufen des letzten Stücks durch zwei Kunden regelt NW-17 (Zeilensperre und atomare Buchung). Bis dahin ist die Kasse eine Vorschau ohne Zahlung und Versand.

## Betrachtete Alternativen

- **Eigene Tabelle für Idempotency-Keys mit Ablaufzeit:** Sie wäre allgemeiner, bräuchte aber Aufräumen und eine zweite Sperre. Eine Spalte an `orders` mit Unique-Index je Kunde reicht, weil nur dieser Endpunkt idempotent sein muss.
- **Sessionsperre statt Kundensperre:** Zwei Sessions desselben Kontos könnten parallel bestellen. Abgelehnt.
- **Optimistische Prüfung über eine Warenkorbversion:** Sie bräuchte eine neue Versionsspalte in jeder Warenkorbänderung. Die Gesamtsumme ist das, was der Kunde bestätigt; sie reicht als Vergleich.
- **Stillschweigend zum aktuellen Preis bestellen:** Der Kunde hätte einen anderen Betrag bestätigt als den bestellten (§ 312j Abs. 2 BGB verlangt die Gesamtsumme unmittelbar vor der Bestellung). Abgelehnt.
- **Statusprüfung nur in Go:** Ein späteres Admin-Skript oder eine Migration könnte sie umgehen. Der Trigger schützt zusätzlich.

## Folgen

- Positiv:
  - Eine Bestellung entsteht höchstens einmal je Schlüssel.
  - Preis, Steuer und Adresse bleiben auch nach Katalogänderungen unverändert.
  - Ein Fehler hinterlässt keine Teildaten.
- Negativ:
  - Bestellungen lassen sich nur per `TRUNCATE` oder neuer Migration entfernen. Testdatenbanken räumen mit `TRUNCATE` auf; echte Daten werden nie gelöscht.
  - Die Sperre der Kundenzeile serialisiert Checkouts desselben Kunden. Das ist gewollt und bei einzelnen Kunden unkritisch.
- Offen:
  - Rechtstexte vor dem Bestellknopf (AGB, Widerrufsbelehrung, Datenschutz) folgen mit NW-21.
  - Die Bestellbestätigung per E-Mail ist noch nicht umgesetzt.

## Validierung

- `internal/orders`: Unit-Tests für Adressregeln, Statusübergänge, Schlüssel- und Nummernformat sowie den Fingerabdruck der Anfrage.
- `orders_integration_test.go` gegen PostgreSQL:
  - Anmeldung, Rolle, CSRF und Schlüssel;
  - leerer Warenkorb, fehlende Adresse, Preisänderung, deaktivierter Rabatt, nicht lieferbare Position;
  - Snapshot-Inhalt, Wiederholung und Schlüsselmissbrauch;
  - 12 gleichzeitige Anfragen ergeben genau eine Bestellung;
  - Rollback durch einen Testtrigger nach Bestellung und Positionen;
  - Zugriff auf fremde Bestellungen, Datenbankschutz und Statuswechsel.
- Ein Gegentest ohne Kundensperre ergab doppelte Bestellungen und 500er; der Test erkennt das.
- Migrationstest: Up/Down/Up mit Version 9, Datenverlustsperre, Altbestellungs-Sperre.
- Ende-zu-Ende über die Kasse, siehe [NW-16](../progress/NW-16.md).
